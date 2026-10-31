<template>
  <div class="ctx-picker" role="listbox" :aria-label="$t('agent.attachContext')" data-cy="agent-context-picker">
    <p v-if="loading && !items.length" class="text-caption text-medium-emphasis pa-2 mb-0">{{ $t('agent.searching') }}</p>
    <p v-else-if="!items.length" class="text-caption text-medium-emphasis pa-2 mb-0">{{ $t('agent.nothingMatches', { q: query }) }}</p>
    <button
      v-for="(item, i) in items"
      :key="`${item.type}:${item.id}`"
      type="button"
      role="option"
      :aria-selected="i === active"
      :class="['ctx-item', { 'ctx-item--active': i === active }]"
      data-cy="agent-context-option"
      @mouseenter="active = i"
      @click="$emit('pick', item)"
    >
      <v-icon :icon="icons[item.type]" size="15" />
      <span class="ctx-label">{{ item.label }}</span>
      <span class="ctx-type">{{ $t(`agent.type.${item.type}`) }}</span>
      <span v-if="item.detail" class="ctx-detail">{{ item.detail }}</span>
    </button>
  </div>
</template>

<script setup>
import { onBeforeUnmount, ref, watch } from 'vue';
import { agentGet } from '../../agent/client';
import { CONTEXT_ICONS } from '../../agent/parts.mjs';

const SEARCH_DELAY_MS = 150;

const props = defineProps({
  query: { type: String, default: '' },
});
const emit = defineEmits(['pick', 'close']);

const icons = CONTEXT_ICONS;
const items = ref([]);
const active = ref(0);
const loading = ref(false);
let timer = null;
let seq = 0;

async function search() {
  seq += 1;
  const mine = seq;
  loading.value = true;
  try {
    const data = await agentGet(`agent/mentions?q=${encodeURIComponent(props.query)}`);
    if (mine === seq) {
      items.value = Array.isArray(data) ? data : [];
      active.value = 0;
    }
  } catch {
    if (mine === seq) items.value = [];
  } finally {
    if (mine === seq) loading.value = false;
  }
}

watch(() => props.query, () => {
  clearTimeout(timer);
  timer = setTimeout(search, SEARCH_DELAY_MS);
}, { immediate: true });

onBeforeUnmount(() => clearTimeout(timer));

// The composer passes key presses here while the picker is open. Returns true if used.
function key(e) {
  if (e.key === 'ArrowDown') active.value = Math.min(items.value.length - 1, active.value + 1);
  else if (e.key === 'ArrowUp') active.value = Math.max(0, active.value - 1);
  else if (e.key === 'Enter' || e.key === 'Tab') {
    if (items.value[active.value]) emit('pick', items.value[active.value]);
  } else if (e.key === 'Escape') emit('close');
  else return false;
  e.preventDefault();
  return true;
}

defineExpose({ key });
</script>

<style scoped>
.ctx-picker {
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), 0.3);
  border-radius: 10px;
  bottom: calc(100% + 6px);
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.18);
  left: 0;
  max-height: 320px;
  overflow-y: auto;
  padding: 4px;
  position: absolute;
  width: min(520px, 100%);
  z-index: 20;
}

.ctx-item {
  align-items: center;
  background: none;
  border: 0;
  border-radius: 6px;
  color: inherit;
  cursor: pointer;
  display: flex;
  font: inherit;
  font-size: 13px;
  gap: 8px;
  padding: 6px 8px;
  text-align: left;
  width: 100%;
}

.ctx-item--active {
  background: rgba(var(--v-theme-primary), 0.12);
}

.ctx-label {
  font-weight: 500;
  max-width: 55%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ctx-type {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  flex: none;
  font-size: 11px;
}

.ctx-detail {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  font-size: 12px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
