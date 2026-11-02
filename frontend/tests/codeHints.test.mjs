import test from 'node:test';
import assert from 'node:assert/strict';

import { ELEMENT, codeHints } from '../src/utils/codeHints.mjs';

const CONTACT = {
  type: 'object',
  properties: {
    status: { type: 'string' },
    boards: { type: 'array', items: { type: 'object', properties: { board: { type: 'string' }, stage: { type: 'string' } } } },
  },
};

const CATALOG = {
  tools: [
    { name: 'contacts.get', output_schema: CONTACT },
    { name: 'contacts.search', output_schema: { type: 'object', properties: { total: { type: 'integer' }, results: { type: 'array', items: CONTACT } } } },
    { name: 'boards.list', output_schema: { type: 'array', items: { type: 'object', properties: { key: { type: 'string' } } } } },
    { name: 'plugin.undescribed' },
  ],
};

function sample() {
  return {
    name: 'x',
    trigger: { type: 'manual' },
    input_schema: { type: 'object', properties: { contact: { type: 'object', properties: { email: { type: 'string' } } } } },
    default_input: { limit: 5 },
    nodes: [
      { id: 'start', type: 'start', next: 'classify' },
      { id: 'classify', type: 'agent', name: 'Classify', agent: { output_schema: { type: 'object', properties: { score: { type: 'number' } } } }, next: 'check' },
      { id: 'check', type: 'branch', cases: [{ if: 'input.a', to: 'code' }], default: 'other' },
      { id: 'other', type: 'tool', tool: 'plugin.undescribed', on_error: 'continue', next: 'done' },
      { id: 'code', type: 'code', code: '', next: 'shape' },
      { id: 'shape', type: 'transform', value: { a: { b: 1 } }, next: 'lookup' },
      { id: 'lookup', type: 'tool', tool: 'contacts.get', next: 'search' },
      { id: 'search', type: 'tool', tool: 'contacts.search', next: 'each' },
      { id: 'each', type: 'foreach', items: 'nodes.search.results', body: 'each_code', next: 'repeat' },
      { id: 'each_code', type: 'code', code: '', next: 'each_more' },
      { id: 'each_more', type: 'code', code: '' },
      { id: 'repeat', type: 'loop', body: 'loop_code', until: 'true', max_iterations: 2, next: 'boards' },
      { id: 'loop_code', type: 'code', code: '' },
      { id: 'boards', type: 'tool', tool: 'boards.list', next: 'after' },
      { id: 'after', type: 'code', code: '', next: 'done' },
      { id: 'done', type: 'end' },
      { id: 'loose', type: 'code', code: '' },
    ],
  };
}

const labels = (path, id = 'code', def = sample()) => codeHints(path, def, id, CATALOG).map((option) => option.label);

test('offers the run data and sandbox globals, not browser ones', () => {
  const globals = labels([]);
  ['input', 'nodes', 'now', 'run', 'console', 'JSON', 'Math'].forEach((name) => assert.ok(globals.includes(name), name));
  ['window', 'document', 'fetch', 'setTimeout'].forEach((name) => assert.ok(!globals.includes(name), name));
});

test('offers keywords that compile in a function body', () => {
  const globals = labels([]);
  ['const', 'return', 'if', 'for'].forEach((name) => assert.ok(globals.includes(name), name));
  ['import', 'export', 'await'].forEach((name) => assert.ok(!globals.includes(name), name));
});

test('input fields come from the schema and default input', () => {
  assert.deepEqual(labels(['input']).sort(), ['contact', 'limit']);
  assert.deepEqual(labels(['input', 'contact']), ['email']);
  assert.deepEqual(labels(['input', 'limit']), []);
});

test('nodes lists only steps that can have finished', () => {
  assert.deepEqual(labels(['nodes']).sort(), ['check', 'classify', 'start']);
  assert.ok(labels(['nodes'], 'lookup').includes('code'));
});

