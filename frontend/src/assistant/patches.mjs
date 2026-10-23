// A short content hash the model can echo back verbatim; the full JSON of a
// long body cannot be reproduced reliably.
export function revision(value) {
  const text = JSON.stringify(value) ?? '';
  let h1 = 0xdeadbeef; let h2 = 0x41c6ce57;
  for (let i = 0; i < text.length; i += 1) {
    const code = text.charCodeAt(i);
    h1 = Math.imul(h1 ^ code, 2654435761); h2 = Math.imul(h2 ^ code, 1597334677);
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909);
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909);
  return `r${(h2 >>> 0).toString(36)}${(h1 >>> 0).toString(36)}`;
}
export function applyEditorPatch(snapshot, proposal) {
  if (proposal.revision !== revision(snapshot)) throw new Error('The editor changed. Ask the assistant to refresh its proposal.');
  const next = JSON.parse(JSON.stringify(snapshot));
  if (!Array.isArray(proposal.operations) || !proposal.operations.length) throw new Error('Empty proposal');
  for (const change of proposal.operations) {
    // The revision already pins the whole snapshot, so whole-value edits only
    // need their replacement; before is checked when it is supplied.
    if (['set', 'replace_text', 'replace_body', 'block'].includes(change.op) && typeof change.after !== 'string') throw new Error('Edits require replacement text');
    if (['replace_text', 'block'].includes(change.op) && typeof change.before !== 'string') throw new Error('Edits require original and replacement text');
    if (change.op === 'set' && ['subject', 'preheader'].includes(change.field)) {
      if (typeof change.before === 'string' && (next[change.field] ?? '') !== change.before) throw new Error('Field changed');
      next[change.field] = change.after;
    } else if (change.op === 'replace_text') {
      if (!change.before || next.body.split(change.before).length !== 2) throw new Error('Text must match exactly once; select a more specific passage.');
      next.body = next.body.replace(change.before, () => change.after);
    } else if (change.op === 'replace_body') {
      if (typeof change.before === 'string' && next.body !== change.before) throw new Error('Body changed');
      next.body = change.after;
    } else if (change.op === 'block' && next.contentType === 'visual') {
      const document = JSON.parse(next.body);
      if (!Object.hasOwn(document, change.blockId)) throw new Error('Block no longer exists');
      if (JSON.stringify(document[change.blockId]) !== JSON.stringify(JSON.parse(change.before))) throw new Error('Block changed');
      document[change.blockId] = JSON.parse(change.after);
      next.body = JSON.stringify(document);
    } else if (['node_upsert', 'node_remove', 'edge_upsert', 'edge_remove'].includes(change.op) && next.kind === 'workflow') {
      const items = change.op.startsWith('node') ? next.nodes : next.edges;
      const index = items.findIndex((item) => item.id === change.id);
      if (change.op.endsWith('remove')) {
        if (index < 0) throw new Error('Graph item no longer exists');
        items.splice(index, 1);
      } else {
        if (!change.value || change.value.id !== change.id) throw new Error('Invalid graph ID');
        if (index < 0) items.push(change.value); else items[index] = change.value;
      }
    } else throw new Error('Unsupported edit operation');
  }
  if (next.contentType === 'visual') JSON.parse(next.body);
  return next;
}
