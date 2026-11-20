// Types for controller.mjs (@pocketbase-mcp/chat-client/controller).

import type {
  ChatClient,
  ChatCompactResult,
  ChatConversation,
  ChatFeatures,
  ChatHunkOutcome,
  ChatLimits,
  ChatMessage,
  ChatModelChoice,
  ChatPart,
  ChatProposal,
  ChatSubmitResult,
  ChatTurnFailure,
  ChatUpload,
  ProposalOutcomeStatus,
} from './client.mjs';
import type { TurnView } from './reducer.mjs';

/** The identity fields that namespace local state. */
export interface ChatAccountKey {
  appUrl: string;
  mount: string;
  authCollectionId: string;
  userId: string;
}

export interface ChatReconnectOptions {
  /** First reconnect delay; doubles per attempt. Defaults to 500ms. */
  initialDelayMs?: number;
  /** Reconnect delay cap. Defaults to 10000ms. */
  maxDelayMs?: number;
  /** Reconnect attempts after a failed stream. Defaults to Infinity. */
  maxAttempts?: number;
}

export interface ChatControllerOptions {
  /** Any object with the ChatClient method shapes; usually a ChatClient. */
  client: ChatClient;
  /** Account namespace; omit only when accounts cannot switch. */
  storageKey?: ChatAccountKey | null;
  reconnect?: ChatReconnectOptions;
  /** Reconnect when no event or heartbeat arrives for this long. 45000ms. */
  idleTimeoutMs?: number;
  /**
   * Host editor adapter (plan §8). Required to approve an editor proposal:
   * decide() calls validatePatch, then applyPatch, then reports the outcome.
   */
  editor?: ChatEditorAdapter | null;
  /**
   * Conversation list sync. On by default; `false` turns it all off. The list
   * is always re-read after a finished turn.
   */
  sync?: ChatSyncOptions | false;
  /**
   * Reads the page context for each send (route, selection, editor revision).
   * Called once per send, only when the server reports `features.context`;
   * a `context` passed to send() wins.
   */
  context?: (() => Record<string, unknown> | null | undefined | Promise<Record<string, unknown> | null | undefined>) | null;
}

/** The host editor's current state, used to name the revision of a stale report. */
export interface ChatSyncOptions {
  /** Tell and hear other controllers for the same account (other elements, other tabs) over a BroadcastChannel. Default true. */
  channel?: boolean;
  /** Refresh when the page becomes visible or the window regains focus. Default true. */
  focus?: boolean;
  /** Refresh this often while the page is visible; 0 turns it off. Default 60000. */
  intervalMs?: number;
}

export interface ChatConversationMerge {
  conversations: ChatConversation[];
  /** `null`: keep the current cursor (rows from loadMore were kept). */
  nextCursor: string | null;
  /** Whether the page's range included the row with this id. */
  covers(id: string): boolean;
  complete: boolean;
}

export interface ChatEditorSnapshot {
  editorId?: string;
  target?: string;
  revision?: string | number;
  content?: string;
}

/** What applyPatch() did: the new revision and, optionally, per-hunk results. */
export interface ChatEditorApplyResult {
  revision: string | number;
  hunks?: ChatHunkOutcome[];
  undoToken?: string;
}

/**
 * Host editor adapter. It validates and applies a draft patch locally; the
 * controller reports applied/stale/failed. Nothing here saves anything: saving
 * stays in the app's own protected API.
 */
export interface ChatEditorAdapter {
  snapshot(): ChatEditorSnapshot | Promise<ChatEditorSnapshot>;
  /**
   * Whether the patch still applies. `false` (or {applicable:false}) reports it
   * as stale; anything else proceeds to applyPatch.
   */
  validatePatch?(
    proposal: ChatProposal,
  ):
    | boolean
    | { applicable?: boolean; stale?: boolean }
    | void
    | Promise<boolean | { applicable?: boolean; stale?: boolean } | void>;
  /** Apply the draft patch locally and return the resulting revision. */
  applyPatch(
    proposal: ChatProposal,
  ): ChatEditorApplyResult | string | void | Promise<ChatEditorApplyResult | string | void>;
  undo?(token: string): unknown;
}

