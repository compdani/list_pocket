<template>
  <div v-if="code" class="json-field-code" role="group" :aria-label="label">
    <div v-if="label" class="text-caption text-medium-emphasis mb-1">{{ label }}</div>
    <code-field v-model="text" lang="json" :disabled="readonly" :error="Boolean(error)" @blur="commit" />
    <div v-if="error || hint" :class="['text-caption', error ? 'text-error' : 'text-medium-emphasis']">{{ error || hint }}</div>
  </div>
  <v-textarea
    v-else
    v-model="text"
    :label="label"
    :hint="error || hint"
    :error="Boolean(error)"
    :rows="rows"
    :readonly="readonly"
    persistent-hint
    auto-grow
    max-rows="16"
    density="compact"
    variant="outlined"
    class="json-field"
    spellcheck="false"
    @blur="commit"
  />
</template>

<script setup>
import { ref, watch } from 'vue';
import CodeField from './CodeField.vue';

const props = defineProps({
  modelValue: { type: null, default: undefined },
  label: { type: String, default: '' },
  hint: { type: String, default: '' },
  rows: { type: Number, default: 3 },
  readonly: { type: Boolean, default: false },
  // Plain text is allowed as well as JSON, for values that are one template string.
  allowText: { type: Boolean, default: false },
  // A code editor instead of a textarea, for values long enough to need one.
  code: { type: Boolean, default: false },
});
const emit = defineEmits(['update:modelValue']);

function show(value) {
  if (value === undefined || value === null) return '';
  if (typeof value === 'string' && props.allowText) return value;
  return JSON.stringify(value, null, 2);
}

const text = ref(show(props.modelValue));
const error = ref('');

watch(() => props.modelValue, (value) => {
  let same = false;
  try {
    same = JSON.stringify(JSON.parse(text.value || 'null')) === JSON.stringify(value ?? null);
  } catch {
    same = text.value === value;
  }
  if (!same) {
    text.value = show(value);
    error.value = '';
  }
});

// Only commits on blur, so half-typed JSON never overwrites the saved value.
function commit() {
  const raw = text.value.trim();
  if (!raw) {
    error.value = '';
    emit('update:modelValue', undefined);
    return;
  }
  try {
    emit('update:modelValue', JSON.parse(raw));
    error.value = '';
  } catch (err) {
    if (props.allowText) {
      error.value = '';
      emit('update:modelValue', raw);
      return;
    }
    error.value = `Not valid JSON: ${err.message}`;
  }
}
</script>

<style scoped>
.json-field-code {
  --code-field-min-height: 96px;
}

.json-field :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.5;
}
</style>
