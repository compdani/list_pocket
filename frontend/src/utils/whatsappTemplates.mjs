// Turns a WhatsApp template definition into the form for filling it in. No browser or
// Vue imports, so it can be unit tested directly.

const PLACEHOLDER_RE = /\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g;

function placeholders(text) {
  return [...String(text || '').matchAll(PLACEHOLDER_RE)].map((m) => m[1]);
}

function textParameters(text, named) {
  return placeholders(text).map((name) => ({
    label: name,
    parameter_name: named ? name : undefined,
    value: '',
    fallback: '',
  }));
}

// templateSections lists what a template needs filled in, in the shape the backend's
// TemplateChoice expects. Sections with nothing to fill in are left out.
export function templateSections(template) {
  const named = String(template.parameter_format || '').toUpperCase() === 'NAMED';
  const sections = [];
  (template.components || []).forEach((component) => {
    const type = String(component.type || '').toUpperCase();
    if (type === 'HEADER') {
      const format = String(component.format || '').toUpperCase();
      if (format === 'TEXT') {
        const parameters = textParameters(component.text, named);
        if (parameters.length > 0) {
          sections.push({ type: 'header', parameters });
        }
      } else if (['IMAGE', 'VIDEO', 'DOCUMENT'].includes(format)) {
        sections.push({ type: 'header', format: format.toLowerCase(), link: '', parameters: [] });
      }
    } else if (type === 'BODY') {
      const parameters = textParameters(component.text, named);
      if (parameters.length > 0) {
        sections.push({ type: 'body', parameters });
      }
    } else if (type === 'BUTTONS') {
      (component.buttons || []).forEach((button, index) => {
        if (String(button.type || '').toUpperCase() !== 'URL') {
          return;
        }
        const parameters = textParameters(button.url, false);
        if (parameters.length > 0) {
          sections.push({
            type: 'button', sub_type: 'url', index: String(index), label: button.text, parameters,
          });
        }
      });
    }
  });
  return sections;
}

export function templateKey(template) {
  return `${template.name}|${template.language}`;
}

export function templatePreview(template) {
  const body = (template.components || []).find((c) => String(c.type).toUpperCase() === 'BODY');
  return body ? body.text : '';
}

// choiceFromSections is what gets saved on the campaign.
export function choiceFromSections(template, sections) {
  return {
    name: template.name,
    language: template.language,
    components: sections.map((section) => ({
      type: section.type,
      sub_type: section.sub_type,
      index: section.index,
      format: section.format,
      link: section.link,
      parameters: (section.parameters || []).map((p) => ({
        parameter_name: p.parameter_name,
        value: p.value,
        fallback: p.fallback,
      })),
    })),
  };
}

// restoreSections puts a saved choice's values back into a template's form. Values only
// carry over by position within a matching section, so a changed template starts blank.
export function restoreSections(template, choice) {
  const sections = templateSections(template);
  if (!choice || choice.name !== template.name || choice.language !== template.language) {
    return sections;
  }
  sections.forEach((section, i) => {
    const saved = (choice.components || [])[i];
    if (!saved || saved.type !== section.type) {
      return;
    }
    if (section.format) {
      section.link = saved.link || '';
    }
    section.parameters.forEach((param, j) => {
      const value = (saved.parameters || [])[j];
      if (value) {
        param.value = value.value || '';
        param.fallback = value.fallback || '';
      }
    });
  });
  return sections;
}

// missingValues reports whether a send could fail for a subscriber: a value using a
// placeholder needs a fallback, and a media header needs a link.
export function missingValues(sections) {
  return sections.some((section) => {
    if (section.format && !section.link) {
      return true;
    }
    return (section.parameters || []).some((p) => {
      const value = String(p.value || '').trim();
      if (!value) {
        return true;
      }
      return value.includes('{{') && !String(p.fallback || '').trim();
    });
  });
}
