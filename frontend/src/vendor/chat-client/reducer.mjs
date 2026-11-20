// Immutable port of chat/reducer.go (TurnView).
//
// The Go reducer folds a turn's event stream into the exact state a browser
// client needs. This module keeps the same rules but returns a new view on
// every accepted event, so a UI can diff snapshots:
//
//   - an event at or below `lastSeq` is a replay duplicate and returns the
//     same view object;
//   - a gap (seq greater than lastSeq + 1) throws GapError;
//   - an event for another turn throws ForeignTurnError;
//   - an event after a terminal state throws AfterTerminalError.
//
// Nothing here touches the DOM or the network.

/** Turn lifecycle states, as stored by the server. */
export const TURN_STATES = Object.freeze({
  QUEUED: 'queued',
  RUNNING: 'running',
  COMPLETED: 'completed',
  FAILED: 'failed',
  CANCELLED: 'cancelled',
  INTERRUPTED: 'interrupted',
});

/** States that end a turn. Every turn has exactly one of them. */
export const TERMINAL_STATES = Object.freeze([
  TURN_STATES.COMPLETED,
  TURN_STATES.FAILED,
  TURN_STATES.CANCELLED,
  TURN_STATES.INTERRUPTED,
]);

/** Event types that end a turn's stream. */
export const TERMINAL_EVENT_TYPES = Object.freeze([
  'turn.completed',
  'turn.failed',
  'turn.cancelled',
]);

/** Tool statuses in a TurnView. */
export const TOOL_STATUSES = Object.freeze({
  RUNNING: 'running',
  COMPLETED: 'completed',
  FAILED: 'failed',
});

/** Proposal lifecycle states, as stored by the server. */
export const PROPOSAL_STATES = Object.freeze({
  PENDING: 'pending',
  REJECTED: 'rejected',
  EXPIRED: 'expired',
  EXECUTING: 'executing',
  SUCCEEDED: 'succeeded',
  FAILED: 'failed',
  UNKNOWN: 'unknown',
  APPROVED: 'approved',
  APPLIED: 'applied',
  STALE: 'stale',
});

/** Proposal locations. */
export const PROPOSAL_LOCATIONS = Object.freeze({
  SERVER: 'server',
  EDITOR: 'editor',
});

const PROPOSAL_STATE_VALUES = Object.freeze(Object.values(PROPOSAL_STATES));
const TERMINAL_PROPOSAL_STATES = Object.freeze([
  PROPOSAL_STATES.REJECTED,
  PROPOSAL_STATES.EXPIRED,
  PROPOSAL_STATES.SUCCEEDED,
  PROPOSAL_STATES.FAILED,
  PROPOSAL_STATES.UNKNOWN,
  PROPOSAL_STATES.APPLIED,
  PROPOSAL_STATES.STALE,
]);
const HUNK_STATUSES = Object.freeze(['applied', 'stale', 'failed', 'skipped']);

export function isTerminalProposalState(state) {
  return TERMINAL_PROPOSAL_STATES.includes(state);
}

export function isTerminalState(state) {
  return TERMINAL_STATES.includes(state);
}

export function isTerminalEventType(type) {
  return TERMINAL_EVENT_TYPES.includes(type);
}

/** Base class for reducer errors; every one carries a stable `code`. */
export class ReducerError extends Error {
  constructor(message, code = 'reducer_error') {
    super(message);
    this.name = 'ReducerError';
    this.code = code;
  }
}

/** The event sequence skipped one or more numbers; reload canonical state. */
export class GapError extends ReducerError {
  constructor(turnId, have, got) {
    super(`chat: event sequence gap for turn ${JSON.stringify(turnId)}: have ${have}, got ${got}`, 'sequence_gap');
    this.name = 'GapError';
    this.turnId = turnId;
    this.have = have;
    this.got = got;
  }
}

/** The event belongs to a different turn than the view. */
export class ForeignTurnError extends ReducerError {
  constructor(eventTurnId, viewTurnId) {
    super(`chat: event for turn ${JSON.stringify(eventTurnId)} applied to turn ${JSON.stringify(viewTurnId)}`, 'foreign_turn');
    this.name = 'ForeignTurnError';
    this.eventTurnId = eventTurnId;
    this.viewTurnId = viewTurnId;
  }
}

