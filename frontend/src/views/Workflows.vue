<template>
  <section class="flows-page" data-cy="flows-page">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h5">{{ $t('menu.workflows') }}</h1>
        <p class="text-body-2 text-medium-emphasis mb-0">{{ $t('workflows.intro') }}</p>
      </div>
      <div class="header-actions">
        <v-btn variant="text" class="text-none" prepend-icon="mdi-creation-outline" data-cy="flows-ask-ai" @click="askAssistant">{{ $t('workflows.askAI') }}</v-btn>
        <v-btn v-if="canManage" color="primary" variant="flat" class="text-none" prepend-icon="mdi-plus" data-cy="flows-new" @click="createOpen = true">{{ $t('workflows.new') }}</v-btn>
      </div>
    </header>

    <v-alert v-if="pendingApprovals > 0" type="warning" variant="tonal" density="comfortable" class="mb-4" data-cy="flows-pending">
      <div class="d-flex align-center justify-space-between flex-wrap ga-2">
        <span>{{ $t('workflows.pendingBanner', { n: pendingApprovals }) }}</span>
        <v-btn :to="{ name: 'workflowApprovals' }" size="small" variant="outlined" class="text-none">{{ $t('workflows.reviewNow') }}</v-btn>
      </div>
    </v-alert>

    <v-skeleton-loader v-if="loading && !flows.length" type="list-item-two-line@4" />

    <div v-else-if="!flows.length" class="flows-empty">
      <h2 class="text-subtitle-1 mb-1">{{ $t('workflows.emptyTitle') }}</h2>
      <p class="text-body-2 text-medium-emphasis">{{ $t('workflows.emptyHelp') }}</p>
    </div>

    <div v-else class="flows-list">
      <router-link v-for="item in flows" :key="item.id" :to="{ name: 'workflowBuilder', params: { id: item.id } }" class="flow-row" data-cy="flow-row">
        <div class="flow-row-main">
          <div class="font-weight-medium text-truncate">{{ item.name }}</div>
          <div class="text-caption text-medium-emphasis text-truncate">{{ item.description || triggerLabel(item) }}</div>
        </div>
        <v-chip v-if="item.pending_approvals > 0" size="small" label color="warning" variant="tonal">{{ $t('workflows.pendingCount', { n: item.pending_approvals }) }}</v-chip>
        <v-chip v-if="item.last_run" size="small" label variant="tonal" :color="runColors[item.last_run.status]">{{ item.last_run.status }} · {{ shortStamp(item.last_run.created) }}</v-chip>
        <v-chip size="small" label :color="statusColor(item)" data-cy="flow-row-status">{{ statusLabel(item) }}</v-chip>
      </router-link>
    </div>

    <template v-if="canManage">
      <h2 class="text-subtitle-1 mt-6 mb-2">{{ $t('workflows.templates') }}</h2>
      <div class="flow-templates">
        <button v-for="tpl in templates" :key="tpl.key" type="button" class="flow-template" :data-cy="`flow-template-${tpl.key}`" @click="useTemplate(tpl)">
          <strong>{{ tpl.title }}</strong>
          <span class="text-caption text-medium-emphasis">{{ tpl.description }}</span>
        </button>
      </div>
    </template>

    <h2 class="text-subtitle-1 mt-6 mb-2">{{ $t('workflows.recentRuns') }}</h2>
    <p v-if="!runs.length" class="text-body-2 text-medium-emphasis">{{ $t('workflows.noRuns') }}</p>
    <div v-else class="flows-list">
      <router-link v-for="run in runs" :key="run.id" :to="{ name: 'workflowRun', params: { id: run.id } }" class="flow-row">
        <v-chip size="small" label :color="runColors[run.status]">{{ run.status }}</v-chip>
        <div class="flow-row-main text-truncate">{{ run.workflow_name }}</div>
        <span class="text-caption text-medium-emphasis">{{ run.source === 'test' ? $t('workflows.testRun') : run.source }} · {{ shortStamp(run.created) }}</span>
      </router-link>
    </div>

    <v-dialog v-model="createOpen" max-width="460">
      <v-card>
        <v-card-title>{{ $t('workflows.new') }}</v-card-title>
        <v-card-text>
          <v-text-field v-model="newTitle" :label="$t('workflows.field.name')" autofocus density="comfortable" variant="outlined" data-cy="flows-new-name" @keydown.enter="create" />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="createOpen = false">{{ $t('globals.buttons.cancel') }}</v-btn>
          <v-btn color="primary" variant="flat" :disabled="!newTitle.trim()" :loading="creating" data-cy="flows-create" @click="create">{{ $t('workflows.create') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, onBeforeUnmount, onMounted, ref,
} from 'vue';
import { useRouter } from 'vue-router';
import { createFlow, getFlowRuns, getFlows } from '../api';
import { assistantContext } from '../assistant/context';
import { RUN_STATUS_COLORS } from '../utils/flowGraph.mjs';
import { FLOW_TEMPLATES } from '../utils/flowTemplates.mjs';
import { shortStamp } from '../utils/crm.mjs';

const REFRESH_MS = 15000;

const { proxy } = getCurrentInstance();
const router = useRouter();
const runColors = RUN_STATUS_COLORS;
const templates = FLOW_TEMPLATES;
const flows = ref([]);
const runs = ref([]);
const loading = ref(false);
const createOpen = ref(false);
const newTitle = ref('');
const creating = ref(false);
let timer = null;

const canManage = computed(() => proxy.$can('workflows:manage'));
const pendingApprovals = computed(() => flows.value.reduce((sum, item) => sum + (item.pending_approvals || 0), 0));

function errorText(err) {
  return (err.response && err.response.message) || err.message;
}

function statusLabel(item) {
  if (!item.published_version) return proxy.$t('workflows.status.draft');
  return item.enabled
    ? proxy.$t('workflows.status.live', { n: item.published_version })
    : proxy.$t('workflows.status.paused', { n: item.published_version });
}

function statusColor(item) {
  if (!item.published_version) return 'default';
  return item.enabled ? 'success' : 'default';
}

function triggerLabel(item) {
  return proxy.$t(`workflows.trigger.types.${item.trigger_type || 'manual'}`);
}

async function load({ quiet = false } = {}) {
  if (!quiet) loading.value = true;
  try {
    const [list, recent] = await Promise.all([getFlows(), getFlowRuns({ limit: 10 })]);
    flows.value = Array.isArray(list) ? list : [];
    runs.value = Array.isArray(recent) ? recent : [];
  } catch (err) {
    if (!quiet) proxy.$utils.toast(errorText(err), 'error');
  } finally {
    loading.value = false;
  }
}

async function create() {
  if (!newTitle.value.trim() || creating.value) return;
  creating.value = true;
  try {
    const out = await createFlow({ title: newTitle.value });
    router.push({ name: 'workflowBuilder', params: { id: out.id } });
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  } finally {
    creating.value = false;
  }
}

async function useTemplate(tpl) {
  try {
    const out = await createFlow({ definition: tpl.definition });
    router.push({ name: 'workflowBuilder', params: { id: out.id } });
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  }
}

function askAssistant() {
  assistantContext.open = true;
}

onMounted(() => {
  load();
  timer = setInterval(() => load({ quiet: true }), REFRESH_MS);
  window.addEventListener('assistant-applied', load);
});

onBeforeUnmount(() => {
  clearInterval(timer);
  window.removeEventListener('assistant-applied', load);
});
</script>

<style scoped>
.flows-list {
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 12px;
  overflow: hidden;
}

.flow-row {
  align-items: center;
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  color: inherit;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 12px 16px;
  text-decoration: none;
}

.flow-row:last-child {
  border-bottom: 0;
}

.flow-row:hover {
  background: rgb(var(--v-theme-surface-light));
}

.flow-row-main {
  flex: 1 1 200px;
  min-width: 0;
}

.flows-empty {
  background: rgb(var(--v-theme-surface));
  border: 1px dashed rgba(var(--v-border-color), 0.4);
  border-radius: 12px;
  padding: 24px;
  text-align: center;
}

.flow-templates {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
}

.flow-template {
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 12px;
  color: inherit;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  font: inherit;
  gap: 4px;
  padding: 14px;
  text-align: left;
}

.flow-template:hover {
  background: rgb(var(--v-theme-surface-light));
}

.flow-template:focus-visible {
  outline: 2px solid rgb(var(--v-theme-primary));
}
</style>
