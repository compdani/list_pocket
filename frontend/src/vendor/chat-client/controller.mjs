// Framework-neutral chat controller.
//
// One controller owns one conversation view: the conversation list, the open
// transcript, the live turn view, limits and errors. It is an EventTarget; every
// state change dispatches a "change" event with no payload, and consumers read
// `controller.state` (an immutable snapshot).
//
// Recovery rules, matching the server contract:
//   - a stream that ends without a terminal event, an idle stream (no event or
//     heartbeat), or a network failure reconnects from the last sequence, so a
//     turn is never re-submitted;
//   - a resync-required answer or a reducer gap reloads canonical state from
//     GET /turns/{id} and GET /conversations/{id}, then polls the turn until it
//     is terminal (there is no cursor to replay after a resync);
//   - a terminal event reloads the transcript once, so messages are canonical.
//
// List sync: the conversation list is re-read after every finished turn, when
// the page becomes visible or focused again, on a slow interval while visible,
// and whenever another controller for the same account (another element on the
// page, or another tab) announces a change over a BroadcastChannel. A refresh
// merges the first page over what is loaded and never shows another account's
// rows (the channel name is the account key).
//
// Account safety: state belongs to the account key derived from
// appUrl|mount|authCollectionId|userId. Changing it aborts every request and
// clears everything before initialising again. Nothing is persisted.

import {
  ChatApiError,
  ChatNetworkError,
  ChatProtocolError,
  ResyncRequiredError,
  newIdempotencyKey,
} from './client.mjs';
import {
  AfterTerminalError,
  ForeignTurnError,
  GapError,
  InvalidEventError,
  applyEvent,
  createTurnView,
  isTerminalProposalState,
  isTerminalState,
  viewFromTranscript,
} from './reducer.mjs';

const IDLE_TIMEOUT_MS = 45000;
const SYNC_INTERVAL_MS = 60000;
const SYNC_DEBOUNCE_MS = 250;
const SYNC_CHANNEL_PREFIX = 'pocketbase-mcp-chat:';

/** File extensions a browser often leaves untyped, mapped to the server's MIME. */
const MIME_BY_EXTENSION = Object.freeze({
  txt: 'text/plain',
  text: 'text/plain',
  log: 'text/plain',
  md: 'text/markdown',
  markdown: 'text/markdown',
  csv: 'text/csv',
  json: 'application/json',
  png: 'image/png',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  gif: 'image/gif',
  webp: 'image/webp',
  pdf: 'application/pdf',
});

function initialState() {
  return {
    status: 'idle',
    conversations: [],
    nextCursor: '',
    activeConversationId: null,
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
}

/** Keep a well-formed context status from the server, or null. */
function contextOf(raw) {
  if (!raw || typeof raw !== 'object') return null;
  const tokens = Number(raw.tokens);
  const window = Number(raw.window);
  return {
    tokens: Number.isFinite(tokens) && tokens > 0 ? tokens : 0,
    window: Number.isFinite(window) && window > 0 ? window : 0,
    estimated: raw.estimated === true,
    summarizedThrough: typeof raw.summarizedThrough === 'string' ? raw.summarizedThrough : '',
    summarizedAt: typeof raw.summarizedAt === 'string' ? raw.summarizedAt : '',
    compacting: raw.compacting === true,
    canCompact: raw.canCompact === true,
  };
}

/** How often, and how many times, a background summary is checked for. */
const CONTEXT_POLL_MS = 3000;
const CONTEXT_POLL_ATTEMPTS = 20;

/**
 * Resolve a picker value or model id to one catalog choice. An opaque choice id
 * wins, then a unique model id, then the default choice. Returns null when the
 * server has no catalog (the legacy single-provider path).
 */
export function resolveChoice(choices, requested) {
  if (!Array.isArray(choices) || choices.length === 0) return null;
  const value = typeof requested === 'string' ? requested : '';
  if (value !== '') {
    const exact = choices.find((choice) => choice && choice.id === value);
    if (exact) return exact;
    const byModel = choices.filter((choice) => choice && choice.model === value);
    if (byModel.length === 1) return byModel[0];
    const fallback = choices.find((choice) => choice && choice.default);
    if (fallback && fallback.model === value) return fallback;
    // Never silently change the provider for a value the picker did not
    // produce. The server can reject an unknown model instead.
    return null;
  }
  return choices.find((choice) => choice && choice.default) ?? choices[0];
}

/**
 * The attachment types a message to `choice` can carry: that choice's own list
 * when it has one, otherwise `limits.attachmentTypes` (the union the upload
 * route accepts). Returns null when nothing is known.
 */
export function attachmentTypesFor(limits, choice = null) {
  const source = choice && Array.isArray(choice.attachmentTypes)
    ? choice.attachmentTypes
    : limits && Array.isArray(limits.attachmentTypes) ? limits.attachmentTypes : null;
  return source ? source.filter((type) => typeof type === 'string' && type !== '') : null;
}

/**
 * Flatten canonical transcript messages into one entry per part. A notice
 * message (role "notice", part "proposal_outcome") and an attachment part
 * therefore become ordinary entries a host can render, with the message id and
 * part index as a stable key.
 */
export function transcriptFromMessages(messages = []) {
  const out = [];
  if (!Array.isArray(messages)) return out;
  for (const message of messages) {
    if (!message || typeof message !== 'object') continue;
    const parts = Array.isArray(message.parts) ? message.parts : [];
    parts.forEach((part, index) => {
      if (!part || typeof part !== 'object') return;
      out.push({
        id: `${String(message.id ?? '')}:${index}`,
        messageId: String(message.id ?? ''),
        role: String(message.role ?? ''),
        turnId: String(message.turnId ?? ''),
        createdAt: String(message.createdAt ?? ''),
        ...part,
      });
    });
  }
  return out;
}

/**
 * Merge proposals into a list, newest revision wins and no entry is ever
 * duplicated. This is the client-side twin of the reducer's putProposal, used
 * for the conversation-level list (which mixes GET responses with events).
 */
export function mergeProposals(list, incoming) {
  let next = Array.isArray(list) ? list : [];
  for (const proposal of incoming ?? []) {
    if (!proposal || typeof proposal !== 'object' || !proposal.id) continue;
    next = mergeProposal(next, proposal);
  }
  return next;
}

function mergeProposal(list, proposal) {
  const index = list.findIndex((p) => p.id === proposal.id);
  if (index === -1) return [...list, { ...proposal, error: proposal.error ?? null }];
  if (Number(proposal.revision) < Number(list[index].revision)) return list;
  const next = list.slice();
  next[index] = { ...list[index], ...proposal, error: proposal.error ?? null };
  return next;
}

/** The best MIME guess for an untyped File, or '' when the name says nothing. */
export function inferAttachmentMime(name) {
  const clean = String(name ?? '').toLowerCase().split(/[\\/]/).pop() ?? '';
  const dot = clean.lastIndexOf('.');
  if (dot < 0) return '';
  return MIME_BY_EXTENSION[clean.slice(dot + 1)] ?? '';
}

/**
 * Read an editor validatePatch() verdict. `true`, undefined and a missing
 * `applicable` mean applicable; `false` or {applicable: false} mean stale.
 */
function applicableVerdict(verdict) {
  if (verdict === undefined || verdict === null) return true;
  if (typeof verdict === 'boolean') return verdict;
  if (typeof verdict === 'object') return verdict.applicable !== false && verdict.stale !== true;
  return Boolean(verdict);
}

/** Keep only well-formed {id, status} hunk outcomes from an adapter. */
function normalizeHunks(hunks) {
  if (!Array.isArray(hunks)) return undefined;
  const out = [];
  for (const hunk of hunks) {
    if (!hunk || typeof hunk !== 'object') continue;
    if (hunk.id === undefined || hunk.id === null || hunk.status === undefined) continue;
    out.push({ id: String(hunk.id), status: String(hunk.status) });
  }
  return out.length > 0 ? out : undefined;
}

function requireAccountFields(storageKey) {
  if (storageKey === undefined || storageKey === null) return null;
  if (typeof storageKey !== 'object') {
    throw new TypeError('storageKey must be an object');
  }
  for (const field of ['appUrl', 'mount', 'authCollectionId', 'userId']) {
    const value = storageKey[field];
    if (value === undefined || value === null || String(value) === '') {
      throw new TypeError(`storageKey.${field} is required`);
    }
  }
  return {
    appUrl: String(storageKey.appUrl),
    mount: String(storageKey.mount),
    authCollectionId: String(storageKey.authCollectionId),
    userId: String(storageKey.userId),
  };
}

/**
 * A storage key identifies one account across every namespace field. Each part
 * is percent-encoded and joined, so two different keys can never collide (for
 * example {"a|b","c"} and {"a","b|c"}), and both delimiters matter.
 */
export function accountKeyOf(storageKey) {
  const key = storageKey === undefined || storageKey === null
    ? null
    : requireAccountFields(storageKey);
  if (!key) return null;
  return [key.appUrl, key.mount, key.authCollectionId, key.userId]
    .map((part) => encodeURIComponent(part))
    .join('|');
}

function isAbort(err, signal) {
  return signal?.aborted === true || err?.name === 'AbortError';
}

function abortError(signal) {
  if (signal?.reason !== undefined && signal.reason !== null) return signal.reason;
  if (typeof DOMException === 'function') {
    return new DOMException('The operation was aborted.', 'AbortError');
  }
  const err = new Error('The operation was aborted.');
  err.name = 'AbortError';
  return err;
}

function sleep(ms, signal) {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(abortError(signal));
      return;
    }
    const cleanup = () => signal?.removeEventListener('abort', onAbort);
    const timer = setTimeout(() => {
      cleanup();
      resolve();
    }, Math.max(0, ms));
    function onAbort() {
      clearTimeout(timer);
      cleanup();
      reject(abortError(signal));
    }
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}

