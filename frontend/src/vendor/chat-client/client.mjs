// Typed HTTP client for the MCP chat API (protocol v1).
//
// The client is transport-only: it sends the caller's token in the
// Authorization header (never in a URL), maps the server's error envelope to
// typed errors, and turns the SSE endpoint into parsed event envelopes. State
// reduction lives in reducer.mjs; reconnect policy lives in controller.mjs.
//
// It uses only standard web APIs (fetch, ReadableStream, AbortController), so
// the same code runs in a browser and in Node.

import { parseSSE } from './sse.mjs';
import { isTerminalEventType } from './reducer.mjs';

const DEFAULT_PREFIX = '/api/mcp/chat/v1';

/** The event envelope version this client implements. */
export const PROTOCOL_VERSION = '1';

/**
 * An error response from the server, decoded from {"error":{"code","message"}}.
 *
 * A decision or outcome conflict (409) also carries the authoritative stored
 * proposal, so the caller can correct its view without a second request; it is
 * exposed as `proposal` and is null on every other error.
 */
export class ChatApiError extends Error {
  constructor(status, code, message, proposal = null) {
    super(message || 'The request failed.');
    this.name = 'ChatApiError';
    this.status = status;
    this.code = code || 'internal';
    this.proposal = proposal && typeof proposal === 'object' ? proposal : null;
  }
}

/** The event cursor is gone; the caller must reload canonical state. */
export class ResyncRequiredError extends ChatApiError {
  constructor(status = 409, message = 'The event cursor is no longer available; reload the conversation.') {
    super(status, 'resync_required', message);
    this.name = 'ResyncRequiredError';
  }
}

/** The request never reached a usable HTTP response. */
export class ChatNetworkError extends Error {
  constructor(message, cause) {
    super(message || 'The server could not be reached.');
    this.name = 'ChatNetworkError';
    if (cause !== undefined) this.cause = cause;
  }
}

/** The server answered but not with the protocol this client implements. */
export class ChatProtocolError extends Error {
  constructor(message, code = 'protocol_error') {
    super(message);
    this.name = 'ChatProtocolError';
    this.code = code;
  }
}

const STATUS_CODES = new Map([
  [400, 'invalid_request'],
  [401, 'unauthorized'],
  [403, 'forbidden'],
  [404, 'not_found'],
  [405, 'method_not_allowed'],
  [409, 'conflict'],
  [413, 'payload_too_large'],
  [429, 'rate_limited'],
  [500, 'internal'],
]);

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

function encodeSegment(value) {
  return encodeURIComponent(String(value));
}

function queryString(query) {
  if (!query) return '';
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === '') continue;
    params.set(key, String(value));
  }
  const encoded = params.toString();
  return encoded ? `?${encoded}` : '';
}

