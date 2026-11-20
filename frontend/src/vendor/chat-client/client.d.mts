// Types for client.mjs (@pocketbase-mcp/chat-client).

/** Protocol version this client implements. */
export declare const PROTOCOL_VERSION: '1';

/** Turn lifecycle state; `''` means "no event seen yet" in a client view. */
export type TurnState =
  | ''
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'interrupted';

export type MessageRole = 'user' | 'assistant' | 'tool';

/** One piece of message content. */
export interface ChatPart {
  type: string;
  text?: string;
  callId?: string;
  tool?: string;
  arguments?: unknown;
  result?: unknown;
  isError?: boolean;
  /** proposal_outcome: the proposal the notice is about. */
  proposalId?: string;
  /** proposal_outcome: the recorded proposal state. */
  state?: string;
  /** attachment: the stored upload it references. */
  uploadId?: string;
  /** attachment: the stored file name. */
  name?: string;
  /** attachment: the stored MIME type. */
  mime?: string;
}

/** One canonical transcript entry. */
export interface ChatMessage {
  id: string;
  conversationId: string;
  turnId?: string;
  role: MessageRole | string;
  parts: ChatPart[];
  createdAt: string;
}

/** One caller-owned conversation. */
export interface ChatConversation {
  id: string;
  title: string;
  target?: string;
  createdAt: string;
  updatedAt: string;
}

/** The recorded failure of a turn. */
export interface ChatTurnFailure {
  code: string;
  message: string;
}

/** One unit of work in a conversation. */
export interface ChatTurn {
  id: string;
  conversationId: string;
  state: TurnState;
  idempotencyKey?: string;
  /** Catalog provider id, absent for the legacy single-provider path. */
  provider?: string;
  model?: string;
  /** Reasoning effort the turn asked for; absent means the model default. */
  effort?: string;
  createdAt: string;
  updatedAt: string;
  error?: ChatTurnFailure | null;
}

/** The envelope carried by every event and every SSE frame's data. */
export interface ChatEvent {
  v: string;
  seq: number;
  conversationId: string;
  turnId: string;
  type: string;
  data?: unknown;
  at: string;
}

export interface ChatFeatures {
  proposals: boolean;
  uploads: boolean;
  /** Turns accept a page context. */
  context?: boolean;
  /** The default model's context window is known: `getContext` reports how full it is. */
  contextManagement?: boolean;
  /** `compactConversation` can summarize a conversation. */
  compaction?: boolean;
}

/** How full a conversation's context is. */
export interface ChatContextStatus {
  /** Input tokens of the latest request, or an estimate of the next one. */
  tokens: number;
  /** The model's input window; 0 means unknown (show no indicator). */
  window: number;
  estimated: boolean;
  /** The last message the conversation's summary covers. */
  summarizedThrough?: string;
  summarizedAt?: string;
  compacting: boolean;
  canCompact: boolean;
}

export interface ChatCompactResult {
  compacted: boolean;
  /** `nothing_to_compact` when there was too little to summarize. */
  reason?: string;
  throughMessageId?: string;
  context: ChatContextStatus;
}

export interface ChatLimits {
  maxMessageChars: number;
  /** Present only when the server serves uploads. */
  maxUploadBytes?: number;
  /** Present only when the server serves uploads. */
  maxAttachmentsPerTurn?: number;
  /** The attachment MIME types this server accepts, in display order. */
  attachmentTypes?: string[];
}

/** Proposal lifecycle states, as stored by the server. */
export type ChatProposalState =
  | 'pending'
  | 'rejected'
  | 'expired'
  | 'executing'
  | 'succeeded'
  | 'failed'
  | 'unknown'
  | 'approved'
  | 'applied'
  | 'stale';

/** Where a proposal's effect happens. */
export type ChatProposalLocation = 'server' | 'editor';

/** One hunk's reported local editor result. */
export interface ChatHunkOutcome {
  id: string;
  status: 'applied' | 'stale' | 'failed' | 'skipped';
}

/**
 * One reviewed model mutation. `revision` is the optimistic-concurrency token a
 * decision and an outcome report address.
 */
