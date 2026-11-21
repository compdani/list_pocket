import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

import { ChatController } from '@pocketbase-mcp/chat-client';
import { bindAgentPanel } from '@pocketbase-mcp/chat-client/agent-panel';

test('the installed chat client exports ChatController and bindAgentPanel', () => {
  assert.equal(typeof ChatController, 'function');
  assert.equal(typeof bindAgentPanel, 'function');
});

test('the installed chat client matches the pocketbase-mcp version in go.mod', async () => {
  const root = new URL('../../', import.meta.url);
  const goMod = await readFile(new URL('go.mod', root), 'utf8');
  const pkg = JSON.parse(await readFile(
    new URL('../node_modules/@pocketbase-mcp/chat-client/package.json', import.meta.url),
    'utf8',
  ));

  const moduleVersion = goMod.match(/^\s*github\.com\/compdani\/pocketbase-mcp\s+v(\S+)/m)?.[1];
  assert.ok(moduleVersion, 'go.mod has no github.com/compdani/pocketbase-mcp require');
  assert.equal(
    pkg.version,
    moduleVersion,
    `@pocketbase-mcp/chat-client is ${pkg.version}, but go.mod uses v${moduleVersion}`,
  );
});
