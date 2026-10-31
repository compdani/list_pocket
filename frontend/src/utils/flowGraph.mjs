// Converts between a workflow definition and canvas nodes and edges. Connections live on
// the definition: node.next, branch cases[].to and default, and foreach/loop body.

export const NODE_TYPES = {
  start: { label: 'Start', icon: 'mdi-play-circle-outline', color: '#2e9e6b', hint: 'Trigger and input' },
  agent: { label: 'AI agent', icon: 'mdi-creation-outline', color: '#8250df', hint: 'AI works with tools' },
  tool: { label: 'Action', icon: 'mdi-flash-outline', color: '#2f81f7', hint: 'Run one tool, no AI' },
  branch: { label: 'Branch', icon: 'mdi-source-branch', color: '#b58105', hint: 'Route on a condition' },
  foreach: { label: 'For each', icon: 'mdi-repeat', color: '#bf3989', hint: 'Repeat for every item' },
  loop: { label: 'Loop', icon: 'mdi-sync', color: '#bf3989', hint: 'Repeat until a condition' },
  review: { label: 'Review', icon: 'mdi-account-check-outline', color: '#d4640f', hint: 'A person approves or edits' },
  wait: { label: 'Wait', icon: 'mdi-timer-sand', color: '#6e7781', hint: 'Delay or wait for an event' },
  subworkflow: { label: 'Sub-workflow', icon: 'mdi-sitemap-outline', color: '#1b8a9a', hint: 'Run another workflow' },
  notify: { label: 'Notify', icon: 'mdi-bell-outline', color: '#6e7781', hint: 'Tell your team' },
  transform: { label: 'Transform', icon: 'mdi-code-json', color: '#6e7781', hint: 'Reshape data' },
  end: { label: 'End', icon: 'mdi-stop-circle-outline', color: '#cf4d45', hint: 'Set the output' },
};

// Types that can be added from the palette. Every workflow already has its one start.
export const PALETTE_TYPES = Object.keys(NODE_TYPES).filter((t) => t !== 'start');

// Actions with their own palette entry. They save as ordinary tool nodes.
export const TOOL_STEPS = {
  'boards.move_contact': { label: 'Move on board', icon: 'mdi-view-column-outline', color: '#1a7f64', hint: 'Put a contact in a stage', id: 'move_on_board' },
  'boards.remove_contact': { label: 'Remove from board', icon: 'mdi-close-box-outline', color: '#1a7f64', hint: 'Take a contact off a board', id: 'remove_from_board' },
};

// How a node is shown: its action if it has its own entry, otherwise its type.
export function nodeInfo(n) {
  return (n.type === 'tool' && TOOL_STEPS[n.tool]) || NODE_TYPES[n.type] || NODE_TYPES.transform;
}

function truncate(s, n) {
  const text = String(s || '');
  return text.length > n ? `${text.slice(0, n - 1)}…` : text;
}

export function sourceHandles(n) {
  switch (n.type) {
    case 'end':
      return [];
    case 'branch':
      return [
        ...(n.cases || []).map((c, i) => ({ id: `case-${i}`, label: c.if ? truncate(c.if, 18) : `case ${i + 1}` })),
        { id: 'default', label: 'else' },
      ];
    case 'foreach':
      return [{ id: 'body', label: 'each item' }, { id: 'next', label: 'after all' }];
    case 'loop':
      return [{ id: 'body', label: 'each time' }, { id: 'next', label: 'when done' }];
    default:
      return [{ id: 'next', label: '' }];
  }
}

function targetOf(n, handle) {
  if (handle === 'next') return n.next || '';
  if (handle === 'default') return n.default || '';
  if (handle === 'body') return n.body || '';
  const m = handle.match(/^case-(\d+)$/);
  return m ? (n.cases?.[+m[1]]?.to || '') : '';
}

