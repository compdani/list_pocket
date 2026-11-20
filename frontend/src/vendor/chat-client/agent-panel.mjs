// Binding between ChatController and the `agent-panel` Web Components.
//
// `<agent-panel>` and `<agent-page>` (github.com/compdani/agent-panel) are
// host-owned views: they hold no data, they draw the `entries`, `running`,
// `error`, `models` and page-sidebar properties they are given, and they report
// intent through DOM events. This module is the thin adapter in between:
//
//   controller.state  ──►  element properties   (every "change")
//   element events    ──►  controller methods
//
// It imports nothing: the element instance is passed in, so this package stays
// dependency-free and neither package has to know about the other at load time.
//
// Entry shapes follow agent-panel/element.d.ts:
//
//   TextEntry  { id, type: 'text', role, content, references? }
//   ToolCall   { id?, key?, name, label?, status: 'running'|'completed'|'failed', detail? }
//   ToolsEntry { id, type: 'tools', label?, tools: ToolCall[] }
//
// and two custom types this binding registers with `defineEntry()`:
//
//   ProposalEntry { id, type: 'proposal', proposalId, tool, summary, state,
//                   location, revision, error, status, detail, canDecide }
//   NoticeEntry   { id, type: 'notice', content, proposalId?, state?, tool?, isError? }
//
// Attachment parts are folded into the user message's TextEntry as
// `references` chips (type 'attachment'), so no server bytes or data URLs ever
// reach the element; a click downloads through the client.
//
// Page sidebar shapes follow agent-panel/page-element.d.ts:
//
//   Conversation { id, title?, updated?, href? }
//
// Nothing here writes markup: model text goes into `content` and the element
// renders (and sanitises) it. The two custom renderers below build their nodes
// with plain DOM calls, so this module still imports nothing.

/** Shown when a submit carries attachments and the server serves none. */
export const ATTACHMENTS_UNSUPPORTED_TEXT =
  'Attachments are not enabled on this server. Remove the files and send the message as text.';

// Plain-language text for the failure codes the chat API and the controller
// can produce. Raw server messages are never shown: an unknown code gets a
// generic line so internal detail cannot leak into the transcript.
const ERROR_TEXT = Object.freeze({
  active_turn: 'A reply is already in progress. Stop it or wait for it to finish.',
  auth_expired: 'Your session has expired. Sign in again.',
  unauthorized: 'Your session has expired. Sign in again.',
  forbidden: 'You do not have access to this conversation.',
  resync_required: 'This conversation changed somewhere else. Reload it to continue.',
  not_found: 'That conversation is no longer available.',
  rate_limited: 'Too many requests. Wait a moment and try again.',
  payload_too_large: 'That message or attachment is too large to send.',
  invalid_attachment: 'That file was not accepted. Check its type and size.',
  unsupported_media: 'That file type is not supported.',
  editor_unavailable: 'Approving this change needs an editor adapter in the page.',
  invalid_request: 'The message was not accepted. Check it and try again.',
  method_not_allowed: 'The message was not accepted. Check it and try again.',
  conflict: 'This conversation is busy. Try again in a moment.',
  provider: 'The assistant could not answer this time. Try again.',
  internal: 'Something went wrong on the server. Try again.',
  // Failure codes the runner records on a failed turn (chat/runner.go).
  provider_error: 'The AI provider refused the request. Try again, or ask an administrator to check the provider settings.',
  max_steps: 'This question needed too many steps. Try asking something narrower.',
  context_too_long: 'This conversation is too long for the selected model. Compact it, start a new chat, or pick a model with a larger context.',
  too_soon: 'The conversation was summarized moments ago.',
  output_limit: 'The reply was cut off at the model\'s output limit. Ask for a shorter answer, or raise the limit.',
  content_filtered: 'The AI provider withheld this reply. Try rephrasing the request.',
  empty_reply: 'The assistant returned an empty reply. Try again.',
  provider_auth: 'The AI provider rejected its credentials. Ask an administrator to check the key or sign in again in the provider settings.',
  usage_limit: 'The AI provider\'s usage limit was reached. Wait a while or switch to another model.',
  internal_error: 'Something went wrong on the server. Try again.',
  timeout: 'The server took too long to answer. Try again.',
  interrupted: 'The reply was interrupted. Try again.',
  cancelled: '',
});

const NETWORK_TEXT = "Can't reach the server. Check your connection and try again.";
const GENERIC_TEXT = 'Something went wrong. Try again.';

// Only these fragments of a server message are trusted enough to turn into a
// friendlier line when the code alone is not enough. Everything else stays
// generic.
const MESSAGE_HINTS = Object.freeze([
  [/could not be reached|could not be reconnected|network|fetch/i, NETWORK_TEXT],
  [/no event or heartbeat|timed out|timeout/i, ERROR_TEXT.timeout],
  [/resync|reload the conversation/i, ERROR_TEXT.resync_required],
]);

/**
 * A user-facing message for a controller error (`{ code, message }`, an
 * `Error`, or nothing). Never returns raw server text.
 */
export function errorText(error) {
  if (!error) return '';
  if (typeof error === 'string') {
    return Object.prototype.hasOwnProperty.call(ERROR_TEXT, error) ? ERROR_TEXT[error] : GENERIC_TEXT;
  }
  if (typeof error !== 'object') return GENERIC_TEXT;

  const code = typeof error.code === 'string' ? error.code.toLowerCase() : '';
  if (Object.prototype.hasOwnProperty.call(ERROR_TEXT, code)) return ERROR_TEXT[code];

  const message = typeof error.message === 'string' ? error.message : '';
  for (const [pattern, text] of MESSAGE_HINTS) {
    if (pattern.test(message)) return text;
  }
  return GENERIC_TEXT;
}

/**
 * Model choices for the element's menu. A catalog gives labelled objects; the
 * legacy single-provider path still yields plain model-id strings.
 */
export function modelsFromState(state) {
  const choices = state && Array.isArray(state.choices) ? state.choices : [];
  if (choices.length > 0) {
    return choices
      .filter((choice) => choice && typeof choice === 'object' && typeof choice.id === 'string' && choice.id !== '')
      .map((choice) => ({
        id: choice.id,
        label: typeof choice.label === 'string' && choice.label !== '' ? choice.label : String(choice.model ?? choice.id),
        group: typeof choice.group === 'string' && choice.group !== '' ? choice.group : undefined,
        disabled: choice.disabled === true,
      }));
  }
  const models = state && Array.isArray(state.models) ? state.models : [];
  return models.filter((model) => typeof model === 'string' && model !== '');
}

