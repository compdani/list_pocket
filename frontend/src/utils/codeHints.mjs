// What a JavaScript step can read, for code hints. Code runs in a sandbox with the run data
// and console, so browser globals are never offered: they wouldn't exist when the step runs.
import { nodeInfo } from './flowGraph.mjs';

// A path step meaning "one element of this array".
export const ELEMENT = '[]';

function hint(type, detail, fields, items) {
  return { type, detail, fields, items };
}

function names(type, list) {
  return Object.fromEntries(list.map((name) => [name, hint(type)]));
}

function schemaHint(schema, type = 'property', detail = schema?.type) {
  const fields = Object.fromEntries(Object.entries(schema?.properties || {}).map(([name, prop]) => [name, schemaHint(prop)]));
  return hint(type, detail, fields, schema?.type === 'array' ? schemaHint(schema.items) : undefined);
}

function valueHint(value) {
  if (Array.isArray(value)) return hint('property', 'array', {}, valueHint(value[0]));
  if (!value || typeof value !== 'object') return hint('property');
  return hint('property', undefined, Object.fromEntries(Object.entries(value).map(([name, item]) => [name, valueHint(item)])));
}

function inputFields(def) {
  return { ...valueHint(def.default_input).fields, ...schemaHint(def.input_schema).fields };
}

const list = () => hint('property', 'array', {}, hint('property'));

// Fields of each node's output, from the node types table in internal/flow/schema.md.
function outputHint(n, def, catalog) {
  const output = (() => {
    switch (n.type) {
      case 'start': return hint('property', undefined, inputFields(def));
      case 'agent': return hint('property', undefined, { result: schemaHint(n.agent?.output_schema, 'property', 'submitted object'), _citations: list() });
      case 'tool': return schemaHint((catalog?.tools || []).find((tool) => tool.name === n.tool)?.output_schema);
      case 'branch': return hint('property', undefined, names('property', ['case']));
      case 'foreach': return hint('property', undefined, { ...names('property', ['count', 'failed']), results: list() });
      case 'loop': return hint('property', undefined, { ...names('property', ['iterations', 'done', 'last']), results: list() });
      case 'review': return hint('property', undefined, names('property', ['approved', 'data', 'decided_by']));
      case 'wait': return hint('property', undefined, names('property', ['waited', 'event', 'timed_out']));
      case 'notify': return hint('property', undefined, names('property', ['message']));
      case 'transform': return valueHint(n.value);
      case 'subworkflow': return hint('property', undefined, n.detach ? names('property', ['run_id']) : {});
      default: return hint('property', undefined, {});
    }
  })();
  // on_error: continue makes the failure the output.
  if (n.on_error === 'continue') output.fields = { ...output.fields, ...names('property', ['error']) };
  return output;
}

function targets(n) {
  return [n.next, n.default, n.body, ...(n.cases || []).map((c) => c.to)].filter(Boolean);
}

function hasBody(n) {
  return n.type === 'foreach' || n.type === 'loop';
}

// The foreach or loop whose body holds this step. Validation doesn't allow nesting them.
function enclosing(def, id) {
  const byId = Object.fromEntries(def.nodes.map((n) => [n.id, n]));
  return def.nodes.find((container) => {
    if (!hasBody(container) || !container.body) return false;
    const seen = new Set();
    const pending = [container.body];
    while (pending.length) {
      const current = pending.pop();
      if (!seen.has(current) && byId[current]) {
        seen.add(current);
        pending.push(...targets(byId[current]));
      }
    }
    return seen.has(id);
  });
}

// Steps that can have finished before this one. A foreach or loop hasn't finished while its
// own body runs, so it isn't one of them.
function earlierNodes(def, id, container) {
  const before = new Set();
  const pending = [id];
  while (pending.length) {
    const current = pending.pop();
    def.nodes.forEach((n) => {
      if (n.id !== id && !before.has(n.id) && targets(n).includes(current)) {
        before.add(n.id);
        pending.push(n.id);
      }
    });
  }
  before.delete(container?.id);
  return def.nodes.filter((n) => before.has(n.id));
}

function isConnected(def, id) {
  return def.nodes.some((n) => n.id === id && n.type === 'start') || def.nodes.some((n) => targets(n).includes(id));
}