/** An idle stream was aborted on purpose; the loop reconnects. */
class IdleTimeoutError extends Error {
  constructor(ms) {
    super(`No event or heartbeat for ${ms}ms.`);
    this.name = 'IdleTimeoutError';
  }
}

/** Attach a child AbortController to a parent signal; returns the unlink. */
function linkAbort(parent, child) {
  if (!parent) return () => {};
  if (parent.aborted) {
    child.abort(parent.reason);
    return () => {};
  }
  const onAbort = () => child.abort(parent.reason);
  parent.addEventListener('abort', onAbort, { once: true });
  return () => parent.removeEventListener('abort', onAbort);
}

/** Newest first by (createdAt, id), the server's list order. */
function isOlder(a, b) {
  const ta = Date.parse(a?.createdAt ?? '');
  const tb = Date.parse(b?.createdAt ?? '');
  if (Number.isNaN(ta) || Number.isNaN(tb)) return null;
  if (ta !== tb) return ta < tb;
  return String(a.id) < String(b.id);
}

/**
 * Merge a fresh first page into the loaded list. Rows the page returns replace
 * their old copies; rows it covers but does not return were deleted; rows
 * older than the page's last row (loaded with loadMore) are kept. `covers(id)`
 * reports whether the page's range included a loaded row.
 */
export function mergeConversationPage(loaded, page) {
  const items = (Array.isArray(page?.items) ? page.items : [])
    .filter((c) => c && typeof c === 'object' && c.id !== undefined && c.id !== null);
  const pageCursor = typeof page?.nextCursor === 'string' ? page.nextCursor : '';
  const current = Array.isArray(loaded) ? loaded : [];
  const fresh = new Set(items.map((c) => String(c.id)));
  if (!pageCursor) {
    // The page is the whole list.
    return { conversations: items, nextCursor: '', covers: () => true, complete: true };
  }
  const last = items[items.length - 1];
  const lastIndex = last ? current.findIndex((c) => String(c?.id) === String(last.id)) : -1;
  const beyond = (row, index) => {
    if (!last) return false;
    const older = isOlder(row, last);
    if (older !== null) return older;
    // No timestamps: fall back to the loaded order.
    return lastIndex >= 0 && index > lastIndex;
  };
  const kept = current.filter((row, index) => row && !fresh.has(String(row.id)) && beyond(row, index));
  const keptIds = new Set(kept.map((c) => String(c.id)));
  return {
    conversations: [...items, ...kept],
    // Rows beyond the first page were loaded with loadMore: keep their cursor.
    nextCursor: kept.length > 0 ? null : pageCursor,
    covers: (id) => fresh.has(String(id)) || !keptIds.has(String(id)),
    complete: false,
  };
}

function sameConversations(a, b) {
  if (a.length !== b.length) return false;
  try {
    return JSON.stringify(a) === JSON.stringify(b);
  } catch {
    return false;
  }
}

function errorOf(err) {
  if (err && typeof err === 'object') {
    return {
      code: typeof err.code === 'string' && err.code ? err.code : 'error',
      message: typeof err.message === 'string' && err.message ? err.message : 'Something went wrong.',
    };
  }
  return { code: 'error', message: String(err ?? 'Something went wrong.') };
}

function isResyncError(err) {
  return err instanceof ResyncRequiredError
    || err instanceof GapError
    || err instanceof ChatProtocolError
    || err instanceof ForeignTurnError
    || err instanceof AfterTerminalError
    || err instanceof InvalidEventError;
}

/**
 * Controller for one chat mount.
 *
 * Consumer contract:
 *   - read `controller.state`; subscribe with addEventListener('change', ...);
 *   - call `init()` once, then `open()`/`send()`;
 *   - call `setAccount()` whenever the signed-in identity can change, before
 *     rendering, so another account's data is never shown.
 */
export class ChatController extends EventTarget {
  constructor({
    client,
    storageKey = null,
    reconnect = {},
    idleTimeoutMs = IDLE_TIMEOUT_MS,
    editor = null,
    sync = {},
    context = null,
  } = {}) {
    super();
    if (!client) throw new TypeError('ChatController requires a client');
    if (editor !== null && editor !== undefined && typeof editor !== 'object') {
      throw new TypeError('ChatController editor must be an object');
    }
    if (context !== null && context !== undefined && typeof context !== 'function') {
      throw new TypeError('ChatController context must be a function');
    }
    this._context = context ?? null;

    this._client = client;
    this._reconnect = {
      initialDelayMs: reconnect.initialDelayMs ?? 500,
      maxDelayMs: reconnect.maxDelayMs ?? 10000,
      maxAttempts: reconnect.maxAttempts ?? Infinity,
    };
    this._idleTimeoutMs = idleTimeoutMs;
    this._editor = editor ?? null;

    const key = requireAccountFields(storageKey);
    this._storageKey = key;
    this._accountKey = key ? accountKeyOf(key) : null;
    this._state = initialState();

    this._epoch = 0;
    this._openToken = 0;
    this._compactionWatch = null;
    this._listToken = 0;
    this._streamToken = 0;
    this._streamAbort = null;
    this._idleTimer = null;
    this._lifecycle = new AbortController();
    this._disposed = false;
    this._active = 0;
    this._idleWaiters = [];

    // List sync (see the header). `sync: false` turns all of it off.
    const syncOptions = sync === false ? null : (sync && typeof sync === 'object' ? sync : {});
    this._sync = syncOptions && {
      channel: syncOptions.channel !== false,
      focus: syncOptions.focus !== false,
      intervalMs: syncOptions.intervalMs === undefined ? SYNC_INTERVAL_MS : Number(syncOptions.intervalMs) || 0,
    };
    this._syncId = newIdempotencyKey();
    this._syncChannel = null;
    this._syncTimer = null;
    this._syncInterval = null;
    this._syncListeners = [];
    this._listVersion = 0;
    this._refreshToken = 0;
    this._startSync();
  }