const EFFORT_OPTION_ID = 'effort';
const EFFORT_LABELS = Object.freeze({
  none: 'None', minimal: 'Minimal', low: 'Low', medium: 'Medium', high: 'High', xhigh: 'Extra high',
});

/**
 * The choice a send would use for the element's `model` value: the exact
 * choice id, else the one choice with that model id, else (for '') the
 * controller's current choice or the server default. Mirrors the
 * controller's resolveChoice without importing it.
 */
function selectedChoice(state, model) {
  const choices = state && Array.isArray(state.choices) ? state.choices : [];
  if (choices.length === 0) return null;
  const value = typeof model === 'string' ? model : '';
  if (value !== '') {
    const exact = choices.find((choice) => choice && choice.id === value);
    if (exact) return exact;
    const byModel = choices.filter((choice) => choice && choice.model === value);
    return byModel.length === 1 ? byModel[0] : null;
  }
  const current = typeof state.choice === 'string' ? choices.find((choice) => choice && choice.id === state.choice) : null;
  return current ?? choices.find((choice) => choice && choice.default) ?? choices[0];
}

/**
 * The "Reasoning effort" entry for the element's options menu, or null when
 * the selected model takes no effort. "Model default" sends no effort.
 */
export function effortOptionFromState(state, model = '') {
  const choice = selectedChoice(state, model);
  const efforts = choice && Array.isArray(choice.efforts)
    ? choice.efforts.filter((effort) => typeof effort === 'string' && effort !== '')
    : [];
  if (efforts.length === 0) return null;
  return {
    id: EFFORT_OPTION_ID,
    label: 'Reasoning effort',
    choices: [
      { value: '', label: 'Model default' },
      ...efforts.map((effort) => ({ value: effort, label: EFFORT_LABELS[effort] || effort })),
    ],
  };
}

/**
 * The page sidebar's `conversations` value. Items without an id are dropped,
 * exactly as the page does, and `updatedAt` becomes the row's `updated`.
 */
export function conversationsFromState(state) {
  const list = state && Array.isArray(state.conversations) ? state.conversations : [];
  const out = [];
  for (const item of list) {
    if (!item || typeof item !== 'object') continue;
    const id = item.id;
    if (typeof id !== 'string' && typeof id !== 'number') continue;
    if (String(id) === '') continue;
    const conversation = { id, title: typeof item.title === 'string' ? item.title : '' };
    const updated = item.updatedAt ?? item.updated;
    if (typeof updated === 'string' || typeof updated === 'number' || updated instanceof Date) {
      conversation.updated = updated;
    }
    out.push(conversation);
  }
  return out;
}

/**
 * Whether this element is the full page (renamable title, sidebar). Since
 * agent-panel 0.4 the compact panel has `conversations` too, so it is told
 * apart by its switcher methods.
 */
export function isPageElement(element) {
  if (!element || typeof element !== 'object') return false;
  const tag = typeof element.tagName === 'string' ? element.tagName.toUpperCase() : '';
  if (tag === 'AGENT-PAGE') return true;
  if (tag === 'AGENT-PANEL') return false;
  if (typeof element.startRename === 'function') return true;
  if (typeof element.openConversations === 'function') return false;
  return element.conversations !== undefined || element.active !== undefined;
}

/**
 * Whether this element shows a conversation list: the page's sidebar, or the
 * switcher of `<agent-panel>` 0.4+ (shown with its `conversations-menu`
 * attribute).
 */
export function hasConversationList(element) {
  if (!element || typeof element !== 'object') return false;
  return isPageElement(element) || element.conversations !== undefined
    || typeof element.openConversations === 'function';
}

function textOfParts(parts) {
  if (!Array.isArray(parts)) return '';
  let out = '';
  for (const part of parts) {
    if (part && part.type === 'text' && typeof part.text === 'string') out += part.text;
  }
  return out;
}

function messageId(message, index) {
  if (message && typeof message.id === 'string' && message.id !== '') return message.id;
  return `#${index}`;
}

function toolStatus(status) {
  if (status === 'completed' || status === 'failed' || status === 'running') return status;
  return 'running';
}

/** Tool results, keyed by call id, folded out of the transcript's tool messages. */
function toolResultsOf(messages) {
  const results = new Map();
  for (const message of messages) {
    if (!message || !Array.isArray(message.parts)) continue;
    for (const part of message.parts) {
      if (!part || part.type !== 'tool_result' || part.callId === undefined) continue;
      results.set(String(part.callId), { failed: part.isError === true });
    }
  }
  return results;
}

function hasUserText(messages, text) {
  for (const message of messages) {
    if (!message || message.role !== 'user' || !Array.isArray(message.parts)) continue;
    if (textOfParts(message.parts).trim() === text) return true;
  }
  return false;
}

// --- proposals -------------------------------------------------------------

/** The `agent-action` name a proposal entry's buttons emit. */
export const PROPOSAL_ACTION = 'proposal-decision';

/**
 * The line shown under a proposal. It says what happened, never what was
 * "saved": an approval only ever authorizes an effect, and an editor report is
 * a local draft change.
 */
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

const PROPOSAL_STATE_LABEL = Object.freeze({
  pending: 'Pending',
  rejected: 'Rejected',
  expired: 'Expired',
  executing: 'Applying',
  succeeded: 'Applied',
  failed: 'Failed',
  unknown: 'Unknown',
  approved: 'Approved',
  applied: 'Applied in editor',
  stale: 'Out of date',
});

const PROPOSAL_STATE_TONE = Object.freeze({
  pending: 'tone-info',
  rejected: 'tone-muted',
  expired: 'tone-muted',
  executing: 'tone-info',
  succeeded: 'tone-success',
  failed: 'tone-error',
  unknown: 'tone-warning',
  approved: 'tone-info',
  applied: 'tone-success',
  stale: 'tone-warning',
});

