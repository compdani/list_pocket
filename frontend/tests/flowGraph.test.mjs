import test from 'node:test';
import assert from 'node:assert/strict';

import {
  connect, disconnect, errorNodeIds, newNode, newVariable, removeNode, renameNode, retypeVariable, sameDefinition, sourceHandles, tidyLayout, toFlow, triggerSummary,
} from '../src/utils/flowGraph.mjs';
import { FLOW_TEMPLATES } from '../src/utils/flowTemplates.mjs';

function sample() {
  return {
    name: 'x',
    trigger: { type: 'manual' },
    nodes: [
      { id: 'start', type: 'start', next: 'check' },
      { id: 'check', type: 'branch', cases: [{ if: 'input.a', to: 'each' }, { if: 'input.b', to: '' }], default: 'done' },
      { id: 'each', type: 'foreach', items: 'input.list', body: 'send', next: 'done' },
      { id: 'send', type: 'tool', tool: 't', input: { v: '{{ nodes.check.case }}' } },
      { id: 'done', type: 'end' },
    ],
  };
}

test('handles match each node type', () => {
  const d = sample();
  assert.deepEqual(sourceHandles(d.nodes[1]).map((h) => h.id), ['case-0', 'case-1', 'default']);
  assert.deepEqual(sourceHandles(d.nodes[2]).map((h) => h.id), ['body', 'next']);
  assert.deepEqual(sourceHandles(d.nodes[4]), []);
});

test('toFlow makes an edge for each connection, skipping empty and dangling ones', () => {
  const d = sample();
  d.nodes[0].next = 'check';
  d.nodes[4].next = 'gone';
  const { edges, nodes } = toFlow(d);
  assert.deepEqual(edges.map((e) => e.id).sort(), ['check:case-0', 'check:default', 'each:body', 'each:next', 'start:next']);
  assert.equal(nodes.find((n) => n.id === 'start').deletable, false);
});

test('connect and disconnect write back to the definition', () => {
  const d = sample();
  connect(d, { source: 'check', sourceHandle: 'case-1', target: 'done' });
  assert.equal(d.nodes[1].cases[1].to, 'done');
  disconnect(d, 'check:case-1');
  assert.equal(d.nodes[1].cases[1].to, '');
  disconnect(d, 'each:next');
  assert.equal(d.nodes[2].next, undefined);
  connect(d, { source: 'done', target: 'done' });
  assert.equal(d.nodes[4].next, undefined, 'a node can\'t connect to itself');
});

test('removing a node clears every reference to it', () => {
  const d = sample();
  removeNode(d, 'done');
  assert.equal(d.nodes[1].default, undefined);
  assert.equal(d.nodes[2].next, undefined);
});

test('renaming updates connections and templates, and refuses bad IDs', () => {
  const d = sample();
  const next = renameNode(d, 'check', 'decide');
  assert.equal(next.nodes[0].next, 'decide');
  assert.equal(next.nodes[3].input.v, '{{ nodes.decide.case }}');
  assert.equal(renameNode(d, 'check', 'Bad Id'), null);
  assert.equal(renameNode(d, 'check', 'send'), null);
});

test('tidyLayout orders by depth and survives cycles', () => {
  const d = sample();
  tidyLayout(d);
  const y = Object.fromEntries(d.nodes.map((n) => [n.id, n.ui.y]));
  assert.ok(y.start < y.check && y.check < y.each && y.each < y.send);
  const cyclic = { nodes: [{ id: 'start', type: 'start', next: 'a' }, { id: 'a', type: 'transform', next: 'start' }, { id: 'lost', type: 'end' }] };
  tidyLayout(cyclic);
  assert.equal(cyclic.nodes[2].ui.x, 900);
});

test('new node IDs stay unique', () => {
  const d = sample();
  d.nodes.push(newNode(d, 'wait', { x: 0, y: 0 }));
  assert.equal(newNode(d, 'wait', { x: 0, y: 0 }).id, 'wait_2');
});

