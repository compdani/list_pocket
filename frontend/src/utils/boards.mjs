// Board helpers. Cards are moved here first so the board responds at once, and the server
// then decides the final order.

export const STAGE_COLORS = ['default', 'info', 'success', 'warning', 'error', 'primary', 'secondary'];

// Where a dragged card lands: the number of cards whose middle is above the pointer.
export function dropIndex(midpoints, y) {
  const i = midpoints.findIndex((mid) => y < mid);
  return i === -1 ? midpoints.length : i;
}

// Returns stages with a card moved. The index doesn't count the card itself. A null stage
// takes the card off the board.
export function moveCard(stages, subscriber, toStage, index = 0) {
  const card = stages.flatMap((s) => s.cards).find((c) => c.subscriber === subscriber);
  if (!card) return stages;
  return stages.map((stage) => {
    const kept = stage.cards.filter((c) => c.subscriber !== subscriber);
    const removed = stage.cards.length - kept.length;
    if (stage.key !== toStage) {
      return { ...stage, cards: kept, total: stage.total - removed };
    }
    const at = Math.min(Math.max(index, 0), kept.length);
    return {
      ...stage,
      cards: [...kept.slice(0, at), card, ...kept.slice(at)],
      total: stage.total - removed + 1,
    };
  });
}

// Returns keys with one moved up (-1) or down (1), or unchanged if it can't go further.
export function shiftKey(keys, key, delta) {
  const from = keys.indexOf(key);
  const to = from + delta;
  if (from === -1 || to < 0 || to >= keys.length) return keys;
  const out = [...keys];
  out.splice(from, 1);
  out.splice(to, 0, key);
  return out;
}
