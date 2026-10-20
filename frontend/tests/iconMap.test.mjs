import test from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

import iconPaths from '../src/plugins/icons.js';

const SRC = new URL('../src', import.meta.url).pathname;
const SKIP = new Set(['icons.js', 'fontello.css']);

function walk(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return walk(path);
    return SKIP.has(name) ? [] : [path];
  });
}

test('every mdi-* icon used in src/ is in the icon map', () => {
  const used = new Set();
  walk(SRC).forEach((file) => {
    (readFileSync(file, 'utf8').match(/(?<![\w/])mdi-[a-z0-9-]+/g) || []).forEach((n) => used.add(n));
  });
  const missing = [...used].filter((n) => !iconPaths[n]);
  assert.deepEqual(missing, []);
});
