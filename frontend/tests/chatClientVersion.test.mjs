import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

import { ChatController } from '@pocketbase-mcp/chat-client';
import { bindAgentPanel } from '@pocketbase-mcp/chat-client/agent-panel';

test('the installed chat client exports ChatController and bindAgentPanel', () => {
  assert.equal(typeof ChatController, 'function');
  assert.equal(typeof bindAgentPanel, 'function');
});

function parseSemver(version) {
  const match = String(version).match(/^(\d+)\.(\d+)\.(\d+)/);
  assert.ok(match, `unparseable version ${version}`);
  return match.slice(1).map(Number);
}

test('the installed chat client is compatible with the pocketbase-mcp version in go.mod', async () => {
  const root = new URL('../../', import.meta.url);
  const goMod = await readFile(new URL('go.mod', root), 'utf8');
  const pkg = JSON.parse(await readFile(
    new URL('../node_modules/@pocketbase-mcp/chat-client/package.json', import.meta.url),
    'utf8',
  ));

  const moduleVersion = goMod.match(/^\s*github\.com\/compdani\/pocketbase-mcp\s+v(\S+)/m)?.[1];
  assert.ok(moduleVersion, 'go.mod has no github.com/compdani/pocketbase-mcp require');
  const [clientMajor, clientMinor, clientPatch] = parseSemver(pkg.version);
  const [modMajor, modMinor, modPatch] = parseSemver(moduleVersion);
  assert.equal(
    `${clientMajor}.${clientMinor}`,
    `${modMajor}.${modMinor}`,
    `@pocketbase-mcp/chat-client is ${pkg.version}, but go.mod uses v${moduleVersion}`,
  );
  assert.ok(
    clientPatch >= modPatch,
    `@pocketbase-mcp/chat-client ${pkg.version} is older than go.mod v${moduleVersion}`,
  );
});
