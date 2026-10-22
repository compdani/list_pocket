import test from 'node:test';
import assert from 'node:assert/strict';
import { anchorLegacyActivity, appendAssistantEvent, conversationTranscript } from '../src/assistant/transcript.mjs';
import { renderAssistantMarkdown } from '../src/assistant/markdown.mjs';
function thread() { return { messages: [{ role: 'user', content: 'Analyze campaigns' }, { role: 'assistant', content: '', parts: [] }], tools: [], proposals: [], turn: 'turn1', editorKey: 'campaign:1' }; }
test('tools and proposals remain at their chronological positions between text segments', () => {
  const data = thread();
  appendAssistantEvent(data, { type: 'text_delta', data: { text: 'Checking. ' } });
  appendAssistantEvent(data, { type: 'tool', data: { id: 'lookup', name: 'campaigns', status: 'running' } });
  appendAssistantEvent(data, { type: 'tool', data: { id: 'lookup', name: 'campaigns', status: 'completed', result: 'found' } });
  appendAssistantEvent(data, { type: 'text_delta', data: { text: 'Here is the ' } });
  appendAssistantEvent(data, { type: 'text_delta', data: { text: 'analysis.' } });
  appendAssistantEvent(data, { type: 'proposal', data: { id: 'proposal', arguments: {}, status: 'pending' } });
  appendAssistantEvent(data, { type: 'text_delta', data: { text: 'Review the changes.' } });
  const rows = conversationTranscript(data);
  assert.deepEqual(rows.map(row => row.type), ['text', 'text', 'tool', 'text', 'proposal', 'text']);
  assert.equal(rows[2].tool.status, 'completed');
  assert.equal(rows[3].content, 'Here is the analysis.');
  assert.equal(data.messages[1].content, 'Checking. Here is the analysis.Review the changes.');
  const restored = JSON.parse(JSON.stringify(data));
  assert.deepEqual(conversationTranscript(restored), rows);
  restored.messages.push({ role: 'user', content: 'Follow up' }, { role: 'assistant', content: 'Next answer', parts: [{ type: 'text', content: 'Next answer' }] });
  assert.equal(conversationTranscript(restored)[2].tool.id, 'lookup');
});
test('legacy empty assistant messages are hidden and unassigned tools stay explicitly grouped', () => {
  const data = { messages: [{ role: 'user', content: 'First try' }, { role: 'assistant', content: '' }, { role: 'user', content: 'Retry' }, { role: 'assistant', content: 'Analysis' }], tools: [{ id: 'old', name: 'campaigns' }], proposals: [] };
  const rows = conversationTranscript(data);
  assert.deepEqual(rows.map(row => row.type), ['text', 'text', 'legacy_tools', 'text']);
  assert.equal(rows.at(-1).content, 'Analysis');
});
test('existing conversations can resume with text and tool events without losing old content', () => {
  const data = thread(); delete data.messages[1].parts; data.messages[1].content = 'Existing text';
  appendAssistantEvent(data, { type: 'tool', data: { id: 'new', name: 'analytics', status: 'running' } });
  appendAssistantEvent(data, { type: 'text_delta', data: { text: 'continued' } });
  assert.deepEqual(conversationTranscript(data).map(row => row.type), ['text', 'text', 'tool', 'text']);
});
test('campaign replies render headings, tables, lists and fenced code', () => {
  const html = renderAssistantMarkdown('## Recent campaign performance\n\n| Campaign | Sent |\n|---|---:|\n| Newsletter | **6,211** |\n\n- Audit tracking\n\n```html\n<script>example</script>\n```');
  assert.match(html, /<h2>Recent campaign performance<\/h2>/);
  assert.match(html, /<table>/); assert.match(html, /<strong>6,211<\/strong>/);
  assert.match(html, /<ul>/); assert.match(html, /&lt;script&gt;example/);
});
test('Markdown escapes raw HTML and blocks executable links', () => {
  const html = renderAssistantMarkdown('<img src=x onerror=alert(1)>\n\n[bad](javascript:alert(1))\n\n[Source](/admin/campaigns)');
  assert.doesNotMatch(html, /<img|href="javascript:/);
  assert.match(html, /&lt;img/); assert.match(html, /rel="noopener noreferrer"/);
  assert.match(html, /href="\/admin\/campaigns"/);
});

test('tool IDs reused by another turn do not move or overwrite previous activity', () => {
  const data = thread();
  appendAssistantEvent(data, { type: 'tool', data: { id: 'lookup', name: 'campaigns', status: 'completed' } });
  appendAssistantEvent(data, { type: 'text_delta', data: { text: 'First answer' } });
  data.turn = 'turn2'; data.messages.push({ role: 'user', content: 'Follow up' }, { role: 'assistant', content: '', parts: [] });
  appendAssistantEvent(data, { type: 'tool', data: { id: 'lookup', name: 'analytics', status: 'running' } });
  assert.deepEqual(conversationTranscript(data).filter(row => row.type === 'tool').map(row => row.tool.name), ['campaigns', 'analytics']);
});
test('legacy tool groups remain anchored when a new turn is added', () => {
  const data = { messages: [{ role: 'assistant', content: 'Old answer' }], tools: [{ id: 'old', name: 'campaigns' }], proposals: [] };
  anchorLegacyActivity(data);
  data.messages.push({ role: 'user', content: 'New question' }, { role: 'assistant', content: 'New answer', parts: [{ type: 'text', content: 'New answer' }] });
  assert.deepEqual(conversationTranscript(data).map(row => row.type), ['legacy_tools', 'text', 'text', 'text']);
});
