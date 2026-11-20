import test from 'node:test';
import assert from 'node:assert/strict';

import { bindAgentPanel, errorText, PROPOSAL_STATE_TEXT as BINDING_PROPOSAL_STATE_TEXT } from '@pocketbase-mcp/chat-client/agent-panel';
import { applyEditorPatch, revision } from '../src/assistant/patches.mjs';
import {
  AGENT_REF_CAP, MID_APPLY_TEXT, PANEL_DRAFT_PICTURES, PROPOSAL_STATE_TEXT, PROVIDER_AUTH_TEXT,
  UNDO_CHANGED_TEXT, USAGE_LIMIT_TEXT, WRONG_EDITOR_TEXT, backendOf,
  cardProposalOf, chatErrorText, choiceIdFor, cleanPictures, contextReferencesFromTranscript,
  createEditorAdapter, editorContextOf, judgeEditorProposal, labelToolEntries,
  moveDraftPictures, navigationWrite, pageContext, panelPageContext, picturesFor,
  proposalProblemText, referencesFromContext, referencesOf, retryText,
  reviewPartOf, savePictures, storedModelFor, undoLastEdit,
} from '../src/assistant/chat-host.mjs';

// --- page context -----------------------------------------------------------

test('the page context names the agent screen and carries only references', () => {
  assert.deepEqual(
    pageContext('/agent/c1?ref=list&id=3', [
      { type: 'campaign', id: '7', label: 'Spring sale' },
      { type: 'file', id: 'f1', label: 'leads.csv' },
      { type: 'list', id: 3 },
      { type: 'list', id: '' },
      null,
    ]),
    {
      screen: 'agent',
      path: '/agent/c1?ref=list&id=3',
      references: [{ type: 'campaign', id: '7', label: 'Spring sale' }, { type: 'list', id: '3' }],
    },
  );
  assert.deepEqual(referencesOf(undefined), []);
  assert.deepEqual(pageContext(undefined, undefined), { screen: 'agent', path: '', references: [] });
});

// --- stored references on user turns ---------------------------------------

test('stored references become the chips the binding option returns', () => {
  assert.deepEqual(
    referencesFromContext({
      screen: 'agent',
      path: '/agent/c1',
      references: [
        { type: 'campaign', id: '7', label: 'Spring sale' },
        { type: 'file', id: 'f1', label: 'leads.csv' },
        { type: 'list', id: 3 },
      ],
    }),
    // A reference stored without a label gets its id, or the binding would drop it.
    [{ type: 'campaign', id: '7', label: 'Spring sale' }, { type: 'list', id: '3', label: '3' }],
  );
  assert.deepEqual(referencesFromContext(null), []);
  assert.deepEqual(referencesFromContext({}), []);
});

test('"in context" keeps the most recent distinct references newest first', () => {
  const context = (messageId, references) => ({
    id: `${messageId}:1`, messageId, role: 'user', type: 'context', result: { references },
  });
  const transcript = [
    context('m1', [{ type: 'list', id: 'one', label: 'One' }, { type: 'contact', id: 'shared' }]),
    { id: 'm2:0', messageId: 'm2', role: 'user', type: 'text' },
    context('m3', [
      { type: 'contact', id: 'shared' },
      { type: 'file', id: 'f1' },
      { type: 'list', id: 'two', label: 'Two' },
    ]),
  ];
  assert.deepEqual(contextReferencesFromTranscript(transcript), [
    { type: 'contact', id: 'shared' },
    { type: 'list', id: 'two', label: 'Two' },
    { type: 'list', id: 'one', label: 'One' },
  ]);
});

test('"in context" stops at the backend cap of 30, keeping the newest', () => {
  const transcript = [];
  for (let i = 0; i < 40; i += 1) {
    transcript.push({
      id: `m${i}:1`,
      messageId: `m${i}`,
      role: 'user',
      type: 'context',
      result: { references: [{ type: 'list', id: `l${i}` }] },
    });
  }
  const refs = contextReferencesFromTranscript(transcript);
  assert.equal(refs.length, AGENT_REF_CAP);
  assert.equal(AGENT_REF_CAP, 30);
  assert.deepEqual(refs[0], { type: 'list', id: 'l39' });
  assert.deepEqual(refs[29], { type: 'list', id: 'l10' });
});

