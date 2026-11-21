// <lp-agent-panel>: the agent panel (the agent-panel package, github.com/compdani/agent-panel)
// as List Pocket uses it. Adds markdown replies, the app's theme, tool labels
// and cards for the proposals the assistant makes. components/AIAssistantChat.vue
// binds it to the chat controller. See AGENTS.md in this folder.
import { AgentPanelElement } from 'agent-panel/element';
import { labelToolEntries, toolLabel } from './chat-host.mjs';
import {
  LP_LABELS, LP_STYLES, referenceIcon, renderMarkdown,
} from './lp-agent-common';

export class ListPocketAgentPanel extends AgentPanelElement {
  static tagName = 'lp-agent-panel';

  static styles = LP_STYLES;

  static defaultLabels = { ...AgentPanelElement.defaultLabels, ...LP_LABELS };

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
  // next binding write.
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

ListPocketAgentPanel.define();
