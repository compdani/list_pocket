import test from 'node:test';
import assert from 'node:assert/strict';

import { bindAgentPanel, errorText } from '../src/vendor/chat-client/agent-panel.mjs';
import { PROPOSAL_STATE_TEXT as BINDING_PROPOSAL_STATE_TEXT } from '../src/vendor/chat-client/agent-panel.mjs';
import {
  AGENT_REF_CAP, BINDING_PROVIDER_AUTH_TEXT, BINDING_USAGE_LIMIT_TEXT, PROPOSAL_STATE_TEXT,
  PROVIDER_AUTH_TEXT, USAGE_LIMIT_TEXT, attachSubmitTracking, backendOf, cardProposalOf,
  choiceIdFor, classifySettlement, contextReferencesFromTranscript, createSubmitTracker,
  hostErrorText, isFreshError, navigationWrite, observeSubmit, pageContext,
  proposalProblemText, referencesOf, reviewPartOf, storedModelFor, withContextReferences,
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

test('stored references become chips on the user turn, before attachment chips', () => {
  const transcript = [
    {
      id: 'm1:1',
      messageId: 'm1',
      role: 'user',
      type: 'context',
      result: { screen: 'agent', path: '/agent/c1', references: [{ type: 'campaign', id: '7', label: 'Spring sale' }] },
    },
    { id: 'm1:2', messageId: 'm1', role: 'user', type: 'attachment', uploadId: 'u1', name: 'shot.png' },
  ];
  const attachment = { type: 'attachment', id: 'u1', label: 'shot.png', attachment: true, mime: 'image/png' };
  const entries = [
    { id: 'msg:m1', type: 'text', role: 'user', content: 'Draft it', references: [attachment] },
    { id: 'msg:m1:1', type: 'text', role: 'user', content: 'Again', references: [attachment] },
    { id: 'msg:m2', type: 'text', role: 'assistant', content: 'Sure' },
  ];
  const out = withContextReferences(entries, transcript);

  assert.notEqual(out, entries);
  assert.deepEqual(out[0].references, [{ type: 'campaign', id: '7', label: 'Spring sale' }, attachment]);
  // A later part of the same message finds its message id too.
  assert.deepEqual(out[1].references, [{ type: 'campaign', id: '7', label: 'Spring sale' }, attachment]);
  assert.equal(out[2], entries[2]);
  // The input is not mutated.
  assert.deepEqual(entries[0].references, [attachment]);
});

test('a turn without stored references is left exactly as it was', () => {
  const entries = [{ id: 'msg:m1', type: 'text', role: 'user', content: 'Hi' }];
  assert.equal(withContextReferences(entries, []), entries);
  assert.equal(withContextReferences(entries, undefined), entries);
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

test('the card wording matches the vendored binding', () => {
  assert.deepEqual(PROPOSAL_STATE_TEXT, BINDING_PROPOSAL_STATE_TEXT);
  assert.equal(errorText({ code: 'provider_auth' }), BINDING_PROVIDER_AUTH_TEXT);
  assert.equal(errorText({ code: 'usage_limit' }), BINDING_USAGE_LIMIT_TEXT);
});

test('provider failures use the app wording, everything else is unchanged', () => {
  assert.equal(hostErrorText(errorText({ code: 'provider_auth' })), PROVIDER_AUTH_TEXT);
  assert.equal(hostErrorText(errorText({ code: 'usage_limit' })), USAGE_LIMIT_TEXT);
  assert.match(hostErrorText(errorText({ code: 'provider_auth' })), /Settings → AI/);
  assert.equal(hostErrorText('A reply is already in progress.'), 'A reply is already in progress.');
  assert.equal(hostErrorText(''), '');
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

// --- listener order ---------------------------------------------------------

// The page context is read by the controller at send time, while the binding's
// agent-submit starts the send. The host's listener must be registered first.
class FakePage extends EventTarget {
  constructor() {
    super();
    this.tagName = 'AGENT-PAGE';
    this.entries = [];
    this.running = false;
    this.error = '';
    this.models = [];
    this.model = '';
    this.options = [];
    this.conversations = [];
    this.active = '';
    this.attributes = new Map();
  }

  setAttribute(name, value) { this.attributes.set(name, String(value)); }

  getAttribute(name) { return this.attributes.has(name) ? this.attributes.get(name) : null; }

  removeAttribute(name) { this.attributes.delete(name); }

  emitAction() { return true; }
}

test('the host captures a submit\'s references before the binding starts the send', async () => {
  const order = [];
  const element = new FakePage();
  const controller = new EventTarget();
  controller.state = {
    status: 'ready',
    conversations: [],
    nextCursor: '',
    activeConversationId: 'c1',
    messages: [],
    transcript: [],
    proposals: [],
    pendingUploads: [],
    turn: null,
    running: false,
    error: null,
    features: null,
    limits: null,
    models: [],
    choices: [],
    defaultChoice: '',
    choice: '',
    context: null,
  };
  controller.send = (text) => {
    order.push(`binding:${text}:${captured.length}`);
    return Promise.resolve();
  };

  let captured = [];
  element.addEventListener('agent-submit', (event) => {
    captured = event.detail.references;
    order.push('host');
  });
  bindAgentPanel(element, controller, { registerEntries: false });

  element.dispatchEvent(new CustomEvent('agent-submit', {
    detail: {
      text: 'Compare these',
      attachments: [],
      references: [{ type: 'campaign', id: '7', label: 'Spring sale' }],
      fromDraft: true,
      model: '',
      options: {},
    },
  }));
  await Promise.resolve();

  assert.deepEqual(order, ['host', 'binding:Compare these:1']);
  assert.deepEqual(captured, [{ type: 'campaign', id: '7', label: 'Spring sale' }]);
});

// --- one submit's snapshot, and which error belongs to it -------------------

test('a second submit is blocked until this submit\'s snapshot is read', () => {
  const tracker = createSubmitTracker();
  const first = tracker.start({
    path: '/agent/c1',
    references: [
      { type: 'campaign', id: '7', label: 'Spring sale' },
      { type: 'file', id: 'f1', label: 'leads.csv' },
    ],
    text: 'Compare these',
    error: null,
    turn: null,
  });
  const second = tracker.start({
    path: '/agent/c2',
    references: [{ type: 'list', id: '3', label: 'Customers' }],
    text: 'Other',
    error: null,
  });

  assert.equal(second.blocked, true);
  assert.equal(second.submit, first.submit);
  assert.deepEqual(tracker.readContext(), {
    screen: 'agent',
    path: '/agent/c1',
    references: [{ type: 'campaign', id: '7', label: 'Spring sale' }],
  });
  assert.equal(first.submit.consumed, true);

  const next = tracker.start({
    path: '/agent/c2',
    references: [{ type: 'list', id: '3', label: 'Customers' }],
    text: 'Other',
    error: null,
  });
  assert.equal(next.blocked, false);
  assert.equal(tracker.readContext().path, '/agent/c2');
  // Settling the first send does not drop the second snapshot.
  assert.equal(tracker.settle(first.submit, { result: { turn: { id: 't1' } } }).action, 'accept');
  assert.equal(tracker.current, next.submit);
});

test('running before the snapshot is read does not accept the send or drop it', () => {
  const tracker = createSubmitTracker();
  const stale = { code: 'error', message: 'earlier' };
  const { submit } = tracker.start({
    path: '/agent', references: [], text: 'Hi', error: stale, turn: null,
  });
  assert.equal(observeSubmit(submit, { running: true, turn: null, error: stale }), 'keep');
  assert.equal(submit.consumed, false);
  assert.equal(tracker.current, submit);
});

test('a stale error is not a failure, and a new error before a turn restores', () => {
  const stale = { code: 'error', message: 'earlier' };
  const fresh = { code: 'internal', message: 'nope' };
  const tracker = createSubmitTracker();
  const { submit } = tracker.start({
    path: '/agent', references: [], text: 'Hi', error: stale, turn: null,
  });
  tracker.noteStarted();
  tracker.readContext();

  assert.equal(isFreshError(stale, stale), false);
  assert.equal(isFreshError(stale, fresh), true);
  assert.equal(isFreshError(stale, null), false);
  assert.equal(observeSubmit(submit, { running: false, turn: null, error: stale }), 'keep');
  assert.equal(classifySettlement(submit, { result: null, error: stale }), 'release');
  assert.equal(classifySettlement(submit, { result: null, error: fresh }), 'restore');
  assert.equal(classifySettlement(submit, { rejected: true, error: fresh }), 'release');
  assert.equal(classifySettlement(submit, { result: { turn: { id: 't1' } }, error: fresh }), 'accept');
  // The turn already on screen when this send started is not this send's turn.
  assert.equal(observeSubmit(submit, { turn: null, error: fresh, running: false }), 'restore');
});

test('a refused submit drops its snapshot so the next send is not blocked', () => {
  const tracker = createSubmitTracker();
  const { submit } = tracker.start({ path: '/agent', references: [], text: 'Hi', error: null });
  assert.deepEqual(tracker.abandonIfUnstarted(), { action: 'release', submit });
  assert.equal(tracker.current, null);
  tracker.noteStarted();
  assert.equal(tracker.abandonIfUnstarted().action, 'none');
});

test('tracking the controller send promise tells a failure from an upload and from a success', async () => {
  const tracker = createSubmitTracker();
  const actions = [];
  const controller = {
    state: { error: null, activeConversationId: null },
    send(text) {
      if (text === 'upload') return Promise.reject(new Error('upload'));
      if (text === 'bad') {
        controller.state.error = { code: 'internal', message: 'nope' };
        return Promise.resolve(null);
      }
      return Promise.resolve({ turn: { id: 't1' } });
    },
    newConversation() {
      controller.state.error = { code: 'internal', message: 'create failed' };
      return Promise.resolve(null);
    },
  };
  attachSubmitTracking(controller, tracker, (outcome) => actions.push(outcome.action));

  tracker.start({ path: '/agent', references: [], text: 'bad', error: null });
  await controller.send('bad');
  await Promise.resolve();
  assert.deepEqual(actions, ['restore']);

  const stale = controller.state.error;
  tracker.start({ path: '/agent', references: [], text: 'ok', error: stale });
  await controller.send('ok');
  await Promise.resolve();
  assert.deepEqual(actions, ['restore', 'accept']);

  tracker.start({ path: '/agent', references: [], text: 'file', error: null });
  await controller.send('upload').catch(() => {});
  await Promise.resolve();
  assert.deepEqual(actions, ['restore', 'accept', 'release']);

  tracker.start({ path: '/agent', references: [], text: 'new', error: null });
  await controller.newConversation();
  await Promise.resolve();
  assert.deepEqual(actions, ['restore', 'accept', 'release', 'restore']);
  assert.equal(tracker.current, null);
  assert.equal(tracker.start({ path: '/agent', text: 'later', references: [], error: null }).blocked, false);
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
