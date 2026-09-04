<template>
  <v-card class="mb-4 query-card" elevation="0">
    <v-card-text class="query-card-body">
      <form class="query-form" @submit.prevent="$emit('submit')">
        <div class="query-main-row">
          <v-text-field
            :model-value="modelValue"
            class="query-input"
            name="query"
            :placeholder="placeholder"
            :aria-label="placeholder"
            prepend-inner-icon="mdi-magnify"
            variant="outlined"
            density="comfortable"
            hide-details
            :disabled="disabled"
            :data-cy="inputCy"
            @update:model-value="$emit('update:modelValue', $event)"
          />
          <slot name="filters" />
          <v-btn
            type="submit"
            class="query-submit"
            color="primary"
            icon="mdi-magnify"
            variant="flat"
            :aria-label="searchLabel || placeholder"
            :disabled="disabled"
            :data-cy="submitCy"
          />
        </div>
        <slot />
      </form>
    </v-card-text>
  </v-card>
</template>

<script setup>
defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  searchLabel: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  inputCy: { type: String, default: 'query' },
  submitCy: { type: String, default: 'btn-query' },
});

defineEmits(['update:modelValue', 'submit']);
</script>

<style scoped>
.query-card {
  background: linear-gradient(180deg, #ffffff 0%, #f6f9ff 100%);
  border: 1px solid rgba(var(--v-theme-primary), 0.16);
  border-radius: 16px;
}

.query-card-body {
  padding: 16px;
}

.query-form {
  display: block;
}

.query-main-row {
  align-items: center;
  display: flex;
  gap: 12px;
}

.query-input {
  flex: 1;
}

.query-input :deep(.v-field) {
  border-radius: 12px;
}

:deep(.query-tags) {
  max-width: 320px;
  min-width: 220px;
}

.query-submit {
  border-radius: 12px;
  min-height: 44px;
  min-width: 44px;
}

@media (max-width: 960px) {
  .query-main-row {
    align-items: stretch;
    flex-direction: column;
  }

  .query-submit {
    width: 100%;
  }
}
</style>