/** The turn already ended, so another event cannot be part of it. */
export class AfterTerminalError extends ReducerError {
  constructor(eventType, state) {
    super(`chat: event ${String(eventType)} after terminal state ${String(state)}`, 'after_terminal');
    this.name = 'AfterTerminalError';
    this.eventType = eventType;
    this.state = state;
  }
}

/** Malformed or unknown event data. Never ignored: it means a broken stream. */
export class InvalidEventError extends ReducerError {
  constructor(message) {
    super(message, 'invalid_event');
    this.name = 'InvalidEventError';
  }
}

/** A fresh view for one turn, matching a zero Go TurnView. */
export function createTurnView(turnId = '') {
  return {
    turnId: String(turnId ?? ''),
    state: '',
    text: '',
    tools: [],
    usage: null,
    error: null,
    proposals: [],
    lastSeq: 0,
  };
}

function decodeData(event) {
  const raw = event.data;
  if (raw === undefined || raw === null) {
    throw new InvalidEventError(`chat: ${event.type} event without data`);
  }
  let value = raw;
  if (typeof value === 'string') {
    if (value.trim() === '') {
      throw new InvalidEventError(`chat: ${event.type} event without data`);
    }
    try {
      value = JSON.parse(value);
    } catch {
      throw new InvalidEventError(`chat: decode ${event.type} data: invalid JSON`);
    }
  }
  if (typeof value !== 'object' || Array.isArray(value)) {
    throw new InvalidEventError(`chat: decode ${event.type} data: not an object`);
  }
  return value;
}

function stringField(data, key, type) {
  const value = data[key];
  if (value === undefined || value === null) return '';
  if (typeof value !== 'string') {
    throw new InvalidEventError(`chat: decode ${type} data: ${key} is not a string`);
  }
  return value;
}

function numberField(data, key, type) {
  const value = data[key];
  if (value === undefined || value === null) return 0;
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new InvalidEventError(`chat: decode ${type} data: ${key} is not a number`);
  }
  return value;
}

function cloneValue(value) {
  if (value === undefined) return null;
  if (value === null || typeof value !== 'object') return value;
  if (typeof structuredClone === 'function') {
    try {
      return structuredClone(value);
    } catch {
      // Fall through to the JSON copy.
    }
  }
  try {
    return JSON.parse(JSON.stringify(value));
  } catch {
    return value;
  }
}

function findTool(tools, callId) {
  return tools.find((t) => t.callId === callId);
}

function putTool(tools, tool) {
  const index = tools.findIndex((t) => t.callId === tool.callId);
  if (index === -1) return [...tools, tool];
  const next = tools.slice();
  next[index] = tool;
  return next;
}

// --- proposals ---

/**
 * Validate one proposal payload exactly like chat.Proposal.Validate: an id, a
 * conversation, a known location and state, a tool, a revision of at least one
 * and JSON object arguments. The reducer refuses anything a store could never
 * have written, because such an event means a broken stream.
 */
