let filterId = 0;

export function nextFilterId() {
  filterId += 1;
  return `fr-${filterId}`;
}

export function emptyFilterGroup(logic = 'and') {
  return {
    logic,
    rules: [emptyFilterRule()],
  };
}

export function emptyFilterRule(field = 'tag') {
  return {
    _id: nextFilterId(),
    field,
    op: defaultOpForField(field),
    key: '',
    value: defaultValueForField(field),
  };
}

export function emptyFilterOrGroup() {
  return {
    _id: nextFilterId(),
    logic: 'or',
    rules: [emptyFilterRule(), emptyFilterRule()],
  };
}

export function defaultOpForField(field) {
  switch (field) {
    case 'tag':
      return 'has_any';
    case 'attrib':
      return 'eq';
    case 'status':
      return 'eq';
    case 'list':
      return 'in';
    default:
      return 'contains';
  }
}

export function defaultValueForField(field) {
  if (field === 'tag' || field === 'list') {
    return [];
  }
  if (field === 'status') {
    return 'enabled';
  }
  return '';
}

export function operatorsForField(field) {
  switch (field) {
    case 'tag':
      return [
        { value: 'has_any', title: 'Has any of' },
        { value: 'has_all', title: 'Has all of' },
        { value: 'has_none', title: 'Has none of' },
      ];
    case 'attrib':
      return [
        { value: 'eq', title: 'Equals' },
        { value: 'neq', title: 'Not equals' },
        { value: 'contains', title: 'Contains' },
        { value: 'exists', title: 'Exists' },
        { value: 'not_exists', title: 'Does not exist' },
        { value: 'gt', title: 'Greater than' },
        { value: 'gte', title: 'Greater or equal' },
        { value: 'lt', title: 'Less than' },
        { value: 'lte', title: 'Less or equal' },
      ];
    case 'email':
    case 'name':
    case 'phone':
      return [
        { value: 'eq', title: 'Equals' },
        { value: 'contains', title: 'Contains' },
        { value: 'starts_with', title: 'Starts with' },
        { value: 'ends_with', title: 'Ends with' },
      ];
    case 'status':
      return [
        { value: 'eq', title: 'Is' },
        { value: 'neq', title: 'Is not' },
      ];
    case 'list':
      return [
        { value: 'in', title: 'In any of' },
        { value: 'not_in', title: 'Not in any of' },
      ];
    default:
      return [];
  }
}

function isCompleteRule(rule) {
  if (!rule || !rule.field || !rule.op) {
    return false;
  }
  if (rule.field === 'attrib') {
    if (!String(rule.key || '').trim()) {
      return false;
    }
    if (rule.op === 'exists' || rule.op === 'not_exists') {
      return true;
    }
    if (['gt', 'gte', 'lt', 'lte'].includes(rule.op)) {
      return rule.value !== '' && rule.value !== null && rule.value !== undefined;
    }
    return String(rule.value ?? '').trim() !== '';
  }
  if (rule.field === 'tag' || rule.field === 'list') {
    return Array.isArray(rule.value) && rule.value.length > 0;
  }
  if (rule.field === 'status') {
    return String(rule.value || '').trim() !== '';
  }
  return String(rule.value ?? '').trim() !== '';
}

function cleanNode(node) {
  if (!node) {
    return null;
  }
  if (Array.isArray(node.rules)) {
    const rules = node.rules.map(cleanNode).filter(Boolean);
    if (rules.length === 0) {
      return null;
    }
    return {
      logic: node.logic === 'or' ? 'or' : 'and',
      rules,
    };
  }
  if (!isCompleteRule(node)) {
    return null;
  }
  const out = {
    field: node.field,
    op: node.op,
  };
  if (node.field === 'attrib') {
    out.key = String(node.key || '').trim();
  }
  if (!(node.field === 'attrib' && (node.op === 'exists' || node.op === 'not_exists'))) {
    if (['gt', 'gte', 'lt', 'lte'].includes(node.op) && node.field === 'attrib') {
      const n = Number(node.value);
      out.value = Number.isFinite(n) ? n : node.value;
    } else {
      out.value = node.value;
    }
  }
  return out;
}