export interface ChatProposal {
  id: string;
  conversationId: string;
  turnId?: string;
  callId?: string;
  tool: string;
  toolFingerprint?: string;
  manifestRevision?: string;
  location: ChatProposalLocation;
  target?: string;
  arguments: unknown;
  argumentsHash: string;
  summary?: string;
  state: ChatProposalState;
  revision: number;
  createdAt: string;
  updatedAt: string;
  expiresAt?: string;
  decidedAt?: string | null;
  result?: unknown;
  failure?: string;
  /** The host's notes about the change (ToolSpec.Warn); never blocking. */
  warnings?: string[];
  editorRevision?: string;
  hunks?: ChatHunkOutcome[];
}

/** One stored chat attachment. The bytes are served only by the download route. */
export interface ChatUpload {
  id: string;
  conversationId: string;
  name: string;
  mime: string;
  /** The server's classification: text, image or pdf today. */
  kind: string;
  size: number;
  sha256: string;
  createdAt: string;
}

/** One selectable provider/model pair. id is opaque to clients. */
export interface ChatModelChoice {
  id: string;
  provider: string;
  model: string;
  label?: string;
  group?: string;
  default?: boolean;
  /** Canonical MIME types this choice can send. */
  attachmentTypes?: string[];
  /** Reasoning efforts a turn may request, lowest first. Absent means none. */
  efforts?: string[];
}

export interface ChatStatus {
  version: string;
  models: string[];
  /** Present when the server has a provider catalog. */
  choices?: ChatModelChoice[];
  defaultChoice?: string;
  features: ChatFeatures;
  limits: ChatLimits;
}

export interface ChatConversationList {
  items: ChatConversation[];
  nextCursor: string;
}

export interface ChatConversationDetail {
  conversation: ChatConversation;
  messages: ChatMessage[];
  nextCursor: string;
  activeTurn: ChatTurn | null;
  /**
   * Pending, approved and executing proposals, oldest first. Present only when
   * the server serves proposals; absent means the feature is off.
   */
  proposals?: ChatProposal[];
  /** Present when the server manages the context window or summaries. */
  context?: ChatContextStatus;
}

export interface ChatSubmitResult {
  turn: ChatTurn;
  created: boolean;
}

export interface RequestOptions {
  signal?: AbortSignal;
}

export interface ListConversationsOptions extends RequestOptions {
  limit?: number;
  cursor?: string;
  target?: string;
}

export interface CreateConversationOptions extends RequestOptions {
  title?: string;
  target?: string;
}

export interface GetConversationOptions extends RequestOptions {
  limit?: number;
  cursor?: string;
}

export interface SubmitTurnOptions extends RequestOptions {
  text: string;
  /** Catalog provider id. Omit to use the default provider. */
  provider?: string;
  model?: string;
  /** Reasoning effort, one of the choice's `efforts`. Omit for the model default. */
  effort?: string;
  /** Upload ids returned by uploadFile(). */
  attachments?: string[];
  /**
   * Page context for this turn (route, selection, editor revision...). Only
   * accepted when the server's status reports `features.context`.
   */
  context?: Record<string, unknown> | null;
  idempotencyKey?: string;
}

export interface DecideProposalOptions extends RequestOptions {
  decision: 'approve' | 'reject';
  /** The proposal revision the user reviewed. */
  revision: number;
  /** Edited arguments, only when the server allows argument edits. */
  arguments?: unknown;
}

export interface ProposalDecisionResult {
  /** The authoritative stored proposal. */
  proposal: ChatProposal;
  /** True when the decision had already been applied. */
  replayed: boolean;
}

export type ProposalOutcomeStatus = 'applied' | 'stale' | 'failed';

export interface ReportOutcomeOptions extends RequestOptions {
  /** The approved proposal's revision. */
  revision: number;
  status: ProposalOutcomeStatus;
  /** Required by the server for an `applied` report. */
  editorRevision?: string;
  hunks?: ChatHunkOutcome[];
  message?: string;
}

export interface StreamTurnOptions extends RequestOptions {
  /** Resume after this sequence. Sent as `?after=` and Last-Event-ID. */
  after?: number;
  onEvent?: (event: ChatEvent) => void | Promise<void>;
  /** Called for every SSE comment line, including `: ping` heartbeats. */
  onHeartbeat?: (comment: string) => void;
}

export interface StreamTurnResult {
  /** Highest sequence number seen (the resume point). */
  lastSeq: number;
  /** Whether a terminal event arrived. */
  terminal: boolean;
}

