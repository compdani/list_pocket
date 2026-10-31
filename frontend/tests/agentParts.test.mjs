import test from 'node:test';
import assert from 'node:assert/strict';

import {
  contextInUse, diffNodes, fileProblem, formatBytes, isBinaryFile, lineDiff, pluginAddons, pluginFileChanges, proposalInfo, proposalTitle,
} from '../src/agent/parts.mjs';
import { sameDefinition } from '../src/utils/flowGraph.mjs';

test('proposals have a label and an apply button, with a fallback for new tools', () => {
  assert.equal(proposalInfo('propose_reply').apply, 'Send');
  assert.equal(proposalInfo('propose_something_new').label, 'propose_something_new');
  assert.equal(proposalTitle({ tool: 'propose_reply', arguments: { summary: 'Say thanks' } }), 'Say thanks');
  assert.equal(proposalTitle({ tool: 'propose_reply', arguments: {} }), 'Send reply');
});

test('anything that sends or does real work warns before applying', () => {
  ['propose_reply', 'propose_test_run', 'propose_workflow_run'].forEach((tool) => assert.ok(proposalInfo(tool).warning, tool));
});

test('context in use lists each item once, in the order it was attached', () => {
  const refs = contextInUse([
    { context: [{ type: 'contact', id: 'a' }, { type: 'file', id: 'f' }] },
    { context: [{ type: 'contact', id: 'a' }, { type: 'contact', id: 'b' }] },
    { role: 'assistant' },
  ]);
  assert.deepEqual(refs.map((r) => r.id), ['a', 'f', 'b']);
});

test('file limits depend on the kind of file', () => {
  assert.ok(isBinaryFile({ type: 'image/png' }));
  assert.ok(isBinaryFile({ type: 'application/pdf' }));
  assert.ok(!isBinaryFile({ type: 'text/csv' }));
  assert.equal(fileProblem({ name: 'a.png', type: 'image/png', size: 5 * 1024 * 1024 }), '');
  assert.match(fileProblem({ name: 'a.png', type: 'image/png', size: 7 * 1024 * 1024 }), /larger than 6 MB/);
  assert.match(fileProblem({ name: 'a.csv', type: 'text/csv', size: 3 * 1024 * 1024 }), /larger than 2 MB/);
  assert.equal(formatBytes(2048), '2 KB');
});

test('workflow previews show what was added, changed and removed, ignoring layout', () => {
  const current = { nodes: [{ id: 'start', type: 'start', next: 'a', ui: { x: 1, y: 1 } }, { id: 'a', type: 'notify', message: 'x' }, { id: 'old', type: 'end' }] };
  const next = { nodes: [{ id: 'start', type: 'start', next: 'a' }, { id: 'a', type: 'notify', message: 'y' }, { id: 'new', type: 'end' }] };
  const diff = diffNodes(current, next, sameDefinition);
  assert.deepEqual(diff.nodes, { a: 'changed', new: 'added' });
  assert.deepEqual(diff.removed, ['old']);
  assert.deepEqual(diffNodes(null, next, sameDefinition).nodes, { start: 'added', a: 'added', new: 'added' });
});

test('a plugin proposal always opens for review first', () => {
  assert.equal(proposalInfo('propose_plugin').reviewFirst, true);
  assert.equal(proposalInfo('propose_plugin').view, 'plugin');
  assert.equal(proposalInfo('propose_reply').reviewFirst, undefined);
});

test('plugin add-ons are named as they are in the builder', () => {
  const addons = pluginAddons({ id: 'a', tools: [{ name: 't', description: 'd' }], fieldEditors: [{ name: 'f' }] });
  assert.deepEqual(addons.map((x) => [x.kind, x.name]), [['plugins.tool', 'a.t'], ['plugins.fieldEditor', 'a.f']]);
  assert.deepEqual(pluginAddons(undefined), []);
});

test('plugin file changes compare against what is installed', () => {
  const out = pluginFileChanges(
    [{ path: 'same.js', content: 'x' }, { path: 'edit.js', content: 'x' }, { path: 'gone.js', content: 'x' }],
    [{ path: 'same.js', content: 'x' }, { path: 'edit.js', content: 'y' }, { path: 'new.js', content: 'z' }],
  );
  assert.deepEqual(out.files.map((f) => f.status), ['same', 'changed', 'added']);
  assert.deepEqual(out.removed, ['gone.js']);
  assert.deepEqual(pluginFileChanges(undefined, [{ path: 'a', content: '' }]).files[0].status, 'added');
});

test('a line diff keeps what is unchanged', () => {
  assert.deepEqual(lineDiff('a\nb\nc', 'a\nx\nc'), [
    { kind: 'same', text: 'a' },
    { kind: 'added', text: 'x' },
    { kind: 'removed', text: 'b' },
    { kind: 'same', text: 'c' },
  ]);
  assert.deepEqual(lineDiff('', 'a').filter((l) => l.kind === 'added').map((l) => l.text), ['a']);
  assert.equal(lineDiff('x\n'.repeat(3000), 'y\n'.repeat(3000)), null);
});
