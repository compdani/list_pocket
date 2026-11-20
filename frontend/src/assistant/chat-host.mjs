// The module-backed Agent page's logic, independent of Vue: the page context a
// turn sends, the stored references on user turns, the proposal card and
// ReviewModal inputs, and the error wording List Pocket prefers.
//
// Views/AgentChat.vue only wires these to the chat client, the controller and
// <lp-agent-page>. Keeping them here makes them testable with `node --test`
// (tests/chatHost.test.mjs), and keeps this module free of imports so the
// legacy page and the route loader don't pull in the vendored chat client.
//
// The chat module's stored page context is one `context` part on a user message
// (StoreContext). Its `result` is the object the browser sent:
//   { "screen": "agent", "path": "/agent/1",
//     "references": [{"type": "contact", "id": "…", "label": "…"}] }
// The binding ignores those parts, so withContextReferences() puts their
// references on the user's text entry as chips.

/** The most references the backend keeps for a conversation (cmd/chat_module.go). */
export const AGENT_REF_CAP = 30;

/** The screen name cmd/chat_module.go treats as the Agent page. */
export const AGENT_SCREEN = 'agent';

const FILE_TYPE = 'file';

/** One context reference as the page context and chips carry it. */
function cleanReference(ref) {
  if (!ref || typeof ref !== 'object') return null;
  const type = typeof ref.type === 'string' ? ref.type : '';
  if (type === '' || type === FILE_TYPE) return null;
  const id = ref.id === undefined || ref.id === null ? '' : String(ref.id);
  if (id === '') return null;
  const out = { type, id };
  if (typeof ref.label === 'string' && ref.label !== '') out.label = ref.label;
  return out;
}

/** Drop files and malformed entries, keep `{ type, id, label }` in order. */
export function referencesOf(list) {
  if (!Array.isArray(list)) return [];
  const out = [];
  for (const ref of list) {
    const clean = cleanReference(ref);
    if (clean) out.push(clean);
  }
  return out;
}

/**
 * The page context of one turn. Files are uploads on the module path, never
 * references, so they never travel here.
 */
export function pageContext(path, references) {
  return {
    screen: AGENT_SCREEN,
    path: typeof path === 'string' ? path : '',
    references: referencesOf(references),
  };
}

/**
 * An error counts for this send only when it is a different object from the
 * one already stored at submit time. The controller replaces `state.error`
 * on a new failure and clears it with null; a leftover object from an earlier
 * failure must not restore that send's draft.
 */
export function isFreshError(before, after) {
  if (after == null || after === '') return false;
  return !Object.is(before, after);
}

/**
 * One in-flight composer submit. `start` captures the path and references for
 * that submit. A later start is blocked until `readContext` consumes the
 * snapshot or the attempt is settled, so a second submit cannot overwrite
 * them while uploads or `newConversation` are still running. `running`
 * becoming true is not consumption: the controller sets it only after
 * `context()`, and a host must not drop the snapshot on that signal.
 */
export function createSubmitTracker() {
  let current = null;

  function finish(submit, info) {
    if (!submit || submit.settled) return { action: 'none', submit };
    submit.settled = true;
    if (current === submit) current = null;
    return { action: classifySettlement(submit, info), submit };
  }

  return {
    get current() {
      return current;
    },
    start({ path, references, text, error, turn } = {}) {
      if (current && !current.consumed && !current.settled) {
        return { blocked: true, submit: current };
      }
      current = {
        path: typeof path === 'string' ? path : '',
        references: referencesOf(references),
        text: typeof text === 'string' ? text : '',
        error: error ?? null,
        turn: turn ?? null,
        consumed: false,
        started: false,
        settled: false,
      };
      return { blocked: false, submit: current };
    },
    noteStarted() {
      if (current && !current.settled) current.started = true;
    },
    /** The binding returned without calling send or newConversation. */
    abandonIfUnstarted() {
      if (!current || current.started || current.settled) return { action: 'none', submit: current };
      const submit = current;
      submit.settled = true;
      current = null;
      return { action: 'release', submit };
    },
    readContext() {
      if (!current || current.settled) return pageContext('', []);
      current.consumed = true;
      return pageContext(current.path, current.references);
    },
    settle(submit, info) {
      return finish(submit, info);
    },
  };
}