export function nodeSummary(n, trigger) {
  switch (n.type) {
    case 'start': return triggerSummary(trigger);
    case 'agent': return n.agent?.system_prompt ? truncate(n.agent.system_prompt, 42) : 'No instructions yet';
    case 'tool':
      if (n.tool === 'boards.move_contact') return n.input?.board ? `${truncate(n.input.board, 20)} → ${truncate(n.input.stage || '?', 20)}` : 'No board chosen';
      if (n.tool === 'boards.remove_contact') return n.input?.board ? `Off ${truncate(n.input.board, 38)}` : 'No board chosen';
      return n.tool || 'No tool chosen';
    case 'foreach': return n.items || '';
    case 'loop': return n.until ? `until ${truncate(n.until, 34)}` : '';
    case 'review': return n.review?.title || '';
    case 'wait':
      if (n.wait?.event) return `event ${n.wait.event}`;
      return n.wait?.delay ? `for ${n.wait.delay}` : '';
    case 'subworkflow': return n.workflow || '';
    case 'notify': return truncate(n.message, 42);
    default: return '';
  }
}

export function triggerSummary(t) {
  switch (t?.type) {
    case 'cron': return `Schedule: ${t.spec || 'not set'}`;
    case 'event':
      if (t.event) return `When ${t.event}`;
      return t.collection ? `When ${t.collection} changes` : 'Event not set';
    case 'webhook': return 'Webhook';
    default: return 'Started by hand';
  }
}

// Lays nodes out top to bottom by distance from the start, side by side where they
// split. Nodes that nothing reaches go in a column to the right.
export function tidyLayout(def) {
  const byId = Object.fromEntries(def.nodes.map((n) => [n.id, n]));
  const depth = {};
  const start = def.nodes.find((n) => n.type === 'start');
  const queue = start ? [[start.id, 0]] : [];
  while (queue.length) {
    const [id, d] = queue.shift();
    if (!byId[id] || (depth[id] !== undefined && depth[id] >= d)) continue;
    if (d > def.nodes.length) continue;
    depth[id] = d;
    sourceHandles(byId[id]).forEach((h) => {
      const t = targetOf(byId[id], h.id);
      if (t) queue.push([t, d + 1]);
    });
  }
  const rows = {};
  let orphan = 0;
  def.nodes.forEach((n) => {
    const d = depth[n.id];
    if (d === undefined) {
      n.ui = { x: 900, y: orphan * 130 };
      orphan += 1;
      return;
    }
    (rows[d] ||= []).push(n);
  });
  Object.entries(rows).forEach(([d, row]) => {
    row.forEach((n, i) => {
      n.ui = { x: Math.round((i - (row.length - 1) / 2) * 280), y: +d * 140 };
    });
  });
}

// Positions new nodes. A workflow with no layout at all gets a full tidy.
export function ensureLayout(def) {
  if (def.nodes.length && def.nodes.some((n) => !n.ui)) {
    tidyLayout(def);
  }
}

export function toFlow(def, extras = {}) {
  const nodes = def.nodes.map((n) => ({
    id: n.id,
    type: 'flow',
    position: { x: n.ui?.x ?? 0, y: n.ui?.y ?? 0 },
    deletable: n.type !== 'start',
    data: {
      node: n,
      summary: nodeSummary(n, def.trigger),
      handles: sourceHandles(n),
      hasError: Boolean(extras.errors?.has(n.id)),
      status: extras.status?.[n.id],
      diff: extras.diff?.[n.id],
    },
  }));
  const ids = new Set(def.nodes.map((n) => n.id));
  const edges = [];
  def.nodes.forEach((n) => {
    sourceHandles(n).forEach((h) => {
      const target = targetOf(n, h.id);
      if (target && ids.has(target)) {
        edges.push({
          id: `${n.id}:${h.id}`, source: n.id, sourceHandle: h.id, target, label: h.label || undefined, type: 'smoothstep',
        });
      }
    });
  });
  return { nodes, edges };
}

export function connect(def, { source, sourceHandle, target }) {
  const n = def.nodes.find((x) => x.id === source);
  if (!n || source === target) return;
  const h = sourceHandle || 'next';
  if (h === 'next' || h === 'default' || h === 'body') {
    n[h] = target;
    return;
  }
  const m = h.match(/^case-(\d+)$/);
  if (m && n.cases?.[+m[1]]) n.cases[+m[1]].to = target;
}