const ARRAY_MEMBERS = {
  length: hint('property', 'number'),
  ...names('function', ['at', 'concat', 'every', 'filter', 'find', 'findIndex', 'flatMap', 'forEach', 'includes', 'indexOf', 'join', 'map', 'reduce', 'reverse', 'slice', 'some', 'sort']),
};

const BUILTINS = {
  JSON: hint('class', undefined, names('function', ['parse', 'stringify'])),
  Math: hint('class', undefined, names('function', ['abs', 'ceil', 'floor', 'max', 'min', 'pow', 'random', 'round', 'sqrt', 'trunc'])),
  Object: hint('class', undefined, names('function', ['assign', 'entries', 'fromEntries', 'keys', 'values'])),
  Array: hint('class', undefined, names('function', ['from', 'isArray', 'of'])),
  Number: hint('class', undefined, names('function', ['isFinite', 'isInteger', 'isNaN', 'parseFloat', 'parseInt'])),
  String: hint('class'),
  Boolean: hint('class'),
  Date: hint('class', undefined, names('function', ['now', 'parse', 'UTC'])),
  RegExp: hint('class'),
  parseInt: hint('function'),
  parseFloat: hint('function'),
};

// A step is the body of a plain function, so import, export and await won't compile.
const KEYWORDS = names('keyword', [
  'break', 'case', 'catch', 'const', 'continue', 'default', 'delete', 'do', 'else', 'false', 'finally', 'for', 'function', 'if', 'in',
  'instanceof', 'let', 'new', 'null', 'of', 'return', 'switch', 'throw', 'true', 'try', 'typeof', 'undefined', 'var', 'while',
]);

function step(current, name) {
  if (name === ELEMENT) return current?.items;
  if (current?.fields && Object.hasOwn(current.fields, name)) return current.fields[name];
  return current?.items && Object.hasOwn(ARRAY_MEMBERS, name) ? ARRAY_MEMBERS[name] : undefined;
}

// The elements of a foreach's items expression, when it's a plain path such as nodes.x.results.
function itemHint(items, root) {
  const expr = String(items || '').replace(/^\s*\{\{\s*|\s*\}\}\s*$/g, '').trim();
  if (!/^[\w$]+(\??\.[\w$]+|\[\d+\])*$/.test(expr)) return hint('variable', 'list item');
  const path = expr.match(/\[\d+\]|[\w$]+/g).map((part) => (part.startsWith('[') ? ELEMENT : part));
  const element = path.reduce(step, root)?.items;
  return hint('variable', 'list item', element?.fields, element?.items);
}

function rootHint(def, id, catalog) {
  const container = enclosing(def, id);
  const connected = isConnected(def, id);
  const steps = connected ? earlierNodes(def, id, container) : def.nodes.filter((n) => n.id !== id);
  const nodes = Object.fromEntries(steps.map((n) => {
    const label = n.name || nodeInfo(n).label;
    return [n.id, { ...outputHint(n, def, catalog), detail: connected ? label : `${label} · connect this step first` }];
  }));
  const root = hint(undefined, undefined, {
    input: hint('variable', 'run input', inputFields(def)),
    nodes: hint('variable', 'finished steps', nodes),
    now: hint('variable', 'RFC 3339 time'),
    run: hint('variable', 'this run', names('property', ['id', 'source'])),
    console: hint('variable', 'writes to the logs', names('function', ['log', 'info', 'warn', 'error', 'debug'])),
    ...BUILTINS,
    ...KEYWORDS,
  });
  // These only exist in a body, and which ones depends on the kind. See runState.env.
  if (container?.type === 'foreach') {
    root.fields.item = itemHint(container.items, root);
    root.fields.index = hint('variable', 'position in the list');
  }
  if (container?.type === 'loop') {
    root.fields.loop = hint('variable', 'this iteration', names('property', ['iteration', 'last']));
    root.fields.index = hint('variable', 'iteration number');
  }
  return root;
}

// The completions for a path: [] for globals, ['nodes', 'x'] after nodes.x., and ELEMENT for
// one element of an array.
export function codeHints(path, def, id, catalog) {
  const current = path.reduce(step, rootHint(def, id, catalog));
  if (!current) return [];
  const members = current.items ? { ...current.fields, ...ARRAY_MEMBERS } : current.fields || {};
  return Object.entries(members).map(([label, { type, detail }]) => ({ label, type, detail }));
}
