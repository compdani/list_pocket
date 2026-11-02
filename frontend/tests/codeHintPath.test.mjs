import test from 'node:test';
import assert from 'node:assert/strict';

import { EditorState } from '@codemirror/state';
import { CompletionContext } from '@codemirror/autocomplete';
import { javascript } from '@codemirror/lang-javascript';
import { ensureSyntaxTree } from '@codemirror/language';

import { ELEMENT } from '../src/utils/codeHints.mjs';
import { codeHintPath } from '../src/utils/codeHintPath.mjs';

// The last | marks the cursor.
function at(source) {
  const pos = source.lastIndexOf('|');
  const doc = source.slice(0, pos) + source.slice(pos + 1);
  const state = EditorState.create({ doc, extensions: [javascript()] });
  ensureSyntaxTree(state, doc.length, 1000);
  return codeHintPath(new CompletionContext(state, pos, false));
}

const path = (source) => at(source)?.path;

test('reads a plain chain', () => {
  assert.deepEqual(at('nodes.x.|'), { path: ['nodes', 'x'], name: '' });
  assert.deepEqual(at('nodes.x.res|'), { path: ['nodes', 'x'], name: 'res' });
  assert.deepEqual(path('nodes?.x?.|'), ['nodes', 'x']);
});

test('a name with no dot is a global', () => {
  assert.deepEqual(at('inp|'), { path: [], name: 'inp' });
});

test('stays out of strings, comments and new names', () => {
  assert.equal(at('const a = "nodes.|"'), null);
  assert.equal(at('// nodes.|'), null);
  assert.equal(at('/* nodes.| */'), null);
  assert.equal(at('const inp|'), null);
});

test('follows local variables', () => {
  assert.deepEqual(path('const raw = nodes.classify;\nraw.|'), ['nodes', 'classify']);
  assert.deepEqual(path('const raw = nodes.classify\nraw.|'), ['nodes', 'classify']);
  assert.deepEqual(path('const raw = nodes.classify;\nconst res = raw.result;\nres.|'), ['nodes', 'classify', 'result']);
  assert.deepEqual(path('let raw = nodes.x, other = input.y;\nother.|'), ['input', 'y']);
  assert.deepEqual(path('const raw = nodes.classify;\nraw.res|'), ['nodes', 'classify']);
});

test('follows fallbacks and parentheses', () => {
  assert.deepEqual(path('const raw = nodes.x || {};\nraw.|'), ['nodes', 'x']);
  assert.deepEqual(path('const raw = nodes.x ?? {};\nraw.|'), ['nodes', 'x']);
  assert.deepEqual(path('const raw = (nodes.x);\nraw.|'), ['nodes', 'x']);
  assert.equal(at('const raw = nodes.x + 1;\nraw.|'), null);
});

test('a variable with an unknown value offers nothing, even named like a global', () => {
  assert.equal(at('const input = load();\ninput.|'), null);
  assert.equal(at('const raw = 1;\nraw.|'), null);
  assert.equal(at('let raw;\nraw.|'), null);
  assert.equal(at('const raw = raw.|'), null);
});

test('reads through indexes and array methods', () => {
  assert.deepEqual(path('nodes.x.results[0].|'), ['nodes', 'x', 'results', ELEMENT]);
  assert.deepEqual(path('nodes.x["results"].|'), ['nodes', 'x', 'results']);
  assert.deepEqual(path('nodes.x.results.filter((r) => r.ok)[0].|'), ['nodes', 'x', 'results', ELEMENT]);
  assert.deepEqual(path('nodes.x.results.find((r) => r.ok).|'), ['nodes', 'x', 'results', ELEMENT]);
  assert.deepEqual(path('const one = nodes.x.results.find((r) => r.ok);\none.|'), ['nodes', 'x', 'results', ELEMENT]);
  assert.equal(at('nodes.x.results.map((r) => r.ok).|'), null);
});

test('callback parameters are elements of the array', () => {
  assert.deepEqual(path('nodes.x.results.filter((r) => r.|)'), ['nodes', 'x', 'results', ELEMENT]);
  assert.deepEqual(path('nodes.x.results.map(r => r.|)'), ['nodes', 'x', 'results', ELEMENT]);
  assert.deepEqual(path('nodes.x.results.forEach(function (r) { r.| })'), ['nodes', 'x', 'results', ELEMENT]);
  assert.deepEqual(path('const list = nodes.x.results;\nlist.some((r) => r.|)'), ['nodes', 'x', 'results', ELEMENT]);
  assert.equal(at('nodes.x.results.map((r, i) => i.|)'), null);
  assert.equal(at('other((r) => r.|)'), null);
});

test('for...of variables are elements of the array', () => {
  assert.deepEqual(path('for (const row of nodes.x.results) { row.| }'), ['nodes', 'x', 'results', ELEMENT]);
});

test('declarations in an inner block win and do not leak out', () => {
  assert.deepEqual(path('const a = input.one;\nif (x) { const a = input.two; a.| }'), ['input', 'two']);
  assert.deepEqual(path('if (x) { const a = input.two; }\na.|'), ['a']);
});

test('a parameter named like a global hides it', () => {
  assert.equal(at('function f(input) { input.| }'), null);
});