/**
 * The outcome of the send promise, not of a `running` change.
 * `rejected` is an upload or a precondition throw: the binding keeps the draft.
 * A resolved turn is acceptance. A new error with no turn is a failure before
 * the server accepted the turn.
 */
export function classifySettlement(submit, { result, rejected = false, error = null } = {}) {
  if (!submit) return 'none';
  if (rejected) return 'release';
  if (result && result.turn) return 'accept';
  if (isFreshError(submit.error, error)) return 'restore';
  return 'release';
}

/**
 * What a controller `change` means for a submit. A turn object that was already
 * current at submit time is not this send being accepted, and `running: true`
 * before the snapshot has been read is not acceptance either (that flag is set
 * before `submitTurn`). A stale error object is ignored.
 */
export function observeSubmit(submit, { running = false, turn = null, error = null } = {}) {
  if (!submit || submit.settled) return 'keep';
  if (turn && turn !== submit.turn) return 'accept';
  // The snapshot is still the one this send will read. `running` flipping
  // true first is not acceptance, and it must not drop the snapshot.
  if (!submit.consumed) return 'keep';
  if (isFreshError(submit.error, error) && running !== true) return 'restore';
  return 'keep';
}

/**
 * Wrap `send` and `newConversation` so the host hears the attempt's outcome.
 * The binding calls these; the host never sees the promises otherwise.
 * Upload rejections are reported as `release` (the binding restores files).
 * A conversation that fails to appear, with nothing left open, is a failure
 * before accept.
 */
export function attachSubmitTracking(controller, tracker, onOutcome) {
  const send = controller.send.bind(controller);
  const create = controller.newConversation.bind(controller);
  const report = (outcome) => {
    if (outcome && outcome.action && outcome.action !== 'none') onOutcome(outcome);
  };
  controller.send = (text, options) => {
    tracker.noteStarted();
    const submit = tracker.current;
    if (submit) submit.sawSend = true;
    let promise;
    try {
      promise = send(text, options);
    } catch (err) {
      report(tracker.settle(submit, { rejected: true, error: controller.state?.error }));
      throw err;
    }
    Promise.resolve(promise).then(
      (result) => report(tracker.settle(submit, { result, error: controller.state?.error })),
      () => report(tracker.settle(submit, { rejected: true, error: controller.state?.error })),
    );
    return promise;
  };
  controller.newConversation = (title) => {
    tracker.noteStarted();
    const submit = tracker.current;
    const promise = create(title);
    Promise.resolve(promise).then((conversation) => {
      if (!submit || submit.settled || submit.sawSend || conversation) return;
      if (controller.state?.activeConversationId) return;
      report(tracker.settle(submit, { result: null, error: controller.state?.error }));
    });
    return promise;
  };
  return controller;
}

/**
 * The history write for a conversation-id change. `push` is a person selecting
 * a conversation, starting a new chat, or deleting the open one. `replace` is
 * the id a draft's first send assigns, so `/agent` does not stay under the new
 * URL. `suspended` while the page is not aligned, or while a newer route is
 * waiting, so a load cannot write itself over that URL.
 */
export function navigationWrite({
  activeId = '', routeId = '', intent = 'replace', suspended = false,
} = {}) {
  if (suspended) return null;
  const active = activeId == null ? '' : String(activeId);
  const route = routeId == null ? '' : String(routeId);
  if (active === route) return null;
  return { method: intent === 'push' ? 'push' : 'replace', id: active };
}

/** Which host the /agent route renders, from /api/config. '' means not loaded. */
export function backendOf(serverConfig) {
  if (!serverConfig || typeof serverConfig !== 'object' || Array.isArray(serverConfig)) return '';
  if (Object.keys(serverConfig).length === 0) return '';
  return serverConfig.chat_backend === 'module' ? 'module' : 'legacy';
}

