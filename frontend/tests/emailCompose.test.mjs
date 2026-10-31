import assert from 'node:assert/strict';
import test from 'node:test';
import {
  applyDraft, bodySummary, composeDraft, needsSendApproval, plainPreviewHTML,
} from '../src/utils/emailCompose.mjs';

test('a new email opens on rich text and an older body stays plain', () => {
  assert.equal(composeDraft({}).contentType, 'richtext');
  assert.equal(composeDraft({ body: 'hi' }).contentType, 'plain');
  assert.equal(composeDraft({ body: '<p>hi</p>', content_type: 'html' }).contentType, 'html');
});

test('a draft writes back without disturbing other input', () => {
  const input = { contact: '{{ input.contact }}', transactional: true, body_source: 'old', content_type: 'visual' };
  const out = applyDraft(input, { subject: ' Hi ', body: 'text', contentType: 'plain', bodySource: null });
  assert.deepEqual(out, { contact: '{{ input.contact }}', transactional: true, subject: 'Hi', body: 'text' });
  assert.equal(input.content_type, 'visual');
  assert.deepEqual(
    applyDraft({}, { subject: '', body: '<p>x</p>', contentType: 'visual', bodySource: '{"a":1}' }),
    { body: '<p>x</p>', content_type: 'visual', body_source: '{"a":1}' },
  );
});

test('approval follows the same rule as the server', () => {
  assert.equal(needsSendApproval({ content_type: 'html', body: '<p>{{ input.x }}</p>' }), true);
  assert.equal(needsSendApproval({ content_type: 'html', body: '<p>fixed</p>' }), false);
  assert.equal(needsSendApproval({ body: '{{ input.x }}' }), false);
  assert.equal(needsSendApproval({ content_type: 'html', body: '{{ input.x }}', template_id: 't' }), false);
});

test('the summary reads as text and the plain preview is escaped', () => {
  assert.equal(bodySummary({ content_type: 'html', body: '<style>a{}</style><h1>Hi</h1><p>there</p>' }), 'Hi there');
  assert.equal(bodySummary({ body: '<b>literal</b>' }), '<b>literal</b>');
  assert.equal(bodySummary({ body: 'x'.repeat(200) }, 5), 'xxxxx…');
  assert.ok(!plainPreviewHTML('<script>x</script>').includes('<script>'));
});
