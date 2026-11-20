import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

// Node's test runner doesn't load Vite's alias table, so these two imports use the same
// files the alias points at. In the app, import them as
// '@pocketbase-mcp/chat-client' and '@pocketbase-mcp/chat-client/agent-panel'.
import { ChatController } from '../src/vendor/chat-client/index.mjs';
import { bindAgentPanel } from '../src/vendor/chat-client/agent-panel.mjs';

test('the vendored chat client exports ChatController and bindAgentPanel', () => {
  assert.equal(typeof ChatController, 'function');
  assert.equal(typeof bindAgentPanel, 'function');
});

test('the vendored chat client matches the pocketbase-mcp version in go.mod', async () => {
  const root = new URL('../../', import.meta.url);
  const goMod = await readFile(new URL('go.mod', root), 'utf8');
  const vendored = await readFile(new URL('frontend/src/vendor/chat-client/VENDORED.md', root), 'utf8');

  const moduleVersion = goMod.match(/^\s*github\.com\/compdani\/pocketbase-mcp\s+v(\S+)/m)?.[1];
  const vendoredVersion = vendored.match(/github\.com\/compdani\/pocketbase-mcp\s+v(\S+?)[`\s]/)?.[1];
  assert.ok(moduleVersion, 'go.mod has no github.com/compdani/pocketbase-mcp require');
  assert.ok(vendoredVersion, 'VENDORED.md has no github.com/compdani/pocketbase-mcp version');
  assert.equal(
    vendoredVersion,
    moduleVersion,
    `the vendored chat client is from v${vendoredVersion}, but go.mod uses v${moduleVersion}; re-copy it and update VENDORED.md`,
  );
});
