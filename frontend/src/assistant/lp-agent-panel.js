// <lp-agent-panel>: the agent panel (the agent-panel package, github.com/compdani/agent-panel)
// as List Pocket uses it. Adds markdown replies, the app's theme, and cards for the
// proposals the assistant makes. The Vue side panel (AIAssistant.vue) supplies the data
// and answers the cards' `agent-action` events. See AGENTS.md in this folder.
import { AgentPanelElement } from 'agent-panel/element';
import {
  LP_LABELS, LP_STYLES, referenceIcon, renderMarkdown, renderProposal,
} from './lp-agent-common';

// What the server accepts: images, PDFs and plain text.
export const ASSISTANT_ACCEPT = 'image/png,image/jpeg,image/gif,image/webp,application/pdf,text/plain,.txt';

export class ListPocketAgentPanel extends AgentPanelElement {
  static tagName = 'lp-agent-panel';

  static styles = LP_STYLES;

  static defaultLabels = { ...AgentPanelElement.defaultLabels, ...LP_LABELS };

  renderMarkdown(text) {
    return renderMarkdown(text);
  }

  renderEntry(entry) {
    if (entry.type === 'proposal') return renderProposal(this, entry);
    return super.renderEntry(entry);
  }

  referenceIcon(ref) {
    return referenceIcon(ref);
  }
}

ListPocketAgentPanel.define();