/**
 * The references each user message was sent with, keyed by message id, from the
 * controller's transcript (`type: 'context'` parts).
 */
function contextReferencesByMessage(transcript) {
  const byMessage = new Map();
  if (!Array.isArray(transcript)) return byMessage;
  for (const part of transcript) {
    if (!part || part.type !== 'context' || part.role !== 'user') continue;
    const messageId = part.messageId === undefined || part.messageId === null
      ? ''
      : String(part.messageId);
    if (messageId === '') continue;
    const result = part.result && typeof part.result === 'object' ? part.result : part;
    const references = referencesOf(result.references);
    if (references.length > 0) byMessage.set(messageId, references);
  }
  return byMessage;
}

/** The message id a transcript text entry belongs to (`msg:<id>[:n]`). */
function messageIdOfEntry(entry, byMessage) {
  const id = typeof entry?.id === 'string' ? entry.id : '';
  if (!id.startsWith('msg:')) return '';
  const rest = id.slice(4);
  if (byMessage.has(rest)) return rest;
  const cut = rest.lastIndexOf(':');
  if (cut > 0 && byMessage.has(rest.slice(0, cut))) return rest.slice(0, cut);
  return '';
}

/**
 * Put each user turn's stored references on its text entry as `{ type, id,
 * label }` chips, before the attachment chips the binding already added.
 *
 * Pure: returns the same array when no turn has stored references, so the
 * legacy host's entries are untouched. Applied once, in ListPocketAgentPage's
 * `entries` setter, which runs on every binding write.
 */
export function withContextReferences(entries, transcript) {
  if (!Array.isArray(entries)) return entries;
  const byMessage = contextReferencesByMessage(transcript);
  if (byMessage.size === 0) return entries;

  let changed = false;
  const out = entries.map((entry) => {
    if (!entry || entry.type !== 'text' || entry.role !== 'user') return entry;
    const messageId = messageIdOfEntry(entry, byMessage);
    if (messageId === '') return entry;
    const stored = byMessage.get(messageId);
    const existing = Array.isArray(entry.references) ? entry.references : [];
    // A stored reference that is also an attachment chip keeps the chip's
    // fields (its download marker), so only add what isn't there yet.
    const seen = new Set(existing.map((ref) => `${ref?.type}:${ref?.id}`));
    const added = stored.filter((ref) => !seen.has(`${ref.type}:${ref.id}`));
    if (added.length === 0) return entry;
    changed = true;
    return { ...entry, references: [...added, ...existing] };
  });
  return changed ? out : entries;
}

/**
 * "In context": the 30 most recent distinct references across the conversation,
 * newest message first, matching cmd/chat_module.go's chatAgentRefs (this
 * turn's first, then older turns newest first, capped at 30). Files never
 * appear: they are attachments, not references.
 */
export function contextReferencesFromTranscript(transcript, cap = AGENT_REF_CAP) {
  const limit = Number.isInteger(cap) && cap > 0 ? cap : AGENT_REF_CAP;
  if (!Array.isArray(transcript)) return [];
  const seen = new Set();
  const out = [];
  for (let i = transcript.length - 1; i >= 0 && out.length < limit; i -= 1) {
    const part = transcript[i];
    if (!part || part.type !== 'context') continue;
    const result = part.result && typeof part.result === 'object' ? part.result : part;
    for (const ref of referencesOf(result.references)) {
      if (out.length >= limit) break;
      const key = `${ref.type}:${ref.id}`;
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(ref);
    }
  }
  return out;
}

// The module's proposal states mapped onto the legacy card's badges. `unknown`
// keeps its own tone: it must never read as a success or a plain failure.
const CARD_STATE = Object.freeze({
  pending: { status: 'pending', label: '' },
  rejected: { status: 'rejected', label: 'Rejected' },
  expired: { status: 'expired', label: 'Expired' },
  executing: { status: 'applying', label: 'Applying' },
  succeeded: { status: 'applied', label: 'Applied' },
  failed: { status: 'failed', label: 'Failed' },
  unknown: { status: 'unknown', label: 'Unknown' },
  approved: { status: 'approved', label: 'Approved' },
  applied: { status: 'applied', label: 'Applied' },
  stale: { status: 'stale', label: 'Out of date' },
});

