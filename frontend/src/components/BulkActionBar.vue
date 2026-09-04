<template>
  <v-card v-if="selectedCount > 0" class="mb-4 bulk-actions-card" elevation="0">
    <v-card-text class="bulk-actions-content">
      <slot />
      <span>
        {{ $tc('globals.messages.numSelected', selectedCount, { num: selectedCount }) }}
        <span v-if="showSelectAll">
          &mdash;
          <v-btn
            variant="text"
            size="small"
            :data-cy="selectAllCy"
            @click="$emit('select-all')"
          >
            {{ $tc('globals.messages.selectAll', total, { num: total }) }}
          </v-btn>
        </span>
      </span>
    </v-card-text>
  </v-card>
</template>

<script setup>
defineProps({
  selectedCount: { type: Number, default: 0 },
  total: { type: Number, default: 0 },
  showSelectAll: { type: Boolean, default: false },
  selectAllCy: { type: String, default: 'select-all' },
});

defineEmits(['select-all']);
</script>

<style scoped>
.bulk-actions-card {
  background: rgba(var(--v-theme-primary), 0.06);
  border: 1px solid rgba(var(--v-theme-primary), 0.16);
  border-radius: 14px;
}

.bulk-actions-content {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
</style>
