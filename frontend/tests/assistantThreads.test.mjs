import test from 'node:test';
import assert from 'node:assert/strict';

import {
  groupToolRows, removeThread, toolGroupStatus, toolLabel,
} from '../src/assistant/threads.mjs';

const chats = () => [{ id: 'a' }, { id: 'b' }, { id: 'c' }];

test('deleting another chat leaves the open one alone', () => {
  const out = removeThread(chats(), 'c', 'a');
  assert.deepEqual(out.threads.map((t) => t.id), ['a', 'b']);
  assert.equal(out.active, 'a');
});

test('deleting the open chat opens the one that took its place', () => {
  assert.equal(removeThread(chats(), 'a', 'a').active, 'b');
  assert.equal(removeThread(chats(), 'c', 'c').active, 'b', 'the last chat falls back to the one before');
});

test('deleting the only chat leaves nothing open, and unknown chats change nothing', () => {
  assert.equal(removeThread([{ id: 'a' }], 'a', 'a').active, null);
  assert.equal(removeThread(chats(), 'zz', 'a').threads.length, 3);
});

test('runs of tool calls collapse, and text keeps them apart', () => {
  const rows = groupToolRows([
    { type: 'text', id: '1' },
    { type: 'tool', id: '2', tool: { name: 'a' } },
    { type: 'tool', id: '3', tool: { name: 'b' } },
    { type: 'text', id: '4' },
    { type: 'tool', id: '5', tool: { name: 'c' } },
  ]);
  assert.deepEqual(rows.map((r) => r.type), ['text', 'tools', 'text', 'tools']);
  assert.equal(rows[1].tools.length, 2);
});

test('tool labels and status read plainly', () => {
  assert.equal(toolLabel('conversation_thread'), 'Conversation thread');
  assert.equal(toolLabel('propose_reply'), 'Reply');
  assert.equal(toolGroupStatus([{ status: 'completed' }, { status: 'failed' }]), 'failed');
  assert.equal(toolGroupStatus([{ status: 'failed' }, { status: 'running' }]), 'running');
  assert.equal(toolGroupStatus([{ status: 'completed' }]), 'completed');
});
