import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
const api = readFileSync(new URL('../src/api/index.js', import.meta.url), 'utf8');
test('picture and tag clients use the permission-checked API', () => {
  for (const name of ['pictures', 'subscriber_tags', 'campaign_tags']) {
    assert.equal(api.includes(`pb.collection('${name}')`), false, `${name} bypasses mailapi`);
  }
  for (const route of ['pictures', 'subscriber-tags', 'campaign-tags']) {
    assert.ok(api.includes(`/api/${route}`));
  }
});
