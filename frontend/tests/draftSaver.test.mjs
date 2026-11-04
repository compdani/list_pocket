import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createDraftSaver } from '../src/utils/draftSaver.mjs';

test('writes snapshots in order and recovers after a failed save', async () => {
  const calls = [];
  let release;
  const blocked = new Promise((resolve) => { release = resolve; });
  const save = createDraftSaver(async (id, draft) => {
    calls.push([id, draft]);
    if (calls.length === 1) { await blocked; throw new Error('offline'); }
    return draft;
  });
  const draft = { title: 'first' };
  const first = save('a', draft);
  const rejected = assert.rejects(first, /offline/);
  draft.title = 'second';
  const second = save('a', draft);
  await Promise.resolve();
  assert.equal(calls.length, 1);
  assert.equal(calls[0][1].title, 'first');
  release();
  await rejected;
  assert.deepEqual(await second, { title: 'second' });
  assert.equal(calls.length, 2);
});

test('flush waits for edits made during an in-flight save', async () => {
  const { createDraftFlusher } = await import('../src/utils/draftSaver.mjs');
  let revision = 1;
  let release;
  const blocked = new Promise((resolve) => { release = resolve; });
  const writes = [];
  const flush = createDraftFlusher(async (_id, draft) => {
    writes.push(draft);
    if (writes.length === 1) await blocked;
    return draft;
  }, () => ({ id: 'a', revision, draft: revision }));
  const result = flush();
  revision = 2;
  release();
  assert.equal((await result).revision, 2);
  assert.deepEqual(writes, [1, 2]);
});