/** Strip incomplete rules and UI-only ids for the API. Returns null if empty. */
export function cleanSubscriberFilters(group) {
  if (!group) {
    return null;
  }
  return cleanNode(group);
}

export function serializeSubscriberFilters(group) {
  const cleaned = cleanSubscriberFilters(group);
  if (!cleaned) {
    return null;
  }
  return JSON.stringify(cleaned);
}

export function countActiveFilterRules(group) {
  const cleaned = cleanSubscriberFilters(group);
  if (!cleaned) {
    return 0;
  }
  const walk = (node) => {
    if (Array.isArray(node.rules)) {
      return node.rules.reduce((sum, r) => sum + walk(r), 0);
    }
    return 1;
  };
  return walk(cleaned);
}

export function describeFilterRule(rule, listNameById = {}) {
  if (!rule || !rule.field) {
    return '';
  }
  const opLabel = (operatorsForField(rule.field).find((o) => o.value === rule.op) || {}).title || rule.op;
  if (rule.field === 'tag') {
    const tags = Array.isArray(rule.value) ? rule.value.join(', ') : rule.value;
    return `Tag ${opLabel.toLowerCase()} ${tags}`;
  }
  if (rule.field === 'attrib') {
    if (rule.op === 'exists' || rule.op === 'not_exists') {
      return `Attribute ${rule.key} ${opLabel.toLowerCase()}`;
    }
    return `Attribute ${rule.key} ${opLabel.toLowerCase()} ${rule.value}`;
  }
  if (rule.field === 'list') {
    const names = (Array.isArray(rule.value) ? rule.value : [])
      .map((id) => listNameById[id] || id)
      .join(', ');
    return `List ${opLabel.toLowerCase()} ${names}`;
  }
  if (rule.field === 'status') {
    return `Status ${opLabel.toLowerCase()} ${rule.value}`;
  }
  return `${rule.field} ${opLabel.toLowerCase()} ${rule.value}`;
}

/** Flat list of leaf rules for chip display (from cleaned tree). */
export function flattenFilterRules(group) {
  const cleaned = cleanSubscriberFilters(group);
  if (!cleaned) {
    return [];
  }
  const out = [];
  const walk = (node) => {
    if (Array.isArray(node.rules)) {
      node.rules.forEach(walk);
      return;
    }
    out.push(node);
  };
  walk(cleaned);
  return out;
}

/** Rehydrate a cleaned filters payload into editable UI state (with _ids). */
export function hydrateFilterBuilder(raw) {
  if (!raw) {
    return emptyFilterGroup();
  }
  let parsed = raw;
  if (typeof raw === 'string') {
    try {
      parsed = JSON.parse(raw);
    } catch {
      return emptyFilterGroup();
    }
  }
  const hydrateNode = (node) => {
    if (!node) {
      return emptyFilterRule();
    }
    if (Array.isArray(node.rules)) {
      return {
        _id: nextFilterId(),
        logic: node.logic === 'or' ? 'or' : 'and',
        rules: node.rules.length ? node.rules.map(hydrateNode) : [emptyFilterRule()],
      };
    }
    return {
      _id: nextFilterId(),
      field: node.field || 'tag',
      op: node.op || defaultOpForField(node.field || 'tag'),
      key: node.key || '',
      value: node.value !== undefined ? node.value : defaultValueForField(node.field || 'tag'),
    };
  };
  if (!parsed.rules) {
    return emptyFilterGroup();
  }
  return {
    logic: parsed.logic === 'or' ? 'or' : 'and',
    rules: parsed.rules.length ? parsed.rules.map(hydrateNode) : [emptyFilterRule()],
  };
}
