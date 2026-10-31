import test from 'node:test';
import assert from 'node:assert/strict';

import { dropIndex, moveCard, shiftKey } from '../src/utils/boards.mjs';

const card = (subscriber) => ({ subscriber });
const stages = () => [
  { key: 'new', total: 3, cards: [card('a'), card('b'), card('c')] },
  { key: 'won', total: 1, cards: [card('d')] },
];
const ids = (stage) => stage.cards.map((c) => c.subscriber).join('');

test('dropIndex counts the cards above the pointer', () => {
  assert.equal(dropIndex([10, 30, 50], 5), 0);
  assert.equal(dropIndex([10, 30, 50], 20), 1);
  assert.equal(dropIndex([10, 30, 50], 99), 3);
  assert.equal(dropIndex([], 10), 0);
});

test('moveCard moves between stages and keeps totals right', () => {
  const [from, to] = moveCard(stages(), 'b', 'won', 1);
  assert.equal(ids(from), 'ac');
  assert.equal(from.total, 2);
  assert.equal(ids(to), 'db');
  assert.equal(to.total, 2);
});

test('moveCard reorders within a stage, not counting the card itself', () => {
  const [stage] = moveCard(stages(), 'a', 'new', 2);
  assert.equal(ids(stage), 'bca');
  assert.equal(stage.total, 3);
});

test('moveCard clamps the index', () => {
  assert.equal(ids(moveCard(stages(), 'a', 'won', 99)[1]), 'da');
  assert.equal(ids(moveCard(stages(), 'a', 'won', -5)[1]), 'ad');
});

test('moveCard with no stage removes the card', () => {
  const [from, to] = moveCard(stages(), 'd', null);
  assert.equal(from.total, 3);
  assert.equal(to.total, 0);
  assert.equal(ids(to), '');
});

test('moveCard ignores an unknown card', () => {
  const before = stages();
  assert.equal(moveCard(before, 'zz', 'won'), before);
});

test('shiftKey moves within bounds', () => {
  assert.deepEqual(shiftKey(['a', 'b', 'c'], 'b', -1), ['b', 'a', 'c']);
  assert.deepEqual(shiftKey(['a', 'b', 'c'], 'b', 1), ['a', 'c', 'b']);
  assert.deepEqual(shiftKey(['a', 'b'], 'a', -1), ['a', 'b']);
  assert.deepEqual(shiftKey(['a', 'b'], 'b', 1), ['a', 'b']);
});
