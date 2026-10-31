import test from 'node:test';
import assert from 'node:assert/strict';
import {
  choiceFromSections,
  missingValues,
  restoreSections,
  templateKey,
  templateSections,
} from '../src/utils/whatsappTemplates.mjs';

const template = {
  name: 'promo',
  language: 'en_US',
  components: [
    { type: 'HEADER', format: 'IMAGE' },
    { type: 'BODY', text: 'Hi {{1}}, your code is {{2}}' },
    { type: 'FOOTER', text: 'Thanks' },
    {
      type: 'BUTTONS',
      buttons: [
        { type: 'QUICK_REPLY', text: 'Stop' },
        { type: 'URL', text: 'Open', url: 'https://x.test/{{1}}' },
        { type: 'URL', text: 'Static', url: 'https://x.test/' },
      ],
    },
  ],
};

test('sections cover what needs filling in and nothing else', () => {
  const sections = templateSections(template);
  assert.deepEqual(sections.map((s) => s.type), ['header', 'body', 'button']);
  assert.equal(sections[0].format, 'image');
  assert.deepEqual(sections[1].parameters.map((p) => p.label), ['1', '2']);
  assert.equal(sections[1].parameters[0].parameter_name, undefined);
  assert.equal(sections[2].index, '1', 'the index counts every button');
});

test('named templates send parameter names', () => {
  const sections = templateSections({
    name: 'n',
    language: 'en',
    parameter_format: 'NAMED',
    components: [{ type: 'BODY', text: 'Hi {{first_name}}' }],
  });
  assert.equal(sections[0].parameters[0].parameter_name, 'first_name');
});

test('a template with no variables needs nothing', () => {
  const sections = templateSections({ name: 'n', language: 'en', components: [{ type: 'BODY', text: 'Hello' }] });
  assert.deepEqual(sections, []);
  assert.equal(missingValues(sections), false);
});

test('a choice survives saving and reloading', () => {
  const sections = templateSections(template);
  sections[0].link = 'https://x.test/a.png';
  sections[1].parameters[0].value = '{{first_name}}';
  sections[1].parameters[0].fallback = 'friend';
  sections[1].parameters[1].value = 'ABC';
  sections[2].parameters[0].value = 'path';
  const choice = choiceFromSections(template, sections);
  assert.equal(choice.name, 'promo');
  assert.equal(choice.components[2].sub_type, 'url');

  const restored = restoreSections(template, JSON.parse(JSON.stringify(choice)));
  assert.equal(restored[0].link, 'https://x.test/a.png');
  assert.equal(restored[1].parameters[0].fallback, 'friend');
  assert.equal(restored[2].parameters[0].value, 'path');
  assert.equal(missingValues(restored), false);

  const other = restoreSections({ ...template, name: 'other' }, choice);
  assert.equal(other[1].parameters[0].value, '', 'another template starts blank');
});

test('missing values are flagged', () => {
  const sections = templateSections(template);
  assert.equal(missingValues(sections), true);
  sections[0].link = 'https://x.test/a.png';
  sections[1].parameters[0].value = '{{first_name}}';
  sections[1].parameters[1].value = 'ABC';
  sections[2].parameters[0].value = 'path';
  assert.equal(missingValues(sections), true, 'a placeholder needs a fallback');
  sections[1].parameters[0].fallback = 'friend';
  assert.equal(missingValues(sections), false);
});

test('templateKey tells languages apart', () => {
  assert.notEqual(templateKey({ name: 'a', language: 'en' }), templateKey({ name: 'a', language: 'es' }));
});