// --- proposals --------------------------------------------------------------

test('a pending proposal maps to the card and ReviewModal as editable', () => {
  const proposal = {
    id: 'p1',
    tool: 'create_campaign_draft',
    state: 'pending',
    revision: 3,
    arguments: { summary: 'Spring launch draft' },
    warnings: ['Test runs do real work.'],
  };
  const card = cardProposalOf(proposal, { busy: true, running: false });
  assert.equal(card.proposal.status, 'pending');
  assert.equal(card.proposal.state, 'pending');
  assert.equal(card.proposal.stateText, '');
  assert.deepEqual(card.proposal.warnings, ['Test runs do real work.']);
  assert.equal(card.busy, true);
  assert.equal(card.actionId, 'p1');

  const part = reviewPartOf(proposal);
  assert.equal(part.id, 'p1');
  assert.equal(part.status, 'pending');
  assert.deepEqual(part.arguments, { summary: 'Spring launch draft' });
});

test('a succeeded proposal reads as applied and keeps its Open link', () => {
  const card = cardProposalOf({
    id: 'p2',
    tool: 'create_campaign_draft',
    state: 'succeeded',
    revision: 4,
    arguments: {},
    result: { source: { href: '/admin/campaigns/9' } },
  });
  assert.equal(card.proposal.status, 'applied');
  assert.equal(card.proposal.statusLabel, 'Applied');
  assert.equal(card.proposal.stateText, 'The server applied this change.');
  assert.equal(card.proposal.result.source.href, '/admin/campaigns/9');
  assert.equal(reviewPartOf({ id: 'p2', tool: 'create_campaign_draft', state: 'succeeded', arguments: {} }).status, 'applied');
});

test('the unknown state says it may or may not have applied, never success or failure', () => {
  const card = cardProposalOf({
    id: 'p3',
    tool: 'propose_reply',
    state: 'unknown',
    revision: 2,
    arguments: {},
    failure: 'Outcome unknown after restart; check before retrying.',
  });
  assert.equal(card.proposal.status, 'unknown');
  assert.equal(card.proposal.statusLabel, 'Unknown');
  assert.match(card.proposal.stateText, /may or may not have applied/);
  assert.match(card.proposal.error, /check before retrying/);
  assert.notEqual(card.proposal.status, 'applied');
  assert.notEqual(card.proposal.status, 'failed');
});

test('a refused or conflicting decision shows its message on the card', () => {
  const refused = cardProposalOf({
    id: 'p4',
    tool: 'propose_contact_update',
    state: 'pending',
    revision: 5,
    arguments: {},
    error: { code: 'refused', message: 'List not found.' },
  });
  assert.match(refused.proposal.error, /List not found\./);
  assert.match(refused.proposal.error, /waiting for your decision again/);

  const conflict = cardProposalOf({
    id: 'p5',
    tool: 'propose_reply',
    state: 'pending',
    revision: 6,
    arguments: {},
    error: { code: 'conflict', message: 'changed elsewhere' },
  });
  assert.match(conflict.proposal.error, /decided somewhere else/);

  const failed = cardProposalOf({
    id: 'p6',
    tool: 'propose_reply',
    state: 'failed',
    revision: 2,
    arguments: {},
    failure: 'The list was deleted.',
  });
  assert.equal(failed.proposal.error, 'The list was deleted.');
  assert.equal(reviewPartOf({ id: 'p6', tool: 'x', state: 'failed', arguments: {}, failure: 'The list was deleted.' }).error, 'The list was deleted.');
});

test('an unknown proposal state keeps its name on the card instead of guessing', () => {
  const card = cardProposalOf({ id: 'p7', tool: 'x', state: 'something_new', revision: 1, arguments: {} });
  assert.equal(card.proposal.status, 'something_new');
  assert.equal(card.proposal.statusLabel, 'something_new');
});

// --- errors -----------------------------------------------------------------

