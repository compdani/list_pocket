// Logic for both module-backed hosts, independent of Vue: the page context a
// turn sends, stored references, proposal card and ReviewModal inputs, error
// wording, and (for the side panel) media pictures and the editor adapter.
//
// The Vue hosts only wire these to the chat client, the controller and the
// elements. Keeping them here makes them testable with `node --test`
// (tests/chatHost.test.mjs), and keeps this module free of imports so the
// legacy hosts and the route loader don't pull in the chat client.
//
// The chat module's stored page context is one `context` part on a user message
// (StoreContext). Its `result` is the object the browser sent:
//   { "screen": "agent", "path": "/agent/1",
//     "references": [{"type": "contact", "id": "…", "label": "…"}] }
// referencesFromContext() is the bindAgentPanel `contextReferences` option:
// the binding puts those chips on the user message. contextReferencesFromTranscript()
// is the "In context" list; the module has no equivalent.

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

/** Plain-English label for one assistant tool name. */
export function toolLabel(name) {
  const text = String(name || '').replace(/^propose_/, '').replace(/_/g, ' ');
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/**
 * Remove the legacy side panel's localStorage keys on load. Keeps
 * `listpocket_agent_model`, `listpocket_assistant_pictures_v1:*`, and
 * `listpocket_ai_builder_threads_v1` (still used by TemplateForm's AI dialog).
 */
export function clearLegacyAssistantStorage(storage) {
  if (!storage || typeof storage.removeItem !== 'function') return;
  try {
    const keys = [];
    if (typeof storage.length === 'number' && typeof storage.key === 'function') {
      for (let i = 0; i < storage.length; i += 1) {
        const key = storage.key(i);
        if (typeof key === 'string' && key.startsWith('listpocket_assistant_v1:')) keys.push(key);
      }
    }
    keys.push('listpocket_ai_builder_legacy_owner');
    for (const key of keys) storage.removeItem(key);
  } catch {
    // Storage access can be blocked in private contexts.
  }
}

/**
 * Chips for one stored page context, for bindAgentPanel's `contextReferences`
 * option. Same `{ type, id, label }` list the composer sent. The binding
 * places them on that user message, ahead of its attachment chips, and drops
 * a chip that has no label.
 */
export function referencesFromContext(context) {
  if (!context || typeof context !== 'object') return [];
  // The binding drops a chip without a label; a stored reference may lack one.
  return referencesOf(context.references).map((ref) => (ref.label ? ref : { ...ref, label: ref.id }));
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

// What happened, in the same plain words the installed binding uses. Copied
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
// the legacy engine did (cmd/ai_assistant.go runAssistant). chatErrorText()
// is bindAgentPanel's errorText option, so the wording is chosen by code.
export const PROVIDER_AUTH_TEXT = 'The AI provider did not accept this app\'s credentials. Check the API key, or sign in to ChatGPT again under Settings → AI.';
export const USAGE_LIMIT_TEXT = 'Your ChatGPT plan\'s usage limit has been reached. Wait for it to reset, or switch models.';

/**
 * The line bindAgentPanel should show for a code it knows, or undefined to
 * keep the binding's own English. `noProviderText` is the translated
 * agent.noProvider string; this module does not import vue-i18n.
 */
export function chatErrorText(code, noProviderText) {
  if (code === 'provider_auth') return PROVIDER_AUTH_TEXT;
  if (code === 'usage_limit') return USAGE_LIMIT_TEXT;
  if (code === 'no_provider' && typeof noProviderText === 'string' && noProviderText !== '') return noProviderText;
  return undefined;
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

// --- Side panel (screen "panel") -------------------------------------------
//
// The module side panel sends the same page context the legacy panel posts
// (path, page, params, filters, the open editor, media pictures) plus the
// @ references of the message. Editor patches are applied here, in the
// browser; the controller reports the outcome.

/** The screen name cmd/chat_module.go treats as the side panel. */
export const PANEL_SCREEN = 'panel';

/** localStorage prefix for media pictures, one map per signed-in user. */
export const PANEL_PICTURES_PREFIX = 'listpocket_assistant_pictures_v1:';

/** Pictures added before the draft's first send has a conversation id. */
export const PANEL_DRAFT_PICTURES = 'draft';

export const WRONG_EDITOR_TEXT = 'Open the original editor before applying this proposal.';
export const MID_APPLY_TEXT = 'The editor changed during approval. Ask for a fresh proposal.';
export const UNDO_CHANGED_TEXT = 'The editor changed after this edit; use its undo controls.';

function plainData(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  try {
    const cloned = JSON.parse(JSON.stringify(value));
    return cloned && typeof cloned === 'object' && !Array.isArray(cloned) ? cloned : {};
  } catch {
    return {};
  }
}

/** Media pictures as `{ name, url }`, de-duplicated by URL. */
export function cleanPictures(list) {
  if (!Array.isArray(list)) return [];
  const out = [];
  for (const picture of list) {
    if (!picture || typeof picture.url !== 'string' || picture.url === '') continue;
    if (out.some((item) => item.url === picture.url)) continue;
    out.push({ name: typeof picture.name === 'string' ? picture.name : '', url: picture.url });
  }
  return out;
}

/**
 * The open editor as the side panel's context carries it, or null when none
 * is open or its snapshot cannot be read. `selectionText` is the window
 * selection, used when the editor does not report one (the legacy panel's
 * `editor.selection?.() || window.getSelection()`).
 */
export function editorContextOf(editor, revisionOf, selectionText = '') {
  if (!editor || typeof editor.snapshot !== 'function') return null;
  const key = editor.key == null ? '' : String(editor.key);
  if (key === '') return null;
  let current;
  try {
    current = editor.snapshot();
  } catch {
    return null;
  }
  if (!current || typeof current !== 'object') return null;
  let selection = '';
  if (typeof editor.selection === 'function') {
    try {
      const value = editor.selection();
      if (typeof value === 'string') selection = value;
    } catch {
      selection = '';
    }
  }
  if (!selection && typeof selectionText === 'string') selection = selectionText;
  let body = current;
  try {
    body = JSON.parse(JSON.stringify(current));
  } catch {
    body = current;
  }
  return {
    key,
    unsaved: true,
    revision: typeof revisionOf === 'function' ? revisionOf(current) : '',
    current: body,
    selection: selection.slice(0, 4000),
    label: typeof editor.label === 'string' ? editor.label : '',
  };
}

/** The page context of one side-panel turn. Describes the page at send time. */
export function panelPageContext({
  path = '', page = '', params, filters, references, editor = null, pictures,
} = {}) {
  return {
    screen: PANEL_SCREEN,
    path: typeof path === 'string' ? path : '',
    page: typeof page === 'string' ? page : '',
    params: plainData(params),
    filters: plainData(filters),
    editor: editor || null,
    pictureReferences: cleanPictures(pictures),
    references: referencesOf(references),
  };
}

function pictureKey(userId) {
  return `${PANEL_PICTURES_PREFIX}${userId || 'signed-out'}`;
}

function readPictureMap(storage, userId) {
  try {
    const raw = storage.getItem(pictureKey(userId));
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

function writePictureMap(storage, userId, map) {
  try {
    storage.setItem(pictureKey(userId), JSON.stringify(map));
    return true;
  } catch {
    return false;
  }
}

/** Pictures kept for one conversation, or the draft when `conversationId` is empty. */
export function picturesFor(storage, userId, conversationId) {
  if (!storage || typeof storage.getItem !== 'function') return [];
  const map = readPictureMap(storage, userId);
  const key = conversationId ? String(conversationId) : PANEL_DRAFT_PICTURES;
  return cleanPictures(map[key]);
}

/** Replace one conversation's pictures. A full or blocked store leaves the previous value and returns false. */
export function savePictures(storage, userId, conversationId, pictures) {
  if (!storage || typeof storage.getItem !== 'function') return false;
  const map = readPictureMap(storage, userId);
  const key = conversationId ? String(conversationId) : PANEL_DRAFT_PICTURES;
  const clean = cleanPictures(pictures);
  if (clean.length === 0) delete map[key];
  else map[key] = clean;
  return writePictureMap(storage, userId, map);
}

/**
 * A draft's pictures move onto the conversation its first send creates.
 * Pictures already stored for that conversation stay, and the draft is cleared.
 */
export function moveDraftPictures(storage, userId, conversationId) {
  const id = conversationId ? String(conversationId) : '';
  if (!id) return picturesFor(storage, userId, '');
  const map = readPictureMap(storage, userId);
  const draft = cleanPictures(map[PANEL_DRAFT_PICTURES]);
  const existing = cleanPictures(map[id]);
  if (draft.length === 0) return existing;
  const merged = existing.slice();
  for (const picture of draft) {
    if (!merged.some((item) => item.url === picture.url)) merged.push(picture);
  }
  delete map[PANEL_DRAFT_PICTURES];
  map[id] = merged;
  writePictureMap(storage, userId, map);
  return merged;
}

/** Add tool labels the binding does not set, leaving a label it did set alone. */
export function labelToolEntries(entries, labelOf) {
  if (!Array.isArray(entries)) return [];
  const label = typeof labelOf === 'function' ? labelOf : (name) => String(name || '');
  return entries.map((entry) => {
    if (!entry || entry.type !== 'tools' || !Array.isArray(entry.tools)) return entry;
    return {
      ...entry,
      tools: entry.tools.map((tool) => {
        if (!tool || typeof tool !== 'object' || tool.label) return tool;
        return { ...tool, label: label(tool.name) };
      }),
    };
  });
}

function editorArgs(proposal) {
  const args = proposal?.arguments;
  const source = args && typeof args === 'object' && !Array.isArray(args) ? args : {};
  const revision = typeof source.baseRevision === 'string' && source.baseRevision !== ''
    ? source.baseRevision
    : source.revision;
  return { ...source, revision };
}

function proposalEditorId(proposal) {
  const args = proposal?.arguments;
  if (args && typeof args.editorId === 'string') return args.editorId;
  return '';
}

/**
 * Check before sending an approval to the server: if an editor proposal targets
 * an editor that is not currently open, refuse locally so the proposal stays
 * pending and the user can open that editor and apply it.
 */
export function editorPrecheckError(proposal, editor) {
  if (!proposal || (proposal.location !== 'editor' && proposal.tool !== 'propose_editor_patch')) return '';
  if (!editor || typeof editor.snapshot !== 'function' || editor.key == null || String(editor.key) === '') {
    return WRONG_EDITOR_TEXT;
  }
  const editorId = proposalEditorId(proposal);
  if (editorId !== String(editor.key)) return WRONG_EDITOR_TEXT;
  return '';
}

/**
 * Whether an editor proposal still applies to the open editor.
 * No editor, or a revision that is no longer current, is not applicable
 * (the controller reports stale). The wrong editor, or a patch the editor
 * cannot apply, is refused with a message (the controller reports failed).
 */
export function judgeEditorProposal(proposal, editor, { revisionOf, applyPatch } = {}) {
  if (!editor || typeof editor.snapshot !== 'function' || editor.key == null || String(editor.key) === '') {
    return { applicable: false, reason: 'stale' };
  }
  const editorId = proposalEditorId(proposal);
  if (editorId !== String(editor.key)) {
    return { applicable: false, reason: 'refused', message: WRONG_EDITOR_TEXT };
  }
  let before;
  try {
    before = editor.snapshot();
  } catch (err) {
    return { applicable: false, reason: 'refused', message: err?.message || WRONG_EDITOR_TEXT };
  }
  const revision = typeof revisionOf === 'function' ? revisionOf(before) : '';
  const args = editorArgs(proposal);
  if (args.revision !== revision) return { applicable: false, reason: 'stale' };
  if (typeof applyPatch !== 'function') {
    return { applicable: false, reason: 'refused', message: 'Empty proposal' };
  }
  try {
    const next = applyPatch(before, args);
    return {
      applicable: true, next, before, revision, editorKey: String(editor.key),
    };
  } catch (err) {
    return { applicable: false, reason: 'refused', message: err?.message || 'Empty proposal' };
  }
}

/**
 * The controller's editor adapter. `snapshot` names the open editor.
 * `validatePatch` returns false when the patch is stale and throws the legacy
 * message when the wrong editor is open. `applyPatch` applies the checked
 * snapshot, refuses if the editor changed since the check, and returns the
 * new revision. The controller reports the outcome; this adapter does not.
 * Undo stays with the host (`onApplied` hands it the before snapshot).
 */
export function createEditorAdapter({ getEditor, revisionOf, applyPatch, onApplied } = {}) {
  const plans = new Map();
  return {
    snapshot() {
      const editor = typeof getEditor === 'function' ? getEditor() : null;
      if (!editor || typeof editor.snapshot !== 'function' || editor.key == null || String(editor.key) === '') return null;
      try {
        return { editorId: String(editor.key), revision: revisionOf(editor.snapshot()) };
      } catch {
        return null;
      }
    },
    async validatePatch(proposal) {
      const id = proposal?.id == null ? '' : String(proposal.id);
      plans.delete(id);
      const verdict = judgeEditorProposal(proposal, typeof getEditor === 'function' ? getEditor() : null, { revisionOf, applyPatch });
      if (!verdict.applicable) {
        if (verdict.reason === 'refused') throw new Error(verdict.message || WRONG_EDITOR_TEXT);
        return false;
      }
      if (id !== '') plans.set(id, verdict);
      return true;
    },
    async applyPatch(proposal) {
      const id = proposal?.id == null ? '' : String(proposal.id);
      const plan = plans.get(id);
      plans.delete(id);
      const editor = typeof getEditor === 'function' ? getEditor() : null;
      if (!plan || !editor || typeof editor.apply !== 'function' || String(editor.key) !== plan.editorKey) {
        throw new Error(MID_APPLY_TEXT);
      }
      let current;
      try {
        current = editor.snapshot();
      } catch {
        throw new Error(MID_APPLY_TEXT);
      }
      if (revisionOf(current) !== plan.revision) throw new Error(MID_APPLY_TEXT);
      await editor.apply(plan.next);
      let afterSnap;
      try {
        afterSnap = editor.snapshot();
      } catch {
        throw new Error(MID_APPLY_TEXT);
      }
      const after = revisionOf(afterSnap);
      if (typeof onApplied === 'function') onApplied({ editor, before: plan.before, after });
      return { revision: after };
    },
  };
}

/** Whether Undo can restore `entry`. The editor must still be the one that was patched, unchanged since. */
export function undoLastEdit(entry, editor, revisionOf) {
  if (!entry || !editor || editor !== entry.editor || typeof editor.snapshot !== 'function') {
    return { ok: false, message: UNDO_CHANGED_TEXT };
  }
  let current;
  try {
    current = editor.snapshot();
  } catch {
    return { ok: false, message: UNDO_CHANGED_TEXT };
  }
  if (typeof revisionOf !== 'function' || revisionOf(current) !== entry.after) {
    return { ok: false, message: UNDO_CHANGED_TEXT };
  }
  return { ok: true, before: entry.before };
}

/**
 * The failed turn's user message, stored upload IDs, and context references to
 * send again, or null when Retry should not be offered. Only offered when an
 * accepted turn failed (`state.turn.error`), never on a pre-accept `state.error`
 * (where the binding already restored the refused draft in the composer).
 */
export function retryPayload(state) {
  if (!state || state.running || state.sending) return null;
  if (!state.turn || !state.turn.error || !Array.isArray(state.transcript)) return null;
  let text = '';
  let messageId = null;
  for (let i = state.transcript.length - 1; i >= 0; i -= 1) {
    const part = state.transcript[i];
    if (part && part.role === 'user' && part.type === 'text' && typeof part.text === 'string' && part.text.trim()) {
      text = part.text;
      messageId = part.messageId ?? null;
      break;
    }
  }
  if (!text) return null;
  const attachments = [];
  const references = [];
  if (messageId !== null) {
    for (const part of state.transcript) {
      if (!part || part.role !== 'user' || part.messageId !== messageId) continue;
      if (part.type === 'attachment' && typeof part.uploadId === 'string' && part.uploadId !== '') {
        if (!attachments.includes(part.uploadId)) attachments.push(part.uploadId);
      } else if (part.type === 'context') {
        const result = part.result && typeof part.result === 'object' ? part.result : part;
        for (const ref of referencesOf(result.references)) references.push(ref);
      }
    }
  }
  return { text, attachments, references };
}

/** The last user message to send again after a failed turn, or '' when Retry does not apply. */
export function retryText(state) {
  return retryPayload(state)?.text || '';
}
