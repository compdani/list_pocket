<template>
  <section data-cy="flow-run">
    <v-btn
      :to="run?.workflow ? { name: 'workflowBuilder', params: { id: run.workflow } } : { name: 'workflows' }"
      variant="text"
      size="small"
      prepend-icon="mdi-arrow-left"
      class="text-none px-1 mb-2"
    >
      {{ run?.workflow_name || $t('menu.workflows') }}
    </v-btn>

    <v-alert v-if="loadError" type="error" variant="tonal">{{ loadError }}</v-alert>
    <div v-else-if="!run" class="d-flex justify-center pa-12"><v-progress-circular indeterminate color="primary" /></div>
    <template v-else>
      <header class="run-head">
        <v-chip label :color="runColors[run.status]" data-cy="flow-run-status">{{ run.status }}</v-chip>
        <span class="text-body-2 text-medium-emphasis">
          {{ run.source === 'test' ? $t('workflows.testRun') : `v${run.version} · ${run.source}` }}
          · {{ $utils.niceDate(run.created, true) }}
          <template v-if="run.cost_usd"> · ${{ Number(run.cost_usd).toFixed(4) }}</template>
        </span>
        <v-spacer />
        <v-btn v-if="active && canStop" size="small" variant="outlined" class="text-none" data-cy="flow-run-cancel" @click="cancel">{{ $t('workflows.stopRun') }}</v-btn>
      </header>

      <v-alert v-if="run.error" type="error" variant="tonal" density="compact" class="mb-3 run-error">{{ run.error }}</v-alert>

      <approval-card v-for="item in pendingApprovals" :key="item.id" :approval="item" @decided="load" />

      <details class="run-step mb-3" open>
        <summary>{{ $t('workflows.field.input') }}</summary>
        <pre class="run-json">{{ pretty(run.input) }}</pre>
      </details>

      <ol class="run-steps">
        <li v-for="step in run.nodes" :key="step.id" class="run-step" data-cy="flow-run-step">
          <details :open="step.status === 'failed' || step.status === 'waiting'">
            <summary class="run-step-head">
              <v-icon :icon="iconFor(step)" size="16" />
              <span class="font-weight-medium">{{ labelFor(step) }}</span>
              <span v-if="step.item_index >= 0" class="text-caption text-medium-emphasis">#{{ step.item_index + 1 }}</span>
              <v-spacer />
              <v-chip size="x-small" label :color="runColors[step.status]">{{ step.status }}</v-chip>
            </summary>
            <div class="run-step-body">
              <p v-if="step.error" class="text-error text-body-2 run-error">{{ step.error }}</p>
              <p v-if="step.wake_at && step.status === 'waiting'" class="text-caption text-medium-emphasis">{{ $t('workflows.waitingUntil', { at: $utils.niceDate(step.wake_at, true) }) }}</p>
              <p v-if="step.waiting_for" class="text-caption text-medium-emphasis">{{ $t('workflows.waitingFor', { event: step.waiting_for }) }}</p>
              <router-link v-if="step.child_run" :to="{ name: 'workflowRun', params: { id: step.child_run } }" class="text-caption">{{ $t('workflows.viewRun') }}</router-link>
              <template v-if="hasValue(step.input)">
                <div class="text-caption text-medium-emphasis">{{ $t('workflows.field.input') }}</div>
                <pre class="run-json">{{ pretty(step.input) }}</pre>
              </template>
              <template v-if="hasValue(step.output)">
                <div class="text-caption text-medium-emphasis">{{ $t('workflows.field.output') }}</div>
                <pre class="run-json">{{ pretty(step.output) }}</pre>
              </template>
              <details v-if="step.agent_steps?.length" class="mt-2">
                <summary class="text-caption">{{ $t('workflows.agentActivity', { n: step.agent_steps.length }) }}</summary>
                <div v-for="item in step.agent_steps" :key="item.id" class="mt-2">
                  <div class="text-caption text-medium-emphasis">{{ item.seq }}. {{ item.type }}<template v-if="item.payload?.name"> · {{ item.payload.name }}</template></div>
                  <p v-if="item.payload?.content" class="text-body-2 run-text mb-1">{{ item.payload.content }}</p>
                  <pre v-if="item.type === 'tool'" class="run-json">{{ pretty({ arguments: item.payload?.arguments, result: item.payload?.result }) }}</pre>
                  <p v-if="item.error" class="text-error text-caption mb-0">{{ item.error }}</p>
                </div>
              </details>
            </div>
          </details>
        </li>
      </ol>

      <details v-if="hasValue(run.output)" class="run-step" open>
        <summary>{{ $t('workflows.field.output') }}</summary>
        <pre class="run-json">{{ pretty(run.output) }}</pre>
      </details>
    </template>
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, onBeforeUnmount, onMounted, ref, watch,
} from 'vue';
import { useRoute } from 'vue-router';
import { cancelFlowRun, getFlowRun } from '../api';
import ApprovalCard from '../components/flow/ApprovalCard.vue';
import { isActiveRun, NODE_TYPES, RUN_STATUS_COLORS } from '../utils/flowGraph.mjs';

const REFRESH_MS = 2000;

const { proxy } = getCurrentInstance();
const route = useRoute();
const runColors = RUN_STATUS_COLORS;
const run = ref(null);
const loadError = ref('');
let timer = null;

const active = computed(() => isActiveRun(run.value?.status));
const canStop = computed(() => proxy.$can('workflows:run', 'workflows:manage'));
const pendingApprovals = computed(() => (run.value?.approvals || []).filter((a) => a.status === 'pending'));

function nodeOf(step) {
  return (run.value?.definition?.nodes || []).find((n) => n.id === step.node_id);
}

function labelFor(step) {
  const node = nodeOf(step);
  return node?.name || step.node_id;
}

function iconFor(step) {
  return (NODE_TYPES[step.node_type] || NODE_TYPES.transform).icon;
}

function pretty(value) {
  return JSON.stringify(value, null, 2);
}

function hasValue(value) {
  if (value === null || value === undefined || value === '') return false;
  return typeof value !== 'object' || Object.keys(value).length > 0;
}

async function load() {
  try {
    run.value = await getFlowRun(route.params.id);
    loadError.value = '';
    clearTimeout(timer);
    // A run that's only waiting on a person changes slowly, but still shows when it moves.
    if (isActiveRun(run.value.status)) {
      timer = setTimeout(load, REFRESH_MS);
    }
  } catch (err) {
    loadError.value = err.message;
  }
}

async function cancel() {
  try {
    await cancelFlowRun(route.params.id);
    await load();
  } catch (err) {
    proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
  }
}

watch(() => route.params.id, (id) => {
  if (id) {
    run.value = null;
    load();
  }
});
onMounted(load);
onBeforeUnmount(() => clearTimeout(timer));
</script>

<style scoped>
.run-head {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 12px;
}

.run-steps {
  list-style: none;
  margin: 0;
  padding: 0;
}

.run-step {
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 10px;
  margin-bottom: 8px;
  padding: 10px 14px;
}

.run-step summary {
  cursor: pointer;
}

.run-step-head {
  align-items: center;
  display: flex;
  gap: 8px;
}

.run-step-body {
  margin-top: 8px;
}

.run-json {
  background: rgb(var(--v-theme-surface-light));
  border-radius: 8px;
  font-size: 12px;
  margin: 4px 0 8px;
  max-height: 320px;
  overflow: auto;
  padding: 8px 10px;
  white-space: pre-wrap;
  word-break: break-word;
}

.run-text,
.run-error {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
</style>