  /** The immutable state snapshot. */
  get state() {
    return this._state;
  }

  /** The encoded account key, or null when no storage key was given. */
  get accountKey() {
    return this._accountKey;
  }

  /** The editor host adapter, or null. */
  get editor() {
    return this._editor;
  }

  // --- plumbing ---

  _set(patch) {
    if (this._disposed) return;
    const next = { ...this._state, ...patch };
    // The transcript is a pure derivation of the canonical messages, so no
    // caller can forget to refresh it.
    if (patch.messages !== undefined) next.transcript = transcriptFromMessages(next.messages);
    this._state = next;
    this.dispatchEvent(new Event('change'));
  }

  // --- list sync ---

  _startSync() {
    this._stopSync();
    const sync = this._sync;
    if (!sync || this._disposed) return;
    if (sync.channel && this._accountKey && typeof globalThis.BroadcastChannel === 'function') {
      try {
        const channel = new globalThis.BroadcastChannel(SYNC_CHANNEL_PREFIX + this._accountKey);
        channel.onmessage = (event) => {
          const data = event?.data;
          if (data && data.type === 'conversations' && data.from !== this._syncId) this._scheduleRefresh();
        };
        // Node keeps a process alive for an open channel; a browser ignores this.
        if (typeof channel.unref === 'function') channel.unref();
        this._syncChannel = channel;
      } catch {
        this._syncChannel = null;
      }
    }
    const doc = globalThis.document;
    const visible = () => !doc || doc.visibilityState !== 'hidden';
    if (sync.focus && doc && typeof doc.addEventListener === 'function') {
      const onVisible = () => { if (visible()) this._scheduleRefresh(); };
      doc.addEventListener('visibilitychange', onVisible);
      this._syncListeners.push([doc, 'visibilitychange', onVisible]);
      if (typeof globalThis.addEventListener === 'function') {
        const onFocus = () => this._scheduleRefresh();
        globalThis.addEventListener('focus', onFocus);
        this._syncListeners.push([globalThis, 'focus', onFocus]);
      }
    }
    if (sync.intervalMs > 0) {
      this._syncInterval = setInterval(() => { if (visible()) this._scheduleRefresh(); }, sync.intervalMs);
      if (typeof this._syncInterval?.unref === 'function') this._syncInterval.unref();
    }
  }

  _stopSync() {
    if (this._syncChannel) {
      try { this._syncChannel.close(); } catch { /* already closed */ }
      this._syncChannel = null;
    }
    for (const [target, type, handler] of this._syncListeners) target.removeEventListener(type, handler);
    this._syncListeners = [];
    if (this._syncInterval !== null) clearInterval(this._syncInterval);
    this._syncInterval = null;
    if (this._syncTimer !== null) clearTimeout(this._syncTimer);
    this._syncTimer = null;
  }

  _scheduleRefresh() {
    if (this._disposed || this._syncTimer !== null) return;
    this._syncTimer = setTimeout(() => {
      this._syncTimer = null;
      void this.refreshConversations();
    }, SYNC_DEBOUNCE_MS);
    if (typeof this._syncTimer?.unref === 'function') this._syncTimer.unref();
  }

  /** Tell other controllers for this account that the list changed. */
  _announce() {
    if (!this._syncChannel) return;
    try {
      this._syncChannel.postMessage({ type: 'conversations', from: this._syncId });
    } catch {
      // A closed channel: nothing to tell.
    }
  }

  /** A local write changed the list: invalidate in-flight refreshes, tell others. */
  _listChanged() {
    this._listVersion += 1;
    this._announce();
  }

  _track(promise) {
    this._active += 1;
    const settle = () => {
      this._active -= 1;
      if (this._active === 0) {
        const waiters = this._idleWaiters;
        this._idleWaiters = [];
        for (const resolve of waiters) resolve();
      }
    };
    Promise.resolve(promise).then(settle, settle);
    return promise;
  }

  /** Resolves when no request, stream, reconnect or poll is in flight. */
  whenSettled() {
    if (this._active === 0) return Promise.resolve();
    return new Promise((resolve) => this._idleWaiters.push(resolve));
  }

  _alive(epoch, token = this._streamToken) {
    return !this._disposed && epoch === this._epoch && token === this._streamToken;
  }

  /** Whether a non-stream request started at `epoch` may still write state. */
  _current(epoch) {
    return !this._disposed && epoch === this._epoch;
  }

  _backoff(attempt) {
    const { initialDelayMs, maxDelayMs } = this._reconnect;
    return Math.min(maxDelayMs, initialDelayMs * (2 ** Math.max(0, attempt - 1)));
  }

  _fail(err) {
    this._set({ error: errorOf(err), running: false });
  }

  _stopStream() {
    this._streamToken += 1;
    if (this._idleTimer !== null) {
      clearTimeout(this._idleTimer);
      this._idleTimer = null;
    }
    const active = this._streamAbort;
    this._streamAbort = null;
    if (active) {
      try {
        active.abort();
      } catch {
        // Already aborted.
      }
    }
  }

  _startTurnStream(turnId, after, openToken = this._openToken) {
    this._stopStream();
    const token = this._streamToken;
    const epoch = this._epoch;
    this._track(this._runTurnStream(turnId, after, { epoch, token, openToken }));
  }

  async _runTurnStream(turnId, after, { epoch, token, openToken }) {
    let lastSeq = after;
    let attempt = 0;

    while (this._alive(epoch, token)) {
      const controller = new AbortController();
      const unlink = linkAbort(this._lifecycle.signal, controller);
      let idleFired = false;
      let received = false;

      this._streamAbort = controller;
      let idleTimer = null;
      const armIdle = () => {
        if (!(this._idleTimeoutMs > 0)) return;
        if (idleTimer !== null) clearTimeout(idleTimer);
        idleTimer = setTimeout(() => {
          idleFired = true;
          controller.abort(new IdleTimeoutError(this._idleTimeoutMs));
        }, this._idleTimeoutMs);
        this._idleTimer = idleTimer;
      };
      const clearIdle = () => {
        if (idleTimer === null) return;
        clearTimeout(idleTimer);
        if (this._idleTimer === idleTimer) this._idleTimer = null;
        idleTimer = null;
      };
      armIdle();

      let result = null;
      let failure = null;
      try {
        result = await this._client.streamTurn(turnId, {
          after: lastSeq,
          signal: controller.signal,
          onEvent: (event) => {
            received = true;
            armIdle();
            this._applyStreamEvent(turnId, event);
            lastSeq = event.seq;
          },
          onHeartbeat: () => {
            armIdle();
          },
        });
      } catch (err) {
        failure = err;
      } finally {
        clearIdle();
        unlink();
        if (this._streamAbort === controller) this._streamAbort = null;
      }

      if (!this._alive(epoch, token)) return;

      if (failure) {
        if (idleFired) {
          // Fall through to the reconnect below.
        } else if (isResyncError(failure)) {
          await this._resync(turnId, { epoch, token, openToken });
          return;
        } else if (isAbort(failure, controller.signal)) {
          return;
        } else if (failure instanceof ChatApiError) {
          this._fail(failure);
          return;
        }
        // Anything else (ChatNetworkError, a transient fetch failure or an
        // idle abort) is a reconnect.
      } else if (result) {
        lastSeq = Number.isFinite(result.lastSeq) ? Math.max(lastSeq, result.lastSeq) : lastSeq;
        if (result.terminal) {
          this._set({ running: false });
          await this._reloadTranscript(this._state.activeConversationId, { epoch, openToken });
          return;
        }
      }

      if (received) attempt = 0;
      attempt += 1;
      if (attempt > this._reconnect.maxAttempts) {
        this._fail(new ChatNetworkError('The event stream could not be reconnected.'));
        return;
      }
      try {
        await sleep(this._backoff(attempt), this._lifecycle.signal);
      } catch {
        return;
      }
    }
  }