// What happened, in the same plain words the vendored binding uses. Copied
// here so this module stays import-free; tests/chatHost.test.mjs checks the two
// stay identical.
export const PROPOSAL_STATE_TEXT = Object.freeze({
  pending: 'Waiting for your approval.',
  rejected: 'Rejected.',
  expired: 'Expired before it was decided.',
  executing: 'Applying this change on the server…',
  succeeded: 'The server applied this change.',
  failed: 'This change could not be applied.',
  unknown: 'The server may or may not have applied this change — check before retrying.',
  approved: 'Approved — applying in the editor…',
  applied: 'Applied in the editor — not saved yet.',
  stale: 'Not applied: the editor changed since this change was proposed.',
});

/**
 * The line under a proposal for a non-fatal conflict or refusal. Raw server
 * messages are never shown: only a refusal's own message (written for the user
 * by the tool) is included, and an unknown code gets a generic line.
 */
export function proposalProblemText(error) {
  if (!error) return '';
  const code = typeof error === 'object' && typeof error.code === 'string'
    ? error.code.toLowerCase()
    : '';
  const message = typeof error === 'object' && typeof error.message === 'string'
    ? error.message.trim()
    : '';
  switch (code) {
    case 'stale':
      return 'This change is out of date. Ask again for a fresh one.';
    case 'expired':
      return 'This change expired before it was decided.';
    case 'conflict':
      return 'This change was decided somewhere else. Review the latest version.';
    case 'refused':
      return message
        ? `${message} Nothing was saved; the change is waiting for your decision again.`
        : 'The change was refused before anything was saved. It is waiting for your decision again.';
    case 'not_found':
      return 'This change is no longer available.';
    default:
      return 'Something went wrong. Try again.';
  }
}

function objectOr(value, fallback) {
  if (value && typeof value === 'object' && !Array.isArray(value)) return value;
  if (typeof value === 'string' && value !== '') {
    try {
      const parsed = JSON.parse(value);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) return parsed;
    } catch {
      // Not JSON: the caller's fallback is safer than raw text.
    }
  }
  return fallback;
}

function stringsOf(value) {
  return Array.isArray(value)
    ? value.filter((item) => typeof item === 'string' && item.trim() !== '').map((item) => item.trim())
    : [];
}

/**
 * The one line a proposal card shows for a non-fatal conflict or refusal, or
 * the server's stored failure. Never raw server text: proposalErrorText()
 * handles codes, and `failure` is documented as user-safe.
 */
function problemTextOf(proposal) {
  if (proposal && proposal.error) {
    const text = proposalProblemText(proposal.error);
    if (text) return text;
  }
  const failure = proposal && typeof proposal.failure === 'string' ? proposal.failure.trim() : '';
  return failure;
}

/**
 * Map one module proposal onto List Pocket's card input
 * ({ proposal, busy, running, actionId }) for lp-agent-common's
 * renderProposal(). Pure.
 */
export function cardProposalOf(proposal, { busy = false, running = false } = {}) {
  if (!proposal || typeof proposal !== 'object') return null;
  const id = proposal.id === undefined || proposal.id === null ? '' : String(proposal.id);
  if (id === '') return null;
  const state = typeof proposal.state === 'string' ? proposal.state : 'pending';
  const mapped = CARD_STATE[state] || { status: state, label: state };
  const stateText = state === 'pending' ? '' : (PROPOSAL_STATE_TEXT[state] || '');
  return {
    proposal: {
      id,
      tool: typeof proposal.tool === 'string' && proposal.tool !== '' ? proposal.tool : 'tool',
      status: mapped.status,
      statusLabel: mapped.label,
      state,
      stateText,
      arguments: objectOr(proposal.arguments, {}),
      warnings: stringsOf(proposal.warnings),
      result: objectOr(proposal.result, null),
      error: problemTextOf(proposal),
    },
    busy: busy === true,
    running: running === true,
    actionId: id,
  };
}

