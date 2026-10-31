<template>
  <section class="builder" data-cy="flow-builder">
    <div v-if="loadError" class="pa-6">
      <v-alert type="error" variant="tonal">{{ loadError }}</v-alert>
    </div>
    <div v-else-if="!definition" class="d-flex justify-center pa-12">
      <v-progress-circular indeterminate color="primary" />
    </div>
    <template v-else>
      <header class="builder-bar">
        <v-btn :to="{ name: 'workflows' }" :aria-label="$t('workflows.back')" icon="mdi-arrow-left" size="small" variant="text" />
        <div class="builder-title">
          <div class="font-weight-medium text-truncate">{{ definition.name }}</div>
          <div class="text-caption text-medium-emphasis text-truncate" data-cy="flow-save-state">{{ saveLabel }}</div>
        </div>
        <v-chip size="small" label :color="statusColor" data-cy="flow-status">{{ statusLabel }}</v-chip>
        <v-spacer />
        <v-btn size="small" variant="text" class="text-none" prepend-icon="mdi-creation-outline" data-cy="flow-ask-ai" @click="askAssistant">
          {{ $t('workflows.askAI') }}
        </v-btn>
        <v-btn v-if="canEnroll" size="small" variant="tonal" class="text-none" prepend-icon="mdi-account-plus-outline" data-cy="flow-enroll-open" @click="enrollOpen = true">{{ $t('workflows.enroll.title') }}</v-btn>
        <template v-if="canManage">
          <v-btn size="small" variant="text" class="text-none" prepend-icon="mdi-auto-fix" @click="tidy">{{ $t('workflows.tidy') }}</v-btn>
          <v-btn size="small" variant="tonal" class="text-none" prepend-icon="mdi-play-outline" :disabled="hasErrors" data-cy="flow-test" @click="testOpen = true">{{ $t('workflows.test') }}</v-btn>
          <v-btn size="small" color="primary" variant="flat" class="text-none" :disabled="hasErrors || !hasUnpublished" :loading="publishing" data-cy="flow-publish" @click="publish">{{ $t('workflows.publish') }}</v-btn>
        </template>
        <v-switch
          v-if="canManage && workflow.published_version > 0"
          :model-value="workflow.enabled"
          :label="$t('workflows.enabled')"
          density="compact"
          color="primary"
          hide-details
          class="ml-2 flex-grow-0"
          @update:model-value="setEnabled"
        />
      </header>

      <v-alert v-if="validation.errors?.length" type="warning" variant="tonal" density="compact" class="builder-alert" data-cy="flow-errors">
        <div class="font-weight-medium">{{ $t('workflows.fixBefore') }}</div>
        <ul class="builder-issues"><li v-for="item in validation.errors" :key="item">{{ item }}</li></ul>
      </v-alert>
      <v-alert v-else-if="validation.warnings?.length" type="info" variant="tonal" density="compact" class="builder-alert">
        <ul class="builder-issues"><li v-for="item in validation.warnings" :key="item">{{ item }}</li></ul>
      </v-alert>

      <div class="builder-frame"><div class="builder-main">
        <aside v-if="canManage" class="builder-palette" :aria-label="$t('workflows.addStep')">
          <div class="text-caption text-medium-emphasis mb-2">{{ $t('workflows.addStep') }}</div>
          <button
            v-for="type in paletteTypes"
            :key="type"
            type="button"
            class="palette-item"
            :data-cy="`flow-add-${type}`"
            @click="addNode(type)"
          >
            <span class="palette-icon" :style="{ background: nodeTypes[type].color }"><v-icon :icon="nodeTypes[type].icon" size="14" color="white" /></span>
            <span>
              <span class="palette-label">{{ nodeTypes[type].label }}</span>
              <span class="palette-hint">{{ nodeTypes[type].hint }}</span>
            </span>
          </button>
          <button
            v-for="(step, tool) in toolSteps"
            :key="tool"
            type="button"
            class="palette-item"
            :data-cy="`flow-add-${step.id}`"
            @click="addNode('tool', tool)"
          >
            <span class="palette-icon" :style="{ background: step.color }"><v-icon :icon="step.icon" size="14" color="white" /></span>
            <span>
              <span class="palette-label">{{ step.label }}</span>
              <span class="palette-hint">{{ step.hint }}</span>
            </span>
          </button>
        </aside>

        <div class="builder-canvas">
          <VueFlow
            :nodes="flow.nodes"
            :edges="flow.edges"
            :node-types="canvasNodeTypes"
            :min-zoom="0.25"
            :max-zoom="1.4"
            :nodes-connectable="canManage"
            :nodes-draggable="canManage"
            :delete-key-code="null"
            fit-view-on-init
            @connect="onConnect"
            @node-click="({ node }) => (selectedId = node.id)"
            @pane-click="selectedId = ''"
            @edge-click="onEdgeClick"
            @node-drag-stop="onDragStop"
          >
            <Background :gap="22" :size="1.2" />
            <Controls position="bottom-left" />
          </VueFlow>
          <v-chip v-if="selectedEdge" class="builder-edge-action" closable prepend-icon="mdi-link-variant-off" data-cy="flow-edge-remove" @click="removeEdge" @click:close="selectedEdge = ''">
            {{ $t('workflows.removeConnection') }}
          </v-chip>
        </div>

        <aside class="builder-side">
          <flow-inspector
            v-if="selectedNode"
            :key="selectedNode.id"
            :node="selectedNode"
            :definition="definition"
            :catalog="catalog"
            :webhook-url="workflow.webhook_url || ''"
            :readonly="!canManage"
            @remove="deleteNode"
            @rename="rename"
          />
          <div v-else class="pa-4">
            <h2 class="text-subtitle-2 mb-2">{{ $t('workflows.recentRuns') }}</h2>
            <p v-if="!runs.length" class="text-body-2 text-medium-emphasis">{{ $t('workflows.noRuns') }}</p>
            <router-link v-for="run in runs" :key="run.id" :to="{ name: 'workflowRun', params: { id: run.id } }" class="side-run">
              <v-chip size="x-small" label :color="runColors[run.status]">{{ run.status }}</v-chip>
              <span class="text-caption text-medium-emphasis">{{ run.source === 'test' ? $t('workflows.testRun') : `v${run.version}` }} · {{ shortStamp(run.created) }}</span>
            </router-link>
            <template v-if="canManage">
              <v-divider class="my-3" />
              <v-textarea v-model="definition.description" :label="$t('workflows.field.description')" rows="2" auto-grow density="compact" variant="outlined" />
              <json-field v-model="budget" :label="$t('workflows.field.budget')" hint='{"max_usd_per_run": 0.5, "max_usd_per_day": 5}' class="mb-3" />
              <v-btn size="small" variant="text" class="text-none" prepend-icon="mdi-content-copy" @click="duplicate">{{ $t('workflows.duplicate') }}</v-btn>
              <v-btn size="small" variant="text" color="error" class="text-none" prepend-icon="mdi-trash-can-outline" @click="removeWorkflow">{{ $t('globals.buttons.delete') }}</v-btn>
            </template>
          </div>
        </aside>
      </div></div>

      <v-dialog v-model="testOpen" max-width="560">
        <v-card>
          <v-card-title>{{ $t('workflows.testTitle') }}</v-card-title>
          <v-card-text>
            <v-alert type="warning" variant="tonal" density="compact" class="mb-3">{{ $t('workflows.testWarning') }}</v-alert>
            <json-field v-model="testInput" :label="$t('workflows.field.input')" :rows="5" data-cy="flow-test-input" />
          </v-card-text>
          <v-card-actions>
            <v-spacer />
            <v-btn @click="testOpen = false">{{ $t('globals.buttons.cancel') }}</v-btn>
            <v-btn color="primary" variant="flat" :loading="testing" data-cy="flow-test-run" @click="runTest">{{ $t('workflows.runTest') }}</v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

      <flow-enroll-dialog v-model="enrollOpen" :workflow-id="workflow.id" @enrolled="loadRuns" />
    </template>
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, markRaw, onBeforeUnmount, onMounted, reactive, ref, watch,
} from 'vue';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import '@vue-flow/core/dist/style.css';
import '@vue-flow/core/dist/theme-default.css';
import '@vue-flow/controls/dist/style.css';
import { Background } from '@vue-flow/background';
import { Controls } from '@vue-flow/controls';
import { VueFlow, useVueFlow } from '@vue-flow/core';
import {
  deleteFlow, duplicateFlow, getFlow, getFlowCatalog, getFlowRuns, publishFlow, saveFlowDraft, setFlowEnabled, testFlow,
} from '../api';
import { assistantContext } from '../assistant/context';
import FlowEnrollDialog from '../components/flow/FlowEnrollDialog.vue';
import FlowInspector from '../components/flow/FlowInspector.vue';
import FlowNode from '../components/flow/FlowNode.vue';
import JsonField from '../components/flow/JsonField.vue';
import {
  connect, disconnect, ensureLayout, errorNodeIds, newNode, NODE_TYPES, PALETTE_TYPES, removeNode, renameNode, RUN_STATUS_COLORS, sameDefinition, tidyLayout, toFlow, TOOL_STEPS,
} from '../utils/flowGraph.mjs';
import { shortStamp } from '../utils/crm.mjs';

