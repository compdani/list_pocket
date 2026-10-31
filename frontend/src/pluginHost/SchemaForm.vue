<template>
  <div class="schema-form">
    <template v-for="field in fields" :key="field.key">
      <v-switch
        v-if="field.type === 'boolean'"
        :model-value="Boolean(modelValue[field.key])"
        :label="field.label"
        :hint="field.help"
        :disabled="disabled"
        :data-cy="`plugin-field-${field.key}`"
        persistent-hint
        color="primary"
        class="mb-3"
        @update:model-value="update(field.key, $event)"
      />
      <v-select
        v-else-if="field.type === 'select'"
        :model-value="modelValue[field.key]"
        :label="field.label"
        :items="field.options"
        :hint="field.help"
        :disabled="disabled"
        :data-cy="`plugin-field-${field.key}`"
        persistent-hint
        variant="outlined"
        density="comfortable"
        class="mb-3"
        @update:model-value="update(field.key, $event)"
      />
      <v-textarea
        v-else-if="field.type === 'textarea'"
        :model-value="modelValue[field.key]"
        :label="field.label"
        :hint="field.help"
        :disabled="disabled"
        :data-cy="`plugin-field-${field.key}`"
        persistent-hint
        variant="outlined"
        density="comfortable"
        rows="3"
        class="mb-3"
        @update:model-value="update(field.key, $event)"
      />
      <v-text-field
        v-else
        :model-value="modelValue[field.key]"
        :label="field.label"
        :hint="fieldHint(field)"
        :type="fieldInputType(field)"
        :disabled="disabled"
        :data-cy="`plugin-field-${field.key}`"
        autocomplete="off"
        persistent-hint
        variant="outlined"
        density="comfortable"
        class="mb-3"
        @update:model-value="update(field.key, field.type === 'number' ? toNumber($event) : $event)"
      />
    </template>
  </div>
</template>

<script setup>
import { getCurrentInstance } from 'vue';

const props = defineProps({
  fields: { type: Array, default: () => [] },
  modelValue: { type: Object, default: () => ({}) },
  // Secrets that already have a stored value, by key. Their values never reach the browser.
  secrets: { type: Object, default: () => ({}) },
  disabled: { type: Boolean, default: false },
});

const emit = defineEmits(['update:modelValue']);
const { proxy } = getCurrentInstance();

function update(key, value) {
  emit('update:modelValue', { ...props.modelValue, [key]: value });
}

function toNumber(value) {
  return value === '' || value === null ? null : Number(value);
}

function fieldInputType(field) {
  if (field.type === 'secret') {
    return 'password';
  }
  return field.type === 'number' ? 'number' : 'text';
}

function fieldHint(field) {
  if (field.type === 'secret' && props.secrets[field.key]) {
    return proxy.$t('plugins.secretKept');
  }
  return field.help;
}
</script>
