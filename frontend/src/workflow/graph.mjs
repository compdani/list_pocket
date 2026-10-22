// Deterministic top-to-bottom layout. Cycles are left together in a final row;
// arranging never rewrites IDs, connections or node configuration.
export function arrangeWorkflowGraph(graph) {
  const nodes = graph.nodes.map(node => ({ ...node }));
  const ids = new Set(nodes.map(node => node.id));
  const degree = new Map(nodes.map(node => [node.id, 0]));
  const outgoing = new Map(nodes.map(node => [node.id, []]));
  for (const edge of graph.edges) {
    if (!ids.has(edge.sourceNodeId) || !ids.has(edge.targetNodeId)) continue;
    degree.set(edge.targetNodeId, degree.get(edge.targetNodeId) + 1);
    outgoing.get(edge.sourceNodeId).push(edge.targetNodeId);
  }
  const queue = nodes.filter(node => degree.get(node.id) === 0).map(node => node.id);
  const rank = new Map(queue.map(id => [id, 0]));
  for (let i = 0; i < queue.length; i += 1) {
    for (const target of outgoing.get(queue[i])) {
      rank.set(target, Math.max(rank.get(target) || 0, rank.get(queue[i]) + 1));
      degree.set(target, degree.get(target) - 1);
      if (degree.get(target) === 0) queue.push(target);
    }
  }
  const visited = new Set(queue);
  const lastRow = Math.max(0, ...rank.values()) + 1;
  const rows = new Map();
  for (const node of nodes) {
    const row = visited.has(node.id) ? rank.get(node.id) : lastRow;
    if (!rows.has(row)) rows.set(row, []);
    rows.get(row).push(node);
  }
  const maxWidth = Math.max(1, ...[...rows.values()].map(row => row.length));
  for (const [rank, row] of rows) row.forEach((node, index) => {
    node.positionX = 80 + (maxWidth - row.length) * 150 + index * 300;
    node.positionY = 80 + rank * 200;
  });
  return { ...graph, nodes };
}
export function canConnectWorkflowNodes(nodes, edges, source, target, handle, edgeId) {
  if (!source || !target || source === target) return false;
  const from = nodes.find(node => node.id === source);
  const to = nodes.find(node => node.id === target);
  if (!from || !to || to.data?.type === 'trigger') return false;
  return !edges.some(edge => (!edgeId || edge.id !== edgeId) && edge.source === source && edge.target === target && (edge.sourceHandle || '') === (handle || ''));
}