const SAVE_DELAY_MS = 700;

const { proxy } = getCurrentInstance();
const route = useRoute();
const router = useRouter();
const { fitView } = useVueFlow();

const nodeTypes = NODE_TYPES;
const paletteTypes = PALETTE_TYPES;
const toolSteps = TOOL_STEPS;
const runColors = RUN_STATUS_COLORS;
const canvasNodeTypes = { flow: markRaw(FlowNode) };

const workflow = ref({});
const definition = ref(null);
const validation = ref({ errors: [], warnings: [] });
const catalog = ref({});
const runs = ref([]);
const loadError = ref('');
const selectedId = ref('');
const selectedEdge = ref('');
const saveState = ref('saved');
const publishing = ref(false);
const testOpen = ref(false);
const testing = ref(false);
const testInput = ref({});
const enrollOpen = ref(false);
let saveTimer = null;
let applying = false;

const canManage = computed(() => proxy.$can('workflows:manage'));
const canEnroll = computed(() => proxy.$can('workflows:run', 'workflows:manage') && workflow.value.published_version > 0);
const selectedNode = computed(() => definition.value?.nodes.find((n) => n.id === selectedId.value) || null);
const hasErrors = computed(() => (validation.value.errors || []).length > 0);
const hasUnpublished = computed(() => {
  const published = workflow.value.published;
  return !published || !sameDefinition(published, definition.value);
});
const statusLabel = computed(() => {
  if (!workflow.value.published_version) return proxy.$t('workflows.status.draft');
  if (hasUnpublished.value) return proxy.$t('workflows.status.changed', { n: workflow.value.published_version });
  return workflow.value.enabled
    ? proxy.$t('workflows.status.live', { n: workflow.value.published_version })
    : proxy.$t('workflows.status.paused', { n: workflow.value.published_version });
});
const statusColor = computed(() => {
  if (!workflow.value.published_version) return 'default';
  if (hasUnpublished.value) return 'warning';
  return workflow.value.enabled ? 'success' : 'default';
});
const saveLabel = computed(() => proxy.$t(`workflows.save.${saveState.value}`));