/** A random, RFC 4122-shaped idempotency key (crypto.randomUUID when present). */
export function newIdempotencyKey() {
  const crypto = globalThis.crypto;
  if (crypto && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  const bytes = new Uint8Array(16);
  if (crypto && typeof crypto.getRandomValues === 'function') {
    crypto.getRandomValues(bytes);
  } else {
    for (let i = 0; i < bytes.length; i += 1) {
      bytes[i] = Math.floor(Math.random() * 256);
    }
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  return [
    hex.slice(0, 8),
    hex.slice(8, 12),
    hex.slice(12, 16),
    hex.slice(16, 20),
    hex.slice(20),
  ].join('-');
}

/**
 * Client for one chat API mount.
 *
 * `getToken` runs before every request, so a rotated or logged-out session is
 * picked up immediately; the token is only ever placed in the Authorization
 * header.
 */
export class ChatClient {
  constructor({ baseUrl = '', prefix = DEFAULT_PREFIX, getToken, fetch: fetchImpl } = {}) {
    if (typeof baseUrl !== 'string') {
      throw new TypeError('ChatClient requires a baseUrl string');
    }
    if (typeof getToken !== 'function') {
      throw new TypeError('ChatClient requires a getToken function');
    }
    let cleanPrefix = String(prefix || DEFAULT_PREFIX);
    if (!cleanPrefix.startsWith('/')) cleanPrefix = `/${cleanPrefix}`;
    cleanPrefix = cleanPrefix.replace(/\/+$/, '');

    this.baseUrl = baseUrl.replace(/\/+$/, '');
    this.prefix = cleanPrefix;
    this.getToken = getToken;
    this.fetchImpl = fetchImpl ?? ((...args) => globalThis.fetch(...args));
    this._root = `${this.baseUrl}${this.prefix}`;
  }

  _url(path, query) {
    return `${this._root}${path}${queryString(query)}`;
  }

  async _open(method, path, { query, body, form, headers, signal, idempotencyKey } = {}) {
    if (signal?.aborted) throw abortError(signal);
    if (body !== undefined && form !== undefined) {
      throw new TypeError('ChatClient request carries both a JSON body and a form');
    }
    // Read the token per request. A failure here is a caller bug, not a
    // network error, so it propagates unchanged.
    const token = await this.getToken();
    if (signal?.aborted) throw abortError(signal);
    const requestHeaders = { ...(headers ?? {}) };
    if (!Object.keys(requestHeaders).some((key) => key.toLowerCase() === 'accept')) {
      requestHeaders.Accept = 'application/json';
    }
    if (token) requestHeaders.Authorization = `Bearer ${token}`;
    // A multipart body must keep the boundary fetch/undici generates, so its
    // Content-Type is deliberately never set here.
    if (body !== undefined) requestHeaders['Content-Type'] = 'application/json';
    if (idempotencyKey) requestHeaders['Idempotency-Key'] = idempotencyKey;

    let response;
    try {
      response = await this.fetchImpl(this._url(path, query), {
        method,
        headers: requestHeaders,
        body: form !== undefined ? form : body === undefined ? undefined : JSON.stringify(body),
        signal,
      });
    } catch (err) {
      if (isAbort(err, signal)) throw abortError(signal);
      throw new ChatNetworkError('The server could not be reached.', err);
    }
    if (!response || typeof response.ok !== 'boolean') {
      throw new ChatNetworkError('fetch() did not return a Response.');
    }
    if (!response.ok) throw await this._errorFor(response);
    return response;
  }

  async _errorFor(response) {
    let body = '';
    try {
      body = await response.text();
    } catch {
      body = '';
    }
    let code = '';
    let message = '';
    let proposal = null;
    if (body) {
      try {
        const parsed = JSON.parse(body);
        if (parsed && typeof parsed === 'object' && parsed.error && typeof parsed.error === 'object') {
          if (typeof parsed.error.code === 'string') code = parsed.error.code;
          if (typeof parsed.error.message === 'string') message = parsed.error.message;
        }
        // A decision or outcome conflict carries the stored proposal next to
        // the error envelope, so the client can adopt it immediately.
        if (parsed && typeof parsed === 'object' && parsed.proposal
          && typeof parsed.proposal === 'object' && !Array.isArray(parsed.proposal)) {
          proposal = parsed.proposal;
        }
      } catch {
        // Not the protocol's JSON envelope.
      }
    }
    if (!code) code = STATUS_CODES.get(response.status) ?? 'internal';
    if (!message) message = response.statusText || 'The request failed.';
    if (code === 'resync_required') return new ResyncRequiredError(response.status, message);
    return new ChatApiError(response.status, code, message, proposal);
  }

  async _json(method, path, options = {}) {
    const response = await this._open(method, path, options);
    if (response.status === 204) {
      try {
        await response.body?.cancel();
      } catch {
        // Nothing to drain.
      }
      return null;
    }
    let text;
    try {
      text = await response.text();
    } catch (err) {
      throw new ChatNetworkError('The response body could not be read.', err);
    }
    if (text === '') return null;
    try {
      return JSON.parse(text);
    } catch {
      throw new ChatApiError(response.status, 'invalid_response', 'The server returned an invalid response.');
    }
  }

  /** GET /status */
  async status(options = {}) {
    return this._json('GET', '/status', { signal: options.signal });
  }

  /** GET /conversations */
  async listConversations({ limit, cursor, target, signal } = {}) {
    return this._json('GET', '/conversations', {
      query: { limit, cursor, target },
      signal,
    });
  }

  /** POST /conversations */
  async createConversation({ title, target, signal } = {}) {
    const body = {};
    if (title !== undefined) body.title = title;
    if (target !== undefined) body.target = target;
    return this._json('POST', '/conversations', { body, signal });
  }

  /** GET /conversations/{id} */
  async getConversation(id, { limit, cursor, signal } = {}) {
    return this._json('GET', `/conversations/${encodeSegment(id)}`, {
      query: { limit, cursor },
      signal,
    });
  }

  /** PATCH /conversations/{id} */
  async renameConversation(id, title, { signal } = {}) {
    return this._json('PATCH', `/conversations/${encodeSegment(id)}`, {
      body: { title },
      signal,
    });
  }

  /**
   * GET /conversations/{id}/context: how full the conversation's context is,
   * `{ tokens, window, estimated, summarizedThrough?, summarizedAt?,
   * compacting, canCompact }`. `window` 0 means unknown.
   */
  async getContext(id, { signal } = {}) {
    return this._json('GET', `/conversations/${encodeSegment(id)}/context`, { signal });
  }

  /**
   * POST /conversations/{id}/compact: summarize the conversation's older
   * messages now. Answers `{ compacted, reason?, throughMessageId?, context }`.
   */
  async compactConversation(id, { signal } = {}) {
    return this._json('POST', `/conversations/${encodeSegment(id)}/compact`, { signal });
  }

  /**
   * POST /conversations/actions/import: bring a thread this browser kept into
   * the server's history (plain user and assistant text). A repeated
   * `sourceId` returns the earlier import with `created: false`.
   */
  async importConversation({ sourceId, title, createdAt, messages, signal } = {}) {
    const body = { sourceId, messages };
    if (title !== undefined) body.title = title;
    if (createdAt !== undefined) body.createdAt = createdAt;
    return this._json('POST', '/conversations/actions/import', { body, signal });
  }

  /** DELETE /conversations/{id} */
  async deleteConversation(id, { signal } = {}) {
    return this._json('DELETE', `/conversations/${encodeSegment(id)}`, { signal });
  }

  /**
   * POST /conversations/{id}/turns
   *
   * An idempotency key is generated when the caller did not supply one. Retrying
   * with the same key can never create a second turn. `attachments` are upload
   * ids returned by uploadFile(); the server re-validates every reference.
   * `context` is a plain object describing the user's page (route, selection,
   * editor revision); it is sent only when the server reports
   * `features.context`, and the server rejects it otherwise.
   */
  async submitTurn(conversationId, { text, provider, model, effort, attachments, context, idempotencyKey, signal } = {}) {
    const key = idempotencyKey || newIdempotencyKey();
    const body = { text };
    if (provider !== undefined && provider !== '') body.provider = provider;
    if (model !== undefined && model !== '') body.model = model;
    if (typeof effort === 'string' && effort !== '') body.effort = effort;
    if (Array.isArray(attachments) && attachments.length > 0) {
      body.attachments = attachments.map((id) => String(id));
    }
    if (context !== undefined && context !== null) {
      if (typeof context !== 'object' || Array.isArray(context)) {
        throw new TypeError('submitTurn context must be a plain object');
      }
      body.context = context;
    }
    const result = await this._json('POST', `/conversations/${encodeSegment(conversationId)}/turns`, {
      body,
      signal,
      idempotencyKey: key,
    });
    if (!result || typeof result !== 'object' || !result.turn) {
      throw new ChatApiError(202, 'invalid_response', 'The server returned an invalid response.');
    }
    return { turn: result.turn, created: result.created === true };
  }

  // --- proposals ---

  /** GET /proposals/{id} */
  async getProposal(id, { signal } = {}) {
    const result = await this._json('GET', `/proposals/${encodeSegment(id)}`, { signal });
    if (!result || typeof result !== 'object' || !result.proposal) {
      throw new ChatApiError(200, 'invalid_response', 'The server returned an invalid response.');
    }
    return result.proposal;
  }

  /**
   * POST /proposals/{id}/decision
   *
   * `revision` is the proposal revision the user reviewed, so a decision can
   * never address arguments that changed after it was shown. Resolves with
   * `{ proposal, replayed }`; a 409 rejects with a ChatApiError whose
   * `proposal` is the authoritative stored state.
   */
  async decideProposal(id, { decision, revision, arguments: edited, signal } = {}) {
    if (decision !== 'approve' && decision !== 'reject') {
      throw new TypeError('decideProposal() requires decision "approve" or "reject"');
    }
    if (!Number.isInteger(revision) || revision < 1) {
      throw new TypeError('decideProposal() requires the reviewed revision');
    }
    const body = { decision, revision };
    if (edited !== undefined && edited !== null) body.arguments = edited;
    const result = await this._json('POST', `/proposals/${encodeSegment(id)}/decision`, {
      body,
      signal,
    });
    if (!result || typeof result !== 'object' || !result.proposal) {
      throw new ChatApiError(200, 'invalid_response', 'The server returned an invalid response.');
    }
    return { proposal: result.proposal, replayed: result.replayed === true };
  }

  /**
   * POST /proposals/{id}/outcome
   *
   * Records what the browser editor did with an approved editor proposal:
   * `applied`, `stale` or `failed`. It is an acknowledgement of a local draft,
   * never proof that anything was saved. `editorRevision` is required by the
   * server for an `applied` report.
   */
  async reportOutcome(id, { revision, status, editorRevision, hunks, message, signal } = {}) {
    if (!Number.isInteger(revision) || revision < 1) {
      throw new TypeError('reportOutcome() requires the approved revision');
    }
    const body = { revision, status };
    if (editorRevision !== undefined && editorRevision !== null && editorRevision !== '') {
      body.editorRevision = String(editorRevision);
    }
    if (Array.isArray(hunks) && hunks.length > 0) {
      body.hunks = hunks.map((hunk) => ({ id: String(hunk.id), status: String(hunk.status) }));
    }
    if (message !== undefined && message !== null && message !== '') body.message = String(message);
    const result = await this._json('POST', `/proposals/${encodeSegment(id)}/outcome`, {
      body,
      signal,
    });
    if (!result || typeof result !== 'object' || !result.proposal) {
      throw new ChatApiError(200, 'invalid_response', 'The server returned an invalid response.');
    }
    return { proposal: result.proposal, replayed: result.replayed === true };
  }

  // --- uploads ---

  /**
   * POST /uploads
   *
   * Sends one attachment as multipart/form-data with the server's two field
   * names, "conversationId" and "file". The bytes are the only thing trusted:
   * the server assigns the MIME type from them, so a renamed file cannot change
   * what the model is sent.
   */
  async uploadFile(conversationId, file, name, { signal } = {}) {
    if (conversationId === undefined || conversationId === null || String(conversationId) === '') {
      throw new TypeError('uploadFile() requires a conversation id');
    }
    if (!file || typeof file !== 'object') {
      throw new TypeError('uploadFile() requires a File or Blob');
    }
    if (typeof FormData !== 'function') {
      throw new Error('uploadFile() requires FormData');
    }
    const form = new FormData();
    form.append('conversationId', String(conversationId));
    const filename = name || file.name || 'upload';
    form.append('file', file, String(filename));
    const result = await this._json('POST', '/uploads', { form, signal });
    if (!result || typeof result !== 'object' || !result.upload) {
      throw new ChatApiError(201, 'invalid_response', 'The server returned an invalid response.');
    }
    return result.upload;
  }

  /** GET /uploads/{id}/meta */
  async getUploadMeta(id, { signal } = {}) {
    const result = await this._json('GET', `/uploads/${encodeSegment(id)}/meta`, { signal });
    if (!result || typeof result !== 'object' || !result.upload) {
      throw new ChatApiError(200, 'invalid_response', 'The server returned an invalid response.');
    }
    return result.upload;
  }

  /**
   * GET /uploads/{id}
   *
   * Reads one attachment with the caller's Authorization header and returns it
   * as a Blob. The token never appears in a URL, and no object URL is created
   * here; the caller owns the lifetime of whatever it builds from the Blob.
   */
  async downloadUpload(id, { signal } = {}) {
    const response = await this._open('GET', `/uploads/${encodeSegment(id)}`, {
      headers: { Accept: '*/*' },
      signal,
    });
    try {
      return await response.blob();
    } catch (err) {
      throw new ChatNetworkError('The attachment could not be read.', err);
    }
  }

  /** GET /turns/{id} */
  async getTurn(id, { signal } = {}) {
    return this._json('GET', `/turns/${encodeSegment(id)}`, { signal });
  }

  /** POST /turns/{id}/cancel */
  async cancelTurn(id, { signal } = {}) {
    return this._json('POST', `/turns/${encodeSegment(id)}/cancel`, { signal });
  }

  /**
   * Follow one turn's SSE stream.
   *
   * Resolves with `{ lastSeq, terminal }`: the highest sequence seen and
   * whether a terminal event arrived. A stream that ends without a terminal
   * event is a normal resolution, so the controller can reconnect from
   * `lastSeq`. A 409 resync_required rejects with ResyncRequiredError before
   * any stream bytes are read.
   *
   * @param {string} turnId
   * @param {object} [options]
   * @param {number} [options.after] Resume after this sequence; sent as both
   *   `?after=` and Last-Event-ID.
   * @param {AbortSignal} [options.signal]
   * @param {(event: object) => void|Promise<void>} [options.onEvent]
   * @param {(comment: string) => void} [options.onHeartbeat]
   * @returns {Promise<{ lastSeq: number, terminal: boolean }>}
   */
  async streamTurn(turnId, { after = 0, signal, onEvent, onHeartbeat } = {}) {
    const resume = Number.isFinite(after) && after > 0 ? Math.trunc(after) : 0;
    const headers = { Accept: 'text/event-stream' };
    if (resume > 0) headers['Last-Event-ID'] = String(resume);

    const response = await this._open('GET', `/turns/${encodeSegment(turnId)}/events`, {
      query: resume > 0 ? { after: resume } : undefined,
      headers,
      signal,
    });
    if (!response.body || typeof response.body.getReader !== 'function') {
      throw new ChatProtocolError('The event stream has no body.');
    }
    const contentType = response.headers?.get?.('content-type') ?? '';
    if (contentType && !contentType.toLowerCase().includes('text/event-stream')) {
      let text = '';
      try {
        text = await response.text();
      } catch {
        text = '';
      }
      try {
        const parsed = JSON.parse(text);
        const code = parsed?.error?.code;
        if (typeof code === 'string') {
          if (code === 'resync_required') throw new ResyncRequiredError(response.status, parsed.error.message);
          throw new ChatApiError(response.status, code, parsed.error.message);
        }
      } catch (err) {
        if (err instanceof ChatApiError) throw err;
      }
      throw new ChatProtocolError(`Unexpected event stream content type ${JSON.stringify(contentType)}.`);
    }

    let lastSeq = resume;
    let terminal = false;

    for await (const frame of parseSSE(response.body, {
      signal,
      onComment: (comment) => onHeartbeat?.(comment),
    })) {
      let event;
      try {
        event = JSON.parse(frame.data);
      } catch {
        throw new ChatProtocolError('The event stream contained invalid JSON.');
      }
      if (!event || typeof event !== 'object' || Array.isArray(event)) {
        throw new ChatProtocolError('The event stream contained an invalid envelope.');
      }
      if (event.v !== '1') {
        throw new ChatProtocolError(
          `Unsupported event version ${JSON.stringify(event.v)}; this client implements version 1.`,
          'unsupported_version',
        );
      }
      if (!Number.isInteger(event.seq) || event.seq < 1) {
        throw new ChatProtocolError('The event stream contained an invalid sequence.');
      }
      if (typeof event.type !== 'string' || event.type === '') {
        throw new ChatProtocolError('The event stream contained an invalid event type.');
      }
      if (event.turnId && event.turnId !== turnId) {
        throw new ChatProtocolError('The event stream contained an event for another turn.');
      }
      if (event.seq > lastSeq) lastSeq = event.seq;

      await onEvent?.(event);

      if (isTerminalEventType(event.type)) {
        terminal = true;
        break;
      }
    }

    return { lastSeq, terminal };
  }
}
