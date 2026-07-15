import test from 'node:test';
import assert from 'node:assert/strict';

import {
  cleanSubscriberFilters,
  countActiveFilterRules,
  emptyFilterGroup,
  emptyFilterRule,
  hydrateFilterBuilder,
  serializeSubscriberFilters,
} from '../src/utils/subscriberFilters.js';

test('cleanSubscriberFilters drops incomplete rules', () => {
  const group = emptyFilterGroup();
  group.rules = [
    { ...emptyFilterRule('tag'), value: ['vip'] },
    { ...emptyFilterRule('attrib'), key: '', value: 'x' },
    { ...emptyFilterRule('email'), value: '' },
  ];
  const cleaned = cleanSubscriberFilters(group);
  assert.equal(cleaned.rules.length, 1);
  assert.equal(cleaned.rules[0].field, 'tag');
});

test('serializeSubscriberFilters returns JSON for complete trees', () => {
  const group = {
    logic: 'and',
    rules: [
      { field: 'attrib', op: 'eq', key: 'city', value: 'Berlin' },
      {
        logic: 'or',
        rules: [
          { field: 'email', op: 'contains', value: '@acme.com' },
          { field: 'status', op: 'eq', value: 'enabled' },
        ],
      },
    ],
  };
  const json = serializeSubscriberFilters(group);
  const parsed = JSON.parse(json);
  assert.equal(parsed.rules.length, 2);
  assert.equal(countActiveFilterRules(group), 3);
});

test('hydrateFilterBuilder restores editable ids', () => {
  const raw = JSON.stringify({
    logic: 'and',
    rules: [{ field: 'tag', op: 'has_any', value: ['vip'] }],
  });
  const hydrated = hydrateFilterBuilder(raw);
  assert.ok(hydrated.rules[0]._id);
  assert.equal(hydrated.rules[0].field, 'tag');
  assert.deepEqual(hydrated.rules[0].value, ['vip']);
});