test('tool outputs come from the catalog', () => {
  assert.deepEqual(labels(['nodes', 'lookup'], 'search'), ['status', 'boards']);
  assert.deepEqual(labels(['nodes', 'search'], 'each'), ['total', 'results']);
});

test('a tool without an output schema offers only what is known', () => {
  assert.deepEqual(labels(['nodes', 'other'], 'done'), ['error']);
});

test('array outputs offer array members and their elements', () => {
  const boards = labels(['nodes', 'boards'], 'after');
  assert.ok(boards.includes('length') && boards.includes('map'));
  assert.deepEqual(labels(['nodes', 'boards', ELEMENT], 'after'), ['key']);
  assert.deepEqual(labels(['nodes', 'search', 'results', ELEMENT, 'boards', ELEMENT], 'each'), ['board', 'stage']);
});

test('item, index and loop exist only where the engine sets them', () => {
  ['item', 'index', 'loop'].forEach((name) => assert.ok(!labels([]).includes(name), name));
  const inForeach = labels([], 'each_more');
  assert.ok(inForeach.includes('item') && inForeach.includes('index') && !inForeach.includes('loop'));
  const inLoop = labels([], 'loop_code');
  assert.ok(inLoop.includes('loop') && inLoop.includes('index') && !inLoop.includes('item'));
  assert.deepEqual(labels(['loop'], 'loop_code'), ['iteration', 'last']);
  assert.ok(!labels([], 'after').includes('item'));
});

test('item takes its fields from the foreach list', () => {
  assert.deepEqual(labels(['item'], 'each_code'), ['status', 'boards']);
  assert.deepEqual(labels(['item', 'boards', ELEMENT], 'each_code'), ['board', 'stage']);
});

test('item handles template braces, schema lists and unknown expressions', () => {
  const def = sample();
  const each = def.nodes.find((n) => n.id === 'each');
  each.items = '{{ nodes.search.results }}';
  assert.deepEqual(labels(['item'], 'each_code', def), ['status', 'boards']);
  def.input_schema.properties.rows = { type: 'array', items: { type: 'object', properties: { sku: { type: 'string' } } } };
  each.items = 'input.rows';
  assert.deepEqual(labels(['item'], 'each_code', def), ['sku']);
  each.items = 'filter(input.rows, .ok)';
  assert.ok(labels([], 'each_code', def).includes('item'));
  assert.deepEqual(labels(['item'], 'each_code', def), []);
});

test('a body does not see its own foreach, which has not finished', () => {
  const steps = labels(['nodes'], 'each_more');
  assert.ok(steps.includes('each_code') && steps.includes('search'));
  assert.ok(!steps.includes('each'));
  assert.ok(labels(['nodes'], 'repeat').includes('each'));
});

test('a step after a body does not see the steps inside it', () => {
  assert.ok(!labels(['nodes'], 'after').includes('each_code'));
});

test('a step that is not connected sees the others, marked as such', () => {
  const options = codeHints(['nodes'], sample(), 'loose', CATALOG);
  assert.equal(options.length, sample().nodes.length - 1);
  assert.equal(options.find((option) => option.label === 'classify').detail, 'Classify · connect this step first');
});

test('unknown paths offer nothing, including names on Object.prototype', () => {
  assert.deepEqual(labels(['raw']), []);
  assert.deepEqual(labels(['nodes', 'missing']), []);
  assert.deepEqual(labels(['constructor']), []);
  assert.deepEqual(labels(['input', 'toString']), []);
  assert.deepEqual(labels(['nodes', 'boards', 'constructor'], 'after'), []);
  assert.deepEqual(labels(['input', ELEMENT]), []);
});

test('connected nodes show their label as detail', () => {
  const option = codeHints(['nodes'], sample(), 'code', CATALOG).find((item) => item.label === 'classify');
  assert.equal(option.detail, 'Classify');
});
