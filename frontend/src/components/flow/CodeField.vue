<template>
  <div class="code-field" :class="{ 'code-field--error': error }">
    <code-editor :model-value="modelValue" :lang="lang" :disabled="disabled" :autofocus="false" :completions="completions" @update:model-value="$emit('update:modelValue', $event)" @blur="$emit('blur')" />
  </div>
</template>

<script setup>
import { defineAsyncComponent } from 'vue';

const CodeEditor = defineAsyncComponent(() => import('../CodeEditor.vue'));

defineProps({
  modelValue: { type: String, default: '' },
  lang: { type: String, default: 'javascript' },
  disabled: { type: Boolean, default: false },
  error: { type: Boolean, default: false },
  completions: { type: Function, default: null },
});
defineEmits(['update:modelValue', 'blur']);
</script>

<style scoped>
.code-field {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 4px;
  font-size: 12px;
  margin-bottom: 8px;
  overflow: hidden;
}

.code-field--error {
  border-color: rgb(var(--v-theme-error));
}

/* The global .code-editor is a fixed 65vh box with its own border; here it fits the editor. */
.code-field :deep(.code-editor) {
  border: 0;
  height: auto;
  min-height: 0;
}

.code-field :deep(.cm-editor) {
  max-height: 420px;
  min-height: var(--code-field-min-height, 180px);
}

.code-field :deep(.cm-scroller) {
  overflow: auto;
}
</style>