  _applyStreamEvent(turnId, event) {
    const current = this._state.turn;
    const view = current && current.turnId === (event.turnId || turnId)
      ? current
      : createTurnView(event.turnId || turnId);
    // applyEvent throws GapError/ForeignTurnError/AfterTerminalError, which the
    // stream loop turns into a resync.
    const next = applyEvent(view, event);
    if (next === view) return;
    const patch = {
      turn: next,
      running: !isTerminalState(next.state),
      error: null,
    };
    // Proposal events also update the conversation-level list, so a decision
    // or a new proposal is visible without waiting for the terminal reload.
    if (next.proposals !== view.proposals) {
      patch.proposals = mergeProposals(this._state.proposals, next.proposals);
    }
    this._set(patch);
  }

  async _reloadTranscript(conversationId, { epoch, openToken } = {}) {
    if (!conversationId) return;
    if (!this._alive(epoch ?? this._epoch, this._streamToken)) return;
    if ((openToken ?? this._openToken) !== this._openToken) return;
    if (this._state.activeConversationId !== conversationId) return;
    try {
      const detail = await this._client.getConversation(conversationId);
      if (!this._alive(epoch ?? this._epoch, this._streamToken)) return;
      if ((openToken ?? this._openToken) !== this._openToken) return;
      if (this._state.activeConversationId !== conversationId) return;
      const patch = {};
      if (Array.isArray(detail?.messages)) patch.messages = detail.messages;
      // A reload clears terminal proposals (the server only returns the ones
      // that can still be decided), but a terminal entry is kept when the
      // server does not mention it at all, so an outcome stays visible for the
      // rest of this view.
      if (Array.isArray(detail?.proposals)) {
        patch.proposals = this._mergeServerProposals(detail.proposals);
      }
      if (detail && 'context' in detail) patch.context = contextOf(detail.context);
      if (Object.keys(patch).length > 0) this._set(patch);
      // A nearly full conversation is summarized in the background after
      // the turn: watch for it so the view can mark where it starts.
      if (patch.context?.compacting) this._watchCompaction(conversationId);
      // A finished turn moves the conversation's activity time, and the server
      // may have titled it: re-read the list here and in other controllers.
      this._announce();
      void this.refreshConversations();
    } catch (err) {
      if (this._disposed || epoch !== this._epoch) return;
      if (isAbort(err)) return;
      this._set({ error: errorOf(err) });
    }
  }

  /**
   * Merge the conversation's open proposals from a GET into the list: the
   * server's set is authoritative for anything still decidable, while a
   * terminal entry that is no longer returned stays visible until the next
   * open (so a decision's outcome does not vanish mid-view).
   */
  _mergeServerProposals(incoming) {
    const ids = new Set(incoming.map((p) => p && p.id));
    const base = this._state.proposals.filter(
      (p) => ids.has(p.id) || isTerminalProposalState(p.state),
    );
    return mergeProposals(base, incoming);
  }

  async _resync(turnId, { epoch, token, openToken }) {
    try {
      const turn = await this._client.getTurn(turnId);
      if (!this._alive(epoch, token)) return;

      const conversationId = turn?.conversationId || this._state.activeConversationId;
      if (!conversationId) return;
      if (this._state.activeConversationId && this._state.activeConversationId !== conversationId) return;

      const detail = await this._client.getConversation(conversationId);
      if (!this._alive(epoch, token)) return;

      const messages = Array.isArray(detail?.messages) ? detail.messages : [];
      const terminal = isTerminalState(turn?.state);
      const patch = {
        activeConversationId: conversationId,
        messages,
        turn: viewFromTranscript(turn, messages),
        running: !terminal,
        error: null,
      };
      if (Array.isArray(detail?.proposals)) {
        patch.proposals = this._mergeServerProposals(detail.proposals);
      }
      this._set(patch);
      if (terminal) return;

      // No cursor exists after a resync: poll canonical state until the turn
      // ends, then reload the transcript once.
      let attempt = 0;
      for (;;) {
        if (!this._alive(epoch, token)) return;
        attempt += 1;
        if (attempt > this._reconnect.maxAttempts) {
          this._fail(new ChatNetworkError('The turn could not be refreshed.'));
          return;
        }
        try {
          await sleep(this._backoff(attempt), this._lifecycle.signal);
        } catch {
          return;
        }
        if (!this._alive(epoch, token)) return;

        let latest;
        try {
          latest = await this._client.getTurn(turnId);
        } catch (err) {
          if (!this._alive(epoch, token)) return;
          if (err instanceof ChatApiError && err.code !== 'internal') {
            this._fail(err);
            return;
          }
          continue;
        }
        if (!this._alive(epoch, token)) return;

        const done = isTerminalState(latest?.state);
        const view = this._state.turn ?? createTurnView(turnId);
        this._set({
          turn: {
            ...view,
            state: String(latest?.state ?? view.state),
            error: latest?.error ?? null,
          },
          running: !done,
          error: null,
        });
        if (done) {
          await this._reloadTranscript(conversationId, { epoch, openToken });
          return;
        }
      }
    } catch (err) {
      if (!this._alive(epoch, token)) return;
      if (isAbort(err)) return;
      this._fail(err);
    }
  }

  // --- lifecycle ---

  /** Load status and the first conversation page. */
  init() {
    return this._track(this._init());
  }

  async _init() {
    if (this._disposed) return this._state;
    const epoch = this._epoch;
    this._set({ status: 'loading', error: null });
    try {
      const [status, page] = await Promise.all([
        this._client.status(),
        this._client.listConversations({}),
      ]);
      if (!this._current(epoch)) return this._state;
      const choices = Array.isArray(status?.choices)
        ? status.choices.filter((choice) => choice && typeof choice === 'object' && typeof choice.id === 'string')
        : [];
      const serverDefault = typeof status?.defaultChoice === 'string' ? status.defaultChoice : '';
      const defaultChoice = choices.some((choice) => choice.id === serverDefault)
        ? serverDefault
        : choices.find((choice) => choice.default)?.id ?? '';
      const currentChoice = choices.some((choice) => choice.id === this._state.choice)
        ? this._state.choice
        : defaultChoice;
      this._set({
        status: 'ready',
        models: Array.isArray(status?.models) ? status.models : [],
        choices,
        defaultChoice,
        choice: currentChoice,
        features: status?.features && typeof status.features === 'object' ? status.features : null,
        limits: status?.limits && typeof status.limits === 'object' ? status.limits : null,
        conversations: Array.isArray(page?.items) ? page.items : [],
        nextCursor: typeof page?.nextCursor === 'string' ? page.nextCursor : '',
        error: null,
      });
    } catch (err) {
      if (!this._current(epoch)) return this._state;
      if (isAbort(err)) return this._state;
      this._set({ status: 'error', error: errorOf(err) });
    }
    return this._state;
  }

