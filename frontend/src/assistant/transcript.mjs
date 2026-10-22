export function anchorLegacyActivity(thread) {
  if (Number.isInteger(thread.legacyMessageIndex)) return;
  const referenced = new Set(thread.messages.flatMap(message => (message.parts || []).map(part => part.id)));
  const hasLegacy = (thread.tools || []).some(tool => !referenced.has(tool.key || tool.id))
    || (thread.proposals || []).some(proposal => !referenced.has(proposal.id));
  if (hasLegacy) thread.legacyMessageIndex = thread.messages.findLastIndex(message => message.role === 'assistant');
}

// Keep the provider-facing message text intact, while recording the order in
// which text, tool calls and review proposals appeared in the conversation.
export function appendAssistantEvent(thread, event) {
  anchorLegacyActivity(thread);
  const message = thread.messages.at(-1);
  if (!message || message.role !== 'assistant') return;
  message.parts ||= message.content ? [{ type: 'text', content: message.content }] : [];
  if (event.type === 'text_delta') {
    message.content += event.data.text;
    const last = message.parts.at(-1);
    if (last?.type === 'text') last.content += event.data.text;
    else message.parts.push({ type: 'text', content: event.data.text });
  } else if (event.type === 'tool') {
    const index = thread.tools.findIndex(tool => tool.id === event.data.id && tool.turn === thread.turn);
    if (index < 0) {
      const key = `${thread.turn}:${event.data.id}`;
      thread.tools.push({ ...event.data, key, turn: thread.turn, messageIndex: thread.messages.length - 1 });
      message.parts.push({ type: 'tool', id: key });
    } else Object.assign(thread.tools[index], event.data);
  } else if (event.type === 'proposal') {
    if (thread.proposals.some(proposal => proposal.id === event.data.id)) return;
    thread.proposals.push({ ...event.data, turn: thread.turn, editorKey: thread.editorKey });
    message.parts.push({ type: 'proposal', id: event.data.id });
  }
}

export function conversationTranscript(thread) {
  if (!thread) return [];
  const tools = new Map((thread.tools || []).map(tool => [tool.key || tool.id, tool]));
  const proposals = new Map((thread.proposals || []).map(proposal => [proposal.id, proposal]));
  const referencedTools = new Set(); const referencedProposals = new Set();
  for (const message of thread.messages) {
    for (const part of message.parts || []) {
      if (part.type === 'tool') referencedTools.add(part.id);
      if (part.type === 'proposal') referencedProposals.add(part.id);
    }
  }
  // Earlier versions stored tools in a separate list, without turn IDs. Keep
  // that history visible before the last answer rather than guessing old turns.
  const legacyTools = [...tools.values()].filter(tool => !referencedTools.has(tool.key || tool.id));
  const legacyProposals = [...proposals.values()].filter(proposal => !referencedProposals.has(proposal.id));
  const lastAssistant = thread.legacyMessageIndex ?? thread.messages.findLastIndex(message => message.role === 'assistant' && !message.parts);
  const rows = [];
  thread.messages.forEach((message, index) => {
    if (index === lastAssistant && legacyTools.length) {
      rows.push({ type: 'legacy_tools', id: 'legacy-tools', tools: legacyTools });
    }
    const parts = message.role === 'user' || !message.parts
      ? (message.content ? [{ type: 'text', content: message.content }] : []) : message.parts;
    parts.forEach((part, partIndex) => {
      const id = `${index}:${partIndex}`;
      if (part.type === 'text' && part.content) rows.push({ ...part, id, role: message.role });
      else if (part.type === 'tool' && tools.has(part.id)) rows.push({ type: 'tool', id, tool: tools.get(part.id) });
      else if (part.type === 'proposal' && proposals.has(part.id)) rows.push({ type: 'proposal', id, proposal: proposals.get(part.id) });
    });
    if (index === lastAssistant) {
      for (const proposal of legacyProposals) rows.push({ type: 'proposal', id: `legacy:${proposal.id}`, proposal });
    }
  });
  if (lastAssistant < 0 && legacyTools.length) rows.push({ type: 'legacy_tools', id: 'legacy-tools', tools: legacyTools });
  return rows;
}