/** A plain-language line for a proposal error; never raw server text. */
export function proposalErrorText(error) {
  if (!error) return '';
  const code = typeof error === 'object' && typeof error.code === 'string'
    ? error.code.toLowerCase()
    : '';
  switch (code) {
    case 'stale':
      return 'This change is out of date. Ask again for a fresh one.';
    case 'expired':
      return 'This change expired before it was decided.';
    case 'conflict':
      return 'This change was decided somewhere else. Review the latest version.';
    case 'refused':
      // The tool's own reason, written for the user, and the change is
      // waiting for approval again.
      // Nothing was saved and the change is waiting again: it can be
      // rejected, or approved once the cause is fixed (or the arguments
      // edited, where the server allows it).
      return typeof error.message === 'string' && error.message.trim() !== ''
        ? `${error.message.trim()} Nothing was saved; the change is waiting for your decision again.`
        : 'The change was refused before anything was saved. It is waiting for your decision again.';
    case 'not_found':
      return 'This change is no longer available.';
    case 'editor_unavailable':
      return ERROR_TEXT.editor_unavailable;
    default:
      return errorText(error);
  }
}

function humanizeTool(name) {
  const text = String(name ?? '').replace(/_/g, ' ').trim();
  if (text === '') return 'Change';
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function proposalFailureText(proposal) {
  const failure = typeof proposal.failure === 'string' ? proposal.failure.trim() : '';
  return failure;
}

/**
 * One proposal as an agent-panel entry. `state` is the stored lifecycle state,
 * `status` the line a person reads, `detail` a server failure message, `error`
 * a non-fatal client conflict/refusal, and `canDecide` whether the buttons show.
 *
 * Pure: no DOM and no controller access.
 */
export function proposalEntryOf(proposal) {
  if (!proposal || typeof proposal !== 'object') return null;
  const id = proposal.id === undefined || proposal.id === null ? '' : String(proposal.id);
  if (id === '') return null;
  const state = typeof proposal.state === 'string' ? proposal.state : 'pending';
  const location = proposal.location === 'editor' ? 'editor' : 'server';
  const tool = typeof proposal.tool === 'string' && proposal.tool !== '' ? proposal.tool : 'tool';
  const summary = typeof proposal.summary === 'string' ? proposal.summary.trim() : '';
  const failure = proposalFailureText(proposal);
  let status = PROPOSAL_STATE_TEXT[state] ?? `State: ${state}.`;
  if (state === 'failed' && failure !== '') status = `${PROPOSAL_STATE_TEXT.failed} ${failure}`;
  return {
    id: `proposal:${id}`,
    type: 'proposal',
    proposalId: id,
    tool,
    summary: summary === '' ? humanizeTool(tool) : summary,
    state,
    location,
    revision: Number.isFinite(Number(proposal.revision)) ? Number(proposal.revision) : 0,
    error: proposalErrorText(proposal.error),
    status,
    detail: failure,
    warnings: Array.isArray(proposal.warnings)
      ? proposal.warnings.filter((w) => typeof w === 'string' && w.trim() !== '').map((w) => w.trim())
      : [],
    canDecide: state === 'pending',
  };
}

/** Every proposal in controller state, in order, as entries. Pure. */
export function proposalEntriesFromState(state) {
  const list = state && Array.isArray(state.proposals) ? state.proposals : [];
  const entries = [];
  for (const proposal of list) {
    const entry = proposalEntryOf(proposal);
    if (entry) entries.push(entry);
  }
  return entries;
}

/** The chip a user message shows for one attachment part, or null. */
function attachmentReferenceOf(part) {
  if (!part || part.type !== 'attachment') return null;
  const id = part.uploadId === undefined || part.uploadId === null ? '' : String(part.uploadId);
  if (id === '') return null;
  const name = typeof part.name === 'string' && part.name !== '' ? part.name : id;
  const reference = { type: 'attachment', id, label: name, attachment: true };
  if (typeof part.mime === 'string' && part.mime !== '') reference.mime = part.mime;
  return reference;
}

function noticeEntryOf(message, part, messageIndex, partIndex) {
  const text = typeof part.text === 'string' ? part.text.trim() : '';
  if (text === '') return null;
  const messageKey = messageId(message, messageIndex);
  return {
    id: `notice:${messageKey}:${partIndex}`,
    type: 'notice',
    content: text,
    proposalId: typeof part.proposalId === 'string' ? part.proposalId : '',
    state: typeof part.state === 'string' ? part.state : '',
    tool: typeof part.tool === 'string' ? part.tool : '',
    isError: part.isError === true,
  };
}

/**
 * Translate a controller snapshot into agent-panel entries.
 *
 *   - `messages` become `text` entries (user bubbles and assistant markdown)
 *     and one `tools` entry per assistant message that called tools;
 *   - the live `turn` adds the part of its streaming text the transcript does
 *     not have yet, plus its tools that have no result yet, so text appears
 *     incrementally and a tool moves running → completed;
 *   - `pendingText` is the message just submitted, shown as a user bubble until
 *     the reloaded transcript contains it.
 *
 * Pure: it reads only its arguments and never touches the DOM. The element
 * replaces a stale error banner itself; entries carry no error state.
 */
export function entriesFromState(state, options = {}) {
  const messages = state && Array.isArray(state.messages) ? state.messages : [];
  const results = toolResultsOf(messages);
  const turn = state && state.turn && typeof state.turn === 'object' ? state.turn : null;
  const activeTurnId = turn && turn.turnId ? String(turn.turnId) : '';
  const running = state?.running === true;

  const entries = [];
  const summarizedThrough = typeof state?.context?.summarizedThrough === 'string'
    ? state.context.summarizedThrough
    : '';
  let markerAt = -1;
  if (summarizedThrough !== '') {
    markerAt = messages.findIndex((m) => m && String(m.id) === summarizedThrough);
    // The marker goes after the summarized message's tool results too.
    while (markerAt >= 0 && markerAt + 1 < messages.length && messages[markerAt + 1]?.role === 'tool') markerAt += 1;
  }
  const pushMarker = (index) => {
    if (index === markerAt) {
      entries.push({ id: `context-marker:${summarizedThrough}`, type: CONTEXT_MARKER_TYPE, content: CONTEXT_MARKER_TEXT });
    }
  };

  for (let index = 0; index < messages.length; index += 1) {
    if (index > 0) pushMarker(index - 1);
    const message = messages[index];
    if (!message || typeof message !== 'object') continue;
    const role = typeof message.role === 'string' && message.role ? message.role : 'assistant';
    // A tool message carries only results; they are folded into the tool calls
    // above it, never shown as text.
    if (role === 'tool') continue;

    const id = messageId(message, index);
    const parts = Array.isArray(message.parts) ? message.parts : [];
    const turnId = typeof message.turnId === 'string' ? message.turnId : '';

    // A notice is framework-generated, not authored: it becomes a small system
    // line instead of a user or assistant bubble.
    if (role === 'notice') {
      parts.forEach((part, partIndex) => {
        if (!part || typeof part !== 'object' || part.type !== 'proposal_outcome') return;
        const notice = noticeEntryOf(message, part, index, partIndex);
        if (notice) entries.push(notice);
      });
      continue;
    }

    let buffer = '';
    let textIndex = 0;
    let tools = [];
    const references = [];
    let firstText = null;

    const flushText = () => {
      if (buffer.trim() === '') {
        buffer = '';
        return;
      }
      const entry = {
        id: textIndex === 0 ? `msg:${id}` : `msg:${id}:${textIndex}`,
        type: 'text',
        role,
        content: buffer,
      };
      if (firstText === null) firstText = entry;
      buffer = '';
      textIndex += 1;
      entries.push(entry);
    };
    const flushTools = () => {
      if (tools.length === 0) return;
      entries.push({ id: `tools:${id}`, type: 'tools', tools });
      tools = [];
    };

    for (const part of parts) {
      if (!part || typeof part !== 'object') continue;
      if (part.type === 'text') {
        buffer += typeof part.text === 'string' ? part.text : '';
        continue;
      }
      if (part.type === 'attachment') {
        const reference = attachmentReferenceOf(part);
        if (reference) references.push(reference);
        continue;
      }
      if (part.type === 'tool_call') {
        flushText();
        const callId = String(part.callId ?? '');
        const result = results.get(callId);
        const tool = {
          id: callId,
          name: typeof part.tool === 'string' && part.tool !== '' ? part.tool : 'tool',
          status: result
            ? (result.failed ? 'failed' : 'completed')
            // An unresolved call in a turn that is not running now was cut off.
            : (running && turnId !== '' && turnId === activeTurnId ? 'running' : 'failed'),
        };
        tools.push(tool);
      }
      // tool_result inside an assistant message was already collected above.
    }
    flushText();
    flushTools();

    // Attachment chips belong to the message, not to a part of their own: they
    // ride on its first text entry, or on a text-less chip entry when the user
    // sent files without words.
    if (references.length > 0) {
      if (firstText !== null) {
        firstText.references = references;
      } else {
        entries.push({
          id: `msg:${id}:attachments`,
          type: 'text',
          role,
          content: '',
          references,
        });
      }
    }
  }

  if (messages.length > 0) pushMarker(messages.length - 1);

  const pending = typeof options.pendingText === 'string' ? options.pendingText.trim() : '';
  if (pending !== '' && !hasUserText(messages, pending)) {
    entries.push({ id: 'pending:user', type: 'text', role: 'user', content: pending });
  }

  if (turn && activeTurnId !== '') {
    // Text already committed to the transcript for this turn is a prefix of
    // the live view's text (the runner appends a message per model step), so
    // only the remainder is live.
    let transcriptText = '';
    const knownCalls = new Set();
    for (const message of messages) {
      if (!message || message.turnId !== activeTurnId || !Array.isArray(message.parts)) continue;
      if (message.role !== 'user') transcriptText += textOfParts(message.parts);
      for (const part of message.parts) {
        if (part && part.type === 'tool_call' && part.callId !== undefined) {
          knownCalls.add(String(part.callId));
        }
      }
    }
    let remainder = typeof turn.text === 'string' ? turn.text : '';
    if (transcriptText !== '') {
      remainder = remainder.startsWith(transcriptText) ? remainder.slice(transcriptText.length) : '';
    }

    const liveTools = [];
    let liveIndex = 0;
    for (const tool of Array.isArray(turn.tools) ? turn.tools : []) {
      if (!tool) continue;
      liveIndex += 1;
      const callId = String(tool.callId ?? '');
      if (callId !== '' && knownCalls.has(callId)) continue;
      liveTools.push({
        id: callId !== '' ? callId : `live-${liveIndex}`,
        name: typeof tool.tool === 'string' && tool.tool !== '' ? tool.tool : 'tool',
        // A tool still marked running in a turn that has ended was cut off; a
        // spinner would never stop.
        status: !running && toolStatus(tool.status) === 'running' ? 'failed' : toolStatus(tool.status),
      });
    }

    const textEntry = remainder.trim() === ''
      ? null
      : { id: `turn:${activeTurnId}`, type: 'text', role: 'assistant', content: remainder };
    const toolsEntry = liveTools.length === 0
      ? null
      : { id: `tools:${activeTurnId}`, type: 'tools', tools: liveTools };

    // A model usually calls tools before it answers, so the live tool group
    // comes first while both are present.
    if (toolsEntry && textEntry) {
      entries.push(toolsEntry, textEntry);
    } else if (toolsEntry) {
      entries.push(toolsEntry);
    } else if (textEntry) {
      entries.push(textEntry);
    }
  }

  // Proposals are conversation-level, so they follow the transcript. A decided
  // one stays until the next open, alongside the server's notice line.
  entries.push(...proposalEntriesFromState(state));

  return entries;
}

// --- custom entry renderers ------------------------------------------------

function makeElement(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined && text !== null) node.textContent = String(text);
  return node;
}

