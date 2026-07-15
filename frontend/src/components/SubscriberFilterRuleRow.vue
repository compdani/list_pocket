<template>
  <div class="filter-rule" :class="{ 'has-key': rule.field === 'attrib' }" data-cy="filter-rule">
    <v-select
      :model-value="rule.field"
      :items="fieldOptions"
      item-title="title"
      item-value="value"
      density="compact"
      variant="outlined"
      hide-details
      class="filter-field"
      data-cy="filter-field"
      @update:model-value="(v) => patch({ field: v })"
    />
    <v-text-field
      v-if="rule.field === 'attrib'"
      :model-value="rule.key"
      density="compact"
      variant="outlined"
      hide-details
      placeholder="city"
      class="filter-key"
      data-cy="filter-attrib-key"
      @update:model-value="(v) => patch({ key: v })"
    />
    <v-select
      :model-value="rule.op"
      :items="operators"
      item-title="title"
      item-value="value"
      density="compact"
      variant="outlined"
      hide-details
      class="filter-op"
      data-cy="filter-op"
      @update:model-value="(v) => patch({ op: v })"
    />
    <template v-if="needsValue">
      <v-combobox
        v-if="rule.field === 'tag'"
        :model-value="rule.value"
        :items="tagOptions"
        multiple
        chips
        closable-chips
        density="compact"
        variant="outlined"
        hide-details
        class="filter-value"
        data-cy="filter-tag-value"
        @update:model-value="(v) => patch({ value: v })"
      />
      <v-select
        v-else-if="rule.field === 'list'"
        :model-value="rule.value"
        :items="listOptions"
        item-title="title"
        item-value="value"
        multiple
        chips
        closable-chips
        density="compact"
        variant="outlined"
        hide-details
        class="filter-value"
        data-cy="filter-list-value"
        @update:model-value="(v) => patch({ value: v })"
      />
      <v-select
        v-else-if="rule.field === 'status'"
        :model-value="rule.value"
        :items="statusOptions"
        item-title="title"
        item-value="value"
        density="compact"
        variant="outlined"
        hide-details
        class="filter-value"
        data-cy="filter-status-value"
        @update:model-value="(v) => patch({ value: v })"
      />
      <v-text-field
        v-else
        :model-value="rule.value"
        :type="isNumericOp ? 'number' : 'text'"
        density="compact"
        variant="outlined"
        hide-details
        class="filter-value"
        data-cy="filter-value"
        @update:model-value="(v) => patch({ value: v })"
      />
    </template>
    <button type="button" class="filter-remove" data-cy="btn-remove-rule" @click="$emit('remove')">
      <v-icon icon="mdi-close" size="16" />
    </button>
  </div>
</template>

<script>
import {
  defaultOpForField,
  defaultValueForField,
  operatorsForField,
} from '../utils/subscriberFilters';

export default {
  name: 'SubscriberFilterRuleRow',
  props: {
    rule: { type: Object, required: true },
    tagOptions: { type: Array, default: () => [] },
    listOptions: { type: Array, default: () => [] },
    fieldOptions: { type: Array, default: () => [] },
  },
  emits: ['update', 'remove'],
  computed: {
    operators() {
      return operatorsForField(this.rule.field);
    },
    needsValue() {
      return !(this.rule.field === 'attrib' && (this.rule.op === 'exists' || this.rule.op === 'not_exists'));
    },
    isNumericOp() {
      return this.rule.field === 'attrib' && ['gt', 'gte', 'lt', 'lte'].includes(this.rule.op);
    },
    statusOptions() {
      return [
        { title: this.$t('subscribers.status.enabled'), value: 'enabled' },
        { title: this.$t('subscribers.status.blocklisted'), value: 'blocklisted' },
        { title: 'Disabled', value: 'disabled' },
      ];
    },
  },
  methods: {
    patch(partial) {
      const next = { ...this.rule, ...partial };
      if (partial.field && partial.field !== this.rule.field) {
        next.op = defaultOpForField(partial.field);
        next.value = defaultValueForField(partial.field);
        next.key = '';
      }
      this.$emit('update', next);
    },
  },
};
</script>

<style scoped>
.filter-rule {
  align-items: flex-start;
  display: grid;
  gap: 8px;
  grid-template-columns: minmax(120px, 1.1fr) minmax(120px, 1fr) minmax(160px, 2fr) auto;
  width: 100%;
}

.filter-rule.has-key {
  grid-template-columns: minmax(110px, 1fr) minmax(100px, 1fr) minmax(110px, 1fr) minmax(140px, 1.6fr) auto;
}

.filter-remove {
  align-items: center;
  background: transparent;
  border: 0;
  border-radius: 8px;
  color: #64748b;
  cursor: pointer;
  display: inline-flex;
  height: 36px;
  justify-content: center;
  width: 36px;
}

.filter-remove:hover {
  background: rgba(15, 23, 42, 0.06);
  color: #0f172a;
}

@media (max-width: 900px) {
  .filter-rule,
  .filter-rule.has-key {
    grid-template-columns: 1fr;
  }

  .filter-remove {
    justify-self: end;
  }
}
</style>
