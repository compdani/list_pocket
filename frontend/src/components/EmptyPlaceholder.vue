<template>
  <v-container class="d-flex align-center justify-center flex-column py-12">
    <v-icon
      :icon="icon || 'mdi-inbox-outline'"
      size="x-large"
      class="mb-4"
      color="medium-emphasis"
    />
    <p class="text-center text-medium-emphasis mb-0">
      {{ label || $t('globals.messages.emptyState') }}
    </p>
    <p v-if="hint" class="text-center text-medium-emphasis text-body-2 mt-2 mb-0">
      {{ hint }}
    </p>
    <v-btn
      v-if="actionLabel"
      class="mt-4"
      color="primary"
      variant="flat"
      :to="actionTo"
      @click="actionTo ? undefined : $emit('action')"
    >
      {{ actionLabel }}
    </v-btn>
  </v-container>
</template>

<script>
export default {
  name: 'EmptyPlaceholder',

  props: {
    icon: { type: String, default: '' },
    label: { type: String, default: '' },
    hint: { type: String, default: '' },
    actionLabel: { type: String, default: '' },
    actionTo: { type: [String, Object], default: null },
  },

  emits: ['action'],
};
</script>
