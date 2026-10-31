<template>
  <v-textarea
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

const props = defineProps({
  modelValue: { type: null, default: undefined },
  label: { type: String, default: '' },
  hint: { type: String, default: '' },
  rows: { type: Number, default: 3 },
  readonly: { type: Boolean, default: false },
  // Plain text is allowed as well as JSON, for values that are one template string.
  allowText: { type: Boolean, default: false },
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
.json-field :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.5;
}
</style>
