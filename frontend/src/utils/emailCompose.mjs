// The workflow email step keeps its message in the tool's input. The compose modal
// edits a draft of it in the shape the shared Editor uses.

export const EMAIL_FORMATS = [
  { value: 'plain', label: 'campaigns.plainText' },
  { value: 'richtext', label: 'campaigns.richText' },
  { value: 'html', label: 'campaigns.rawHTML' },
  { value: 'markdown', label: 'campaigns.markdown' },
  { value: 'visual', label: 'campaigns.visual' },
  { value: 'grapes_mjml', title: 'GrapesJS (MJML)' },
];

const text = (value) => (value === undefined || value === null ? '' : String(value));

export function composeDraft(input = {}) {
  const body = text(input.body);
  return {
    subject: text(input.subject),
    body,
    bodySource: input.body_source || null,
    // A new email opens on rich text. An older body is plain text, as the server sends it.
    contentType: input.content_type || (body ? 'plain' : 'richtext'),
    templateId: null,
  };
}

export function applyDraft(input = {}, draft = {}) {
  const out = { ...input };
  const set = (key, value) => {
    if (value) out[key] = value;
    else delete out[key];
  };
  set('subject', text(draft.subject).trim());
  set('body', text(draft.body));
  set('content_type', draft.contentType === 'plain' ? '' : draft.contentType);
  set('body_source', draft.bodySource);
  return out;
}

// Mirrors the server: a body in a formatted email that comes from run data needs a person.
export function needsSendApproval(input = {}) {
  const format = input.content_type;
  return Boolean(format && format !== 'plain' && !input.template_id && text(input.body).includes('{{'));
}

export function bodySummary(input = {}, limit = 140) {
  const source = text(input.body);
  const plain = !input.content_type || input.content_type === 'plain' || input.content_type === 'markdown'
    ? source
    : source.replace(/<(style|head|script)\b[\s\S]*?<\/\1>/gi, ' ').replace(/<[^>]+>/g, ' ');
  const line = plain.replace(/\s+/g, ' ').trim();
  return line.length > limit ? `${line.slice(0, limit)}…` : line;
}

export function plainPreviewHTML(value) {
  const safe = text(value).replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  return `<pre style="white-space:pre-wrap;font:inherit;margin:0">${safe}</pre>`;
}
