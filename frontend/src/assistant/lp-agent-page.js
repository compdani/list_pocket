// <lp-agent-page>: the agent page (the agent-panel package's <agent-page>) as List Pocket
// uses it: the app's theme, markdown, proposal cards, icons for context references, and
// the server's size limits for attachments. views/Agent.vue supplies the data.
import { AgentPageElement } from 'agent-panel/page-element';
import { fileProblem } from '../agent/parts.mjs';
import {
  LP_LABELS, LP_STYLES, referenceIcon, renderMarkdown, renderProposal,
} from './lp-agent-common';

// What `agent/attachments` takes: images and PDFs up to 6 MB, text files up to 2 MB.
export const AGENT_PAGE_ACCEPT = 'image/*,application/pdf,text/*,.csv,.json,.txt,.md,.tsv,.html,.xml,.yaml,.yml';

export class ListPocketAgentPage extends AgentPageElement {
  static tagName = 'lp-agent-page';

  static styles = LP_STYLES;

  static defaultLabels = { ...AgentPageElement.defaultLabels, ...LP_LABELS };

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

  // Text files have a lower limit than images and PDFs.
  fileProblem(file, current) {
    return fileProblem(file) || super.fileProblem(file, current);
  }
}

ListPocketAgentPage.define();