function hasDocument() {
  return typeof document !== 'undefined' && document !== null
    && typeof document.createElement === 'function';
}

/**
 * Draw one proposal card: what the model wants to change, its state in plain
 * words, any failure/conflict, and Approve/Reject while it is pending.
 *
 * The buttons only emit (`proposal-decision` with `{ id, decision }`); the
 * binding owns `controller.decide()`. Returns null outside a browser, so the
 * panel falls back to the `entry-{id}` slot.
 */
export function renderProposalEntry(entry, panel) {
  if (!hasDocument() || !entry) return null;
  const card = makeElement('div', 'card proposal');
  card.setAttribute('part', 'proposal');
  card.dataset.state = String(entry.state ?? '');

  const head = makeElement('div', 'proposal-head');
  head.style.cssText = 'display:flex;align-items:center;justify-content:space-between;gap:8px;';
  head.appendChild(makeElement('strong', null, entry.summary || humanizeTool(entry?.tool)));
  const tone = PROPOSAL_STATE_TONE[entry.state] ?? 'tone-muted';
  const label = PROPOSAL_STATE_LABEL[entry.state] ?? String(entry.state ?? '');
  head.appendChild(makeElement('span', `badge ${tone}`, label));
  card.appendChild(head);

  const where = entry.location === 'editor' ? 'editor change' : 'server change';
  const meta = makeElement(
    'p',
    'proposal-meta',
    `${humanizeTool(entry.tool)} · ${where} · revision ${entry.revision}`,
  );
  meta.style.cssText = 'font-size:12px;color:var(--agent-muted);margin:2px 0 0;';
  card.appendChild(meta);

  const status = makeElement('p', 'proposal-status', entry.status);
  status.style.cssText = 'font-size:13px;margin:8px 0 0;line-height:1.45;';
  card.appendChild(status);

  // A failed proposal's failure text is already part of its status line.
  if (entry.detail && entry.state !== 'failed' && entry.detail !== entry.status) {
    const detail = makeElement('p', 'proposal-detail', entry.detail);
    detail.style.cssText = 'font-size:12px;color:var(--agent-muted);margin:6px 0 0;';
    card.appendChild(detail);
  }
  for (const warning of Array.isArray(entry.warnings) ? entry.warnings : []) {
    const line = makeElement('p', 'proposal-warning tone-warning', warning);
    line.setAttribute('part', 'proposal-warning');
    line.style.cssText = 'font-size:12px;margin:6px 0 0;padding:4px 8px;border-radius:6px;'
      + 'overflow-wrap:anywhere;';
    card.appendChild(line);
  }

  if (entry.error) {
    const line = makeElement('p', 'alert tone-error', entry.error);
    line.style.cssText = 'font-size:12px;margin:8px 0 0;padding:6px 8px;';
    card.appendChild(line);
  }

  if (entry.canDecide) {
    const actions = makeElement('div', 'proposal-actions');
    actions.style.cssText = 'display:flex;gap:8px;margin-top:10px;';
    const approve = makeElement('button', 'btn btn--primary', 'Approve');
    approve.type = 'button';
    approve.addEventListener('click', () => {
      if (panel && typeof panel.emitAction === 'function') {
        panel.emitAction(PROPOSAL_ACTION, { id: entry.proposalId, decision: 'approve' });
      }
    });
    const reject = makeElement('button', 'btn btn--tonal', 'Reject');
    reject.type = 'button';
    reject.addEventListener('click', () => {
      if (panel && typeof panel.emitAction === 'function') {
        panel.emitAction(PROPOSAL_ACTION, { id: entry.proposalId, decision: 'reject' });
      }
    });
    actions.append(approve, reject);
    card.appendChild(actions);
  }

  return card;
}