const errorIds = computed(() => errorNodeIds(validation.value.errors));
const flow = computed(() => toFlow(definition.value, { errors: errorIds.value }));

const budget = computed({
  get: () => definition.value.ai?.budget,
  set(value) {
    if (value) {
      definition.value.ai = { ...(definition.value.ai || {}), budget: value };
    } else if (definition.value.ai) {
      delete definition.value.ai.budget;
    }
  },
});

function errorText(err) {
  return (err.response && err.response.message) || err.message;
}

function adopt(data) {
  workflow.value = data;
  applying = true;
  const def = reactive(data.draft || {});
  def.nodes ||= [];
  def.trigger ||= { type: 'manual' };
  ensureLayout(def);
  definition.value = def;
  validation.value = data.validation || { errors: [], warnings: [] };
  testInput.value = def.default_input || {};
  queueMicrotask(() => { applying = false; });
}

async function load() {
  loadError.value = '';
  try {
    const [data, cat] = await Promise.all([getFlow(route.params.id), getFlowCatalog()]);
    catalog.value = cat;
    adopt(data);
    await loadRuns();
  } catch (err) {
    loadError.value = errorText(err);
  }
}

async function loadRuns() {
  try {
    const data = await getFlowRuns({ workflow: route.params.id, limit: 15 });
    runs.value = Array.isArray(data) ? data : [];
  } catch {
    // The recent runs panel isn't worth interrupting editing over.
  }
}