test('the card wording matches the installed binding', () => {
  assert.deepEqual(PROPOSAL_STATE_TEXT, BINDING_PROPOSAL_STATE_TEXT);
  assert.match(errorText({ code: 'provider_auth' }), /provider/i);
  assert.match(errorText({ code: 'usage_limit' }), /usage limit/i);
  assert.match(errorText({ code: 'no_provider' }), /isn't set up/);
});

test('provider failures and a missing provider use the app wording', () => {
  const noProvider = "AI isn't set up. Add an API key or sign in with ChatGPT in Settings → AI, then reload this page.";
  assert.equal(chatErrorText('provider_auth', noProvider), PROVIDER_AUTH_TEXT);
  assert.equal(chatErrorText('usage_limit', noProvider), USAGE_LIMIT_TEXT);
  assert.match(PROVIDER_AUTH_TEXT, /Settings → AI/);
  assert.equal(chatErrorText('no_provider', noProvider), noProvider);
  assert.equal(chatErrorText('internal', noProvider), undefined);
  assert.equal(chatErrorText('no_provider', ''), undefined);
});

test('only a refusal carries the server message; every other code stays generic', () => {
  assert.match(proposalProblemText({ code: 'refused', message: 'List not found.' }), /List not found\./);
  assert.equal(proposalProblemText({ code: 'conflict', message: 'anything at all' }), 'This change was decided somewhere else. Review the latest version.');
  assert.equal(proposalProblemText({ code: 'teapot', message: 'raw text' }), 'Something went wrong. Try again.');
  assert.equal(proposalProblemText(null), '');
});

// --- host choice and model memory ------------------------------------------

test('the /agent route waits for config, then picks exactly one host', () => {
  assert.equal(backendOf(undefined), '');
  assert.equal(backendOf([]), '');
  assert.equal(backendOf({}), '');
  assert.equal(backendOf({ chat_backend: 'module' }), 'module');
  assert.equal(backendOf({ chat_backend: 'legacy' }), 'legacy');
  assert.equal(backendOf({ chat_backend: 'something' }), 'legacy');
  assert.equal(backendOf({ from_email: 'x' }), 'legacy');
});

test('the remembered legacy model preselects a matching choice, or none', () => {
  const choices = [
    { id: 'codex-work::gpt-5-codex', provider: 'codex-work', model: 'gpt-5-codex' },
    { id: 'api::gpt-4.1', provider: 'api', model: 'gpt-4.1' },
  ];
  assert.equal(choiceIdFor({ provider: 'api', model: 'gpt-4.1' }, choices), 'api::gpt-4.1');
  assert.equal(choiceIdFor({ model: 'gpt-5-codex' }, choices), 'codex-work::gpt-5-codex');
  assert.equal(choiceIdFor('gpt-4.1', choices), 'api::gpt-4.1');
  assert.equal(choiceIdFor({ provider: 'api', model: 'gone' }, choices), '');
  assert.equal(choiceIdFor(null, choices), '');
  assert.equal(choiceIdFor({ model: 'gpt-4.1' }, []), '');
  assert.deepEqual(storedModelFor('api::gpt-4.1', choices), { provider: 'api', model: 'gpt-4.1' });
  assert.equal(storedModelFor('', choices), null);
});

// The binding reads the page context synchronously inside its agent-submit
// handler, before a later listener (the element's own cleanup) clears the
// composer. The host's context() can therefore read the element directly.
class FakePage extends EventTarget {
  constructor() {
    super();
    this.tagName = 'AGENT-PAGE';
    this.entries = [];
    this.running = false;
    this.sending = false;
    this.disabled = false;
    this.draft = '';
    this.references = [];
    this.error = '';
    this.models = [];
    this.model = '';
    this.options = [];
    this.conversations = [];
    this.active = '';
    this.heading = '';
    this.hasMore = false;
    this.attributes = new Map();
  }

  setAttribute(name, value) { this.attributes.set(name, String(value)); }

  getAttribute(name) { return this.attributes.has(name) ? this.attributes.get(name) : null; }

  removeAttribute(name) { this.attributes.delete(name); }

  emitAction() { return true; }
}

function readyController() {
  const controller = new EventTarget();
  controller.state = {
    status: 'ready',
    conversations: [],
    nextCursor: '',
    activeConversationId: 'c1',
    messages: [],
    transcript: [],
    proposals: [],
    decidedProposals: [],
    pendingUploads: [],
    turn: null,
    running: false,
    sending: false,
    error: null,
    features: { context: true, ready: true, uploads: true },
    limits: null,
    models: [],
    choices: [],
    defaultChoice: '',
    choice: '',
    context: null,
  };
  return controller;
}

test('the binding reads page context during submit, before the composer is cleared', async () => {
  const element = new FakePage();
  const chips = [{ type: 'campaign', id: '7', label: 'Spring sale' }];
  element.references = chips;
  element.draft = 'Compare these';
  const seen = [];
  const controller = readyController();
  controller.readContext = () => {
    seen.push(element.references.map((ref) => ({ ...ref })));
    return pageContext('/agent/c1', element.references);
  };
  let sent = null;
  controller.send = (_text, options) => {
    sent = options;
    return Promise.resolve({ turn: { id: 't1' }, created: true });
  };
  bindAgentPanel(element, controller, { registerEntries: false });
  element.addEventListener('agent-submit', () => {
    element.references = [];
    element.draft = '';
  });

  element.dispatchEvent(new CustomEvent('agent-submit', {
    detail: {
      text: 'Compare these',
      attachments: [],
      references: chips,
      fromDraft: true,
      model: '',
      options: {},
    },
  }));
  await Promise.resolve();

  assert.deepEqual(seen, [chips]);
  assert.deepEqual(sent.context, pageContext('/agent/c1', chips));
});

test('a second submit is refused while the first is still being sent', async () => {
  const element = new FakePage();
  const controller = readyController();
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  const sent = [];
  controller.readContext = () => ({ screen: 'agent', path: '/agent', references: [] });
  controller.send = (text) => {
    sent.push(text);
    return gate.then(() => ({ turn: { id: 't1' }, created: true }));
  };
  const outcomes = [];
  bindAgentPanel(element, controller, {
    registerEntries: false,
    onSend: (result) => outcomes.push(result),
  });

  const detail = {
    attachments: [], references: [], fromDraft: true, model: '', options: {},
  };
  element.dispatchEvent(new CustomEvent('agent-submit', { detail: { ...detail, text: 'One' } }));
  element.dispatchEvent(new CustomEvent('agent-submit', { detail: { ...detail, text: 'Two' } }));
  assert.deepEqual(sent, ['One']);
  assert.deepEqual(outcomes.map((result) => result.code), ['busy']);
  release();
  await gate;
  for (let i = 0; i < 5; i += 1) await Promise.resolve();
  assert.equal(outcomes.at(-1).status, 'accepted');
});

test('a failed send puts the draft and its references back', async () => {
  const element = new FakePage();
  const chips = [{ type: 'campaign', id: '7', label: 'Spring sale' }];
  element.draft = 'Keep this';
  element.references = chips.slice();
  const controller = readyController();
  controller.readContext = () => pageContext('/agent/c1', element.references);
  controller.send = () => {
    controller.state = { ...controller.state, error: { code: 'internal', message: 'nope' } };
    return Promise.resolve(null);
  };
  bindAgentPanel(element, controller, { registerEntries: false });
  element.addEventListener('agent-submit', () => {
    element.draft = '';
    element.references = [];
  });

  element.dispatchEvent(new CustomEvent('agent-submit', {
    detail: {
      text: 'Keep this',
      attachments: [],
      references: chips,
      fromDraft: true,
      model: '',
      options: {},
    },
  }));
  for (let i = 0; i < 6; i += 1) await Promise.resolve();
  assert.equal(element.draft, 'Keep this');
  assert.deepEqual(element.references, chips);
});

test('history push is for a choice and replace is for a draft\'s new id', () => {
  assert.deepEqual(
    navigationWrite({ activeId: 'new1', routeId: '', intent: 'replace' }),
    { method: 'replace', id: 'new1' },
  );
  assert.deepEqual(
    navigationWrite({ activeId: 'c2', routeId: 'c1', intent: 'push' }),
    { method: 'push', id: 'c2' },
  );
  assert.deepEqual(
    navigationWrite({ activeId: '', routeId: 'c1', intent: 'push' }),
    { method: 'push', id: '' },
  );
  assert.equal(navigationWrite({ activeId: 'c1', routeId: 'c1', intent: 'push' }), null);
  // A load that is no longer the URL must not write the URL.
  assert.equal(navigationWrite({
    activeId: 'c1', routeId: 'c2', intent: 'replace', suspended: true,
  }), null);
});

// --- side panel context -----------------------------------------------------

test('the side panel context describes the page, the editor and the pictures', () => {
  const current = { subject: 'Hello', body: 'Shop today', contentType: 'html' };
  const editor = {
    key: 'campaign:example',
    label: 'Unsaved campaign editor',
    snapshot: () => current,
    selection: () => 'Shop',
  };
  assert.deepEqual(panelPageContext({
    path: '/campaigns/example',
    page: 'campaign',
    params: { id: 'example' },
    filters: { status: 'live' },
    references: [{ type: 'list', id: '3', label: 'Customers' }, { type: 'file', id: 'f1' }],
    editor: editorContextOf(editor, revision, 'window selection'),
    pictures: [{ name: 'hero.png', url: 'https://cdn.example/hero.png' }, { url: 'https://cdn.example/hero.png' }],
  }), {
    screen: 'panel',
    path: '/campaigns/example',
    page: 'campaign',
    params: { id: 'example' },
    filters: { status: 'live' },
    editor: {
      key: 'campaign:example',
      unsaved: true,
      revision: revision(current),
      current,
      selection: 'Shop',
      label: 'Unsaved campaign editor',
    },
    pictureReferences: [{ name: 'hero.png', url: 'https://cdn.example/hero.png' }],
    references: [{ type: 'list', id: '3', label: 'Customers' }],
  });
  assert.equal(editorContextOf(null, revision), null);
  assert.equal(editorContextOf({ key: 'campaign:1', snapshot: () => { throw new Error('closed'); } }, revision), null);
  const bare = { key: 'campaign:1', snapshot: () => current };
  assert.equal(editorContextOf(bare, revision, 'from the window').selection, 'from the window');
});

test('media pictures are stored per conversation and a draft moves on the first send', () => {
  const storage = new Map();
  const box = {
    getItem: (key) => (storage.has(key) ? storage.get(key) : null),
    setItem: (key, value) => { storage.set(key, String(value)); },
  };
  assert.equal(savePictures(box, 'user-1', '', [{ name: 'hero.png', url: 'https://cdn.example/hero.png' }]), true);
  assert.deepEqual(picturesFor(box, 'user-1', ''), [{ name: 'hero.png', url: 'https://cdn.example/hero.png' }]);
  assert.deepEqual(picturesFor(box, 'user-2', ''), []);
  assert.deepEqual(
    moveDraftPictures(box, 'user-1', 'conv-1'),
    [{ name: 'hero.png', url: 'https://cdn.example/hero.png' }],
  );
  assert.deepEqual(picturesFor(box, 'user-1', ''), []);
  assert.deepEqual(picturesFor(box, 'user-1', 'conv-1'), [{ name: 'hero.png', url: 'https://cdn.example/hero.png' }]);
  savePictures(box, 'user-1', 'conv-1', [
    { name: 'hero.png', url: 'https://cdn.example/hero.png' },
    { name: 'side.png', url: 'https://cdn.example/side.png' },
  ]);
  savePictures(box, 'user-1', '', [{ name: 'other.png', url: 'https://cdn.example/side.png' }]);
  assert.deepEqual(moveDraftPictures(box, 'user-1', 'conv-1').map((picture) => picture.url), [
    'https://cdn.example/hero.png',
    'https://cdn.example/side.png',
  ]);
  assert.equal(picturesFor(box, 'user-1', PANEL_DRAFT_PICTURES).length, 0);
  assert.deepEqual(cleanPictures([{ url: '' }, null, { url: 'https://cdn.example/a.png' }]), [
    { name: '', url: 'https://cdn.example/a.png' },
  ]);
});

test('picture storage failures leave the caller with nothing new', () => {
  const broken = {
    getItem: () => { throw new Error('denied'); },
    setItem: () => { throw new Error('full'); },
  };
  assert.deepEqual(picturesFor(broken, 'user-1', 'conv-1'), []);
  assert.equal(savePictures(broken, 'user-1', 'conv-1', [{ url: 'https://cdn.example/a.png' }]), false);
  assert.deepEqual(moveDraftPictures(broken, 'user-1', 'conv-1'), []);
});

test('tool rows gain the legacy label when the binding did not set one', () => {
  assert.deepEqual(labelToolEntries([
    { type: 'tools', tools: [{ name: 'propose_editor_patch' }, { name: 'campaigns', label: 'Kept' }] },
    { type: 'text' },
  ], (name) => String(name).replace(/^propose_/, '')), [
    { type: 'tools', tools: [{ name: 'propose_editor_patch', label: 'editor_patch' }, { name: 'campaigns', label: 'Kept' }] },
    { type: 'text' },
  ]);
});

function fakeEditor(key, value) {
  const editor = {
    key,
    label: key,
    value,
    snapshot: () => editor.value,
    apply: async (next) => { editor.value = next; },
  };
  return editor;
}

test('an editor patch applies on a campaign, a template and a workflow', async () => {
  const cases = [
    {
      key: 'campaign:example',
      value: { subject: 'Welcome', preheader: 'Preview', body: '<a>Shop today</a>', contentType: 'html' },
      operations: [{ op: 'replace_text', before: 'Shop today', after: 'Shop' }],
      check: (value) => assert.equal(value.body, '<a>Shop</a>'),
    },
    {
      key: 'campaign:body',
      value: { subject: 'Hello', preheader: 'Old', body: '<p>Old</p>', contentType: 'html' },
      operations: [{ op: 'replace_body', before: '<p>Old</p>', after: '<p>New</p>' }],
      check: (value) => assert.equal(value.body, '<p>New</p>'),
    },
    {
      key: 'template:example:campaign:html',
      value: { subject: 'Hello', preheader: '', body: 'Hello there', contentType: 'plain' },
      operations: [{ op: 'set', field: 'subject', before: 'Hello', after: 'Hi' }],
      check: (value) => assert.equal(value.subject, 'Hi'),
    },
    {
      key: 'workflow:example',
      value: {
        kind: 'workflow',
        nodes: [{ id: 'wait', type: 'wait', config: { days: 1 } }],
        edges: [{ id: 'e1', source: 'trigger', target: 'wait' }],
      },
      operations: [
        { op: 'node_upsert', id: 'wait', value: { id: 'wait', type: 'wait', config: { days: 2 } } },
        { op: 'edge_remove', id: 'e1' },
      ],
      check: (value) => {
        assert.equal(value.nodes[0].config.days, 2);
        assert.deepEqual(value.edges, []);
      },
    },
  ];
  for (const item of cases) {
    const editor = fakeEditor(item.key, item.value);
    let applied = null;
    const adapter = createEditorAdapter({
      getEditor: () => editor,
      revisionOf: revision,
      applyPatch: applyEditorPatch,
      onApplied: (entry) => { applied = entry; },
    });
    const proposal = {
      id: 'e1',
      arguments: { editorId: item.key, baseRevision: revision(item.value), operations: item.operations },
    };
    assert.deepEqual(adapter.snapshot(), { editorId: item.key, revision: revision(item.value) });
    assert.equal(await adapter.validatePatch(proposal), true);
    const result = await adapter.applyPatch(proposal);
    item.check(editor.value);
    assert.equal(result.revision, revision(editor.value));
    assert.equal(applied.editor, editor);
    assert.deepEqual(undoLastEdit(applied, editor, revision), { ok: true, before: applied.before });
  }
});

test('a revision mismatch is stale and the wrong editor is refused', () => {
  const editor = fakeEditor('campaign:example', { subject: 'Welcome', body: 'Shop today', contentType: 'html' });
  const proposal = {
    id: 'e1',
    arguments: {
      editorId: 'campaign:example',
      baseRevision: revision(editor.value),
      operations: [{ op: 'replace_text', before: 'Shop today', after: 'Shop' }],
    },
  };
  editor.value = { ...editor.value, body: 'Shop today, edited' };
  assert.deepEqual(
    judgeEditorProposal(proposal, editor, { revisionOf: revision, applyPatch: applyEditorPatch }),
    { applicable: false, reason: 'stale' },
  );
  assert.deepEqual(
    judgeEditorProposal(proposal, null, { revisionOf: revision, applyPatch: applyEditorPatch }),
    { applicable: false, reason: 'stale' },
  );
  const other = fakeEditor('template:example', { subject: 'Hello', body: 'Hi', contentType: 'plain' });
  const wrong = judgeEditorProposal(
    { ...proposal, arguments: { ...proposal.arguments, baseRevision: revision(other.value) } },
    other,
    { revisionOf: revision, applyPatch: applyEditorPatch },
  );
  assert.equal(wrong.applicable, false);
  assert.equal(wrong.reason, 'refused');
  assert.equal(wrong.message, WRONG_EDITOR_TEXT);
});

test('the adapter reports a stale patch as not applicable and refuses the wrong editor', async () => {
  const editor = fakeEditor('campaign:example', { subject: 'Welcome', body: 'Shop today', contentType: 'html' });
  const adapter = createEditorAdapter({
    getEditor: () => editor,
    revisionOf: revision,
    applyPatch: applyEditorPatch,
  });
  const proposal = {
    id: 'e1',
    arguments: { editorId: 'campaign:example', baseRevision: 'stale', operations: [{ op: 'set', field: 'subject', after: 'Hi' }] },
  };
  assert.equal(await adapter.validatePatch(proposal), false);
  const wrong = {
    id: 'e2',
    arguments: { editorId: 'template:other', baseRevision: revision(editor.value), operations: [] },
  };
  await assert.rejects(adapter.validatePatch(wrong), { message: WRONG_EDITOR_TEXT });
});

test('applying after the editor changed refuses with the legacy message', async () => {
  const editor = fakeEditor('campaign:example', { subject: 'Welcome', body: 'Shop today', contentType: 'html' });
  const adapter = createEditorAdapter({
    getEditor: () => editor,
    revisionOf: revision,
    applyPatch: applyEditorPatch,
  });
  const proposal = {
    id: 'e1',
    arguments: {
      editorId: 'campaign:example',
      baseRevision: revision(editor.value),
      operations: [{ op: 'replace_text', before: 'Shop today', after: 'Shop' }],
    },
  };
  assert.equal(await adapter.validatePatch(proposal), true);
  editor.value = { ...editor.value, body: 'Shop today!' };
  await assert.rejects(adapter.applyPatch(proposal), { message: MID_APPLY_TEXT });
  assert.equal(editor.value.body, 'Shop today!');
});

test('undo is refused once the editor has moved on', () => {
  const editor = fakeEditor('campaign:example', { body: 'Shop' });
  const entry = { editor, before: { body: 'Shop today' }, after: revision(editor.value) };
  editor.value = { body: 'Shop now' };
  assert.deepEqual(undoLastEdit(entry, editor, revision), { ok: false, message: UNDO_CHANGED_TEXT });
  assert.deepEqual(undoLastEdit(entry, fakeEditor('other', editor.value), revision).ok, false);
});

test('retry is the last user message after a failed turn, and nothing while one is running', () => {
  const transcript = [
    { role: 'user', type: 'text', text: 'First' },
    { role: 'assistant', type: 'text', text: 'Ok' },
    { role: 'user', type: 'text', text: 'Second' },
  ];
  assert.equal(retryText({ running: false, transcript, turn: { error: { code: 'provider_error' } } }), 'Second');
  assert.equal(retryText({ running: true, transcript, error: { code: 'network' } }), '');
  assert.equal(retryText({ running: false, transcript }), '');
});
