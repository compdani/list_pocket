<template>
  <div class="subscriber-filter-builder" data-cy="filter-builder">
    <div
      v-for="(rule, index) in localFilters.rules"
      :key="rule._id || index"
      class="filter-node"
    >
      <div v-if="isGroup(rule)" class="filter-group" data-cy="filter-or-group">
        <div class="filter-group-header">
          <span class="filter-group-label">{{ $t('subscribers.filters.orGroup') }}</span>
          <button type="button" class="filter-remove" data-cy="btn-remove-group" @click="removeRule(index)">
            <v-icon icon="mdi-close" size="16" />
          </button>
        </div>
        <div
          v-for="(child, childIndex) in rule.rules"
          :key="child._id || childIndex"
          class="nested-rule"
        >
          <subscriber-filter-rule-row
            :rule="child"
            :tag-options="tagOptions"
            :list-options="listOptions"
            :field-options="fieldOptions"
            @update="(next) => updateNestedRule(index, childIndex, next)"
            @remove="removeNestedRule(index, childIndex)"
          />
        </div>
        <v-btn
          type="button"
          size="small"
          variant="text"
          prepend-icon="mdi-plus"
          class="text-none"
          data-cy="btn-add-nested-rule"
          @click="addNestedRule(index)"
        >
          {{ $t('subscribers.filters.addCondition') }}
        </v-btn>
      </div>

      <subscriber-filter-rule-row
        v-else
        :rule="rule"
        :tag-options="tagOptions"
        :list-options="listOptions"
        :field-options="fieldOptions"
        @update="(next) => updateRule(index, next)"
        @remove="removeRule(index)"
      />
    </div>

    <div class="filter-actions">
      <v-btn
        type="button"
        size="small"
        variant="tonal"
        prepend-icon="mdi-plus"
        class="text-none"
        data-cy="btn-add-condition"
        @click="addRule"
      >
        {{ $t('subscribers.filters.addCondition') }}
      </v-btn>
      <v-btn
        type="button"
        size="small"
        variant="text"
        prepend-icon="mdi-set-center"
        class="text-none"
        data-cy="btn-add-or-group"
        @click="addOrGroup"
      >
        {{ $t('subscribers.filters.addOrGroup') }}
      </v-btn>
    </div>
  </div>
</template>

<script>
import {
  emptyFilterOrGroup,
  emptyFilterRule,
} from '../utils/subscriberFilters';
import SubscriberFilterRuleRow from './SubscriberFilterRuleRow.vue';

export default {
  name: 'SubscriberFilterBuilder',
  components: { SubscriberFilterRuleRow },
  props: {
    modelValue: {
      type: Object,
      required: true,
    },
    tagOptions: {
      type: Array,
      default: () => [],
    },
    listOptions: {
      type: Array,
      default: () => [],
    },
  },
  emits: ['update:modelValue'],
  computed: {
    localFilters() {
      return this.modelValue;
    },
    fieldOptions() {
      return [
        { title: this.$t('globals.terms.tags'), value: 'tag' },
        { title: this.$t('subscribers.filters.attribute'), value: 'attrib' },
        { title: this.$t('subscribers.email'), value: 'email' },
        { title: this.$t('globals.fields.name'), value: 'name' },
        { title: 'Phone', value: 'phone' },
        { title: this.$t('globals.fields.status'), value: 'status' },
        { title: this.$t('globals.terms.lists'), value: 'list' },
      ];
    },
  },
  methods: {
    isGroup(rule) {
      return Array.isArray(rule.rules);
    },
    emitNext(rules) {
      this.$emit('update:modelValue', {
        ...this.localFilters,
        logic: this.localFilters.logic || 'and',
        rules,
      });
    },
    updateRule(index, next) {
      const rules = [...this.localFilters.rules];
      rules[index] = next;
      this.emitNext(rules);
    },
    removeRule(index) {
      const rules = this.localFilters.rules.filter((_, i) => i !== index);
      this.emitNext(rules.length ? rules : [emptyFilterRule()]);
    },
    addRule() {
      this.emitNext([...this.localFilters.rules, emptyFilterRule()]);
    },
    addOrGroup() {
      this.emitNext([...this.localFilters.rules, emptyFilterOrGroup()]);
    },
    updateNestedRule(groupIndex, childIndex, next) {
      const rules = [...this.localFilters.rules];
      const group = { ...rules[groupIndex], rules: [...rules[groupIndex].rules] };
      group.rules[childIndex] = next;
      rules[groupIndex] = group;
      this.emitNext(rules);
    },
    removeNestedRule(groupIndex, childIndex) {
      const rules = [...this.localFilters.rules];
      const nested = rules[groupIndex].rules.filter((_, i) => i !== childIndex);
      if (nested.length === 0) {
        this.removeRule(groupIndex);
        return;
      }
      rules[groupIndex] = { ...rules[groupIndex], rules: nested };
      this.emitNext(rules);
    },
    addNestedRule(groupIndex) {
      const rules = [...this.localFilters.rules];
      rules[groupIndex] = {
        ...rules[groupIndex],
        rules: [...rules[groupIndex].rules, emptyFilterRule()],
      };
      this.emitNext(rules);
    },
  },
};
</script>

<style scoped>
.subscriber-filter-builder {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.filter-node {
  width: 100%;
}

.nested-rule {
  margin-bottom: 8px;
}

.filter-group {
  background: #f8fafc;
  border: 1px dashed #c7d5ea;
  border-radius: 12px;
  padding: 12px;
}

.filter-group-header {
  align-items: center;
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
}

.filter-group-label {
  color: #475569;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  text-transform: uppercase;
}

.filter-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 4px;
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
</style>
