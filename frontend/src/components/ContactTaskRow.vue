<template>
  <div :class="['task-row', { 'task-row--done': task.status === 'done' }]" data-cy="task">
    <v-checkbox-btn
      :model-value="task.status === 'done'"
      :readonly="!canEdit"
      :aria-label="$t(task.status === 'done' ? 'crm.tasks.reopen' : 'crm.tasks.markDone', { title: task.title })"
      density="compact"
      @update:model-value="$emit('toggle', task)"
    />
    <div class="task-main">
      <div class="task-title">{{ task.title }}</div>
      <div class="task-meta text-caption text-medium-emphasis">
        <router-link
          v-if="showContact"
          :to="{ name: 'subscriber', params: { id: task.subscriber } }"
        >
          {{ task.subscriberName || contactEmail || task.subscriber }}
        </router-link>
        <span v-if="task.assigneeName">{{ task.assigneeName }}</span>
      </div>
    </div>
    <v-chip
      v-if="dueLabel"
      size="x-small"
      label
      :color="overdue ? 'error' : undefined"
      :variant="overdue ? 'tonal' : 'outlined'"
      prepend-icon="mdi-calendar-clock-outline"
    >
      {{ dueLabel }}
    </v-chip>
    <v-btn
      v-if="canEdit"
      :aria-label="$t('globals.buttons.delete')"
      icon="mdi-trash-can-outline"
      variant="text"
      size="x-small"
      @click="$emit('remove', task)"
    />
  </div>
</template>

<script setup>
import { computed, getCurrentInstance } from 'vue';
import {
  parseStamp, shortStamp, taskBucket, visibleEmail,
} from '../utils/crm.mjs';

const props = defineProps({
  task: { type: Object, required: true },
  showContact: { type: Boolean, default: false },
});
defineEmits(['toggle', 'remove']);

const { proxy } = getCurrentInstance();
const canEdit = computed(() => proxy.$can('subscribers:manage'));
const contactEmail = computed(() => visibleEmail(props.task.subscriberEmail));
const dueLabel = computed(() => (parseStamp(props.task.dueAt) ? shortStamp(props.task.dueAt) : ''));
const overdue = computed(() => props.task.status !== 'done' && taskBucket(props.task) === 'overdue');
</script>

<style scoped>
.task-row {
  align-items: center;
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  display: flex;
  gap: 8px;
  padding: 6px 4px;
}

/* The checkbox otherwise stretches and pushes the title to the middle of the row. */
.task-row :deep(.v-selection-control) {
  flex: none;
}

.task-main {
  flex: 1;
  min-width: 0;
}

.task-title {
  font-size: 14px;
  overflow-wrap: anywhere;
}

.task-row--done .task-title {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  text-decoration: line-through;
}

.task-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