/** Draw one notice (for example a proposal outcome) as a small system line. */
export function renderNoticeEntry(entry) {
  if (!hasDocument() || !entry) return null;
  const line = makeElement('div', 'notice', entry.content);
  line.setAttribute('part', 'notice');
  line.dataset.state = String(entry.state ?? '');
  // A notice can carry a long URL or identifier: let it wrap anywhere rather
  // than overflow a narrow panel.
  line.style.cssText = 'font-size:12px;color:var(--agent-muted);text-align:center;'
    + 'line-height:1.45;padding:2px 10px;overflow-wrap:anywhere;';
  if (entry.isError) line.style.color = 'var(--agent-error)';
  return line;
}

/**
 * Register the proposal and notice renderers on one element instance (an
 * `<agent-panel>` or `<agent-page>`). `bindAgentPanel()` calls this itself, so
 * a host only needs it when it sets `entries` without the binding. Returns
 * whether an element with `defineEntry()` was given.
 *
 * agent-panel keeps renderers per instance; there is no class-level hook, so a
 * subclass registers in its constructor (or after `define()` on an instance).
 */
export function registerProposalEntry(panel) {
  if (!panel || typeof panel.defineEntry !== 'function') return false;
  panel.defineEntry('proposal', renderProposalEntry);
  panel.defineEntry('notice', renderNoticeEntry);
  panel.defineEntry(CONTEXT_MARKER_TYPE, renderContextMarkerEntry);
  return true;
}

// --- context window ----------------------------------------------------------

/** The custom entry type of the "earlier messages are summarized" divider. */
export const CONTEXT_MARKER_TYPE = 'context-marker';
export const CONTEXT_MARKER_TEXT = 'Earlier messages are summarized for the model';

/** Draw the summary marker: a thin divider with a short label. */
export function renderContextMarkerEntry(entry) {
  if (!hasDocument() || !entry) return null;
  const line = makeElement('div', 'context-marker');
  line.setAttribute('part', 'context-marker');
  line.setAttribute('role', 'separator');
  line.style.cssText = 'display:flex;align-items:center;gap:10px;font-size:12px;'
    + 'color:var(--agent-muted);margin:6px 0;';
  const rule = () => {
    const hr = makeElement('span');
    hr.style.cssText = 'flex:1;height:1px;background:var(--agent-border);';
    return hr;
  };
  line.append(rule(), makeElement('span', '', entry.content || CONTEXT_MARKER_TEXT), rule());
  return line;
}

/**
 * The element's `contextUsage` (agent-panel 0.5+) from controller state, or
 * null when the server reports no known window. Pure.
 */
export function contextUsageFromState(state) {
  const context = state && state.context && typeof state.context === 'object' ? state.context : null;
  if (!context || !(context.window > 0)) return null;
  const usage = {
    used: Math.max(0, Number(context.tokens) || 0),
    window: context.window,
    estimated: context.estimated === true,
    compacting: context.compacting === true,
    canCompact: context.canCompact === true,
  };
  if (context.summarizedAt) usage.summarizedAt = context.summarizedAt;
  return usage;
}

/** The share of the window at which the ring (and the slot fallback) shows. */
const CONTEXT_THRESHOLD = 0.6;

// --- binding ---------------------------------------------------------------

function serialized(value) {
  try {
    return JSON.stringify(value);
  } catch {
    return null;
  }
}

/**
 * Copy `value` into `element[key]` only when it changed. Returns whether it
 * wrote. The element diffs entries itself, but skipping no-op writes keeps
 * unrelated redraws (and the scroll following) quiet.
 */
function writeProperty(element, key, value) {
  const current = element[key];
  if (value !== null && typeof value === 'object') {
    const before = serialized(current);
    const after = serialized(value);
    if (before !== null && before === after) return false;
  } else if (current === value) {
    return false;
  }
  element[key] = value;
  return true;
}

/** `heading` is an attribute on <agent-page>, not a property. */
function writeHeading(element, title) {
  if (typeof element.setAttribute === 'function') {
    const current = typeof element.getAttribute === 'function'
      ? (element.getAttribute('heading') ?? '')
      : null;
    if (current === title) return false;
    element.setAttribute('heading', title);
    return true;
  }
  return writeProperty(element, 'heading', title);
}