test('code nodes start runnable and show their first line of code', () => {
  const d = sample();
  const n = newNode(d, 'code', { x: 0, y: 0 });
  assert.equal(n.id, 'code');
  assert.equal(n.code, 'return input;\n');
  assert.deepEqual(sourceHandles(n), [{ id: 'next', label: '' }]);
  d.nodes.push({ ...n, code: '// Totals\n\nconst list = nodes.check.list;\nreturn list.length;' });
  assert.equal(toFlow(d).nodes.find((x) => x.id === 'code').data.summary, 'const list = nodes.check.list;');
  assert.match(renameNode(d, 'check', 'decide').nodes.find((x) => x.id === 'code').code, /nodes\.decide\.list/);
});

test('set nodes start with a free variable and list their names', () => {
  const d = sample();
  const n = newNode(d, 'set', { x: 0, y: 0 });
  assert.equal(n.id, 'set');
  assert.deepEqual(n.variables, [{ name: 'var_1', type: 'template', value: '' }]);
  assert.equal(newVariable(n).name, 'var_2');
  n.variables.push({ name: 'subject', type: 'string', value: '' });
  d.nodes.push(n);
  assert.equal(toFlow(d).nodes.find((x) => x.id === 'set').data.summary, 'var_1, subject');
});

test('changing a variable type keeps a value the new type can use', () => {
  const retype = (value, type, extra = {}) => retypeVariable({ name: 'a', type: 'string', value, ...extra }, type);
  assert.deepEqual(retype('hi', 'template'), { name: 'a', type: 'template', value: 'hi' });
  assert.deepEqual(retype('hi', 'number'), { name: 'a', type: 'number', value: 0 });
  assert.deepEqual(retype('{{ input.n }}', 'number'), { name: 'a', type: 'number', value: '{{ input.n }}' });
  assert.deepEqual(retype(5, 'boolean'), { name: 'a', type: 'boolean', value: false });
  assert.deepEqual(retype(false, 'string'), { name: 'a', type: 'string', value: 'false' });
  assert.deepEqual(retype('', 'code'), { name: 'a', type: 'code', value: 'return input;\n' });
  assert.deepEqual(retype('<b>', 'html'), { name: 'a', type: 'html', value: '<b>', editor: 'visual' });
  assert.deepEqual(retype('<b>', 'template', { type: 'html', editor: 'code' }), { name: 'a', type: 'template', value: '<b>' });
});

test('helpers read validation messages and compare without layout', () => {
  assert.deepEqual([...errorNodeIds(['node "send": tool is required', 'workflow needs one start'])], ['send']);
  const a = sample();
  const b = sample();
  b.nodes[0].ui = { x: 5, y: 5 };
  assert.ok(sameDefinition(a, b));
  b.nodes[0].next = '';
  assert.ok(!sameDefinition(a, b));
  assert.ok(sameDefinition({ trigger: { type: 'event', filter: '' }, m: { a: 1, b: 2 } }, { m: { b: 2, a: 1 }, trigger: { type: 'event' } }), 'key order and empty values don\'t count');
  assert.equal(triggerSummary({ type: 'event', event: 'inbound.email' }), 'When inbound.email');
});

test('starter templates are valid-looking definitions', () => {
  FLOW_TEMPLATES.forEach((t) => {
    assert.match(t.definition.name, /^[a-z][a-z0-9_]+$/);
    assert.equal(t.definition.nodes.filter((n) => n.type === 'start').length, 1);
    const ids = new Set(t.definition.nodes.map((n) => n.id));
    t.definition.nodes.forEach((n) => {
      [n.next, n.default, n.body, ...(n.cases || []).map((c) => c.to)].filter(Boolean).forEach((to) => assert.ok(ids.has(to), `${t.key}: ${to}`));
    });
  });
});