export function validateProposal(raw, type = 'proposal') {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    throw new InvalidEventError(`chat: decode ${type} data: proposal is not an object`);
  }
  const fail = (reason) => {
    throw new InvalidEventError(`chat: ${type} data: ${reason}`);
  };
  if (typeof raw.id !== 'string' || raw.id === '') fail('proposal has no id');
  if (typeof raw.conversationId !== 'string' || raw.conversationId === '') {
    fail(`proposal ${JSON.stringify(raw.id)} has no conversation`);
  }
  if (raw.location !== PROPOSAL_LOCATIONS.SERVER && raw.location !== PROPOSAL_LOCATIONS.EDITOR) {
    fail(`proposal ${JSON.stringify(raw.id)} has invalid location ${JSON.stringify(raw.location)}`);
  }
  if (typeof raw.tool !== 'string' || raw.tool === '') {
    fail(`proposal ${JSON.stringify(raw.id)} has no tool`);
  }
  if (!PROPOSAL_STATE_VALUES.includes(raw.state)) {
    fail(`proposal ${JSON.stringify(raw.id)} has invalid state ${JSON.stringify(raw.state)}`);
  }
  if (!Number.isInteger(raw.revision) || raw.revision < 1) {
    fail(`proposal ${JSON.stringify(raw.id)} has revision ${String(raw.revision)}, want at least 1`);
  }
  let args = raw.arguments;
  if (typeof args === 'string') {
    try {
      args = JSON.parse(args);
    } catch {
      fail(`proposal ${JSON.stringify(raw.id)} has invalid arguments`);
    }
  }
  if (!args || typeof args !== 'object' || Array.isArray(args)) {
    fail(`proposal ${JSON.stringify(raw.id)} arguments are not a JSON object`);
  }
  if (raw.hunks !== undefined && raw.hunks !== null) {
    if (!Array.isArray(raw.hunks)) fail(`proposal ${JSON.stringify(raw.id)} has invalid hunks`);
    for (const hunk of raw.hunks) {
      if (!hunk || typeof hunk !== 'object' || typeof hunk.id !== 'string'
        || !HUNK_STATUSES.includes(hunk.status)) {
        fail(`proposal ${JSON.stringify(raw.id)} has an invalid hunk`);
      }
    }
  }
  return raw;
}

/**
 * Insert or replace one proposal by id, mirroring the Go reducer:
 *
 *   - a newer-or-equal revision replaces the stored entry;
 *   - an older revision that arrives out of order cannot undo a newer state;
 *   - a replayed frame with the same revision is harmless;
 *   - no frame ever duplicates an entry.
 */
export function putProposal(proposals, proposal) {
  const list = Array.isArray(proposals) ? proposals : [];
  const index = list.findIndex((p) => p.id === proposal.id);
  if (index === -1) return [...list, proposal];
  if (proposal.revision < list[index].revision) return list;
  const next = list.slice();
  next[index] = proposal;
  return next;
}

/**
 * Fold one envelope into a view.
 *
 * @param {object} view A TurnView, normally from createTurnView().
 * @param {object} event An event envelope (`seq`, `type`, `data`, ...). `data`
 *   may be the parsed payload or the JSON string from an SSE frame.
 * @returns {object} A new view, or `view` itself for a replay duplicate.
 */