export interface ChatClientOptions {
  /** Origin plus optional mount path, for example "https://example.com". */
  baseUrl?: string;
  /** API root; defaults to "/api/mcp/chat/v1". */
  prefix?: string;
  /** Called before every request; its result goes in Authorization only. */
  getToken: () => string | Promise<string>;
  /** Override for tests or non-global fetch implementations. */
  fetch?: typeof fetch;
}

/** An error response decoded from {"error":{"code","message"}}. */
export declare class ChatApiError extends Error {
  status: number;
  code: string;
  /**
   * The stored proposal a 409 decision or outcome conflict carried, so a caller
   * can adopt the authoritative state immediately. Null otherwise.
   */
  proposal: ChatProposal | null;
  constructor(status: number, code: string, message: string, proposal?: ChatProposal | null);
}

/** The event cursor is gone; reload canonical state. */
export declare class ResyncRequiredError extends ChatApiError {
  constructor(status?: number, message?: string);
}

/** The request never reached a usable HTTP response. */
export declare class ChatNetworkError extends Error {
  constructor(message?: string, cause?: unknown);
}

/** The server answered outside the protocol this client implements. */
export declare class ChatProtocolError extends Error {
  code: string;
  constructor(message: string, code?: string);
}

/** A random, RFC 4122-shaped idempotency key. */
export declare function newIdempotencyKey(): string;

/**
 * Transport client for one chat API mount.
 *
 * `getToken` runs before every request, so a rotated or logged-out session is
 * picked up immediately. The token is never placed in a URL.
 */
export declare class ChatClient {
  constructor(options: ChatClientOptions);
  readonly baseUrl: string;
  readonly prefix: string;
  readonly getToken: ChatClientOptions['getToken'];
  readonly fetchImpl: typeof fetch;

  status(options?: RequestOptions): Promise<ChatStatus>;

  listConversations(options?: ListConversationsOptions): Promise<ChatConversationList>;

  createConversation(options?: CreateConversationOptions): Promise<ChatConversation>;

  getConversation(id: string, options?: GetConversationOptions): Promise<ChatConversationDetail>;

  renameConversation(id: string, title: string, options?: RequestOptions): Promise<ChatConversation>;

  deleteConversation(id: string, options?: RequestOptions): Promise<null>;

  getContext(id: string, options?: RequestOptions): Promise<ChatContextStatus>;

  /** Import a browser-kept thread (server option `Imports`). */
  importConversation(thread: {
    sourceId: string;
    title?: string;
    createdAt?: string;
    messages: Array<{ role: 'user' | 'assistant'; text: string; createdAt?: string }>;
  } & RequestOptions): Promise<{ conversation: ChatConversation; created: boolean }>;

  compactConversation(id: string, options?: RequestOptions): Promise<ChatCompactResult>;

  /** Generates an Idempotency-Key when none is supplied. */
  submitTurn(conversationId: string, options: SubmitTurnOptions): Promise<ChatSubmitResult>;

  getTurn(id: string, options?: RequestOptions): Promise<ChatTurn>;

  cancelTurn(id: string, options?: RequestOptions): Promise<ChatTurn>;

  /** GET /proposals/{id} */
  getProposal(id: string, options?: RequestOptions): Promise<ChatProposal>;

  /**
   * Approve or reject one proposal at the revision the user reviewed. A 409
   * rejects with ChatApiError carrying the stored `proposal`.
   */
  decideProposal(id: string, options: DecideProposalOptions): Promise<ProposalDecisionResult>;

  /**
   * Report what the browser editor did with an approved editor proposal.
   * The report is an acknowledgement, never proof of a save.
   */
  reportOutcome(id: string, options: ReportOutcomeOptions): Promise<ProposalDecisionResult>;

  /**
   * Upload one attachment as multipart/form-data ("conversationId", "file").
   * `name` defaults to the File's name.
   */
  uploadFile(
    conversationId: string,
    file: Blob | File,
    name?: string,
    options?: RequestOptions,
  ): Promise<ChatUpload>;

  getUploadMeta(id: string, options?: RequestOptions): Promise<ChatUpload>;

  /** Download one attachment as a Blob with the token in the header only. */
  downloadUpload(id: string, options?: RequestOptions): Promise<Blob>;

  /** Follow one turn; resolves on end or terminal, rejects on HTTP errors. */
  streamTurn(turnId: string, options?: StreamTurnOptions): Promise<StreamTurnResult>;
}