function titleOfConversation(state, id) {
  if (!id) return '';
  const list = state && Array.isArray(state.conversations) ? state.conversations : [];
  const conversation = list.find((item) => item && String(item.id) === String(id));
  return conversation && typeof conversation.title === 'string' ? conversation.title : '';
}

/**
 * Set or remove one attribute when it changed. A null/empty value removes it,
 * which is how boolean attributes (`no-attachments`) turn off. Falls back to a
 * property write on a test double without attributes.
 */
function writeAttribute(element, name, value) {
  const next = value === null || value === undefined || value === '' ? null : String(value);
  if (typeof element.setAttribute === 'function') {
    const current = typeof element.getAttribute === 'function' ? element.getAttribute(name) : null;
    if (next === null) {
      if (current === null) return false;
      if (typeof element.removeAttribute === 'function') element.removeAttribute(name);
      else element.setAttribute(name, '');
      return true;
    }
    if (current === next) return false;
    element.setAttribute(name, next);
    return true;
  }
  if (next === null) return writeProperty(element, name, '');
  return writeProperty(element, name, next);
}

/**
 * Mirror the server's attachment rules onto the composer. A deployment that
 * serves no uploads (or names no accepted type) hides the paperclip and ignores
 * pasted/dropped files instead of letting a submit fail after the upload.
 */
function writeAttachmentLimits(element, snapshot) {
  const features = snapshot && snapshot.features && typeof snapshot.features === 'object'
    ? snapshot.features
    : null;
  const limits = snapshot && snapshot.limits && typeof snapshot.limits === 'object'
    ? snapshot.limits
    : {};
  // limits.attachmentTypes is the union the upload route accepts. The element
  // only reports the chosen model on submit, so the picker cannot narrow to
  // one model here; send() checks the files against the chosen model.
  const types = Array.isArray(limits.attachmentTypes)
    ? limits.attachmentTypes.filter((type) => typeof type === 'string' && type !== '')
    : null;

  // Until /status has answered, leave the element's defaults alone.
  if (features === null) return;
  const noAttachments = features.uploads !== true || (types !== null && types.length === 0);
  writeAttribute(element, 'no-attachments', noAttachments ? 'true' : null);
  if (noAttachments) return;

  if (types && types.length > 0) writeAttribute(element, 'accept', types.join(','));
  const maxBytes = Number(limits.maxUploadBytes);
  if (Number.isFinite(maxBytes) && maxBytes > 0) {
    writeAttribute(element, 'max-file-size', String(Math.trunc(maxBytes)));
  }
  const maxFiles = Number(limits.maxAttachmentsPerTurn);
  if (Number.isFinite(maxFiles) && maxFiles > 0) {
    writeAttribute(element, 'max-files', String(Math.trunc(maxFiles)));
  }
}

/** The File/Blob values behind agent-panel's submit attachments. */
function filesFromAttachments(items) {
  const files = [];
  for (const item of items) {
    const file = item && typeof item === 'object' ? item.file ?? item : null;
    if (file && typeof file === 'object') files.push(file);
  }
  return files;
}

/** Set or clear the busy/error state of the composer chips, when supported. */
function setAttachmentState(element, items, state) {
  if (typeof element.setAttachmentState !== 'function') return;
  for (const item of items) {
    const id = item && typeof item === 'object' ? item.id : '';
    if (typeof id === 'string' && id !== '') element.setAttachmentState(id, state);
  }
}