export function applyEvent(view, event) {
  if (!view || typeof view !== 'object') {
    throw new TypeError('applyEvent() requires a TurnView');
  }
  if (!event || typeof event !== 'object') {
    throw new TypeError('applyEvent() requires an event envelope');
  }
  const seq = event.seq;
  if (!Number.isInteger(seq) || seq < 1) {
    throw new InvalidEventError(`chat: event sequence ${String(seq)} is not a positive integer`);
  }
  if (view.turnId && event.turnId && event.turnId !== view.turnId) {
    throw new ForeignTurnError(event.turnId, view.turnId);
  }
  if (seq <= view.lastSeq) return view;
  if (seq !== view.lastSeq + 1) {
    throw new GapError(view.turnId || event.turnId || '', view.lastSeq, seq);
  }
  if (isTerminalState(view.state)) {
    throw new AfterTerminalError(event.type, view.state);
  }

  const next = {
    ...view,
    turnId: view.turnId || event.turnId || '',
    lastSeq: seq,
  };

  switch (event.type) {
    case 'turn.queued':
      next.state = TURN_STATES.QUEUED;
      break;

    case 'turn.started':
      next.state = TURN_STATES.RUNNING;
      break;

    case 'text.delta': {
      const data = decodeData(event);
      next.text = view.text + stringField(data, 'text', event.type);
      break;
    }

    case 'tool.started': {
      const data = decodeData(event);
      const tool = {
        callId: stringField(data, 'callId', event.type),
        tool: stringField(data, 'tool', event.type),
        status: TOOL_STATUSES.RUNNING,
        result: null,
        message: '',
      };
      next.tools = putTool(view.tools, tool);
      break;
    }

    case 'tool.completed': {
      const data = decodeData(event);
      const existing = findTool(view.tools, stringField(data, 'callId', event.type))
        ?? {
          callId: stringField(data, 'callId', event.type),
          tool: '',
          status: TOOL_STATUSES.RUNNING,
          result: null,
          message: '',
        };
      const name = stringField(data, 'tool', event.type);
      const tool = {
        ...existing,
        callId: existing.callId,
        tool: name || existing.tool,
        status: TOOL_STATUSES.COMPLETED,
        result: cloneValue(data.result),
        message: '',
      };
      next.tools = putTool(view.tools, tool);
      break;
    }

    case 'tool.failed': {
      const data = decodeData(event);
      const callId = stringField(data, 'callId', event.type);
      const existing = findTool(view.tools, callId)
        ?? {
          callId,
          tool: '',
          status: TOOL_STATUSES.RUNNING,
          result: null,
          message: '',
        };
      const name = stringField(data, 'tool', event.type);
      const tool = {
        ...existing,
        callId,
        tool: name || existing.tool,
        status: TOOL_STATUSES.FAILED,
        message: stringField(data, 'message', event.type),
      };
      next.tools = putTool(view.tools, tool);
      break;
    }

    case 'usage.updated': {
      const data = decodeData(event);
      next.usage = {
        inputTokens: numberField(data, 'inputTokens', event.type),
        outputTokens: numberField(data, 'outputTokens', event.type),
      };
      break;
    }

    case 'turn.completed':
      next.state = TURN_STATES.COMPLETED;
      break;

    case 'turn.failed': {
      const data = decodeData(event);
      const failure = {
        code: stringField(data, 'code', event.type),
        message: stringField(data, 'message', event.type),
      };
      next.error = failure;
      // Restart recovery ends a turn as interrupted but still emits
      // turn.failed, with the "interrupted" code, so the reducer restores the
      // stored state from the data (same rule as chat/reducer.go).
      next.state = failure.code === 'interrupted'
        ? TURN_STATES.INTERRUPTED
        : TURN_STATES.FAILED;
      break;
    }

    case 'turn.cancelled':
      next.state = TURN_STATES.CANCELLED;
      break;

    case 'proposal.created':
    case 'proposal.updated': {
      const data = decodeData(event);
      const proposal = validateProposal(data.proposal, event.type);
      next.proposals = putProposal(view.proposals ?? [], proposal);
      break;
    }

    default:
      throw new InvalidEventError(`chat: unknown event type ${JSON.stringify(String(event.type))}`);
  }

  return next;
}

/**
 * Rebuild a view from canonical transcript records, for a client that attached
 * to a turn without replaying its events (after a resync, or on open).
 *
 * The server has no "text so far" field outside the event stream, so the
 * assistant text and tool outcomes are read from the turn's messages. The
 * sequence is left at 0: after a resync the client polls the turn instead of
 * pretending it can replay a cursor.
 */
export function viewFromTranscript(turn, messages = []) {
  const turnId = String(turn?.id ?? '');
  const view = {
    ...createTurnView(turnId),
    state: String(turn?.state ?? ''),
    error: turn?.error ?? null,
  };
  if (!Array.isArray(messages)) return view;

  let text = '';
  let tools = [];

  for (const message of messages) {
    if (!message || message.turnId !== turnId) continue;
    for (const part of message.parts ?? []) {
      if (!part) continue;
      switch (part.type) {
        case 'text':
          // The user message is already part of the transcript; the live view
          // only reduces assistant output.
          if (message.role !== 'user') text += String(part.text ?? '');
          break;
        case 'tool_call':
          tools = putTool(tools, {
            callId: String(part.callId ?? ''),
            tool: String(part.tool ?? ''),
            status: TOOL_STATUSES.RUNNING,
            result: null,
            message: '',
          });
          break;
        case 'tool_result': {
          const callId = String(part.callId ?? '');
          const existing = findTool(tools, callId)
            ?? {
              callId,
              tool: String(part.tool ?? ''),
              status: TOOL_STATUSES.RUNNING,
              result: null,
              message: '',
            };
          tools = putTool(tools, {
            ...existing,
            status: part.isError ? TOOL_STATUSES.FAILED : TOOL_STATUSES.COMPLETED,
            result: cloneValue(part.result),
            message: '',
          });
          break;
        }
        default:
          break;
      }
    }
  }

  view.text = text;
  view.tools = tools;
  return view;
}