/**
 * Adapt one module proposal to ReviewModal's `part` prop. ReviewModal only
 * edits while `status === 'pending'`, so a decided proposal opens read-only.
 */
export function reviewPartOf(proposal) {
  if (!proposal || typeof proposal !== 'object') return null;
  const state = typeof proposal.state === 'string' ? proposal.state : 'pending';
  return {
    id: proposal.id === undefined || proposal.id === null ? '' : String(proposal.id),
    tool: typeof proposal.tool === 'string' ? proposal.tool : '',
    arguments: objectOr(proposal.arguments, {}),
    status: state === 'succeeded' ? 'applied' : state,
    state,
    revision: Number.isFinite(Number(proposal.revision)) ? Number(proposal.revision) : 0,
    warnings: stringsOf(proposal.warnings),
    result: objectOr(proposal.result, null),
    error: problemTextOf(proposal),
  };
}

// List Pocket's wording for the two provider failures the module reports, as
// the legacy engine did (cmd/ai_assistant.go runAssistant). The binding's own
// generic lines are reproduced here so this module stays import-free; tests
// compare them with the vendored errorText().
export const PROVIDER_AUTH_TEXT = 'The AI provider did not accept this app\'s credentials. Check the API key, or sign in to ChatGPT again under Settings → AI.';
export const USAGE_LIMIT_TEXT = 'Your ChatGPT plan\'s usage limit has been reached. Wait for it to reset, or switch models.';
export const BINDING_PROVIDER_AUTH_TEXT = 'The AI provider rejected its credentials. Ask an administrator to check the key or sign in again in the provider settings.';
export const BINDING_USAGE_LIMIT_TEXT = 'The AI provider\'s usage limit was reached. Wait a while or switch to another model.';

const BINDING_ALIASES = new Map([
  [BINDING_PROVIDER_AUTH_TEXT, PROVIDER_AUTH_TEXT],
  [BINDING_USAGE_LIMIT_TEXT, USAGE_LIMIT_TEXT],
]);

/**
 * Replace the binding's generic provider wording with List Pocket's, and leave
 * every other line alone. Used by <lp-agent-page> so the element's own error
 * banner keeps the wording the app used before the module.
 */
export function hostErrorText(text) {
  if (typeof text !== 'string' || text === '') return text ?? '';
  return BINDING_ALIASES.get(text) ?? text;
}

/**
 * The remembered model's choice id, or '' when it is no longer offered. The
 * key holds the legacy `{ provider, model }` shape (or null for the default).
 */
export function choiceIdFor(stored, choices) {
  if (!Array.isArray(choices) || choices.length === 0) return '';
  const wanted = stored && typeof stored === 'object'
    ? { provider: typeof stored.provider === 'string' ? stored.provider : '', model: typeof stored.model === 'string' ? stored.model : '' }
    : { provider: '', model: typeof stored === 'string' ? stored : '' };
  if (wanted.model === '') return '';
  const exact = choices.find((choice) => choice
    && choice.model === wanted.model
    && (wanted.provider === '' || choice.provider === wanted.provider));
  if (exact) return exact.id;
  const byModel = choices.filter((choice) => choice && choice.model === wanted.model);
  return byModel.length === 1 ? byModel[0].id : '';
}

/**
 * What to store in localStorage for a chosen choice id: the same
 * `{ provider, model }` the legacy hosts read, or null for the default.
 */
export function storedModelFor(choiceId, choices) {
  if (!choiceId) return null;
  const choice = Array.isArray(choices) ? choices.find((item) => item && item.id === choiceId) : null;
  if (!choice) return { model: choiceId };
  return {
    provider: typeof choice.provider === 'string' ? choice.provider : '',
    model: typeof choice.model === 'string' ? choice.model : choiceId,
  };
}