  /** Change the signed-in account. Different key: abort, clear, re-init. */
  async setAccount(storageKey) {
    const key = requireAccountFields(storageKey);
    const accountKey = key ? accountKeyOf(key) : null;
    if (accountKey === this._accountKey) {
      this._storageKey = key;
      return this._state;
    }
    if (this._disposed) return this._state;

    this._storageKey = key;
    this._accountKey = accountKey;
    this._epoch += 1;
    this._openToken += 1;
    this._listToken += 1;
    this._stopStream();
    const lifecycle = this._lifecycle;
    this._lifecycle = new AbortController();
    lifecycle.abort();

    // Clear everything the previous account owned before any new request.
    this._state = initialState();
    this.dispatchEvent(new Event('change'));
    // The sync channel is named after the account: reopen it for the new one.
    this._startSync();

    if (!accountKey) return this._state;
    return this._track(this._init());
  }

  /** Drop every subscription. The controller is unusable afterwards. */
  dispose() {
    if (this._disposed) return;
    this._stopSync();
    this._disposed = true;
    this._epoch += 1;
    this._openToken += 1;
    this._listToken += 1;
    this._stopStream();
    this._lifecycle.abort();
  }

  // --- conversations ---

  /** Append the next conversation page. */
  loadMore() {
    return this._track(this._loadMore());
  }

  async _loadMore() {
    const cursor = this._state.nextCursor;
    if (!cursor || this._disposed) return this._state;
    const epoch = this._epoch;
    const token = (this._listToken += 1);
    try {
      const page = await this._client.listConversations({ cursor });
      if (!this._current(epoch) || token !== this._listToken) return this._state;
      const known = new Set(this._state.conversations.map((c) => c.id));
      const items = Array.isArray(page?.items) ? page.items : [];
      this._set({
        conversations: [
          ...this._state.conversations,
          ...items.filter((c) => !known.has(c.id)),
        ],
        nextCursor: typeof page?.nextCursor === 'string' ? page.nextCursor : '',
      });
    } catch (err) {
      if (!this._current(epoch) || token !== this._listToken) return this._state;
      if (isAbort(err)) return this._state;
      this._set({ error: errorOf(err) });
    }
    return this._state;
  }

  /**
   * Re-read the first conversation page and merge it into the list: new and
   * renamed rows appear, deleted ones go, rows loaded with loadMore stay. When
   * the open conversation was deleted elsewhere, the view is cleared.
   */
  refreshConversations() {
    return this._track(this._refreshConversations());
  }

  async _refreshConversations(retry = 0) {
    if (this._disposed || this._state.status !== 'ready') return this._state;
    const epoch = this._epoch;
    const token = (this._refreshToken += 1);
    const version = this._listVersion;
    let page;
    try {
      page = await this._client.listConversations({});
    } catch (err) {
      // A background refresh never shows an error: the next one tries again.
      return this._state;
    }
    if (!this._current(epoch) || token !== this._refreshToken) return this._state;
    if (version !== this._listVersion) {
      // A local write landed while the page was in flight; the page may predate
      // it. Read again rather than undo it.
      return retry < 2 ? this._refreshConversations(retry + 1) : this._state;
    }
    const merged = mergeConversationPage(this._state.conversations, page);
    const patch = {
      conversations: merged.conversations,
      nextCursor: merged.nextCursor === null ? this._state.nextCursor : merged.nextCursor,
    };
    const activeId = this._state.activeConversationId;
    const activeRow = activeId
      ? this._state.conversations.find((c) => String(c?.id) === String(activeId))
      : null;
    let gone = false;
    if (activeRow && merged.covers(activeId)
      && !merged.conversations.some((c) => String(c.id) === String(activeId))) {
      // Missing from a page that should hold it: deleted elsewhere (another
      // tab, device or element)? Only a 404 says so; anything else keeps it.
      try {
        await this._client.getConversation(activeId, { limit: 1 });
      } catch (err) {
        gone = err instanceof ChatApiError && err.status === 404;
      }
      if (!this._current(epoch) || token !== this._refreshToken) return this._state;
      if (activeId !== this._state.activeConversationId) return this._state;
      if (!gone) patch.conversations = [activeRow, ...patch.conversations];
    }
    if (gone) {
      this._stopStream();
      this._openToken += 1;
      Object.assign(patch, {
        activeConversationId: null,
        messages: [],
        proposals: [],
        pendingUploads: [],
        turn: null,
        running: false,
      });
    }
    const same = patch.nextCursor === this._state.nextCursor
      && !('activeConversationId' in patch)
      && sameConversations(patch.conversations, this._state.conversations);
    if (!same) this._set(patch);
    return this._state;
  }

  /**
   * Leave the open conversation without creating one: the view is empty and
   * the next send() creates the conversation (a "New chat" that never gets a
   * message leaves nothing behind on the server).
   */
  startDraft() {
    if (this._disposed) return this._state;
    this._stopStream();
    this._openToken += 1;
    this._set({
      activeConversationId: null,
      messages: [],
      proposals: [],
      pendingUploads: [],
      turn: null,
      running: false,
      error: null,
      context: null,
    });
    return this._state;
  }

  /** Open a conversation and attach to its active turn, if any. */
  open(conversationId) {
    return this._track(this._open(conversationId));
  }

  async _open(conversationId) {
    if (this._disposed) return this._state;
    const epoch = this._epoch;
    const token = (this._openToken += 1);
    this._stopStream();
    this._set({
      activeConversationId: conversationId,
      messages: [],
      proposals: [],
      pendingUploads: [],
      turn: null,
      running: false,
      error: null,
      context: null,
    });

    try {
      const detail = await this._client.getConversation(conversationId);
      if (!this._current(epoch) || token !== this._openToken) return this._state;

      const messages = Array.isArray(detail?.messages) ? detail.messages : [];
      const proposals = Array.isArray(detail?.proposals) ? detail.proposals : [];
      const conversation = detail?.conversation;
      const conversations = conversation && !this._state.conversations.some((c) => c.id === conversation.id)
        ? [conversation, ...this._state.conversations]
        : this._state.conversations;

      const active = detail?.activeTurn ?? null;
      if (active && active.id) {
        const view = {
          ...createTurnView(active.id),
          state: String(active.state ?? ''),
          error: active.error ?? null,
        };
        this._set({
          conversations,
          messages,
          proposals: mergeProposals([], proposals),
          turn: view,
          running: !isTerminalState(active.state),
          context: contextOf(detail?.context),
        });
        // Stream from the beginning: the reducer is the source of the live
        // turn text, and replay from 0 is exactly the subscription's starting
        // point.
        this._startTurnStream(active.id, 0, token);
      } else {
        this._set({
          conversations,
          messages,
          proposals: mergeProposals([], proposals),
          turn: null,
          running: false,
          context: contextOf(detail?.context),
        });
      }
    } catch (err) {
      if (!this._current(epoch) || token !== this._openToken) return this._state;
      if (isAbort(err)) return this._state;
      this._set({ error: errorOf(err), running: false });
    }
    return this._state;
  }

