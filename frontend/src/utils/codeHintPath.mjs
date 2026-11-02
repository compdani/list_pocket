// Works out what the code before the cursor refers to, following local variables, callback
// parameters and for...of back to the run data. codeHints.mjs turns the result into hints.
import { syntaxTree } from '@codemirror/language';
import { completionPath } from '@codemirror/lang-javascript';
import { ELEMENT } from './codeHints.mjs';

// Array methods whose result, or first callback parameter, is known from the array.
const RETURNS_ARRAY = new Set(['concat', 'filter', 'reverse', 'slice', 'sort', 'toReversed', 'toSorted']);
const RETURNS_ELEMENT = new Set(['at', 'find', 'findLast', 'pop', 'shift']);
const PASSES_ELEMENT = new Set(['every', 'filter', 'find', 'findIndex', 'findLast', 'flatMap', 'forEach', 'map', 'some']);
const FUNCTIONS = new Set(['ArrowFunction', 'FunctionExpression', 'FunctionDeclaration']);

function children(node) {
  const out = [];
  for (let child = node.firstChild; child; child = child.nextSibling) out.push(child);
  return out;
}

// The array a callback was passed to, as in nodes.x.filter((r) => ...).
function callbackElement(fn, read) {
  const args = fn.parent;
  const call = args?.parent;
  if (args?.name !== 'ArgList' || call?.name !== 'CallExpression' || children(args).find((c) => FUNCTIONS.has(c.name))?.from !== fn.from) return null;
  const callee = call.firstChild;
  const method = callee?.lastChild;
  if (callee?.name !== 'MemberExpression' || method?.name !== 'PropertyName' || !PASSES_ELEMENT.has(read(method))) return null;
  const array = exprPath(callee.firstChild, read);
  return array && [...array, ELEMENT];
}

function paramPath(fn, name, read) {
  const params = fn.getChild('ParamList');
  const defined = params ? children(params).filter((c) => c.name === 'VariableDefinition') : [];
  const index = defined.findIndex((c) => read(c) === name);
  if (index < 0) return undefined;
  return index === 0 ? callbackElement(fn, read) : null;
}

function declarationPath(scope, name, before, read) {
  let found;
  children(scope).filter((c) => c.name === 'VariableDeclaration' && c.from < before).forEach((declaration) => {
    const parts = children(declaration);
    parts.forEach((part, i) => {
      if (part.name !== 'VariableDefinition' || read(part) !== name) return;
      const value = parts[i + 1]?.name === 'Equals' ? parts[i + 2] : null;
      // const raw = raw.x reads the outer raw, which isn't declared yet, so it's unknown.
      found = value && value.to <= before ? exprPath(value, read) : null;
    });
  });
  return found;
}

function forOfPath(statement, name, read) {
  const spec = statement.getChild('ForOfSpec');
  const parts = spec ? children(spec) : [];
  const index = parts.findIndex((c) => c.name === 'VariableDefinition' && read(c) === name);
  if (index < 0) return undefined;
  const array = parts.slice(index + 1).find((c) => !['of', 'in'].includes(c.name) && !c.type.isKeyword);
  const path = array ? exprPath(array, read) : null;
  return path && [...path, ELEMENT];
}

// undefined means nothing here declares the name, so it's a global. null means something
// does, but its value isn't known.
function variablePath(node, read) {
  const name = read(node);
  for (let scope = node.parent; scope; scope = scope.parent) {
    let path;
    if (FUNCTIONS.has(scope.name)) path = paramPath(scope, name, read);
    else if (scope.name === 'ForStatement') path = forOfPath(scope, name, read);
    else if (scope.name === 'Block' || scope.name === 'Script') path = declarationPath(scope, name, node.from, read);
    if (path !== undefined) return path;
  }
  return [name];
}

function exprPath(node, read) {
  switch (node.name) {
    case 'VariableName':
      return variablePath(node, read);
    case 'ParenthesizedExpression':
      return node.firstChild?.nextSibling ? exprPath(node.firstChild.nextSibling, read) : null;
    case 'BinaryExpression': {
      // nodes.x || {} and nodes.x ?? {} have the shape of the left side.
      const op = node.getChild('LogicOp');
      return op && /^(\|\||\?\?)$/.test(read(op)) ? exprPath(node.firstChild, read) : null;
    }
    case 'MemberExpression': {
      const object = exprPath(node.firstChild, read);
      if (!object) return null;
      const last = node.lastChild;
      if (last.name === 'PropertyName') return [...object, read(last)];
      if (last.name !== ']') return null;
      const index = last.prevSibling;
      if (index.name === 'String') return [...object, read(index).slice(1, -1)];
      return [...object, ELEMENT];
    }
    case 'CallExpression': {
      const callee = node.firstChild;
      const method = callee?.lastChild;
      if (callee?.name !== 'MemberExpression' || method?.name !== 'PropertyName') return null;
      const array = exprPath(callee.firstChild, read);
      if (!array) return null;
      if (RETURNS_ARRAY.has(read(method))) return array;
      return RETURNS_ELEMENT.has(read(method)) ? [...array, ELEMENT] : null;
    }
    default:
      return null;
  }
}

// The path and the part of a name already typed, or null if nothing should be offered.
export function codeHintPath(context) {
  const { state, pos } = context;
  const read = (node) => state.sliceDoc(node.from, node.to);
  const inner = syntaxTree(state).resolveInner(pos, -1);
  const member = inner.parent?.name === 'MemberExpression' ? inner.parent : null;
  let object = null;
  let name = '';
  if (inner.name === 'PropertyName' && member) {
    object = member.firstChild;
    name = read(inner);
  } else if ((inner.name === '.' || inner.name === '?.') && member) {
    object = member.firstChild;
  } else if (inner.name === 'MemberExpression' && /\.\s*$/.test(read(inner))) {
    object = inner.firstChild;
  }
  if (!object) {
    const at = completionPath(context);
    return at && at.path.length === 0 ? at : null;
  }
  const path = exprPath(object, read);
  return path && { path, name };
}
