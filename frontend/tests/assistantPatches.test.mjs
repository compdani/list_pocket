import test from 'node:test';
import assert from 'node:assert/strict';
import { applyEditorPatch, revision } from '../src/assistant/patches.mjs';
const proposal = (snapshot, operations) => ({ revision: revision(snapshot), operations });
test('CTA follow-up preserves unrelated HTML, subject and preheader', () => {
  const before = { subject: 'Hello', preheader: 'Preview', contentType: 'html', body: '<h1>Welcome</h1><a href="/shop">Visit our shop today</a><p>Footer</p>' };
  const after = applyEditorPatch(before, proposal(before, [{ op: 'replace_text', before: 'Visit our shop today', after: 'Shop now' }]));
  assert.equal(after.body, '<h1>Welcome</h1><a href="/shop">Shop now</a><p>Footer</p>');
  assert.equal(after.subject, before.subject); assert.equal(after.preheader, before.preheader); assert.match(before.body, /Visit our shop today/);
});
test('rejects stale revisions and ambiguous text without partial edits', () => {
  const before = { body: 'CTA CTA', subject: 'Original' };
  assert.throws(() => applyEditorPatch(before, { revision: 'old', operations: [] }), /editor changed/);
  assert.throws(() => applyEditorPatch(before, proposal(before, [{ op: 'set', field: 'subject', before: 'Original', after: 'Changed' }, { op: 'replace_text', before: 'CTA', after: 'Go' }])), /exactly once/);
  assert.equal(before.subject, 'Original');
});
test('replacements retain literal dollar characters across text editor formats', () => {
  for (const contentType of ['plain', 'markdown', 'richtext', 'html', 'grapes_mjml']) {
    const before = { contentType, body: 'Pay $10 now' };
    const after = applyEditorPatch(before, proposal(before, [{ op: 'replace_text', before: '$10', after: '$& $5' }]));
    assert.equal(after.body, 'Pay $& $5 now');
  }
});
test('visual edits address stable blocks and preserve other block values', () => {
  const document = { root: { type: 'EmailLayout', childrenIds: ['cta', 'footer'] }, cta: { type: 'Button', text: 'Shop today' }, footer: { text: 'Unsubscribe' } };
  const before = { contentType: 'visual', body: JSON.stringify(document) };
  const after = applyEditorPatch(before, proposal(before, [{ op: 'block', blockId: 'cta', before: JSON.stringify(document.cta), after: JSON.stringify({ ...document.cta, text: 'Shop' }) }]));
  assert.deepEqual(JSON.parse(after.body).footer, document.footer);
  assert.equal(JSON.parse(after.body).cta.text, 'Shop');
});
test('workflow edits retain stable IDs and other configurations', () => {
  const before = { kind: 'workflow', nodes: [{ id: 'trigger', type: 'trigger' }, { id: 'wait', type: 'wait_until', config: { days: 1 } }], edges: [] };
  const after = applyEditorPatch(before, proposal(before, [{ op: 'node_upsert', id: 'wait', value: { id: 'wait', type: 'wait_until', config: { days: 2 } } }]));
  assert.deepEqual(after.nodes[0], before.nodes[0]); assert.equal(after.nodes[1].config.days, 2);
  assert.throws(() => applyEditorPatch(before, proposal(before, [{ op: 'node_upsert', id: 'wait', value: { id: 'other' } }])), /Invalid graph ID/);
});
