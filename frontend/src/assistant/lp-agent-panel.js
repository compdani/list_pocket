// <lp-agent-panel>: the agent panel (the agent-panel package, github.com/compdani/agent-panel)
// as List Pocket uses it. Adds markdown replies, the app's theme, and cards for the
// proposals the assistant makes. The Vue side panel (AIAssistant.vue) supplies the data
// and answers the cards' `agent-action` events. See AGENTS.md in this folder.
import { AgentPanelElement } from 'agent-panel/element';
import { labelToolEntries } from './chat-host.mjs';
import {
  LP_LABELS, LP_STYLES, referenceIcon, renderMarkdown, renderProposal,
} from './lp-agent-common';
import { toolLabel } from './threads.mjs';

// What the server accepts: images, PDFs and plain text.
export const ASSISTANT_ACCEPT = 'image/png,image/jpeg,image/gif,image/webp,application/pdf,text/plain,.txt';

export class ListPocketAgentPanel extends AgentPanelElement {
  static tagName = 'lp-agent-panel';

  static styles = LP_STYLES;

  static defaultLabels = { ...AgentPanelElement.defaultLabels, ...LP_LABELS };

  constructor() {
    super();
    // Set by the module host. The legacy host leaves it null, and then this
    // class passes `entries` through unchanged.
    this.chat = null;
    this.busyProposals = new Set();
    this.rawEntries = [];
  }

  get entries() {
    return super.entries;
  }

  set entries(value) {
    if (!this.chat) {
      super.entries = value;
      return;
    }
    this.rawEntries = Array.isArray(value) ? value : [];
    const running = this.chat?.state?.running === true;
    const labeled = labelToolEntries(this.rawEntries, toolLabel);
    super.entries = labeled.map((entry) => {
      if (!entry || entry.type !== 'proposal') return entry;
      const busy = this.busyProposals.has(entry.proposalId);
      if (!busy && !running) return entry;
      return { ...entry, ...(busy ? { busy: true } : {}), ...(running ? { running: true } : {}) };
    });
  }

  // Redraw with the same entries, so a decision's busy flag shows before the
  // next binding write. The legacy host never calls this.
  repaint() {
    if (!this.chat) return;
    this.entries = this.rawEntries;
  }

  renderMarkdown(text) {
    return renderMarkdown(text);
  }

  renderEntry(entry) {
    // The legacy host's card is `{ proposal, busy, running }` and has no
    // proposalId. A module entry also carries `proposal`, plus proposalId,
    // and its renderer is the one registered with defineEntry.
    if (entry.type === 'proposal' && entry.proposal && !entry.proposalId) return renderProposal(this, entry);
    return super.renderEntry(entry);
  }

  referenceIcon(ref) {
    return referenceIcon(ref);
  }
}

ListPocketAgentPanel.define();