async function saveNow() {
  clearTimeout(saveTimer);
  saveTimer = null;
  if (!canManage.value || !definition.value) return;
  saveState.value = 'saving';
  try {
    const out = await saveFlowDraft(workflow.value.id, definition.value);
    validation.value = out.validation;
    saveState.value = saveTimer ? 'unsaved' : 'saved';
  } catch (err) {
    saveState.value = 'failed';
    proxy.$utils.toast(errorText(err), 'error');
  }
}

watch(definition, () => {
  if (applying || !definition.value || !canManage.value) return;
  saveState.value = 'unsaved';
  clearTimeout(saveTimer);
  saveTimer = setTimeout(saveNow, SAVE_DELAY_MS);
}, { deep: true });

function addNode(type, tool = '') {
  const def = definition.value;
  const anchor = selectedNode.value || def.nodes[def.nodes.length - 1];
  const position = anchor?.ui ? { x: anchor.ui.x + 40, y: anchor.ui.y + 150 } : { x: 0, y: 0 };
  const node = newNode(def, type, position, tool);
  def.nodes.push(node);
  // Adding after a step with an open exit joins them, which is what's wanted nearly always.
  if (anchor && !anchor.next && !['branch', 'end', 'foreach', 'loop'].includes(anchor.type)) {
    anchor.next = node.id;
  }
  selectedId.value = node.id;
}

function deleteNode(id) {
  removeNode(definition.value, id);
  selectedId.value = '';
}

function rename(oldId, newId) {
  const next = renameNode(definition.value, oldId, String(newId).trim());
  if (!next) {
    proxy.$utils.toast(proxy.$t('workflows.field.idInvalid'), 'error');
    return;
  }
  definition.value = reactive(next);
  selectedId.value = newId.trim();
}

function onConnect(params) {
  connect(definition.value, params);
}

function onEdgeClick({ edge }) {
  if (canManage.value) selectedEdge.value = edge.id;
}

function removeEdge() {
  disconnect(definition.value, selectedEdge.value);
  selectedEdge.value = '';
}

function onDragStop({ node }) {
  const target = definition.value.nodes.find((n) => n.id === node.id);
  if (target) target.ui = { x: Math.round(node.position.x), y: Math.round(node.position.y) };
}

function tidy() {
  tidyLayout(definition.value);
  setTimeout(() => fitView({ padding: 0.2 }), 50);
}

async function publish() {
  publishing.value = true;
  try {
    await saveNow();
    const out = await publishFlow(workflow.value.id);
    proxy.$utils.toast(proxy.$t('workflows.published', { n: out.version }));
    adopt(await getFlow(workflow.value.id));
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  } finally {
    publishing.value = false;
  }
}

async function setEnabled(enabled) {
  try {
    const out = await setFlowEnabled(workflow.value.id, Boolean(enabled));
    workflow.value = { ...workflow.value, enabled: out.enabled };
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  }
}

async function runTest() {
  testing.value = true;
  try {
    await saveNow();
    const out = await testFlow(workflow.value.id, testInput.value || {});
    testOpen.value = false;
    router.push({ name: 'workflowRun', params: { id: out.run_id } });
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  } finally {
    testing.value = false;
  }
}

async function duplicate() {
  try {
    const out = await duplicateFlow(workflow.value.id);
    router.push({ name: 'workflowBuilder', params: { id: out.id } });
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  }
}

function removeWorkflow() {
  proxy.$utils.confirm(proxy.$t('workflows.deleteConfirm', { name: definition.value.name }), async () => {
    try {
      clearTimeout(saveTimer);
      saveTimer = null;
      await deleteFlow(workflow.value.id);
      router.push({ name: 'workflows' });
    } catch (err) {
      proxy.$utils.toast(errorText(err), 'error');
    }
  });
}

function askAssistant() {
  assistantContext.open = true;
}

