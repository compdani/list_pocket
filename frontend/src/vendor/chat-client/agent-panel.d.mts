// Types for agent-panel.mjs (@pocketbase-mcp/chat-client/agent-panel).
//
// The `agent-panel` package is not a dependency: the element is passed in, and
// the shapes below mirror its documented properties (agent-panel element.d.ts
// and page-element.d.ts) without importing it.

import type { ChatController, ChatControllerState, ChatProposalEntry } from './controller.mjs';
import type {
  ChatClient,
  ChatProposalLocation,
  ChatProposalState,
  ChatTurnFailure,
} from './client.mjs';

/** A chip shown above a user bubble (agent-panel `Reference`). */
export interface AgentReference {
  type: string;
  id: string | number;
  label?: string;
  [extra: string]: unknown;
}

/**
 * A chip for one stored attachment on a user message. Clicking it fires
 * `agent-reference-open`; the binding then downloads through the client.
 */
export interface AgentAttachmentReference extends AgentReference {
  type: 'attachment';
  id: string;
  /** Marker so a host can tell an attachment chip from an ordinary reference. */
  attachment: true;
  mime?: string;
}

/** agent-panel `TextEntry`. */
export interface AgentTextEntry {
  id: string;
  type: 'text';
  role: string;
  content: string;
  references?: AgentReference[];
}

/** agent-panel `ToolCall`. */
export interface AgentToolCall {
  id?: string;
  key?: string;
  name: string;
  label?: string;
  status: 'running' | 'completed' | 'failed';
  detail?: unknown;
}

/** agent-panel `ToolsEntry`. */
export interface AgentToolsEntry {
  id: string;
  type: 'tools';
  label?: string;
  tools: AgentToolCall[];
}

/**
 * One proposal, drawn by the binding's `defineEntry('proposal', …)` renderer.
 * `status` is the sentence a person reads, `detail` a server failure message
 * and `error` a non-fatal client conflict; `canDecide` shows Approve/Reject.
 */
export interface AgentProposalEntry {
  id: string;
  type: 'proposal';
  proposalId: string;
  tool: string;
  summary: string;
  state: ChatProposalState;
  location: ChatProposalLocation;
  revision: number;
  error: string;
  status: string;
  detail: string;
  /** The host's notes about the change, shown under it. */
  warnings: string[];
  canDecide: boolean;
}

/** One framework-generated transcript note, drawn as a small system line. */
export interface AgentNoticeEntry {
  id: string;
  type: 'notice';
  content: string;
  proposalId?: string;
  state?: string;
  tool?: string;
  isError?: boolean;
}

/** agent-panel `Entry`. */
export type AgentEntry =
  | AgentTextEntry
  | AgentToolsEntry
  | AgentProposalEntry
  | AgentNoticeEntry
  | { id: string; type: string; [field: string]: unknown };

/** agent-panel page `Conversation`. */
export interface AgentConversation {
  id: string | number;
  title?: string;
  updated?: string | number | Date;
  href?: string;
}

/** agent-panel `ModelChoice`; plain strings are accepted too. */
export interface AgentModelChoice {
  id: string;
  label?: string;
  group?: string;
  description?: string;
  disabled?: boolean;
}

/**
 * The properties the binding writes. `<agent-page>` and `<agent-panel>` 0.4+
 * add `conversations`, `active` and `hasMore`; the page also gets `heading`,
 * an attribute set through `setAttribute`.
 */
export interface AgentPanelLike extends EventTarget {
  entries: AgentEntry[];
  running: boolean;
  error: string;
  models: string | Array<string | AgentModelChoice>;
  conversations?: AgentConversation[];
  active?: string;
  hasMore?: boolean;
  draft?: string;
  /** agent-panel 0.5+: the composer's context ring. */
  contextUsage?: AgentContextUsage | null;
  setAttribute?(name: string, value: string): void;
  getAttribute?(name: string): string | null;
  removeAttribute?(name: string): void;
  /** agent-panel: register a renderer for one custom entry type. */
  defineEntry?(type: string, render: ((entry: AgentEntry, panel: AgentPanelLike) => unknown) | null): unknown;
  /** agent-panel: fire `agent-action` from inside a custom entry. */
  emitAction?(action: string, detail?: Record<string, unknown>): boolean;
  clearComposer?(options?: { draft?: boolean; attachments?: boolean; references?: boolean }): void;
  removeAttachment?(id: string): void;
  setAttachmentState?(id: string, state: { busy?: boolean; error?: string } | null): void;
}

/** Anything `errorText` accepts: a controller failure, an Error or a code. */
export type AgentError = ChatTurnFailure | Error | { code?: string; message?: string } | null | undefined;

