<template>
  <v-container class="d-flex align-center justify-center flex-column py-12">
    <v-icon
      :icon="!icon ? 'mdi-plus' : icon"
      size="x-large"
      class="mb-4"
      color="grey"
    />
    <p class="text-center text-grey">
      {{ !label ? $t('globals.messages.emptyState') : label }}
    </p>
  </v-container>
</template>

<script>
export default {
  name: 'EmptyPlaceholder',

  props: {
    icon: { type: String, default: '' },
    label: { type: String, default: '' },
  },
};
</script>