/** One proposal in controller state, plus a non-fatal client-side error. */
export interface ChatProposalEntry extends ChatProposal {
  /** A stale/conflict message, or a local refusal; never fatal to the view. */
  error: ChatTurnFailure | null;
}

/** One transcript entry: a single part of one canonical message. */
export interface ChatTranscriptEntry extends ChatPart {
  /** `${messageId}:${partIndex}`, stable until the transcript reloads. */
  id: string;
  messageId: string;
  role: string;
  turnId: string;
  createdAt: string;
}

/** The immutable controller snapshot. */
export interface ChatControllerState {
  status: 'idle' | 'loading' | 'ready' | 'error';
  conversations: ChatConversation[];
  nextCursor: string;
  activeConversationId: string | null;
  messages: ChatMessage[];
  /** One entry per message part; notices and attachments are ordinary entries. */
  transcript: ChatTranscriptEntry[];
  /**
   * The conversation's proposals: the server's open set merged with live
   * events. Terminal entries stay visible until the next open.
   */
  proposals: ChatProposalEntry[];
  /** Uploaded attachments not yet sent, cleared on send/account switch. */
  pendingUploads: ChatUpload[];
  /** Live view of the current (or last) turn. */
  turn: TurnView | null;
  running: boolean;
  error: ChatTurnFailure | null;
  /** /status features: proposals and uploads. */
  features: ChatFeatures | null;
  /** /status limits, including attachment types, size and count. */
  limits: ChatLimits | null;
  /** Legacy flat model list; still populated from /status. */
  models: string[];
  /** Catalog choices when the server has one. */
  choices: ChatModelChoice[];
  /** The server's default catalog choice id, or ''. */
  defaultChoice: string;
  /** The picker's current choice id, or ''. */
  choice: string;
  /**
   * How full the open conversation's context is, or null when the server
   * doesn't say. Updated on open, after each turn and by compact().
   */
  context: ChatControllerContext | null;
}

export interface ChatControllerContext {
  tokens: number;
  /** 0 means unknown. */
  window: number;
  estimated: boolean;
  /** The last message the summary covers, or ''. */
  summarizedThrough: string;
  summarizedAt: string;
  compacting: boolean;
  canCompact: boolean;
}

export interface ChatDecideOptions {
  /** Edited arguments for a pending proposal, when the server allows them. */
  arguments?: unknown;
  signal?: AbortSignal;
}

/** The result of decide()/reportOutcome(); a conflict is not thrown. */
export interface ChatDecisionResult {
  proposal: ChatProposalEntry | null;
  replayed: boolean;
  /** A non-fatal conflict message, already recorded on the proposal. */
  error: ChatTurnFailure | null;
}

export interface ChatReportOutcomeOptions {
  status: ProposalOutcomeStatus;
  editorRevision?: string;
  hunks?: ChatHunkOutcome[];
  message?: string;
  signal?: AbortSignal;
}

/** Flatten canonical messages into one transcript entry per part. */
/** Resolve a choice id or model id against /status choices. Pure. */
export declare function resolveChoice(
  choices: ChatModelChoice[] | null | undefined,
  requested?: string,
): ChatModelChoice | null;
/**
 * The attachment types a message to `choice` can carry: its own list, or
 * `limits.attachmentTypes` (the union the upload route accepts). Null when
 * nothing is known.
 */
export declare function attachmentTypesFor(
  limits: { attachmentTypes?: readonly string[] } | null | undefined,
  choice?: ChatModelChoice | null,
): string[] | null;

export declare function transcriptFromMessages(messages?: ChatMessage[]): ChatTranscriptEntry[];

/** Merge proposals by id, newest revision wins and no duplicates. */
export declare function mergeProposals(
  list: ChatProposalEntry[] | ChatProposal[],
  incoming: ChatProposal[],
): ChatProposalEntry[];

/** The MIME type a file name implies, or ''. */
export declare function inferAttachmentMime(name?: string): string;

/** Percent-encoded account identity, or null when no key is set. */
/** Merge a fresh first conversation page over the loaded list. */
export declare function mergeConversationPage(
  loaded: ChatConversation[] | null | undefined,
  page: { items?: ChatConversation[]; nextCursor?: string } | null | undefined,
): ChatConversationMerge;