export interface AgentPanelBindingOptions {
  /**
   * Create a conversation when a message is submitted with none open.
   * Defaults to true.
   */
  autoCreate?: boolean;
  /**
   * Register the proposal and notice renderers through `element.defineEntry()`
   * when the element supports it. Defaults to true.
   */
  registerEntries?: boolean;
  /**
   * The ChatClient used to download attachment chips. Defaults to the
   * controller's own client.
   */
  client?: ChatClient;
  /**
   * Replace the default download: receives the upload id and the chip's
   * reference. The default fetches the Blob and saves it.
   */
  download?: (uploadId: string, reference: AgentReference) => unknown | Promise<unknown>;
  /**
   * The context indicator. `'auto'` (default) writes agent-panel 0.5's
   * `contextUsage` and compacts on `agent-compact`; `'slot'` also draws a
   * small button into the `composer-actions` slot on older elements; `false`
   * shows nothing.
   */
  contextUsage?: 'auto' | 'slot' | false;
}

/** agent-panel 0.5 `ContextUsage`. */
export interface AgentContextUsage {
  used: number;
  window: number;
  estimated?: boolean;
  summarizedAt?: string;
  compacting?: boolean;
  canCompact?: boolean;
}

/** The custom entry type of the "earlier messages are summarized" divider. */
export declare const CONTEXT_MARKER_TYPE: 'context-marker';
export declare const CONTEXT_MARKER_TEXT: string;

/** Draw the summary divider; null outside a browser. */
export declare function renderContextMarkerEntry(entry: AgentEntry): HTMLElement | null;

/** The element's `contextUsage` from controller state, or null without a known window. Pure. */
export declare function contextUsageFromState(
  state: Pick<ChatControllerState, 'context'> | null | undefined,
): AgentContextUsage | null;

export interface AgentPanelBinding {
  /** Remove every listener; the binding then never writes to the element. */
  unbind(): void;
  /** Re-read controller state and repaint. */
  refresh(): void;
  /** Append the next conversation page (for a host "Load more" button). */
  loadMore(): Promise<unknown>;
}

/** Shown when a submit carries attachments and the server serves no uploads. */
export declare const ATTACHMENTS_UNSUPPORTED_TEXT: string;

/** The `agent-action` name a proposal entry's buttons emit. */
export declare const PROPOSAL_ACTION: 'proposal-decision';

/** The line shown under a proposal, keyed by its stored state. */
export declare const PROPOSAL_STATE_TEXT: Readonly<Record<string, string>>;

/** A plain-language message for a controller error; never raw server text. */
export declare function errorText(error: AgentError | string | null | undefined): string;

/** A plain-language line for a proposal's non-fatal conflict/refusal. */
export declare function proposalErrorText(error: AgentError | string | null | undefined): string;

/** One proposal as an agent-panel entry, or null when it has no id. Pure. */
export declare function proposalEntryOf(proposal: ChatProposalEntry | null | undefined): AgentProposalEntry | null;

/** Every proposal in controller state, in order, as entries. Pure. */
export declare function proposalEntriesFromState(
  state: Pick<ChatControllerState, 'proposals'> | null | undefined,
): AgentProposalEntry[];

/** Draw one proposal card; returns null outside a browser. */
export declare function renderProposalEntry(
  entry: AgentProposalEntry,
  panel?: AgentPanelLike,
): unknown;

/** Draw one notice as a small system line; returns null outside a browser. */
export declare function renderNoticeEntry(entry: AgentNoticeEntry): unknown;

/**
 * Register the proposal and notice renderers on one element instance. Returns
 * false when no object with `defineEntry()` was given.
 */
export declare function registerProposalEntry(panel: AgentPanelLike): boolean;

/**
 * Model choices for the element's menu. Returns labelled objects when the
 * controller has catalog choices, otherwise the legacy model-id strings.
 */
/** One entry of the element's options menu (agent-panel's option shape). */
export interface AgentEffortOption {
  id: 'effort';
  label: string;
  choices: Array<{ value: string; label: string }>;
}

/**
 * The "Reasoning effort" options-menu entry for the model the element has
 * selected (`''` = the current or default choice), or null when that model
 * takes no effort. The binding keeps it in `element.options` and sends the
 * picked value with each turn.
 */
export declare function effortOptionFromState(
  state: Pick<ChatControllerState, 'choices' | 'choice'> | null | undefined,
  model?: string,
): AgentEffortOption | null;

export declare function modelsFromState(
  state: Pick<ChatControllerState, 'models' | 'choices'> | null | undefined,
): Array<string | AgentModelChoice>;

/** The page sidebar's `conversations` value. */
export declare function conversationsFromState(
  state: Pick<ChatControllerState, 'conversations'> | null | undefined,
): AgentConversation[];

/** Whether this element is `<agent-page>` (renamable title and sidebar). */
export declare function isPageElement(element: object): boolean;

/** Whether this element shows a conversation list (`<agent-page>`, or `<agent-panel>` 0.4+). */
export declare function hasConversationList(element: object): boolean;

/** Map a controller snapshot to agent-panel entries. Pure. */
export declare function entriesFromState(
  state:
    | (Pick<ChatControllerState, 'messages' | 'turn' | 'running'> & {
      proposals?: ChatProposalEntry[];
      context?: ChatControllerState['context'];
    })
    | null
    | undefined,
  options?: { pendingText?: string },
): AgentEntry[];

/** Copy controller state into the element and route its events back. */
export declare function bindAgentPanel(
  element: AgentPanelLike,
  controller: ChatController,
  options?: AgentPanelBindingOptions,
): AgentPanelBinding;
