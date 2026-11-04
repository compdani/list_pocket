// Serialize immutable draft snapshots. A failed write rejects its caller but
// does not poison later saves; flush callers can stop publish/test/navigation.
export function createDraftSaver(write) {
  let tail = Promise.resolve();
  return (id, draft) => {
    const snapshot = JSON.parse(JSON.stringify(draft));
    const result = tail.then(() => write(id, snapshot));
    tail = result.catch(() => {});
    return result;
  };
}

// A flush only succeeds once the latest revision is durable. Changing documents
// while flushing is a failed flush, never permission to adopt another draft.
export function createDraftFlusher(save, read) {
  return async () => {
    for (;;) {
      const { id, revision, draft } = read();
      const out = await save(id, draft);
      const current = read();
      if (current.id !== id) throw new Error('Draft changed while saving');
      if (current.revision === revision) return { id, revision, out };
    }
  };
}
