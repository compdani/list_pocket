// Pure helpers for the assistant side panel's chats.

// Removes a chat and says which should be open next: the one that took its place in the
// list, or null if none are left.
export function removeThread(threads, id, activeId) {
  const index = threads.findIndex((thread) => thread.id === id);
  if (index < 0) return { threads, active: activeId };
  const next = threads.filter((thread) => thread.id !== id);
  if (id !== activeId) return { threads: next, active: activeId };
  const neighbour = next[Math.min(index, next.length - 1)];
  return { threads: next, active: neighbour ? neighbour.id : null };
}

// Runs of tool calls collapse into one row, so they don't push the answer down the panel.
export function groupToolRows(rows) {
  const out = [];
  rows.forEach((row) => {
    if (row.type !== 'tool') {
      out.push(row);
      return;
    }
    const last = out[out.length - 1];
    if (last?.type === 'tools') last.tools.push(row.tool);
    else out.push({ type: 'tools', id: row.id, tools: [row.tool] });
  });
  return out;
}

export function toolLabel(name) {
  const text = String(name || '').replace(/^propose_/, '').replace(/_/g, ' ');
  return text.charAt(0).toUpperCase() + text.slice(1);
}

export function toolGroupStatus(tools) {
  if (tools.some((tool) => tool.status === 'running')) return 'running';
  return tools.some((tool) => tool.status === 'failed') ? 'failed' : 'completed';
}