  newConversation(title) {
    return this._track(this._newConversation(title));
  }

  async _newConversation(title) {
    if (this._disposed) return null;
    const epoch = this._epoch;
    try {
      const conversation = await this._client.createConversation(
        title === undefined ? {} : { title },
      );
      if (!this._current(epoch)) return null;
      this._stopStream();
      this._openToken += 1;
      this._set({
        conversations: [conversation, ...this._state.conversations.filter((c) => c.id !== conversation.id)],
        activeConversationId: conversation.id,
        messages: [],
        proposals: [],
        pendingUploads: [],
        turn: null,
        running: false,
        error: null,
        context: null,
      });
      this._listChanged();
      return conversation;
    } catch (err) {
      if (!this._current(epoch)) return null;
      if (!isAbort(err)) this._set({ error: errorOf(err) });
      return null;
    }
  }

  /** Re-read how full the open conversation's context is. */
  refreshContext() {
    return this._track(this._refreshContext(this._state.activeConversationId));
  }

  async _refreshContext(conversationId) {
    if (this._disposed || !conversationId) return this._state.context;
    const epoch = this._epoch;
    const openToken = this._openToken;
    try {
      const context = contextOf(await this._client.getContext(conversationId));
      if (!this._current(epoch) || openToken !== this._openToken) return this._state.context;
      if (this._state.activeConversationId !== conversationId) return this._state.context;
      this._set({ context });
      return context;
    } catch (err) {
      // The indicator is advisory: a failed read leaves the last value.
      return this._state.context;
    }
  }

  /** Poll for a background summary until it lands or the attempts run out. */
  _watchCompaction(conversationId) {
    if (this._compactionWatch === conversationId) return;
    this._compactionWatch = conversationId;
    const openToken = this._openToken;
    this._track((async () => {
      try {
        for (let i = 0; i < CONTEXT_POLL_ATTEMPTS; i += 1) {
          try {
            await sleep(CONTEXT_POLL_MS, this._lifecycle.signal);
          } catch {
            return;
          }
          if (openToken !== this._openToken || this._state.activeConversationId !== conversationId) return;
          const context = await this._refreshContext(conversationId);
          if (!context?.compacting) return;
        }
      } finally {
        if (this._compactionWatch === conversationId) this._compactionWatch = null;
      }
    })());
  }

  /**
   * Summarize the open conversation's older messages now (the server keeps
   * the last turns in full). Resolves to the server's answer, or null when
   * nothing is open or the request failed (the error is in state).
   */
  compact() {
    return this._track(this._compact(this._state.activeConversationId));
  }

  async _compact(conversationId) {
    if (this._disposed || !conversationId) return null;
    const epoch = this._epoch;
    const openToken = this._openToken;
    const before = this._state.context;
    this._set({ context: { ...(before ?? contextOf({})), compacting: true }, error: null });
    try {
      const result = await this._client.compactConversation(conversationId);
      if (!this._current(epoch) || openToken !== this._openToken) return result;
      if (this._state.activeConversationId !== conversationId) return result;
      this._set({ context: contextOf(result?.context) ?? { ...(before ?? contextOf({})), compacting: false } });
      return result;
    } catch (err) {
      if (!this._current(epoch) || openToken !== this._openToken) return null;
      if (this._state.activeConversationId === conversationId) {
        const patch = { context: before ? { ...before, compacting: false } : null };
        if (!isAbort(err)) patch.error = errorOf(err);
        this._set(patch);
      }
      return null;
    }
  }

  rename(conversationId, title) {
    return this._track(this._rename(conversationId, title));
  }

  async _rename(conversationId, title) {
    if (this._disposed) return null;
    const epoch = this._epoch;
    try {
      const conversation = await this._client.renameConversation(conversationId, title);
      if (!this._current(epoch)) return null;
      this._set({
        conversations: this._state.conversations.map(
          (c) => (c.id === conversation.id ? conversation : c),
        ),
        error: null,
      });
      this._listChanged();
      return conversation;
    } catch (err) {
      if (!this._current(epoch)) return null;
      if (!isAbort(err)) this._set({ error: errorOf(err) });
      return null;
    }
  }

  remove(conversationId) {
    return this._track(this._remove(conversationId));
  }

  async _remove(conversationId) {
    if (this._disposed) return false;
    const epoch = this._epoch;
    if (this._state.activeConversationId === conversationId) this._stopStream();
    try {
      await this._client.deleteConversation(conversationId);
      if (!this._current(epoch)) return false;
      // Re-read the active id: another open() may have finished meanwhile.
      const clearActive = this._state.activeConversationId === conversationId;
      if (clearActive) this._openToken += 1;
      this._set({
        conversations: this._state.conversations.filter((c) => c.id !== conversationId),
        ...(clearActive
          ? {
            activeConversationId: null,
            messages: [],
            proposals: [],
            pendingUploads: [],
            turn: null,
            running: false,
          }
          : {}),
        error: null,
      });
      this._listChanged();
      return true;
    } catch (err) {
      if (!this._current(epoch)) return false;
      if (!isAbort(err)) this._set({ error: errorOf(err) });
      return false;
    }
  }

  // --- proposals ---

  /**
   * Approve or reject one proposal.
   *
   * The decision always carries the revision currently in state, so it can
   * never address arguments that changed after they were shown. A 409 conflict
   * is not fatal: the server's stored proposal is adopted into state, the
   * message is recorded on that proposal, and the call resolves with it.
   *
   * Approving an editor proposal runs the editor host adapter: validatePatch,
   * then applyPatch, then an outcome report. It never reports "saved".
   */
  decide(proposalId, decision, options = {}) {
    return this._track(this._decide(proposalId, decision, options));
  }

  async _decide(proposalId, decision, { arguments: editedArguments, signal } = {}) {
    if (decision !== 'approve' && decision !== 'reject') {
      throw new TypeError('ChatController.decide() requires "approve" or "reject"');
    }
    if (this._disposed) return null;
    const epoch = this._epoch;
    const proposal = this._state.proposals.find((p) => p.id === proposalId);
    if (!proposal) {
      throw new ChatApiError(404, 'not_found', 'The proposal is no longer available.');
    }
    if (decision === 'approve' && proposal.location === 'editor' && !this._editor) {
      const message = 'Approving an editor change requires an editor adapter.';
      this._setProposalError(proposalId, { code: 'editor_unavailable', message });
      throw new ChatApiError(400, 'editor_unavailable', message);
    }

    let result;
    try {
      result = await this._client.decideProposal(proposalId, {
        decision,
        revision: proposal.revision,
        arguments: editedArguments,
        signal,
      });
    } catch (err) {
      if (!this._current(epoch)) return null;
      // 409: decided elsewhere or stale. 422 refused: the tool refused the
      // approved change before writing anything, and the proposal is pending
      // again with the reason. Either way the server's copy is the truth.
      if (err instanceof ChatApiError && (err.status === 409 || (err.status === 422 && err.code === 'refused')) && err.proposal) {
        const adopted = this._adoptProposal(err.proposal, { code: err.code, message: err.message });
        return { proposal: adopted, replayed: false, error: { code: err.code, message: err.message } };
      }
      throw err;
    }
    if (!this._current(epoch) || !result) return null;

    const stored = this._adoptProposal(result.proposal);
    const replayed = result.replayed === true;
    if (decision === 'approve' && stored.location === 'editor' && stored.state === 'approved') {
      const final = await this._applyEditorProposal(stored);
      return { proposal: final, replayed, error: final?.error ?? null };
    }
    return { proposal: stored, replayed, error: null };
  }

