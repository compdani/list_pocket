// <lp-agent-page>: the agent page (the agent-panel package's <agent-page>) as List Pocket
// uses it: the app's theme, markdown, proposal cards, and icons for context references.
// views/AgentChat.vue sets `chat` to its ChatController and binds it.
//
// The overridden `entries` setter copies busy/running onto proposal cards. That
// setter is the single choke point: the binding writes `entries` on every
// controller change, so no watcher can race it.
import { AgentPageElement } from 'agent-panel/page-element';
import {
  LP_LABELS, LP_STYLES, referenceIcon, renderMarkdown,
} from './lp-agent-common';

export class ListPocketAgentPage extends AgentPageElement {
  static tagName = 'lp-agent-page';

  static styles = LP_STYLES;

  static defaultLabels = { ...AgentPageElement.defaultLabels, ...LP_LABELS };

  constructor() {
    super();
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

  referenceIcon(ref) {
    return referenceIcon(ref);
  }
}

ListPocketAgentPage.define();
