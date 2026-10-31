<template>
  <v-menu v-model="open" :close-on-content-click="false" location="bottom end" max-width="460">
    <template #activator="{ props: menuProps }">
      <v-btn v-bind="menuProps" size="small" variant="tonal" class="text-none model-trigger" append-icon="mdi-chevron-down" data-cy="agent-model">
        <span class="text-truncate">{{ label }}</span>
      </v-btn>
    </template>
    <v-card class="model-card" elevation="8" rounded="lg">
      <div class="d-flex ga-1 pa-2">
        <v-text-field v-model="search" :label="$t('agent.searchModels')" density="compact" variant="outlined" hide-details autofocus data-cy="agent-model-search" />
        <v-btn :aria-label="$t('agent.reloadModels')" icon="mdi-refresh" size="small" variant="text" :loading="loading" @click="load(true)" />
      </div>
      <div class="model-list">
        <button type="button" :class="['model-opt', { 'model-opt--sel': !value?.model }]" @click="pick(null)">
          <span>{{ $t('agent.defaultModel') }}</span>
          <span class="text-caption text-medium-emphasis">{{ defaultLabel }}</span>
        </button>
        <template v-for="group in groups" :key="group.name">
          <div class="model-group">{{ group.label }}</div>
          <p v-if="group.error" class="text-caption text-medium-emphasis px-3 py-1 mb-0">{{ group.error }}</p>
          <button
            v-for="model in group.shown"
            :key="model.id"
            type="button"
            :class="['model-opt', { 'model-opt--sel': value?.model === model.id && value?.provider === group.name }]"
            data-cy="agent-model-option"
            @click="pick({ provider: group.name, model: model.id })"
          >
            <span class="text-truncate">{{ model.id }}</span>
            <span v-if="model.name !== model.id" class="text-caption text-medium-emphasis text-truncate">{{ model.name }}</span>
          </button>
          <p v-if="group.total > group.shown.length" class="text-caption text-medium-emphasis px-3 py-1 mb-0">{{ $t('agent.moreModels', { n: group.total - group.shown.length }) }}</p>
        </template>
        <p v-if="error" class="text-caption text-error px-3 py-1 mb-0">{{ error }}</p>
      </div>
    </v-card>
  </v-menu>
</template>

<script setup>
import {
  computed, getCurrentInstance, ref, watch,
} from 'vue';
import { agentGet } from '../../agent/client';

const LIMIT = 50;

const props = defineProps({
  value: { type: Object, default: null },
});
const emit = defineEmits(['change']);

const { proxy } = getCurrentInstance();
const open = ref(false);
const search = ref('');
const data = ref(null);
const loading = ref(false);
const error = ref('');

const defaultLabel = computed(() => {
  const d = data.value?.default;
  return d?.model ? `${d.model} · ${providerLabel(d.provider)}` : '';
});

const label = computed(() => {
  if (props.value?.model) return props.value.model;
  return data.value?.default?.model ? proxy.$t('agent.defaultNamed', { model: data.value.default.model }) : proxy.$t('agent.defaultModel');
});

const groups = computed(() => {
  const terms = search.value.toLowerCase().split(/\s+/).filter(Boolean);
  return (data.value?.providers || []).map((p) => {
    const all = p.models.filter((m) => terms.every((t) => `${m.id} ${m.name}`.toLowerCase().includes(t)));
    return { ...p, total: all.length, shown: all.slice(0, LIMIT) };
  });
});

function providerLabel(name) {
  return (data.value?.providers || []).find((p) => p.name === name)?.label || name;
}

async function load(refresh = false) {
  loading.value = true;
  error.value = '';
  try {
    data.value = await agentGet(`agent/models${refresh ? '?refresh=1' : ''}`);
  } catch (err) {
    error.value = err.message;
  } finally {
    loading.value = false;
  }
}

function pick(next) {
  open.value = false;
  emit('change', next);
}

watch(open, (value) => {
  if (value) {
    search.value = '';
    if (!data.value) load();
  }
});

load();
</script>

<style scoped>
.model-trigger {
  max-width: 300px;
}

.model-card {
  width: min(440px, 90vw);
}

.model-list {
  max-height: 400px;
  overflow-y: auto;
  padding-bottom: 6px;
}

.model-group {
  background: rgb(var(--v-theme-surface));
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  font-size: 11px;
  letter-spacing: 0.04em;
  padding: 8px 12px 4px;
  position: sticky;
  text-transform: uppercase;
  top: 0;
}

.model-opt {
  background: none;
  border: 0;
  color: inherit;
  cursor: pointer;
  display: flex;
  font: inherit;
  font-size: 13px;
  gap: 10px;
  justify-content: space-between;
  padding: 5px 12px;
  text-align: left;
  width: 100%;
}

.model-opt:hover {
  background: rgb(var(--v-theme-surface-light));
}

.model-opt--sel {
  background: rgba(var(--v-theme-primary), 0.12);
}
</style>