// The assistant changes drafts on the server, so pick its changes up when one's applied.
async function onAssistantApplied() {
  if (saveTimer) await saveNow();
  try {
    adopt(await getFlow(workflow.value.id));
    await loadRuns();
    setTimeout(() => fitView({ padding: 0.2 }), 50);
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  }
}

onBeforeRouteLeave(async () => {
  if (saveTimer) await saveNow();
});

watch(() => route.params.id, (id, old) => {
  if (id && id !== old) {
    definition.value = null;
    selectedId.value = '';
    load();
  }
});

onMounted(() => {
  load();
  window.addEventListener('assistant-applied', onAssistantApplied);
});

onBeforeUnmount(() => {
  window.removeEventListener('assistant-applied', onAssistantApplied);
  if (saveTimer) saveNow();
});
</script>

<style scoped>
.builder {
  --builder-border: rgba(var(--v-border-color), var(--v-border-opacity));
  background: rgb(var(--v-theme-surface));
  border: 1px solid var(--builder-border);
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  height: calc(100vh - 112px);
  min-height: 560px;
  overflow: hidden;
}

.builder-bar {
  align-items: center;
  border-bottom: 1px solid var(--builder-border);
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 8px 12px;
}

.builder-title {
  max-width: 280px;
  min-width: 0;
}

.builder-alert {
  border-radius: 0;
  flex: none;
  max-height: 140px;
  overflow-y: auto;
}

.builder-issues {
  font-size: 13px;
  margin: 0;
  padding-left: 18px;
}

.builder-main {
  display: flex;
  flex: 1;
  min-height: 0;
}

.builder-frame {
  container: builder / inline-size;
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}

.builder-palette {
  border-right: 1px solid var(--builder-border);
  flex: none;
  overflow-y: auto;
  padding: 12px;
  width: 200px;
}

.palette-item {
  align-items: center;
  background: none;
  border: 0;
  border-radius: 8px;
  color: inherit;
  cursor: pointer;
  display: flex;
  font: inherit;
  gap: 8px;
  margin-bottom: 2px;
  padding: 6px;
  text-align: left;
  width: 100%;
}

.palette-item:hover {
  background: rgb(var(--v-theme-surface-light));
}

.palette-item:focus-visible {
  outline: 2px solid rgb(var(--v-theme-primary));
}

.palette-icon {
  align-items: center;
  border-radius: 6px;
  display: flex;
  flex: none;
  height: 24px;
  justify-content: center;
  width: 24px;
}

.palette-label {
  display: block;
  font-size: 13px;
  font-weight: 500;
}

.palette-hint {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  display: block;
  font-size: 11px;
  line-height: 1.3;
}

.builder-canvas {
  flex: 1;
  min-width: 0;
  position: relative;
}

.builder-edge-action {
  left: 50%;
  position: absolute;
  top: 12px;
  transform: translateX(-50%);
  z-index: 5;
}

.builder-side {
  border-left: 1px solid var(--builder-border);
  flex: none;
  overflow-y: auto;
  width: 340px;
}

.side-run {
  align-items: center;
  color: inherit;
  display: flex;
  gap: 8px;
  padding: 4px 0;
  text-decoration: none;
}

.side-run:hover {
  text-decoration: underline;
}

/* Sized by the space the builder really has, because the assistant can take half the screen. */
@container builder (max-width: 1000px) {
  .builder-palette {
    padding: 8px;
    width: 52px;
  }

  .builder-palette > .text-caption,
  .palette-item > span:last-child {
    display: none;
  }

  .palette-item {
    justify-content: center;
    padding: 6px 0;
  }

  .builder-side {
    width: 280px;
  }
}

/* The canvas needs room, so when it's tight the panels stack beneath it. */
@container builder (max-width: 680px) {
  .builder-main {
    flex-direction: column;
    overflow-y: auto;
  }

  .builder-palette {
    border-right: 0;
    border-bottom: 1px solid var(--builder-border);
    display: flex;
    flex: none;
    flex-wrap: wrap;
    gap: 4px;
    width: 100%;
  }

  .palette-item {
    width: 36px;
  }

  .builder-canvas {
    flex: none;
    height: 420px;
  }

  .builder-side {
    border-left: 0;
    border-top: 1px solid var(--builder-border);
    width: 100%;
  }
}
</style>
