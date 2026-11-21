// What List Pocket adds to both <lp-agent-panel> and <lp-agent-page>: the app's theme,
// markdown, proposal cards and icons for context references. Defines no tags.
import {
  mdiAccountOutline, mdiFileImageOutline, mdiFormatListBulletedSquare, mdiForumOutline, mdiPaperclip,
  mdiPlayOutline, mdiRocketLaunchOutline, mdiSourceBranch,
} from '@mdi/js';
import { h, icon } from 'agent-panel/element';
import { proposalInfo, proposalTitle } from '../agent/parts.mjs';
import { renderAssistantMarkdown } from './markdown.mjs';

const STATUS_TONES = {
  pending: 'warning', applying: 'info', applied: 'success', approved: 'success', failed: 'error', discarded: 'muted',
  rejected: 'muted', expired: 'muted', unknown: 'warning', stale: 'warning',
};

// The same types as CONTEXT_ICONS in agent/parts.mjs, as SVG paths.
const REFERENCE_ICONS = {
  contact: mdiAccountOutline,
  conversation: mdiForumOutline,
  campaign: mdiRocketLaunchOutline,
  list: mdiFormatListBulletedSquare,
  template: mdiFileImageOutline,
  workflow: mdiSourceBranch,
  run: mdiPlayOutline,
  file: mdiPaperclip,
};

export const LP_LABELS = {
  discard: 'Discard',
  open: 'Open',
  proposedEdit: 'Proposed edit',
  review: 'Review',
  view: 'View',
};

export const LP_STYLES = `
:host {
  --agent-primary: rgb(var(--v-theme-primary, 37, 99, 235));
  --agent-on-primary: rgb(var(--v-theme-on-primary, 255, 255, 255));
  --agent-surface: rgb(var(--v-theme-surface, 255, 255, 255));
  --agent-surface-alt: rgb(var(--v-theme-surface-light, 243, 244, 246));
  --agent-text: rgb(var(--v-theme-on-surface, 31, 35, 40));
  --agent-muted: rgba(var(--v-theme-on-surface, 31, 35, 40), var(--v-medium-emphasis-opacity, .62));
  --agent-border: rgba(var(--v-border-color, 0, 0, 0), .22);
  --agent-error: rgb(var(--v-theme-error, 198, 40, 40));
  --agent-success: rgb(var(--v-theme-success, 46, 125, 50));
  --agent-warning: rgb(var(--v-theme-warning, 178, 106, 0));
  --agent-info: rgb(var(--v-theme-info, 2, 119, 189));
}
.proposal-head { align-items: flex-start; display: flex; gap: 8px; }
.proposal-title { flex: 1; font-size: 13px; font-weight: 600; min-width: 0; overflow-wrap: anywhere; }
.proposal-kind { color: var(--agent-muted); font-size: 12px; font-weight: 400; }
.proposal-error { color: var(--agent-error); font-size: 12px; margin-top: 4px; }
.proposal-state { color: var(--agent-muted); font-size: 12px; line-height: 1.45; margin: 6px 0 0; }
.proposal-state.tone-warning { border-radius: 6px; padding: 4px 8px; }
.proposal .alert { margin-top: 8px; }
.op { margin-top: 10px; }
.op-field { font-size: 12px; font-weight: 500; margin-bottom: 4px; }
.op-before, .op-after { border-radius: 6px; font-family: inherit; font-size: 12px; line-height: 1.5; margin: 0 0 4px; max-height: 180px; overflow: auto; padding: 6px 8px; white-space: pre-wrap; word-break: break-word; }
.op-before { background: color-mix(in srgb, var(--agent-error) 8%, transparent); text-decoration: line-through; text-decoration-color: color-mix(in srgb, var(--agent-error) 50%, transparent); }
.op-after { background: color-mix(in srgb, var(--agent-success) 10%, transparent); }
.actions { align-items: center; display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }
.actions:empty { display: none; }
`;

export const renderMarkdown = renderAssistantMarkdown;

export function referenceIcon(ref) {
  return icon(REFERENCE_ICONS[ref.type] || 'link', 13);
}

// { proposal, busy, running, actionId? }: busy while a decision is in flight, running
// while the agent is still answering. Buttons fire `agent-action` with
// { action: 'apply' | 'review' | 'discard', id: actionId ?? proposal.id }.
// A module-backed card adds `stateText` (what happened, in plain words) and a
// `statusLabel`; the unknown state keeps its warning tone and never reads as a
// success or a plain failure.
export function renderProposal(el, { proposal, busy = false, running = false, actionId }) {
  const { labels } = el;
  const info = proposalInfo(proposal.tool);
  const title = proposalTitle(proposal);
  const operations = proposal.arguments?.operations;
  const act = (action) => () => el.emitAction(action, { id: actionId ?? proposal.id });
  const button = (label, variant, action, attrs = {}) => h('button', {
    type: 'button', class: `btn btn--${variant}`, onClick: act(action), ...attrs,
  }, label);

  const actions = [];
  if (proposal.status === 'pending') {
    if (!info.reviewFirst) actions.push(button(info.apply, 'primary', 'apply', { disabled: busy || running, 'data-cy': 'assistant-apply' }));
    if (!operations) actions.push(button(labels.review, 'tonal', 'review', { 'data-cy': 'assistant-review' }));
    actions.push(button(labels.discard, 'text', 'discard', { disabled: busy || running, 'data-cy': 'assistant-discard' }));
  } else {
    if (!operations) actions.push(button(labels.view, 'text', 'review'));
    const href = proposal.result?.source?.href;
    if (proposal.status === 'applied' && href) actions.push(h('a', { class: 'btn btn--text', href, 'data-cy': 'assistant-open-result' }, labels.open));
  }

  return h(
    'article',
    { class: 'card proposal', part: 'proposal', 'data-cy': 'assistant-proposal' },
    h(
      'div',
      { class: 'proposal-head' },
      h(
        'div',
        { class: 'proposal-title' },
        title === proposal.tool ? labels.proposedEdit : title,
        info.label !== proposal.tool ? h('div', { class: 'proposal-kind' }, info.label) : null,
      ),
      proposal.status === 'pending' ? null : h('span', { class: `badge tone-${STATUS_TONES[proposal.status] || 'muted'}`, 'data-cy': 'assistant-proposal-status' }, proposal.statusLabel || proposal.status),
    ),
    (proposal.error || proposal.result?.error) ? h('div', { class: 'proposal-error' }, proposal.error || proposal.result?.error) : null,
    proposal.stateText ? h('p', { class: `proposal-state${proposal.status === 'unknown' ? ' tone-warning' : ''}` }, proposal.stateText) : null,
    (proposal.warnings || []).map((warning) => h('div', { class: 'alert tone-warning' }, h('span', null, warning))),
    // Edits to the open editor are short, so they show in place.
    (operations || []).map((op) => h(
      'div',
      { class: 'op' },
      h('div', { class: 'op-field' }, op.field || op.blockId || op.id || op.op),
      op.before ? h('pre', { class: 'op-before' }, op.before) : null,
      h('pre', { class: 'op-after' }, op.after || JSON.stringify(op.value)),
    )),
    h('div', { class: 'actions' }, actions),
  );
}
