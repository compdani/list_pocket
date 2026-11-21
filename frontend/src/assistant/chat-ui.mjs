// DOM glue shared by the module-backed Agent page and the side panel.
// Pure decisions stay in chat-host.mjs. This file talks to the element, the
// controller and the installed binding, which neither host should copy.

import { proposalErrorText, renderNoticeEntry } from '@pocketbase-mcp/chat-client/agent-panel';
import { cardProposalOf, reviewPartOf } from './chat-host.mjs';
import { renderProposal } from './lp-agent-common';

/** The @ chips of the submit in flight. The binding clears them before it reads context when the message has files. */
export function createSubmitCapture() {
  let submitReferences = null;
  return {
    onSubmit(event) {
      submitReferences = Array.isArray(event.detail?.references) ? event.detail.references : null;
    },
    consume(fallback) {
      const references = submitReferences ?? fallback;
      submitReferences = null;
      return references;
    },
    clear() {
      submitReferences = null;
    },
  };
}

/** Clear all unsent composer state (draft, attachments, references, mention picker) on account switch. */
export function clearHostComposer(el) {
  if (!el) return;
  if (typeof el.clearComposer === 'function') {
    el.clearComposer({ draft: true, attachments: true, references: true });
    return;
  }
  el.draft = '';
  el.references = [];
  el.clearAttachments?.();
  el.closeMention?.();
}

/**
 * Hold the composer closed until /status has answered. The binding only
 * disables it once features.ready is false, and it will not turn a composer
 * back on unless it disabled that composer itself. Returns the new held flag.
 */
export function holdComposerUntilReady(el, state, held) {
  if (!el) return held;
  if (!state || state.status !== 'ready') {
    el.disabled = true;
    return true;
  }
  if (!held) return false;
  if (state.features?.ready !== false) el.disabled = false;
  return false;
}

/** The proposal the card and the review modal show: the entry's copy, plus the controller's error (a 409 or a 422). */
export function proposalFor(chat, id, stored) {
  const live = [...(chat?.state?.proposals || []), ...(chat?.state?.decidedProposals || [])]
    .find((item) => item && String(item.id) === String(id));
  if (stored && typeof stored === 'object') return { ...stored, error: stored.error ?? live?.error };
  return live || null;
}

/** One proposal card from the binding's entry. The binding omits `error`, so that field is read from the controller. */
export function renderModuleProposal(entry, panel, { chat, busyId } = {}) {
  const full = proposalFor(chat, entry?.proposalId, entry?.proposal) || {
    id: entry?.proposalId,
    tool: entry?.tool,
    state: entry?.state,
    revision: entry?.revision,
    arguments: {},
    warnings: [],
  };
  const card = cardProposalOf(full, {
    busy: entry?.busy === true || busyId === entry?.proposalId,
    running: entry?.running === true || panel?.chat?.state?.running === true,
  });
  return card ? renderProposal(panel, card) : null;
}

/** The binding's notice, plus the Open link a decided proposal's stored result still carries. */
export function renderNoticeWithOpen(entry, panel) {
  const node = renderNoticeEntry(entry);
  const href = entry?.result?.source?.href;
  if (!node || typeof href !== 'string' || href === '') return node;
  const link = document.createElement('a');
  link.className = 'btn btn--text';
  link.href = href;
  link.setAttribute('data-cy', 'assistant-open-result');
  link.textContent = panel?.labels?.open || 'Open';
  node.append(document.createTextNode(' '), link);
  return node;
}

/**
 * Approve or reject one proposal. A succeeded approval fires `assistant-applied`.
 * A 409 or a 422 comes back as `{ error }` on the result; anything else thrown
 * is the same shape with no proposal, so the host can put it in a banner.
 */
export async function decideModuleProposal({
  chat, element, id, decision, edited, isBusy, setBusy, onStart,
} = {}) {
  if (!chat || !id || (typeof isBusy === 'function' && isBusy(id))) return { ok: false };
  if (typeof setBusy === 'function') setBusy(id);
  element?.busyProposals?.add(id);
  element?.repaint?.();
  if (typeof onStart === 'function') onStart();
  try {
    const out = await chat.decide(id, decision, edited ? { arguments: edited } : undefined);
    const stored = out?.proposal ?? null;
    if (out?.error) {
      return { ok: false, proposal: stored, error: proposalErrorText(out.error) };
    }
    if (decision === 'approve' && stored?.state === 'succeeded') {
      window.dispatchEvent(new CustomEvent('assistant-applied', { detail: { tool: stored.tool, result: stored.result } }));
    }
    const problem = stored ? reviewPartOf(stored)?.error || '' : '';
    return {
      ok: Boolean(stored) && stored.state !== 'failed',
      proposal: stored,
      error: stored?.state === 'failed' ? problem : '',
    };
  } catch (err) {
    return { ok: false, error: proposalErrorText(err) };
  } finally {
    if (typeof setBusy === 'function') setBusy('');
    element?.busyProposals?.delete(id);
    element?.repaint?.();
  }
}