/** Save a Blob through a temporary anchor; the caller owns the Blob. */
function saveBlob(blob, name) {
  if (!hasDocument() || typeof URL === 'undefined' || typeof URL.createObjectURL !== 'function') return;
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = String(name || 'attachment');
  if (document.body && typeof document.body.appendChild === 'function') {
    document.body.appendChild(anchor);
  }
  anchor.click();
  if (typeof anchor.remove === 'function') anchor.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

/**
 * Bind one ChatController to one `<agent-panel>` or `<agent-page>`.
 *
 * Properties written on every controller "change": `entries`, `running`,
 * `error`, `models`, and, on a page, `conversations`, `active` and `heading`.
 * Attachment attributes (`no-attachments`, `accept`, `max-file-size`,
 * `max-files`) mirror `/status` once it has answered.
 *
 * Events handled: `agent-submit` (send, with files), `agent-stop` (stop),
 * `agent-error-dismiss` (clears the binding's copy of the error),
 * `agent-action` with `proposal-decision` (decide) or `attachment-download`,
 * `agent-reference-open` on an attachment chip (download), and on a page
 * `agent-select-conversation`, `agent-new-conversation`,
 * `agent-delete-conversation` and `agent-rename`. Any other `agent-action` is
 * ignored. `agent-load-more` is honoured when a host draws its own button in
 * the page's `sidebar-end` slot (the page itself has no load-more control).
 *
 * @param {object} element An `<agent-panel>` / `<agent-page>` (or a test double).
 * @param {object} controller A ChatController (or the same shape).
 * @param {object} [options]
 * @param {boolean} [options.autoCreate=true] Create a conversation when a
 *   submit arrives with none open.
 * @param {boolean} [options.registerEntries=true] Register the proposal and
 *   notice renderers with `element.defineEntry()` when the element has it.
 * @param {object} [options.client] A ChatClient used for attachment downloads;
 *   defaults to the controller's own client.
 * @param {(uploadId: string, reference: object) => unknown} [options.download]
 *   Replace the default download (which fetches and saves the Blob).
 * @param {'auto' | 'slot' | false} [options.contextUsage='auto'] How the
 *   context indicator is shown: 'auto' writes agent-panel 0.5's
 *   `contextUsage` (nothing on older versions), 'slot' also draws a small
 *   button into the `composer-actions` slot when the element has no
 *   `contextUsage` of its own, and false shows nothing.
 * @returns {{ unbind: () => void, refresh: () => void, loadMore: () => Promise<unknown> }}
 */
export function bindAgentPanel(element, controller, options = {}) {
  if (!element || typeof element.addEventListener !== 'function') {
    throw new TypeError('bindAgentPanel() requires the element');
  }
  if (!controller || typeof controller.addEventListener !== 'function') {
    throw new TypeError('bindAgentPanel() requires the controller');
  }
  const page = isPageElement(element);
  const lists = hasConversationList(element);
  const autoCreate = options.autoCreate !== false;
  const contextMode = options.contextUsage === false ? 'off' : options.contextUsage === 'slot' ? 'slot' : 'auto';
  let contextButton = null; // the 'slot' fallback, created on first use
  if (options.registerEntries !== false) registerProposalEntry(element);

  const listeners = [];
  let unbound = false;
  let pendingText = '';
  let pendingConversationId = null;
  let localError = null; // a binding-only message (e.g. refused attachments)
  let dismissedError = null; // a state error the person closed

  const state = () => controller.state ?? {};

  const listen = (target, type, handler) => {
    target.addEventListener(type, handler);
    listeners.push([target, type, handler]);
  };

  function paint() {
    if (unbound) return;
    const snapshot = state();

    // Drop the optimistic user bubble once the transcript has the message, or
    // once the person moved to another conversation.
    if (pendingText !== '') {
      if (pendingConversationId && snapshot.activeConversationId
        && String(snapshot.activeConversationId) !== String(pendingConversationId)) {
        pendingText = '';
      } else if (hasUserText(Array.isArray(snapshot.messages) ? snapshot.messages : [], pendingText.trim())) {
        pendingText = '';
        pendingConversationId = null;
      }
    }

    writeProperty(element, 'entries', entriesFromState(snapshot, { pendingText }));
    writeProperty(element, 'running', snapshot.running === true);

    const mapped = errorText(snapshot.error) || errorText(snapshot.turn && snapshot.turn.error);
    const text = localError || mapped;
    if (text === '') {
      dismissedError = null;
      writeProperty(element, 'error', '');
    } else if (text === dismissedError) {
      // The element already cleared the banner (agent-error-dismiss).
    } else {
      dismissedError = null;
      writeProperty(element, 'error', text);
    }

    writeContextUsage(snapshot);
    writeProperty(element, 'models', modelsFromState(snapshot));
    writeEffortOption(snapshot);
    writeAttachmentLimits(element, snapshot);

    if (lists) {
      const conversations = conversationsFromState(snapshot);
      writeProperty(element, 'conversations', conversations);
      const active = typeof snapshot.activeConversationId === 'string'
        ? snapshot.activeConversationId
        : '';
      writeProperty(element, 'active', active);
      writeProperty(element, 'hasMore', Boolean(snapshot.nextCursor));
      // The panel's heading is the host's (e.g. the assistant's name).
      if (page) writeHeading(element, titleOfConversation(snapshot, active));
    }
  }

  /** Send one message; resolves with whether the server accepted it. */
  async function deliver(text, model, attachments = [], effort = '') {
    try {
      let conversationId = state().activeConversationId;
      if (!conversationId) {
        if (!autoCreate) {
          throw Object.assign(new Error('No conversation is open.'), { code: 'invalid_request' });
        }
        const conversation = await controller.newConversation();
        conversationId = conversation?.id ?? state().activeConversationId;
        if (!conversationId) {
          // The controller reported the failure; the message was never sent, so
          // do not keep echoing it.
          pendingText = '';
          pendingConversationId = null;
          paint();
          return false;
        }
      }
      pendingConversationId = String(conversationId);
      paint();
      const sendOptions = {};
      if (model) sendOptions.model = model;
      if (effort) sendOptions.effort = effort;
      if (attachments.length > 0) sendOptions.attachments = attachments;
      await controller.send(text, Object.keys(sendOptions).length > 0 ? sendOptions : undefined);
      return true;
    } catch (err) {
      pendingText = '';
      pendingConversationId = null;
      localError = proposalErrorText(err) || errorText(err);
      paint();
      return false;
    }
  }

  function nativeContextUsage() {
    // agent-panel 0.5 defines contextUsage on its prototype; an expando on an
    // older element would draw nothing.
    return 'contextUsage' in Object.getPrototypeOf(element);
  }

  function writeContextUsage(snapshot) {
    if (contextMode === 'off') return;
    const usage = contextUsageFromState(snapshot);
    if (nativeContextUsage()) {
      writeProperty(element, 'contextUsage', usage);
      return;
    }
    if (contextMode !== 'slot' || !hasDocument()) return;
    const share = usage ? usage.used / usage.window : 0;
    const show = usage !== null && (share >= CONTEXT_THRESHOLD || usage.compacting);
    if (!show) {
      if (contextButton) contextButton.hidden = true;
      return;
    }
    if (!contextButton) {
      contextButton = makeElement('button', 'chat-context-usage');
      contextButton.type = 'button';
      contextButton.slot = 'composer-actions';
      contextButton.style.cssText = 'font:inherit;font-size:12px;border:1px solid var(--agent-border,#ccc);'
        + 'border-radius:999px;background:none;color:inherit;padding:2px 8px;cursor:pointer;';
      contextButton.addEventListener('click', () => {
        const current = contextUsageFromState(state());
        if (current && current.canCompact && !current.compacting) void controller.compact();
      });
      element.append(contextButton);
    }
    const percent = Math.round(Math.min(1, share) * 100);
    contextButton.hidden = false;
    contextButton.disabled = usage.compacting || !usage.canCompact || snapshot.running === true;
    contextButton.textContent = usage.compacting ? 'Compacting…' : `${percent}% context`;
    contextButton.title = usage.canCompact
      ? `${percent}% of the context window used. Click to summarize older messages.`
      : `${percent}% of the context window used.`;
  }

  // The binding owns only the "effort" entry of the options menu; options a
  // host added itself are kept.
  function writeEffortOption(snapshot) {
    const existing = Array.isArray(element.options) ? element.options : [];
    const others = existing.filter((option) => !option || option.id !== EFFORT_OPTION_ID);
    const effort = effortOptionFromState(snapshot, typeof element.model === 'string' ? element.model : '');
    writeProperty(element, 'options', effort ? [...others, effort] : others);
  }

  function onModelChange() {
    // The element applies the new model after this (cancelable) event.
    queueMicrotask(paint);
  }

  function onSubmit(event) {
    const detail = event && event.detail && typeof event.detail === 'object' ? event.detail : {};
    const items = Array.isArray(detail.attachments) ? detail.attachments : [];
    const files = filesFromAttachments(items);
    const text = typeof detail.text === 'string' ? detail.text.trim() : '';
    if (text === '') {
      if (files.length > 0) {
        // The API requires text; keep the files and say why nothing was sent.
        if (typeof event.preventDefault === 'function') event.preventDefault();
        localError = 'A message with attachments still needs some text.';
        paint();
      }
      return;
    }

    const features = state().features;
    if (files.length > 0 && features && features.uploads === false) {
      if (typeof event.preventDefault === 'function') event.preventDefault();
      localError = ATTACHMENTS_UNSUPPORTED_TEXT;
      paint();
      return;
    }

    const model = typeof detail.model === 'string' ? detail.model : '';
    const chosen = detail.options && typeof detail.options === 'object' ? detail.options[EFFORT_OPTION_ID] : '';
    const effort = typeof chosen === 'string' ? chosen : '';
    localError = null;
    pendingText = text;
    pendingConversationId = state().activeConversationId
      ? String(state().activeConversationId)
      : null;
    paint();

    if (files.length === 0) {
      void deliver(text, model, [], effort);
      return;
    }

    // Upload before sending. The panel would clear the composer after the
    // event, so cancel it and clear only the draft: the chips stay until the
    // turn is accepted, and a failure keeps them for a retry.
    if (typeof event.preventDefault === 'function') event.preventDefault();
    const fromDraft = detail.fromDraft !== false;
    if (typeof element.clearComposer === 'function') {
      element.clearComposer({ draft: fromDraft, attachments: false });
    }
    setAttachmentState(element, items, { busy: true });
    void deliver(text, model, files, effort).then((sent) => {
      if (sent) {
        for (const item of items) {
          const id = item && typeof item === 'object' ? item.id : '';
          if (typeof id === 'string' && id !== '' && typeof element.removeAttachment === 'function') {
            element.removeAttachment(id);
          }
        }
        return;
      }
      setAttachmentState(element, items, { error: localError || 'The files could not be uploaded.' });
      // Put the draft back so the person can retry without retyping; never
      // clobber words typed while the upload was running.
      if (fromDraft && element.draft === '') element.draft = text;
      paint();
    });
  }

  function onStop() {
    void controller.stop();
  }

  function onErrorDismiss() {
    dismissedError = localError || errorText(state().error)
      || errorText(state().turn && state().turn.error);
    localError = null;
    dismissedError = dismissedError || '';
  }

  function onLoadMore() {
    void controller.loadMore();
  }

  async function decide(proposalId, decision) {
    localError = null;
    paint();
    try {
      await controller.decide(proposalId, decision);
    } catch (err) {
      localError = proposalErrorText(err) || errorText(err);
      paint();
    }
  }

  /**
   * The function that turns one attachment chip click into bytes. A host can
   * override it with `options.download`; otherwise the controller's client
   * fetches the Blob (the token stays in the Authorization header) and it is
   * saved through a temporary anchor.
   */
  function downloader() {
    if (typeof options.download === 'function') return options.download;
    // ChatController keeps its transport private; this is the only place the
    // binding reads it, and `options.client` is the explicit alternative.
    const client = options.client ?? controller._client ?? null;
    if (!client || typeof client.downloadUpload !== 'function') return null;
    return async (uploadId, reference) => {
      const blob = await client.downloadUpload(uploadId);
      saveBlob(blob, reference?.label || reference?.name || uploadId);
    };
  }

  function download(uploadId, reference) {
    const run = downloader();
    if (!run) {
      localError = 'This attachment cannot be downloaded here.';
      paint();
      return;
    }
    void Promise.resolve(run(uploadId, reference)).catch((err) => {
      localError = errorText(err);
      paint();
    });
  }

  function onAction(event) {
    const detail = event && event.detail && typeof event.detail === 'object' ? event.detail : {};
    if (detail.action === PROPOSAL_ACTION) {
      const id = typeof detail.id === 'string' ? detail.id : '';
      const decision = detail.decision === 'approve' || detail.decision === 'reject'
        ? detail.decision
        : '';
      if (id === '' || decision === '') return;
      if (typeof event.preventDefault === 'function') event.preventDefault();
      void decide(id, decision);
      return;
    }
    if (detail.action === 'attachment-download') {
      const id = detail.id === undefined || detail.id === null ? '' : String(detail.id);
      if (id === '') return;
      if (typeof event.preventDefault === 'function') event.preventDefault();
      download(id, { type: 'attachment', id, label: detail.name });
    }
  }

  function onReferenceOpen(event) {
    const reference = event && event.detail ? event.detail.reference : null;
    if (!reference || reference.type !== 'attachment') return;
    const id = reference.id === undefined || reference.id === null ? '' : String(reference.id);
    if (id === '') return;
    download(id, reference);
  }

  listen(controller, 'change', paint);
  listen(element, 'agent-submit', onSubmit);
  listen(element, 'agent-model-change', onModelChange);
  listen(element, 'agent-stop', onStop);
  listen(element, 'agent-error-dismiss', onErrorDismiss);
  listen(element, 'agent-action', onAction);
  listen(element, 'agent-reference-open', onReferenceOpen);
  listen(element, 'agent-compact', () => {
    if (contextMode !== 'off') void controller.compact();
  });

  if (lists) {
    listen(element, 'agent-select-conversation', (event) => {
      // The binding owns selection; never let a row navigate away.
      if (typeof event.preventDefault === 'function') event.preventDefault();
      const id = event.detail?.id;
      if (id === undefined || id === null) return;
      void controller.open(String(id));
    });
    listen(element, 'agent-new-conversation', () => {
      // A draft: the first message creates the conversation, so "New chat"
      // pressed twice leaves no empty conversations behind. Without
      // autoCreate there is no first-message creation, so create it now.
      if (autoCreate && typeof controller.startDraft === 'function') controller.startDraft();
      else void controller.newConversation();
    });
    listen(element, 'agent-delete-conversation', (event) => {
      const id = event.detail?.id;
      if (id === undefined || id === null) return;
      void controller.remove(String(id));
    });
    listen(element, 'agent-load-more', onLoadMore);
  }
  if (page) {
    listen(element, 'agent-rename', (event) => {
      const id = state().activeConversationId;
      const title = typeof event.detail?.title === 'string' ? event.detail.title.trim() : '';
      if (!id || title === '') {
        if (typeof event.preventDefault === 'function') event.preventDefault();
        return;
      }
      void controller.rename(String(id), title);
    });
  }

  paint();

  return {
    unbind() {
      if (unbound) return;
      unbound = true;
      for (const [target, type, handler] of listeners) target.removeEventListener(type, handler);
      listeners.length = 0;
      if (contextButton) contextButton.remove();
      contextButton = null;
    },
    refresh: paint,
    loadMore() {
      if (unbound) return Promise.resolve(state());
      return controller.loadMore();
    },
  };
}
