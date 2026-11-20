// <lp-agent-page>: the agent page (the agent-panel package's <agent-page>) as List Pocket
// uses it: the app's theme, markdown, proposal cards, icons for context references, and
// the server's size limits for attachments. views/Agent.vue supplies the data.
//
// The module-backed host (views/AgentChat.vue) also sets `chat` to its ChatController.
// The overridden `entries` setter copies busy/running onto proposal cards. That
// setter is the single choke point: the binding writes `entries` on every
// controller change, so no watcher can race it.
import { AgentPageElement } from 'agent-panel/page-element';
import { fileProblem } from '../agent/parts.mjs';
import {
  LP_LABELS, LP_STYLES, referenceIcon, renderMarkdown, renderProposal,
} from './lp-agent-common';

// What `agent/attachments` takes: images and PDFs up to 6 MB, text files up to 2 MB.
// The module-backed host takes its accept and size limits from /status instead (the
// binding mirrors them onto the element), so neither this nor fileProblem applies there.
export const AGENT_PAGE_ACCEPT = 'image/*,application/pdf,text/*,.csv,.json,.txt,.md,.tsv,.html,.xml,.yaml,.yml';

export class ListPocketAgentPage extends AgentPageElement {
  static tagName = 'lp-agent-page';

  static styles = LP_STYLES;

  static defaultLabels = { ...AgentPageElement.defaultLabels, ...LP_LABELS };

  constructor() {
    super();
    // The module-backed host's controller (null on the legacy page) and the
    // proposals with a decision in flight, which the proposal renderer reads.
    this.chat = null;
    this.busyProposals = new Set();
    this.rawEntries = [];
  }

  get entries() {
    return super.entries;
  }

  set entries(value) {
    this.rawEntries = Array.isArray(value) ? value : [];
    // busy/running are host state (a decision in flight, a turn still running),
    // not fields the binding puts on the proposal entry. Copy them in here:
    // the element only redraws an entry whose JSON changed, and repaint()
    // assigns the same raw entries again so the flags take effect.
    const running = this.chat?.state?.running === true;
    const decorated = this.rawEntries.map((entry) => {
      if (!entry || entry.type !== 'proposal') return entry;
      const busy = this.busyProposals.has(entry.proposalId);
      if (!busy && !running) return entry;
      return { ...entry, ...(busy ? { busy: true } : {}), ...(running ? { running: true } : {}) };
    });
    super.entries = decorated;
  }

  // Redraw with the same entries, for host-only state the entries carry (a
  // proposal's busy flag). The next binding write does the same by itself.
  repaint() {
    this.entries = this.rawEntries;
  }

  renderMarkdown(text) {
    return renderMarkdown(text);
  }

  renderEntry(entry) {
    // The legacy host's card is `{ proposal, busy, running }` with no
    // proposalId. The module binding's entry also has a `proposal`, plus
    // proposalId, and its renderer is registered with defineEntry.
    if (entry.type === 'proposal' && entry.proposal && !entry.proposalId) return renderProposal(this, entry);
    return super.renderEntry(entry);
  }

  referenceIcon(ref) {
    return referenceIcon(ref);
  }

  // Text files have a lower limit than images and PDFs on the legacy host. The
  // module host takes its accept and size limits from /status (the binding
  // mirrors them onto the element), so this override would undercut them.
  fileProblem(file, current) {
    if (this.chat) return super.fileProblem(file, current);
    return fileProblem(file) || super.fileProblem(file, current);
  }
}

ListPocketAgentPage.define();