export function disconnect(def, edgeId) {
  const [source, h] = edgeId.split(':');
  const n = def.nodes.find((x) => x.id === source);
  if (!n) return;
  if (h === 'next' || h === 'default' || h === 'body') {
    delete n[h];
    return;
  }
  const m = h.match(/^case-(\d+)$/);
  if (m && n.cases?.[+m[1]]) n.cases[+m[1]].to = '';
}

export function removeNode(def, id) {
  def.nodes = def.nodes.filter((n) => n.id !== id);
  def.nodes.forEach((n) => {
    ['next', 'default', 'body'].forEach((f) => {
      if (n[f] === id) delete n[f];
    });
    (n.cases || []).forEach((c) => {
      if (c.to === id) c.to = '';
    });
  });
}

export function uniqueId(def, base) {
  const ids = new Set(def.nodes.map((n) => n.id));
  let id = base;
  let i = 2;
  while (ids.has(id)) {
    id = `${base}_${i}`;
    i += 1;
  }
  return id;
}

// A tool names one of TOOL_STEPS, for a node that starts as that action.
export function newNode(def, type, position, tool = '') {
  const n = { id: uniqueId(def, TOOL_STEPS[tool]?.id || type), type, ui: position };
  if (type === 'agent') n.agent = { system_prompt: '', tools: [] };
  if (type === 'branch') n.cases = [{ if: 'true', to: '' }];
  if (type === 'foreach') { n.items = ''; n.concurrency = 4; }
  if (type === 'loop') { n.until = ''; n.max_iterations = 10; }
  if (type === 'review') n.review = { title: 'Please review', on_deny: 'skip' };
  if (type === 'wait') n.wait = { delay: '1h' };
  if (type === 'notify') n.message = '';
  if (type === 'transform') n.value = {};
  if (type === 'tool') { n.tool = tool; n.input = tool ? { contact: '{{ input.contact }}' } : {}; }
  return n;
}

const ID_RE = /^[a-z][a-z0-9_]{0,40}$/;

// Renames a node, updating connections and nodes.<id> in templates. Returns null if the
// new ID can't be used.
export function renameNode(def, oldId, newId) {
  if (!ID_RE.test(newId) || def.nodes.some((n) => n.id === newId)) return null;
  const json = JSON.stringify(def).replace(new RegExp(`nodes\\.${oldId}\\b`, 'g'), `nodes.${newId}`);
  const next = JSON.parse(json);
  next.nodes.forEach((n) => {
    if (n.id === oldId) n.id = newId;
    ['next', 'default', 'body'].forEach((f) => {
      if (n[f] === oldId) n[f] = newId;
    });
    (n.cases || []).forEach((c) => {
      if (c.to === oldId) c.to = newId;
    });
  });
  return next;
}

// Node IDs named in validation messages, such as: node "x": tool is required.
export function errorNodeIds(messages = []) {
  const ids = new Set();
  messages.forEach((m) => {
    [...String(m).matchAll(/node "([a-z0-9_]+)"/g)].forEach((x) => ids.add(x[1]));
  });
  return ids;
}

// A definition's meaning as text: sorted keys, no canvas positions, and no empty values,
// because the server leaves those out when it saves.
function canonical(value) {
  const norm = (x) => {
    if (Array.isArray(x)) return x.map(norm);
    if (x && typeof x === 'object') {
      const out = {};
      Object.keys(x).sort().forEach((k) => {
        if (k === 'ui') return;
        const v = norm(x[k]);
        const empty = v === undefined || v === null || v === '' || v === 0 || v === false
          || (Array.isArray(v) && v.length === 0) || (typeof v === 'object' && v && !Array.isArray(v) && Object.keys(v).length === 0);
        if (!empty) out[k] = v;
      });
      return out;
    }
    return x;
  };
  return JSON.stringify(norm(value));
}

export function sameDefinition(a, b) {
  return canonical(a) === canonical(b);
}

export function isActiveRun(status) {
  return status === 'queued' || status === 'running' || status === 'waiting';
}

export const RUN_STATUS_COLORS = {
  queued: 'default', running: 'info', waiting: 'warning', succeeded: 'success', failed: 'error', cancelled: 'default', skipped: 'default',
};
