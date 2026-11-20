// Types for reducer.mjs (@pocketbase-mcp/chat-client/reducer).

import type { ChatEvent, ChatMessage, ChatProposal, ChatTurn, ChatTurnFailure } from './client.mjs';

/** One tool call as reduced from the event stream. */
export interface ToolView {
  callId: string;
  tool: string;
  status: 'running' | 'completed' | 'failed';
  /** Parsed tool result, or null while running/absent. */
  result: unknown;
  /** Failure message for tool.failed; empty otherwise. */
  message: string;
}

export interface TurnUsage {
  inputTokens: number;
  outputTokens: number;
}

/** The reducer's view of one turn. */
export interface TurnView {
  turnId: string;
  state: string;
  text: string;
  tools: ToolView[];
  usage: TurnUsage | null;
  error: ChatTurnFailure | null;
  /** One entry per proposal id, in first-seen order. */
  proposals: ChatProposal[];
  lastSeq: number;
}

export declare const TURN_STATES: Readonly<{
  QUEUED: 'queued';
  RUNNING: 'running';
  COMPLETED: 'completed';
  FAILED: 'failed';
  CANCELLED: 'cancelled';
  INTERRUPTED: 'interrupted';
}>;

export declare const TERMINAL_STATES: ReadonlyArray<
  'completed' | 'failed' | 'cancelled' | 'interrupted'
>;

export declare const TERMINAL_EVENT_TYPES: ReadonlyArray<
  'turn.completed' | 'turn.failed' | 'turn.cancelled'
>;

export declare const TOOL_STATUSES: Readonly<{
  RUNNING: 'running';
  COMPLETED: 'completed';
  FAILED: 'failed';
}>;

export declare const PROPOSAL_STATES: Readonly<{
  PENDING: 'pending';
  REJECTED: 'rejected';
  EXPIRED: 'expired';
  EXECUTING: 'executing';
  SUCCEEDED: 'succeeded';
  FAILED: 'failed';
  UNKNOWN: 'unknown';
  APPROVED: 'approved';
  APPLIED: 'applied';
  STALE: 'stale';
}>;

export declare const PROPOSAL_LOCATIONS: Readonly<{
  SERVER: 'server';
  EDITOR: 'editor';
}>;

export declare function isTerminalState(state: string): boolean;

export declare function isTerminalEventType(type: string): boolean;

/** Whether a proposal state ends its life. */
export declare function isTerminalProposalState(state: string): boolean;

/** Validate one proposal payload like chat.Proposal.Validate; throws InvalidEventError. */
export declare function validateProposal(raw: unknown, type?: string): ChatProposal;

/** Insert or replace one proposal by id; an older revision never wins. */
export declare function putProposal(proposals: ChatProposal[], proposal: ChatProposal): ChatProposal[];

export declare class ReducerError extends Error {
  code: string;
  constructor(message: string, code?: string);
}

/** The sequence skipped one or more numbers; reload canonical state. */
export declare class GapError extends ReducerError {
  turnId: string;
  have: number;
  got: number;
  constructor(turnId: string, have: number, got: number);
}

/** The event belongs to a different turn. */
export declare class ForeignTurnError extends ReducerError {
  eventTurnId: string;
  viewTurnId: string;
  constructor(eventTurnId: string, viewTurnId: string);
}

/** The turn already ended. */
export declare class AfterTerminalError extends ReducerError {
  eventType: string;
  state: string;
  constructor(eventType: string, state: string);
}

/** Malformed or unknown event data. */
export declare class InvalidEventError extends ReducerError {
  constructor(message: string);
}

/** A fresh view for one turn. */
export declare function createTurnView(turnId?: string): TurnView;

/**
 * Fold one event into a view. Returns the same object for a replay duplicate
 * and a new object otherwise; throws a typed ReducerError subclass on a broken
 * stream.
 */
export declare function applyEvent(view: TurnView, event: ChatEvent): TurnView;

/** Rebuild a view from canonical transcript records (used after a resync). */
export declare function viewFromTranscript(
  turn: ChatTurn | null | undefined,
  messages?: ChatMessage[],
): TurnView;
