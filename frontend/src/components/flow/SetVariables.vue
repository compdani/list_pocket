<template>
  <div data-cy="flow-set">
    <section v-for="(variable, i) in variables" :key="keyOf(variable)" class="variable" data-cy="flow-variable">
      <div class="d-flex ga-1 align-start">
        <v-text-field
          v-model="variable.name"
          :label="$t('workflows.set.name')"
          :hint="$t('workflows.set.nameHint', { name: variable.name })"
          persistent-hint
          v-bind="field"
          data-cy="flow-variable-name"
        />
        <v-btn v-if="!readonly && variables.length > 1" :aria-label="$t('globals.buttons.delete')" icon="mdi-close" size="x-small" variant="text" data-cy="flow-variable-remove" @click="variables.splice(i, 1)" />
      </div>
      <v-select
        :model-value="variable.type"
        :items="typeItems"
        :label="$t('workflows.set.type')"
        v-bind="field"
        data-cy="flow-variable-type"
        @update:model-value="variables[i] = retypeVariable(variable, $event)"
      />

      <v-textarea
        v-if="variable.type === 'string' || variable.type === 'template'"
        v-model="variable.value"
        :label="$t('workflows.set.value')"
        :hint="$t(`workflows.set.${variable.type}Hint`)"
        persistent-hint
        rows="2"
        auto-grow
        v-bind="field"
        data-cy="flow-variable-value"
      />
      <v-text-field
        v-else-if="variable.type === 'number'"
        :model-value="variable.value"
        :label="$t('workflows.set.value')"
        :hint="$t('workflows.set.numberHint')"
        persistent-hint
        v-bind="field"
        data-cy="flow-variable-value"
        @change="variable.value = numberValue($event.target.value)"
      />
      <v-combobox
        v-else-if="variable.type === 'boolean'"
        :model-value="String(variable.value ?? false)"
        :items="['true', 'false']"
        :label="$t('workflows.set.value')"
        :hint="$t('workflows.set.booleanHint')"
        persistent-hint
        v-bind="field"
        data-cy="flow-variable-value"
        @update:model-value="variable.value = booleanValue($event)"
      />
      <template v-else-if="variable.type === 'code'">
        <code-field v-model="variable.value" lang="javascript" :disabled="readonly" :completions="completions" data-cy="flow-variable-code" />
        <p class="text-caption text-medium-emphasis mb-0">{{ $t('workflows.set.codeHint') }}</p>
      </template>
      <template v-else-if="variable.type === 'html'">
        <v-btn-toggle
          :model-value="variable.editor || 'visual'"
          mandatory
          density="compact"
          variant="outlined"
          divided
          class="mb-3"
          :disabled="readonly"
          data-cy="flow-variable-editor"
          @update:model-value="variable.editor = $event"
        >
          <v-btn value="visual" size="small" class="text-none">{{ $t('workflows.set.visual') }}</v-btn>
          <v-btn value="code" size="small" class="text-none">{{ $t('workflows.set.code') }}</v-btn>
        </v-btn-toggle>
        <code-field v-if="variable.editor === 'code'" v-model="variable.value" lang="html" :disabled="readonly" data-cy="flow-variable-html" />
        <template v-else>
          <p class="html-summary text-body-2" :class="{ 'text-medium-emphasis': !summary(variable) }" data-cy="flow-variable-summary">
            {{ summary(variable) || $t('workflows.set.htmlEmpty') }}
          </p>
          <v-btn block variant="tonal" color="primary" class="text-none mb-2" prepend-icon="mdi-pencil-outline" data-cy="flow-variable-edit" @click="editing = variable">
            {{ readonly ? $t('workflows.set.viewHtml') : $t('workflows.set.editHtml') }}
          </v-btn>
        </template>
        <p class="text-caption text-medium-emphasis mb-0">{{ $t('workflows.set.htmlHint') }}</p>
      </template>
    </section>

    <v-btn v-if="!readonly" size="small" variant="text" class="text-none mb-2" prepend-icon="mdi-plus" data-cy="flow-variable-add" @click="variables.push(newVariable(node))">
      {{ $t('workflows.set.add') }}
    </v-btn>
    <p class="text-caption text-medium-emphasis">{{ $t('workflows.set.help') }}</p>

    <html-variable-dialog
      v-if="editing"
      :name="editing.name"
      :model-value="editing.value"
      :readonly="readonly"
      @close="editing = null"
      @save="saveHtml"
    />
  </div>
</template>

<script setup>
import {
  computed, defineAsyncComponent, getCurrentInstance, ref,
} from 'vue';
import CodeField from './CodeField.vue';
import { VARIABLE_TYPES, newVariable, retypeVariable } from '../../utils/flowGraph.mjs';
import { bodySummary } from '../../utils/emailCompose.mjs';

// It brings the rich text editor with it, so it loads when it opens.
const HtmlVariableDialog = defineAsyncComponent(() => import('./HtmlVariableDialog.vue'));

const props = defineProps({
  node: { type: Object, required: true },
  readonly: { type: Boolean, default: false },
  completions: { type: Function, default: null },
});

const { proxy } = getCurrentInstance();
const field = computed(() => ({
  density: 'compact', variant: 'outlined', class: 'mb-3', readonly: props.readonly,
}));
const typeItems = computed(() => VARIABLE_TYPES.map((value) => ({ value, title: proxy.$t(`workflows.set.types.${value}`) })));

const variables = computed(() => {
  // eslint-disable-next-line vue/no-mutating-props
  if (!props.node.variables) props.node.variables = [];
  return props.node.variables;
});

// Names change as people type, so they can't be the keys.
const keys = new WeakMap();
let nextKey = 0;
function keyOf(variable) {
  if (!keys.has(variable)) {
    nextKey += 1;
    keys.set(variable, nextKey);
  }
  return keys.get(variable);
}

const editing = ref(null);

function saveHtml(html) {
  editing.value.value = html;
  editing.value = null;
}

function summary(variable) {
  return bodySummary({ content_type: 'html', body: variable.value });
}

// Anything that isn't a plain number stays as text, for an expression.
function numberValue(text) {
  const raw = text.trim();
  return raw !== '' && Number.isFinite(Number(raw)) ? Number(raw) : raw;
}

function booleanValue(text) {
  const raw = String(text ?? '').trim();
  if (raw === 'true') return true;
  return raw === 'false' || raw === '' ? false : raw;
}
</script>

<style scoped>
.variable {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 6px;
  margin-bottom: 10px;
  padding: 10px 10px 6px;
}

.html-summary {
  background: rgba(var(--v-theme-on-surface), 0.04);
  border-radius: 6px;
  margin-bottom: 8px;
  overflow-wrap: anywhere;
  padding: 8px 10px;
}
</style>
