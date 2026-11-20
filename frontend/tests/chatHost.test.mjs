import test from 'node:test';
import assert from 'node:assert/strict';

import { bindAgentPanel, errorText, PROPOSAL_STATE_TEXT as BINDING_PROPOSAL_STATE_TEXT } from '@pocketbase-mcp/chat-client/agent-panel';
import {
  AGENT_REF_CAP, PROPOSAL_STATE_TEXT, PROVIDER_AUTH_TEXT, USAGE_LIMIT_TEXT, backendOf,
  cardProposalOf, chatErrorText, choiceIdFor, contextReferencesFromTranscript,
  navigationWrite, pageContext, proposalProblemText, referencesFromContext, referencesOf,
  reviewPartOf, storedModelFor,
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
    [{ type: 'campaign', id: '7', label: 'Spring sale' }, { type: 'list', id: '3' }],
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
