// Helpers for the agent page: labels for proposals and context, and reading files.

export const CONTEXT_ICONS = {
  contact: 'mdi-account-outline',
  conversation: 'mdi-forum-outline',
  campaign: 'mdi-rocket-launch-outline',
  list: 'mdi-format-list-bulleted-square',
  template: 'mdi-file-image-outline',
  workflow: 'mdi-source-branch',
  run: 'mdi-play-outline',
  file: 'mdi-paperclip',
};

// How each proposal is reviewed. See ReviewModal.vue.
const PROPOSAL_INFO = {
  propose_workflow: { label: 'New workflow draft', apply: 'Save as draft', view: 'workflow' },
  propose_workflow_patch: { label: 'Workflow changes', apply: 'Apply to draft', view: 'workflow' },
  propose_publish: { label: 'Publish workflow', apply: 'Publish', view: 'publish' },
  propose_test_run: { label: 'Test run', apply: 'Start test run', view: 'run', warning: 'Test runs do real work: emails, messages and changes all happen.' },
  propose_workflow_run: { label: 'Run workflow', apply: 'Run', view: 'run', warning: 'This starts the published workflow for real.' },
  propose_approval_decision: { label: 'Decide approval', apply: 'Apply decision', view: 'data' },
  propose_reply: { label: 'Send reply', apply: 'Send', view: 'reply', warning: 'This sends a real message when you apply it.' },
  create_campaign_draft: { label: 'New campaign draft', apply: 'Save as draft', view: 'content', path: 'draft' },
  propose_create_template: { label: 'New template', apply: 'Create', view: 'content', path: 'body' },
  propose_contact_update: { label: 'Update contact', apply: 'Update', view: 'data' },
  propose_board_move: { label: 'Move on board', apply: 'Move', view: 'data' },
  propose_contact_note: { label: 'Add note', apply: 'Add', view: 'data' },
  propose_contact_task: { label: 'Add task', apply: 'Add', view: 'data' },
  propose_create_contact: { label: 'Add contact', apply: 'Add', view: 'data' },
  propose_create_list: { label: 'Create list', apply: 'Create', view: 'data' },
  // A plugin can do anything the app can, so its code is always opened before it's installed.
  propose_plugin: {
    label: 'Install plugin', apply: 'Install', view: 'plugin', reviewFirst: true, warning: 'Plugins run with full access to this app and its data. Read the code before you install it.',
  },
};

export function proposalInfo(tool) {
  return PROPOSAL_INFO[tool] || { label: tool, apply: 'Apply', view: 'data' };
}

export function proposalTitle(part) {
  const args = part.arguments || {};
  return args.summary || proposalInfo(part.tool).label;
}

// What a plugin adds to workflows, as it's named in the builder.
export function pluginAddons(manifest) {
  const kinds = [
    ['tools', 'plugins.tool'],
    ['events', 'plugins.event'],
    ['agentPresets', 'plugins.agentPreset'],
    ['fieldEditors', 'plugins.fieldEditor'],
  ];
  return kinds.flatMap(([list, kind]) => (manifest?.[list] || []).map((addon) => ({
    key: `${list}:${addon.name}`,
    kind,
    name: `${manifest.id}.${addon.name}`,
    description: addon.description,
  })));
}

// How a proposed plugin's files differ from the installed ones. With nothing installed,
// every file is new.
export function pluginFileChanges(current, next) {
  const before = new Map((current || []).map((f) => [f.path, f.content]));
  const files = (next || []).map((f) => {
    let status = 'added';
    if (before.has(f.path)) status = before.get(f.path) === f.content ? 'same' : 'changed';
    return { path: f.path, status };
  });
  const paths = new Set((next || []).map((f) => f.path));
  return { files, removed: [...before.keys()].filter((p) => !paths.has(p)) };
}

const MAX_DIFF_CELLS = 4000000;

// A line diff, as [{ kind: 'same' | 'added' | 'removed', text }], or null if the files
// are too long to compare in the browser.
export function lineDiff(before, after) {
  const a = String(before ?? '').split('\n');
  const b = String(after ?? '').split('\n');
  if (a.length * b.length > MAX_DIFF_CELLS) return null;
  const width = b.length + 1;
  const lengths = new Uint32Array((a.length + 1) * width);
  for (let i = a.length - 1; i >= 0; i -= 1) {
    for (let j = b.length - 1; j >= 0; j -= 1) {
      lengths[i * width + j] = a[i] === b[j]
        ? lengths[(i + 1) * width + j + 1] + 1
        : Math.max(lengths[(i + 1) * width + j], lengths[i * width + j + 1]);
    }
  }
  const out = [];
  let i = 0;
  let j = 0;
  while (i < a.length || j < b.length) {
    if (i < a.length && j < b.length && a[i] === b[j]) {
      out.push({ kind: 'same', text: a[i] });
      i += 1;
      j += 1;
    } else if (j < b.length && (i === a.length || lengths[i * width + j + 1] >= lengths[(i + 1) * width + j])) {
      out.push({ kind: 'added', text: b[j] });
      j += 1;
    } else {
      out.push({ kind: 'removed', text: a[i] });
      i += 1;
    }
  }
  return out;
}

export function refKey(ref) {
  return `${ref.type}:${ref.id}`;
}

// Everything attached so far stays in the agent's context, once each.
export function contextInUse(messages) {
  const seen = new Set();
  const out = [];
  (messages || []).forEach((message) => {
    (message.context || []).forEach((ref) => {
      if (!seen.has(refKey(ref))) {
        seen.add(refKey(ref));
        out.push(ref);
      }
    });
  });
  return out;
}

export const MAX_TEXT_BYTES = 2 * 1024 * 1024;
export const MAX_BINARY_BYTES = 6 * 1024 * 1024;

// Images and PDFs go to the model as files. Anything else is read as text.
export function isBinaryFile(file) {
  return String(file.type).startsWith('image/') || file.type === 'application/pdf';
}

// Why a file can't be attached, or an empty string if it can.
export function fileProblem(file) {
  const limit = isBinaryFile(file) ? MAX_BINARY_BYTES : MAX_TEXT_BYTES;
  if (file.size > limit) {
    return `${file.name} is larger than ${limit / 1024 / 1024} MB.`;
  }
  return '';
}

export function formatBytes(size) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`;
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}

// What changes between two workflow definitions, node by node.
export function diffNodes(current, next, same) {
  if (!current) {
    return { nodes: Object.fromEntries((next?.nodes || []).map((n) => [n.id, 'added'])), removed: [], isNew: true };
  }
  const before = Object.fromEntries((current.nodes || []).map((n) => [n.id, n]));
  const nodes = {};
  (next?.nodes || []).forEach((n) => {
    if (!before[n.id]) nodes[n.id] = 'added';
    else if (!same(before[n.id], n)) nodes[n.id] = 'changed';
  });
  const ids = new Set((next?.nodes || []).map((n) => n.id));
  const removed = (current.nodes || []).filter((n) => !ids.has(n.id)).map((n) => n.id);
  return { nodes, removed, isNew: false };
}
