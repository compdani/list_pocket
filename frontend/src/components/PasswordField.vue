<template>
  <v-text-field
    :model-value="modelValue"
    :label="label"
    :autocomplete="autocomplete"
    :type="visible ? 'text' : 'password'"
    :disabled="disabled"
    :append-inner-icon="visible ? 'mdi-eye-off-outline' : 'mdi-eye-outline'"
    :rules="rules"
    :hint="hint"
    hide-details="auto"
    variant="outlined"
    density="comfortable"
    :class="fieldClass"
    @click:append-inner="visible = !visible"
    @update:model-value="$emit('update:modelValue', $event)"
  />
</template>

<script setup>
import { ref } from 'vue';

defineProps({
  modelValue: { type: String, default: '' },
  label: { type: String, default: '' },
  autocomplete: { type: String, default: 'current-password' },
  rules: { type: Array, default: () => [] },
  hint: { type: String, default: '' },
  fieldClass: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
});

defineEmits(['update:modelValue']);

const visible = ref(false);
</script>
