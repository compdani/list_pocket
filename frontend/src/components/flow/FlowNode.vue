<template>
  <div :class="['flow-node', { 'flow-node--error': data.hasError, 'flow-node--selected': selected }, data.diff && `flow-node--${data.diff}`]" :data-cy="`flow-node-${data.node.id}`">
    <Handle v-if="data.node.type !== 'start'" type="target" :position="Position.Top" class="flow-handle" />
    <div class="flow-node-head">
      <span class="flow-node-icon" :style="{ background: info.color }"><v-icon :icon="info.icon" size="15" color="white" /></span>
      <div class="flow-node-title">
        <div class="flow-node-name">{{ data.node.name || info.label }}</div>
        <div class="flow-node-id">{{ data.node.id }}</div>
      </div>
      <v-icon v-if="data.hasError" icon="mdi-alert-circle-outline" size="16" color="error" :aria-label="$t('workflows.hasError')" />
    </div>
    <div v-if="data.summary" class="flow-node-summary">{{ data.summary }}</div>
    <div v-if="labelled.length" class="flow-node-handles">
      <div v-for="handle in labelled" :key="handle.id" class="flow-node-handle-label">{{ handle.label }}</div>
    </div>
    <Handle
      v-for="(handle, index) in data.handles"
      :id="handle.id"
      :key="handle.id"
      type="source"
      :position="Position.Bottom"
      class="flow-handle"
      :style="{ left: `${((index + 1) / (data.handles.length + 1)) * 100}%` }"
    />
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { Handle, Position } from '@vue-flow/core';
import { nodeInfo } from '../../utils/flowGraph.mjs';

const props = defineProps({
  data: { type: Object, required: true },
  selected: { type: Boolean, default: false },
});

const info = computed(() => nodeInfo(props.data.node));
const labelled = computed(() => props.data.handles.filter((h) => h.label));
</script>

<style scoped>
.flow-node {
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), 0.35);
  border-radius: 10px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
  color: rgb(var(--v-theme-on-surface));
  padding: 10px 12px;
  width: 230px;
}

.flow-node--selected {
  border-color: rgb(var(--v-theme-primary));
  box-shadow: 0 0 0 1px rgb(var(--v-theme-primary));
}

.flow-node--error {
  border-color: rgb(var(--v-theme-error));
}

.flow-node--added {
  border-color: rgb(var(--v-theme-success));
  box-shadow: 0 0 0 1px rgb(var(--v-theme-success));
}

.flow-node--changed {
  border-color: rgb(var(--v-theme-warning));
  box-shadow: 0 0 0 1px rgb(var(--v-theme-warning));
}

.flow-node-head {
  align-items: center;
  display: flex;
  gap: 8px;
}

.flow-node-icon {
  align-items: center;
  border-radius: 6px;
  display: flex;
  flex: none;
  height: 26px;
  justify-content: center;
  width: 26px;
}

.flow-node-title {
  flex: 1;
  min-width: 0;
}

.flow-node-name {
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.flow-node-id {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  font-size: 11px;
}

.flow-node-summary {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  font-size: 12px;
  margin-top: 6px;
  overflow-wrap: anywhere;
}

.flow-node-handles {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  display: flex;
  font-size: 10px;
  justify-content: space-around;
  margin-top: 6px;
}

.flow-node-handle-label {
  min-width: 0;
  overflow: hidden;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.flow-handle {
  background: rgb(var(--v-theme-primary));
  height: 10px;
  width: 10px;
}
</style>