  /**
   * Report what the browser editor did with an approved editor proposal:
   * `applied`, `stale` or `failed`. The report uses the proposal's current
   * revision. A 409 conflict adopts the server's stored proposal like decide().
   */
  reportOutcome(proposalId, report = {}) {
    return this._track(this._reportOutcome(proposalId, report));
  }

  async _reportOutcome(proposalId, { status, editorRevision, hunks, message, signal } = {}) {
    if (this._disposed) return null;
    const epoch = this._epoch;
    const proposal = this._state.proposals.find((p) => p.id === proposalId);
    if (!proposal) {
      throw new ChatApiError(404, 'not_found', 'The proposal is no longer available.');
    }
    let result;
    try {
      result = await this._client.reportOutcome(proposalId, {
        revision: proposal.revision,
        status,
        editorRevision,
        hunks,
        message,
        signal,
      });
    } catch (err) {
      if (!this._current(epoch)) return null;
      // 409: decided elsewhere or stale. 422 refused: the tool refused the
      // approved change before writing anything, and the proposal is pending
      // again with the reason. Either way the server's copy is the truth.
      if (err instanceof ChatApiError && (err.status === 409 || (err.status === 422 && err.code === 'refused')) && err.proposal) {
        const adopted = this._adoptProposal(err.proposal, { code: err.code, message: err.message });
        return { proposal: adopted, replayed: false, error: { code: err.code, message: err.message } };
      }
      throw err;
    }
    if (!this._current(epoch) || !result) return null;
    const stored = this._adoptProposal(result.proposal);
    return { proposal: stored, replayed: result.replayed === true, error: null };
  }

  /** Merge one server proposal into state and return the stored entry. */
  _adoptProposal(proposal, error = null) {
    if (!proposal || typeof proposal !== 'object' || !proposal.id) return proposal ?? null;
    const entry = { ...proposal, error: error ?? null };
    const proposals = mergeProposals(this._state.proposals, [entry]);
    this._set({ proposals });
    return proposals.find((p) => p.id === entry.id) ?? entry;
  }

  _setProposalError(proposalId, error) {
    this._set({
      proposals: this._state.proposals.map(
        (p) => (p.id === proposalId ? { ...p, error: error ?? null } : p),
      ),
    });
  }

  /**
   * Apply an approved editor proposal through the host adapter.
   *
   * The adapter decides applicability (validatePatch) and performs the local
   * draft edit (applyPatch). The controller then reports exactly one outcome,
   * so the server records what the browser did; nothing here is a save.
   */
  async _applyEditorProposal(proposal) {
    const editor = this._editor;
    if (!editor) {
      const message = 'Approving an editor change requires an editor adapter.';
      this._setProposalError(proposal.id, { code: 'editor_unavailable', message });
      throw new ChatApiError(400, 'editor_unavailable', message);
    }

    if (typeof editor.validatePatch === 'function') {
      let verdict;
      try {
        verdict = await editor.validatePatch(proposal);
      } catch (err) {
        return this._recordEditorOutcome(proposal, {
          status: 'failed',
          message: errorOf(err).message,
        });
      }
      if (!applicableVerdict(verdict)) {
        return this._recordEditorOutcome(proposal, {
          status: 'stale',
          editorRevision: await this._editorRevision(),
        });
      }
    }

    let applied;
    try {
      applied = await editor.applyPatch(proposal);
    } catch (err) {
      return this._recordEditorOutcome(proposal, {
        status: 'failed',
        message: errorOf(err).message,
      });
    }

    const revision = typeof applied === 'string'
      ? applied
      : applied && typeof applied === 'object' && applied.revision !== undefined
        ? String(applied.revision)
        : '';
    const hunks = normalizeHunks(applied && typeof applied === 'object' ? applied.hunks : undefined);
    if (!revision) {
      return this._recordEditorOutcome(proposal, {
        status: 'failed',
        message: 'The editor did not report a new revision.',
        hunks,
      });
    }
    return this._recordEditorOutcome(proposal, {
      status: 'applied',
      editorRevision: revision,
      hunks,
    });
  }

  async _recordEditorOutcome(proposal, report) {
    try {
      const result = await this._reportOutcome(proposal.id, report);
      return result?.proposal ?? proposal;
    } catch (err) {
      // Reporting is the durable acknowledgement; if it could not be stored,
      // keep the approval visible and surface why the outcome is missing.
      this._setProposalError(proposal.id, errorOf(err));
      throw err;
    }
  }

  /** The host editor's current revision, as a string, or ''. */
  async _editorRevision() {
    if (!this._editor || typeof this._editor.snapshot !== 'function') return '';
    try {
      const snapshot = await this._editor.snapshot();
      if (snapshot && snapshot.revision !== undefined && snapshot.revision !== null) {
        return String(snapshot.revision);
      }
    } catch {
      // A snapshot failure only means a stale report cannot name the revision.
    }
    return '';
  }

  // --- attachments ---

  /**
   * Validate and upload files to the open conversation, creating one when none
   * is open. Limits are checked here for a fast, clear error; the server
   * re-validates every byte and decides the stored MIME type.
   *
   * Resolves with the stored upload descriptors, which are also kept in
   * `state.pendingUploads` until they are sent.
   *
   * Without `options.model` the type check is the union every ready model
   * accepts; pass the picker value (a choice id or model id) to check against
   * that model instead. `send()` always checks against the model it sends to.
   */
  attach(files, options = {}) {
    return this._track(this._attach(files, options));
  }

  async _attach(files, { signal, model } = {}) {
    if (this._disposed) return [];
    const list = Array.from(files ?? []);
    if (list.length === 0) return [];
    this._requireUploads();
    const epoch = this._epoch;

    let conversationId = this._state.activeConversationId;
    if (!conversationId) {
      const conversation = await this._newConversation();
      if (!conversation) {
        throw new ChatApiError(400, 'invalid_request', 'A conversation is needed before attaching a file.');
      }
      conversationId = conversation.id;
      if (!this._current(epoch)) return [];
    }

    const pending = this._state.pendingUploads.filter(
      (upload) => upload.conversationId === conversationId,
    );
    // Validate the whole batch before storing anything, so a refused file never
    // leaves earlier files of the same batch uploaded.
    const choice = typeof model === 'string' && model !== ''
      ? resolveChoice(this._state.choices, model)
      : null;
    this._validateAttachments(list, pending.length, choice);
    const uploads = [];
    for (const file of list) {
      const upload = await this._client.uploadFile(conversationId, file, file.name, { signal });
      if (!this._current(epoch)) return [];
      if (!upload || typeof upload !== 'object' || !upload.id) {
        throw new ChatApiError(201, 'invalid_response', 'The server returned an invalid response.');
      }
      uploads.push(upload);
    }
    this._set({
      pendingUploads: [...this._state.pendingUploads, ...uploads],
      error: null,
    });
    return uploads;
  }

  /** Forget every attachment that has not been sent yet. */
  clearAttachments() {
    this._set({ pendingUploads: [] });
    return this._state;
  }

  /** Throw when this deployment does not accept attachments at all. */
  _requireUploads() {
    if (this._state.features && this._state.features.uploads === false) {
      throw new ChatApiError(400, 'invalid_request', 'Attachments are not enabled on this server.');
    }
  }