export declare function accountKeyOf(storageKey?: ChatAccountKey | null): string | null;

/**
 * Framework-neutral controller. Emits "change" (no payload); consumers read
 * `state`.
 */
export declare class ChatController extends EventTarget {
  constructor(options: ChatControllerOptions);

  readonly state: ChatControllerState;

  /** Encoded account key, or null when no storageKey was supplied. */
  readonly accountKey: string | null;

  /** The editor host adapter, or null. */
  readonly editor: ChatEditorAdapter | null;

  /** Load status + the first conversation page. */
  init(): Promise<ChatControllerState>;

  /** Append the next conversation page. */
  loadMore(): Promise<ChatControllerState>;
  /** Re-read the first page and merge it in; clears the view if the open conversation was deleted elsewhere. */
  refreshConversations(): Promise<ChatControllerState>;
  /** Leave the open conversation; the next send() creates one. */
  startDraft(): ChatControllerState;

  /** Open a conversation, attaching to its active turn when one exists. */
  open(conversationId: string): Promise<ChatControllerState>;

  /** Create a conversation and make it active. */
  newConversation(title?: string): Promise<ChatConversation | null>;

  rename(conversationId: string, title: string): Promise<ChatConversation | null>;

  /** Re-read how full the open conversation's context is. */
  refreshContext(): Promise<ChatControllerContext | null>;

  /**
   * Summarize the open conversation's older messages now. Resolves to the
   * server's answer, or null when nothing is open or it failed (see `error`).
   */
  compact(): Promise<ChatCompactResult | null>;

  /** Delete a conversation; the active one is cleared. */
  remove(conversationId: string): Promise<boolean>;

  /**
   * Submit a message, then follow its turn. Network failures before a response
   * are retried with the same Idempotency-Key (three retries); the stream
   * continues in the background. Attachments may be upload ids or File/Blob
   * values, which are uploaded first.
   */
  send(
    text: string,
    options?: {
      /** A choice id or a model id; resolved against the catalog. */
      model?: string;
      /** Explicit provider id; normally derived from the catalog choice. */
      provider?: string;
      /**
       * Reasoning effort for the turn. Sent only when the resolved choice
       * lists it in `efforts`; otherwise dropped (model default).
       */
      effort?: string;
      attachments?: Array<string | Blob | File>;
      /** Page context for this send; overrides the controller's `context` option. */
      context?: Record<string, unknown> | null;
    },
  ): Promise<ChatSubmitResult | null>;

  /** Cancel on the server, then drop the local stream. */
  stop(): Promise<ChatControllerState>;

  /**
   * Approve or reject one proposal. An editor approval runs the editor adapter
   * and reports its outcome; a 409 conflict is adopted, not thrown.
   */
  decide(
    proposalId: string,
    decision: 'approve' | 'reject',
    options?: ChatDecideOptions,
  ): Promise<ChatDecisionResult | null>;

  /** Report the browser editor's outcome for an approved editor proposal. */
  reportOutcome(
    proposalId: string,
    report: ChatReportOutcomeOptions,
  ): Promise<ChatDecisionResult | null>;

  /**
   * Validate and upload files, creating a conversation when none is open.
   * Resolves with the stored uploads, also kept in state.pendingUploads.
   */
  /**
   * Upload files. `model` (a choice id or model id) checks the types against
   * that model; without it the check is the union `limits.attachmentTypes`.
   */
  attach(
    files: Iterable<Blob | File>,
    options?: { signal?: AbortSignal; model?: string },
  ): Promise<ChatUpload[]>;

  /** Drop every attachment that has not been sent yet. */
  clearAttachments(): ChatControllerState;

  /** Switch account: abort, clear state and re-init. */
  setAccount(storageKey: ChatAccountKey | null): Promise<ChatControllerState>;

  /** Resolves when no request, stream, reconnect or poll is in flight. */
  whenSettled(): Promise<void>;

  /** Abort every stream and request; the controller stops updating. */
  dispose(): void;
}