  /**
   * Check the client-side attachment limits: the type list, the per-file size
   * and the per-turn count. A file whose type the browser cannot name is sent
   * anyway and left to the server, which types it from the bytes.
   */
  _validateAttachments(files, alreadyPending = 0, choice = null) {
    this._requireUploads();
    const limits = this._state.limits ?? {};
    const maxBytes = Number(limits.maxUploadBytes) > 0 ? Number(limits.maxUploadBytes) : 0;
    const maxCount = Number(limits.maxAttachmentsPerTurn) > 0
      ? Number(limits.maxAttachmentsPerTurn)
      : 0;
    // With a choice, check what that model can read; without one, the union
    // the upload route accepts.
    const allowed = attachmentTypesFor(limits, choice);
    const types = allowed ? allowed.map((t) => String(t).toLowerCase()) : null;

    if (maxCount > 0 && alreadyPending + files.length > maxCount) {
      throw new ChatApiError(
        400,
        'invalid_attachment',
        `A message can carry at most ${maxCount} attachment(s).`,
      );
    }
    for (const file of files) {
      if (!file || typeof file !== 'object') {
        throw new ChatApiError(400, 'invalid_request', 'An attachment must be a File or Blob.');
      }
      if (maxBytes > 0 && Number(file.size) > maxBytes) {
        throw new ChatApiError(
          413,
          'payload_too_large',
          `An attachment may be at most ${maxBytes} bytes.`,
        );
      }
      const mime = String(file.type || inferAttachmentMime(file.name) || '').toLowerCase();
      if (types && mime !== '' && !types.includes(mime)) {
        throw new ChatApiError(415, 'unsupported_media', `${mime} files are not accepted.`);
      }
    }
  }

  /**
   * Resolve the attachments of one send: upload ids are used as they are and
   * File/Blob values are uploaded first. Returns the ids in the submitted
   * order and keeps the new uploads pending until the turn is accepted.
   */
  async _resolveAttachments(conversationId, attachments, epoch, choice = null) {
    const list = Array.isArray(attachments) ? attachments : [];
    if (list.length === 0) return [];
    this._requireUploads();
    const blobs = list.filter((item) => typeof item !== 'string');
    this._validateAttachments(blobs, list.length - blobs.length, choice);

    const uploads = [];
    for (const file of blobs) {
      const upload = await this._client.uploadFile(conversationId, file, file.name);
      if (!this._current(epoch)) return null;
      if (!upload || typeof upload !== 'object' || !upload.id) {
        throw new ChatApiError(201, 'invalid_response', 'The server returned an invalid response.');
      }
      uploads.push(upload);
    }
    if (uploads.length > 0) {
      this._set({ pendingUploads: [...this._state.pendingUploads, ...uploads] });
    }
    return list.map((item) => (typeof item === 'string' ? item : uploads.shift().id));
  }

  // --- turns ---

  /**
   * Submit one message, then follow its turn.
   *
   * A network failure before any response is retried with the same
   * Idempotency-Key (three times), so a lost response can never create a second
   * turn. `attachments` accepts upload ids or File/Blob values; blobs are
   * uploaded first. Resolves with `{ turn, created }` once the submission is
   * accepted; the stream continues in the background and reports through
   * `state`.
   */
  send(text, options = {}) {
    return this._track(this._send(text, options));
  }

  async _send(text, { model, provider, effort, attachments, context } = {}) {
    const conversationId = this._state.activeConversationId;
    if (!conversationId) {
      throw new Error('ChatController.send() requires an open conversation');
    }
    if (typeof text !== 'string' || text.trim() === '') {
      throw new TypeError('ChatController.send() requires non-empty text');
    }
    if (this._disposed) return null;

    const selected = resolveChoice(this._state.choices, model ?? this._state.choice);
    if (selected) {
      provider = selected.provider;
      model = selected.model;
      this._set({ choice: selected.id });
    }
    // An effort only goes with a model that lists it; a stale pick from a
    // previously selected model is dropped instead of failing the send.
    const efforts = selected && Array.isArray(selected.efforts) ? selected.efforts : [];
    if (typeof effort !== 'string' || !efforts.includes(effort)) effort = undefined;
    const epoch = this._epoch;
    let attachmentIds = [];
    if (Array.isArray(attachments) && attachments.length > 0) {
      attachmentIds = await this._resolveAttachments(conversationId, attachments, epoch, selected);
      if (attachmentIds === null || !this._current(epoch)) return null;
    }

    // The page context is read once per send, so every retry of this
    // submission carries the same one. Only a server that accepts contexts
    // gets one.
    let turnContext;
    if (this._state.features?.context) {
      turnContext = context !== undefined ? context : this._context ? await this._context() : undefined;
      if (!this._current(epoch)) return null;
    }

    const idempotencyKey = newIdempotencyKey();
    this._set({ running: true, error: null });

    let result = null;
    for (let attempt = 0; attempt <= 3; attempt += 1) {
      try {
        result = await this._client.submitTurn(conversationId, {
          text,
          provider,
          model,
          effort,
          attachments: attachmentIds.length > 0 ? attachmentIds : undefined,
          context: turnContext ?? undefined,
          idempotencyKey,
        });
        break;
      } catch (err) {
        if (!this._current(epoch)) return null;
        const retryable = err instanceof ChatNetworkError && attempt < 3;
        if (!retryable) {
          if (isAbort(err)) {
            this._set({ running: false });
            return null;
          }
          this._fail(err);
          return null;
        }
        try {
          await sleep(
            Math.min(this._reconnect.maxDelayMs, this._reconnect.initialDelayMs * (attempt + 1)),
            this._lifecycle.signal,
          );
        } catch {
          return null;
        }
      }
    }
    if (!result || !this._current(epoch)) return null;

    // The accepted turn owns these uploads now, so they leave the composer.
    const sent = new Set(attachmentIds);
    const pendingUploads = this._state.pendingUploads.filter((upload) => !sent.has(upload.id));

    const turn = result.turn;
    const view = {
      ...createTurnView(turn.id),
      state: String(turn.state ?? ''),
      error: turn.error ?? null,
    };
    this._set({ turn: view, running: !isTerminalState(turn.state), error: null, pendingUploads });
    // The conversation's activity time moved on the server.
    this._listChanged();

    if (isTerminalState(turn.state)) {
      await this._reloadTranscript(conversationId, { epoch });
      return result;
    }
    this._startTurnStream(turn.id, 0, this._openToken);
    return result;
  }

  /** Cancel the running turn first, then drop the local stream. */
  stop() {
    return this._track(this._stop());
  }

  async _stop() {
    const turn = this._state.turn;
    const conversationId = this._state.activeConversationId;
    if (!turn || !turn.turnId || this._disposed) return this._state;
    const epoch = this._epoch;

    let cancelled = null;
    try {
      cancelled = await this._client.cancelTurn(turn.turnId);
    } catch (err) {
      if (!this._current(epoch)) return this._state;
      if (!isAbort(err)) this._set({ error: errorOf(err) });
    }
    if (!this._current(epoch)) return this._state;

    // Cancel must reach the server before the reader is dropped; the turn
    // keeps running server-side, so aborting first would leave it orphaned
    // while the UI looked stopped.
    this._stopStream();

    const current = this._state.turn ?? turn;
    this._set({
      turn: cancelled
        ? { ...current, state: String(cancelled.state ?? current.state), error: cancelled.error ?? current.error }
        : current,
      running: false,
    });

    await this._reloadTranscript(conversationId, { epoch, openToken: this._openToken });
    return this._state;
  }
}
