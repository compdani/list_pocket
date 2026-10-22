<script setup>
/* eslint-disable */
import { computed, markRaw, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import "@vue-flow/core/dist/style.css";
import "@vue-flow/core/dist/theme-default.css";
import "@vue-flow/controls/dist/style.css";
import "@vue-flow/minimap/dist/style.css";
import { Background } from "@vue-flow/background";
import { Controls } from "@vue-flow/controls";
import { MiniMap } from "@vue-flow/minimap";
import { useVueFlow, VueFlow } from "@vue-flow/core";
import { useTheme } from "vuetify";
import { arrangeWorkflowGraph, canConnectWorkflowNodes } from "../workflow/graph.mjs";
import { nodePresentation } from "../workflow/nodeCatalog.mjs";
import { openConfirm } from "../utils/confirmDialog";
import { useBuilderState } from "../composables/useBuilderState";
import { useRouter } from "vue-router";
import { registerAssistantEditor } from '../assistant/context';
import { assistantJSON } from '../assistant/client';
import NodeInspector from "./NodeInspector.vue";
import AppRightSidebar from "./AppRightSidebar.vue";
import WorkflowNodeCard from "./WorkflowNodeCard.vue";

const props = defineProps({
  contacts: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  nodeLibrary: { type: Array, default: () => [] },
  onSaveRequest: { type: Function, default: null },
  saveMessage: { type: String, default: "" },
  saveState: { type: String, default: "idle" },
  validationErrors: { type: Array, default: () => [] },
  validationFindings: { type: Array, default: () => [] },
  workflow: { type: Object, default: null },
});

const emit = defineEmits(["captureSchema", "createWorkflow", "deleteWorkflow", "publish", "run", "save", "updateWorkflowName", "validate"]);

const assistantRouter = useRouter();
let unregisterAssistant;
const builder = useBuilderState(computed(() => props.workflow));
const { fitView, setCenter, setViewport, viewport, zoomIn, zoomOut } = useVueFlow();

// Canvas colors passed as props, so they can't come from CSS theme variables.
const theme = useTheme();
const canvasColors = computed(() => (theme.current.value.dark
  ? { pattern: "#2a323d", mask: "rgba(15, 19, 24, 0.6)", node: "#3a4452" }
  : { pattern: "#d9dee7", mask: "rgba(240, 240, 240, 0.6)", node: "#e2e2e2" }));
const committedSignature = ref("");
const initializedWorkflowId = ref("");
const isDirty = ref(false);
const nodeInspectorRef = ref(null);
const nodeTypes = { workflow: markRaw(WorkflowNodeCard) };
const restoredViewport = ref({ x: 0, y: 0, zoom: 1 });
const hasRestoredViewport = ref(false);
const showNodeCommand = ref(false);
const commandSearch = ref("");
const librarySearch = ref("");
const libraryOpen = ref(true);
const pastGraphs = ref([]); const futureGraphs = ref([]); let restoringGraph = false;
const actionBusy = ref(false);
const editorNotice = ref("");
const hasTrigger = computed(() => builder.nodes.value.some(node => node.data?.type === 'trigger'));
const libraryGroups = computed(() => {
  const needle = (librarySearch.value || "").toLowerCase();
  return ['Start', 'Logic', 'Timing', 'Actions'].map(category => ({ category, nodes: props.nodeLibrary.filter(node => {
    const presentation = nodePresentation(node.type);
    return presentation.category === category && `${node.label} ${presentation.description}`.toLowerCase().includes(needle);
  }) })).filter(group => group.nodes.length);
});
const operationDisabled = computed(() => !props.workflow || props.loading || props.saveState === 'saving' || actionBusy.value);
const saveLabel = computed(() => isDirty.value ? 'Unsaved changes' : props.saveMessage || 'Ready');

const selectedNode = computed(() => builder.nodes.value.find((node) => node.id === builder.selectedNodeId.value));
const selectedEdge = computed(() => builder.edges.value.find((edge) => edge.id === builder.selectedEdgeId.value));
const selectedNodeDescription = computed(() => selectedNode.value?.data?.description ?? "");
const currentSignature = computed(() => JSON.stringify({ ...builder.exportWorkflowGraph(), name: props.workflow?.workflow?.name || "" }));
const selectedEdgeExpression = computed(() => selectedEdge.value?.data?.condition?.expression ?? "");
const webhookEndpoint = computed(() => {
  const trigger = props.workflow?.nodes.find((node) => node.type === "trigger");
  if (String(trigger?.config?.mode ?? "webhook") !== "webhook") {
    return "";
  }

  const rawPath = String(trigger?.config?.path ?? "").trim();
  if (!rawPath) {
    return "";
  }

  let normalized = `/${rawPath.replace(/^\/+/, "")}`;
  if (normalized.startsWith("/hooks/")) {
    normalized = normalized.slice("/hooks".length);
  }

  return `/api/hooks${normalized}`;
});
const nodeFindingIds = computed(() => new Set((props.validationFindings ?? []).filter((finding) => finding.targetType === "node").map((finding) => finding.targetId)));
const edgeFindingIds = computed(() => new Set((props.validationFindings ?? []).filter((finding) => finding.targetType === "edge").map((finding) => finding.targetId)));
const decoratedNodes = computed(() => builder.nodes.value.map((node) => ({ ...node, class: nodeFindingIds.value.has(node.id) ? "flow-node-invalid" : undefined })));
const decoratedEdges = computed(() => builder.edges.value.map((edge) => ({ ...edge, class: edgeFindingIds.value.has(edge.id) ? "flow-edge-invalid" : undefined })));
const showNodeModal = computed(() => Boolean(selectedNode.value));
const selectedNodeLabel = computed(() => selectedNode.value?.data?.label ?? "Selected Node");
const canCaptureSelectedNodeSchema = computed(() => selectedNode.value?.data?.type === "trigger" && String(selectedNode.value?.data?.config?.mode ?? "manual") === "webhook");
const filteredNodeLibrary = computed(() => {
  const needle = commandSearch.value.trim().toLowerCase();
  if (!needle) {
    return props.nodeLibrary;
  }
  return props.nodeLibrary.filter((node) => String(node.label ?? "").toLowerCase().includes(needle) || String(node.type ?? "").toLowerCase().includes(needle));
});

watch(
  () => props.workflow,
  async () => {
    await nextTick();

    if (!props.workflow) {
      committedSignature.value = currentSignature.value;
      initializedWorkflowId.value = "";
      isDirty.value = false;
      restoredViewport.value = { x: 0, y: 0, zoom: 1 };
      hasRestoredViewport.value = false;
      return;
    }

    if (!initializedWorkflowId.value || initializedWorkflowId.value !== props.workflow.workflow.id) {
      initializedWorkflowId.value = props.workflow.workflow.id;
      committedSignature.value = currentSignature.value;
      isDirty.value = false;
      const savedViewport = loadSavedViewport(props.workflow.workflow.id);
      hasRestoredViewport.value = Boolean(savedViewport);
      restoredViewport.value = savedViewport ?? { x: 0, y: 0, zoom: 1 };

      if (savedViewport) {
        await nextTick();
        setViewport(savedViewport, { duration: 0 });
      }
    }
  },
  { immediate: true }
);

watch(currentSignature, (signature, previous) => {
  if (!restoringGraph && previous && previous !== signature) { pastGraphs.value.push(previous); if (pastGraphs.value.length > 50) pastGraphs.value.shift(); futureGraphs.value = []; }
  if (!props.workflow) {
    return;
  }

  isDirty.value = signature !== committedSignature.value;
});

watch(
  viewport,
  (nextViewport) => {
    persistViewport(nextViewport);
  },
  { deep: true }
);

onBeforeUnmount(() => {
  unregisterAssistant?.();
  window.removeEventListener("keydown", onWindowKeydown);
  window.removeEventListener('beforeunload', warnBeforeUnload);
});

onMounted(() => {
  unregisterAssistant = registerAssistantEditor({
    get key() { return `workflow:${props.workflow?.workflow?.id || 'new'}`; },
    label: 'Unsaved workflow editor',
    snapshot: () => ({ kind: 'workflow', workflowId: props.workflow?.workflow?.id, savedRevision: props.workflow?.workflow?.revision, ...builder.exportWorkflowGraph() }),
    apply: async (graph) => {
      const signature = currentSignature.value;
      await assistantJSON('workflows/validate', { graph });
      if (currentSignature.value !== signature) throw new Error('The workflow changed during validation. Request a fresh proposal.');
      if (props.workflow?.workflow?.status === 'published') {
        const draft = await assistantJSON('workflows/draft', { id: props.workflow.workflow.id, revision: graph.savedRevision, graph: { name: props.workflow.workflow.name, nodes: graph.nodes, edges: graph.edges } });
        await assistantRouter.push({ name: 'workflowBuilder', params: { workflowId: draft.id } });
      } else { builder.replaceGraph(graph); }
    },
  });
  window.addEventListener("keydown", onWindowKeydown);
  window.addEventListener('beforeunload', warnBeforeUnload);
});

function saveNodeConfig(key, value) {
  if (selectedNode.value) {
    builder.updateNodeConfig(selectedNode.value.id, key, value);
  }
}

function saveNodeLabel(value) {
  if (selectedNode.value) {
    builder.updateNodeLabel(selectedNode.value.id, value);
  }
}

function applyNodeConfigValues(nodeId, updates) {
  Object.entries(updates).forEach(([key, value]) => {
    builder.updateNodeConfig(nodeId, key, value);
  });
}

function removeSelectedNode() {
  builder.removeSelectedNode();
}

async function closeNodeModal() {
  await nodeInspectorRef.value?.flushPendingChanges?.();
  builder.clearSelection();
}
function warnBeforeUnload(event) {
  if (isDirty.value) { event.preventDefault(); event.returnValue = ''; }
}
async function selectCanvasNode(id) {
  await nodeInspectorRef.value?.flushPendingChanges?.(); builder.selectNode(id);
}
async function selectCanvasEdge(id) {
  await nodeInspectorRef.value?.flushPendingChanges?.(); builder.selectEdge(id);
}

function removeSelectedEdge() {
  builder.removeSelectedEdge();
}

function updateSelectedEdgeField(field, event) {
  const value = typeof event === "string" ? event : event?.target?.value ?? "";
  builder.updateSelectedEdgeField(field, value);
}

function focusFinding(finding) {
  if (finding.targetType === "node" && finding.targetId) {
    builder.selectNode(finding.targetId);
    void focusNode(finding.targetId);
  }

  if (finding.targetType === "edge" && finding.targetId) {
    builder.selectEdge(finding.targetId);
  }
}

function normalizeViewport(value) {
  const x = Number(value?.x);
  const y = Number(value?.y);
  const zoom = Number(value?.zoom);
  if (!Number.isFinite(x) || !Number.isFinite(y) || !Number.isFinite(zoom)) {
    return null;
  }

  return { x, y, zoom: Math.min(2, Math.max(0.1, zoom)) };
}

function loadSavedViewport(workflowId) {
  if (!workflowId || typeof window === "undefined") {
    return null;
  }

  try {
    const raw = window.localStorage.getItem(`workflow-builder:viewport:${workflowId}`);
    if (!raw) {
      return null;
    }
    return normalizeViewport(JSON.parse(raw));
  } catch (_error) {
    return null;
  }
}

function persistViewport(viewport) {
  if (!props.workflow?.workflow?.id || typeof window === "undefined") {
    return;
  }

  const normalized = normalizeViewport(viewport);
  if (!normalized) {
    return;
  }

  restoredViewport.value = normalized;
  hasRestoredViewport.value = true;
  window.localStorage.setItem(`workflow-builder:viewport:${props.workflow.workflow.id}`, JSON.stringify(normalized));
}

async function fitCanvasToGraph() {
  await nextTick();
  if (typeof fitView === "function") {
    fitView({ padding: 0.22, minZoom: 0.2, maxZoom: 1.1, duration: 180 });
  }
}

function centerOnSelection() {
  if (selectedNode.value) {
    void focusNode(selectedNode.value.id);
    return;
  }

  void fitCanvasToGraph();
}

function clearSavedViewport() {
  if (!props.workflow?.workflow?.id || typeof window === "undefined") {
    return;
  }

  window.localStorage.removeItem(`workflow-builder:viewport:${props.workflow.workflow.id}`);
  hasRestoredViewport.value = false;
  restoredViewport.value = { x: 0, y: 0, zoom: 1 };
  setViewport(restoredViewport.value, { duration: 120 });
  void fitCanvasToGraph();
}

function openNodeCommand() {
  showNodeCommand.value = true;
  commandSearch.value = "";
}

function closeNodeCommand() {
  showNodeCommand.value = false;
  commandSearch.value = "";
}

function addNodeFromCommand(type) {
  void addNode(type);
  closeNodeCommand();
}

function onWindowKeydown(event) {
  const target = event.target;
  const tag = target?.tagName ? String(target.tagName).toLowerCase() : "";
  const isTypingTarget = tag === "input" || tag === "textarea" || target?.isContentEditable;
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
    if (props.workflow) { event.preventDefault(); void saveWorkflow('manual'); } return;
  }
  if (isTypingTarget || showNodeCommand.value) {
    return;
  }

  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'z') {
    event.preventDefault(); void restoreGraph(event.shiftKey ? 'redo' : 'undo'); return;
  }
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'd' && selectedNode.value) {
    event.preventDefault(); void duplicateNode(); return;
  }
  if ((event.metaKey || event.ctrlKey) && event.key === "0") {
    event.preventDefault();
    void clearSavedViewport();
    return;
  }

  if (event.key === "f" || event.key === "F") {
    event.preventDefault();
    void fitCanvasToGraph();
    return;
  }

  if (event.key === "a" || event.key === "A") {
    event.preventDefault();
    openNodeCommand();
    return;
  }

  if (event.key === "=" || event.key === "+") {
    event.preventDefault();
    zoomIn?.({ duration: 120 });
    return;
  }

  if (event.key === "-") {
    event.preventDefault();
    zoomOut?.({ duration: 120 });
  }
}

function onNodeDragStop({ node }) {
  builder.updateNodePosition(node.id, node.position.x, node.position.y);
}

async function saveWorkflow(mode = "manual") {
  if (!props.workflow) {
    return;
  }

  if (mode === "manual") {
    await nodeInspectorRef.value?.flushPendingChanges?.();
  }

  const graph = builder.exportWorkflowGraph();
  const payload = {
    workflow: props.workflow.workflow,
    nodes: graph.nodes,
    edges: graph.edges,
  };

  if (props.onSaveRequest) {
    const savedSignature = JSON.stringify({ ...graph, name: payload.workflow.name?.trim() || 'Untitled Workflow' });
    const saved = await props.onSaveRequest(payload, mode);
    if (saved !== false) {
      committedSignature.value = savedSignature;
      isDirty.value = currentSignature.value !== savedSignature;
    }
    return saved;
  }

  emit("save", payload, mode);
}

async function restoreGraph(direction) {
  if (operationDisabled.value) return;
  await nodeInspectorRef.value?.flushPendingChanges?.();
  await nextTick();
  const source = direction === 'undo' ? pastGraphs : futureGraphs;
  const destination = direction === 'undo' ? futureGraphs : pastGraphs;
  if (!source.value.length) return;
  const graph = source.value.pop(); destination.value.push(currentSignature.value);
  restoringGraph = true;
  const restored = JSON.parse(graph); builder.replaceGraph(restored);
  if (typeof restored.name === 'string') emit('updateWorkflowName', restored.name);
  await nextTick(); restoringGraph = false;
}
async function duplicateNode() {
  await nodeInspectorRef.value?.flushPendingChanges?.();
  const id = builder.duplicateSelectedNode(); if (id) await focusNode(id);
}
async function arrangeNodes() {
  builder.replaceGraph(arrangeWorkflowGraph(builder.exportWorkflowGraph())); await fitCanvasToGraph();
}
async function performAction(action) {
  if (operationDisabled.value) return;
  if (action === 'publish' && !(await openConfirm({ message: 'Publish this workflow and enable its automatic triggers?' }))) return;
  actionBusy.value = true;
  try {
    const saved = await saveWorkflow('manual');
    if (saved === false) return;
    if (isDirty.value) { editorNotice.value = 'The graph changed while saving. Save your latest edits before continuing.'; return; }
    editorNotice.value = '';
    emit(action, props.workflow.workflow.id);
  } finally { actionBusy.value = false; }
}
async function requestNewWorkflow() {
  if (isDirty.value && !(await openConfirm({ message: 'Start a new workflow? Unsaved changes in this graph will be discarded.' }))) return;
  emit('createWorkflow');
}
function isValidConnection(connection) {
  return canConnectWorkflowNodes(builder.nodes.value, builder.edges.value, connection.source, connection.target, connection.sourceHandle, connection.id);
}

async function focusNode(nodeId) {
  const node = builder.nodes.value.find((item) => item.id === nodeId);
  if (!node) {
    return;
  }

  await nextTick();
  setCenter(node.position.x + 120, node.position.y + 50, {
    duration: 250,
    zoom: 1,
  });
}

async function addNode(type) {
  await nodeInspectorRef.value?.flushPendingChanges?.();
  const nodeId = builder.addNode(type);
  if (!nodeId) {
    return;
  }

  if (type === 'trigger') builder.updateNodeConfig(nodeId, 'mode', 'manual');
  builder.selectNode(nodeId);
  await focusNode(nodeId);
}

defineExpose({
  hasUnsavedChanges: () => isDirty.value,
  addNode,
  applyNodeConfigValues,
  openNodeCommand,
});
</script>

<template>
  <section class="builder-shell">
    <v-sheet class="builder-toolbar builder-toolbar-refresh" color="transparent">
      <div class="builder-heading">
        <span class="builder-eyebrow">WORKFLOW STUDIO</span>
        <div class="builder-editor-bar">
          <v-text-field v-if="workflow" class="workflow-title-input" :model-value="workflow.workflow.name" placeholder="Untitled workflow" variant="plain" density="compact" hide-details aria-label="Workflow name" @update:model-value="emit('updateWorkflowName', $event || '')" />
          <h2 v-else class="builder-welcome-title">Make your work flow.</h2>
          <v-chip size="small" class="save-indicator" :color="saveState === 'error' ? 'error' : isDirty ? 'warning' : 'success'" variant="tonal" role="status">{{ saveLabel }}</v-chip>
        </div>
        <div v-if="workflow" class="builder-meta">
          <v-chip size="x-small" :color="workflow.workflow.status === 'published' ? 'success' : 'primary'" variant="tonal">{{ workflow.workflow.status }}</v-chip>
          <span class="builder-meta-caption">Version {{ workflow.workflow.version }} · {{ builder.nodes.value.length }} {{ builder.nodes.value.length === 1 ? 'step' : 'steps' }} · {{ builder.edges.value.length }} {{ builder.edges.value.length === 1 ? 'connection' : 'connections' }}</span>
        </div>
      </div>
      <div class="toolbar-actions toolbar-actions-compact toolbar-actions-primary">
        <v-btn variant="text" type="button" :disabled="loading || saveState === 'saving'" @click="requestNewWorkflow">New workflow</v-btn>
        <v-btn variant="outlined" :disabled="operationDisabled" @click="saveWorkflow('manual')">{{ saveState === 'saving' ? 'Saving...' : 'Save' }}</v-btn>
        <v-btn variant="tonal" color="primary" :disabled="operationDisabled || !builder.nodes.value.length" @click="performAction('run')">Run Test</v-btn>
        <v-btn color="primary" variant="flat" :disabled="operationDisabled || !builder.nodes.value.length" @click="performAction('publish')">Publish</v-btn>
        <v-menu location="bottom end">
          <template #activator="{ props: menuProps }"><v-btn v-bind="menuProps" icon="mdi-dots-vertical" size="small" variant="text" aria-label="More workflow actions" /></template>
          <v-list density="compact"><v-list-item title="Delete workflow" prepend-icon="mdi-delete-outline" base-color="error" :disabled="operationDisabled" @click="workflow && emit('deleteWorkflow', workflow.workflow.id)" /></v-list>
        </v-menu>
      </div>
    </v-sheet>
    <v-alert v-if="editorNotice" type="warning" variant="tonal" closable @click:close="editorNotice = ''">{{ editorNotice }}</v-alert>
    <div v-if="workflow" class="builder-editing-bar" role="toolbar" aria-label="Graph editing">
      <v-btn size="small" variant="text" :aria-pressed="libraryOpen" @click="libraryOpen = !libraryOpen">{{ libraryOpen ? 'Hide steps' : 'Show steps' }}</v-btn>
      <span class="editing-divider" />
      <v-btn size="small" variant="text" :disabled="!pastGraphs.length || operationDisabled" aria-keyshortcuts="Control+Z Meta+Z" @click="restoreGraph('undo')">Undo</v-btn>
      <v-btn size="small" variant="text" :disabled="!futureGraphs.length || operationDisabled" aria-keyshortcuts="Control+Shift+Z Meta+Shift+Z" @click="restoreGraph('redo')">Redo</v-btn>
      <v-btn size="small" variant="text" :disabled="!builder.nodes.value.length" @click="arrangeNodes">Auto-arrange</v-btn>
      <v-spacer />
      <v-btn size="small" variant="text" :color="validationErrors.length ? 'error' : undefined" :disabled="operationDisabled || !builder.nodes.value.length" @click="performAction('validate')">{{ validationErrors.length ? `Check issues (${validationErrors.length})` : 'Validate' }}</v-btn>
      <span class="builder-keyboard-hint">A add · F fit · ⌘/Ctrl S save</span>
    </div>

    <div v-if="workflow" class="builder-body builder-body-single builder-workspace" :class="{ 'library-hidden': !libraryOpen }">
      <aside v-if="libraryOpen" class="workflow-step-library" aria-label="Step library">
        <div class="step-library-heading"><strong>Add a step</strong><span>Click to add, then connect.</span></div>
        <v-text-field v-model="librarySearch" placeholder="Find a step…" aria-label="Search steps" prepend-inner-icon="mdi-magnify" variant="outlined" density="compact" hide-details clearable />
        <div class="step-library-scroll">
          <section v-for="group in libraryGroups" :key="group.category" class="step-library-group">
            <h3>{{ group.category }}</h3>
            <button v-for="node in group.nodes" :key="node.type" class="step-library-item" type="button" :disabled="node.type === 'trigger' && hasTrigger" :title="node.type === 'trigger' && hasTrigger ? 'This workflow already has a trigger' : nodePresentation(node.type).description" @click="addNode(node.type)">
              <span class="step-library-icon" :data-color="nodePresentation(node.type).color"><v-icon :icon="nodePresentation(node.type).icon" size="19" /></span>
              <span><strong>{{ node.label }}</strong><small>{{ nodePresentation(node.type).description }}</small></span>
              <span class="step-library-plus">+</span>
            </button>
          </section>
          <p v-if="!libraryGroups.length" class="text-caption pa-3">No matching steps. Try another search.</p>
        </div>
      </aside>
      <v-sheet class="canvas-frame" border>
        <v-sheet class="canvas-action-dock" role="toolbar" aria-label="Canvas actions" elevation="0">
          <v-btn type="button" size="small" variant="outlined" class="canvas-action-btn canvas-action-btn-wide" title="Add node (A)" @click="openNodeCommand">+ Add node</v-btn>
          <v-btn type="button" size="small" variant="outlined" class="canvas-action-btn canvas-action-btn-wide" title="Center selected node" @click="centerOnSelection">Center</v-btn>
          <v-btn type="button" size="small" variant="outlined" class="canvas-action-btn canvas-action-btn-wide" title="Reset saved view" @click="clearSavedViewport">Reset</v-btn>
        </v-sheet>
        <div v-if="!builder.nodes.value.length" class="canvas-start-guide">
          <div class="start-guide-icon"><v-icon icon="mdi-rocket-launch-outline" size="28" /></div>
          <h2>Every good workflow starts somewhere.</h2>
          <p>Add a trigger, connect your steps, and let the busywork take care of itself.</p>
          <v-btn color="primary" @click="addNode('trigger')">Add trigger</v-btn>
        </div>
        <VueFlow :is-valid-connection="isValidConnection" :delete-key-code="null" @pane-click="closeNodeModal" :node-types="nodeTypes" :nodes="decoratedNodes" :edges="decoratedEdges" :default-viewport="restoredViewport" :fit-view-on-init="!hasRestoredViewport" :min-zoom="0.2" :max-zoom="1.2" :pan-on-scroll="true" :pan-on-drag="true" :zoom-on-double-click="false" :zoom-on-scroll="true" :selection-on-drag="false" class="workflow-canvas" @connect="builder.connectNodes" @edge-click="({ edge }) => selectCanvasEdge(edge.id)" @node-click="({ node }) => selectCanvasNode(node.id)" @node-drag-stop="onNodeDragStop">
          <MiniMap position="bottom-right" :pannable="true" :mask-color="canvasColors.mask" :node-color="canvasColors.node" />
          <Controls position="bottom-left" />
          <Background :gap="22" :size="1.2" :pattern-color="canvasColors.pattern" />
        </VueFlow>

        <v-dialog v-model="showNodeCommand" width="560">
          <v-card class="node-command-card">
            <div class="node-command-header">
              <strong>Add node</strong>
              <v-btn :aria-label="$t('globals.buttons.close')" type="button" size="small" icon="mdi-close" variant="text" @click="closeNodeCommand" />
            </div>
            <v-text-field v-model="commandSearch" class="node-command-input" placeholder="Search nodes (A)" autofocus hide-details />
            <v-list class="node-command-list">
              <v-list-item v-for="node in filteredNodeLibrary" :key="node.type" :disabled="node.type === 'trigger' && hasTrigger" class="node-command-item" @click="addNodeFromCommand(node.type)">
                <v-list-item-title>{{ node.label }}</v-list-item-title>
                <v-list-item-subtitle>{{ nodePresentation(node.type).description }}</v-list-item-subtitle>
              </v-list-item>
              <p v-if="!filteredNodeLibrary.length" class="node-command-empty">No matching node types.</p>
            </v-list>
          </v-card>
        </v-dialog>
      </v-sheet>
    </div>

    <div v-if="workflow && (validationErrors?.length || selectedEdge)" class="builder-secondary-panels">
      <v-card v-if="validationErrors?.length" class="panel validation-panel detail-panel" variant="outlined">
        <div class="panel-header">
          <h2>Validation</h2>
          <p>Click an issue to find the step that needs attention.</p>
        </div>
        <p v-if="!validationFindings.length" v-for="message in validationErrors" :key="message" class="pa-3 text-error">{{ message }}</p>
        <v-btn v-for="finding in validationFindings" :key="`${finding.code}:${finding.targetId}:${finding.message}`" class="validation-item" variant="text" @click="focusFinding(finding)">
          <span class="validation-severity" :data-severity="finding.severity">{{ finding.severity }}</span>
          {{ finding.message }}
        </v-btn>
      </v-card>

      <v-card v-if="selectedEdge" class="panel edge-panel detail-panel" variant="outlined">
        <div class="panel-header">
          <h2>Transition</h2>
          <p>{{ builder.nodes.value.find(node => node.id === selectedEdge.source)?.data?.label || selectedEdge.source }} → {{ builder.nodes.value.find(node => node.id === selectedEdge.target)?.data?.label || selectedEdge.target }}</p>
        </div>
        <label class="form-field">
          <span>Branch Label</span>
          <v-text-field :model-value="String(selectedEdge.label ?? '')" hide-details @update:model-value="updateSelectedEdgeField('branch', $event || '')" />
        </label>
        <label class="form-field">
          <span>Condition</span>
          <v-textarea rows="4" :model-value="selectedEdgeExpression" hide-details @update:model-value="updateSelectedEdgeField('expression', $event || '')" />
        </label>
        <v-btn type="button" color="error" variant="outlined" @click="removeSelectedEdge">Delete Edge</v-btn>
      </v-card>
    </div>

    <v-card v-if="!workflow" class="panel detail-panel builder-empty-state" variant="outlined">
      <div class="panel-header">
        <h2>No workflow selected</h2>
        <p>Create or select a workflow to start editing the graph.</p>
        <v-btn class="mt-4" type="button" color="primary" :loading="!loading && saveState === 'saving'" :disabled="loading || saveState === 'saving'" @click="emit('createWorkflow')">Create workflow</v-btn>
      </div>
    </v-card>

    <AppRightSidebar :model-value="showNodeModal" :width="480" @update:model-value="(next) => !next && closeNodeModal()" @close="closeNodeModal">
      <template #header>
        <div class="modal-header mt-2 px-2">
          <div>
            <span class="builder-eyebrow">Node Settings</span>
            <h2>{{ selectedNodeLabel }}</h2>
            <p v-if="selectedNodeDescription" class="field-help">{{ selectedNodeDescription }}</p>
          </div>
          <div class="node-modal-actions">
            <v-btn v-if="canCaptureSelectedNodeSchema" type="button" variant="outlined" size="small" @click="workflow && selectedNode && emit('captureSchema', workflow.workflow.id, selectedNode.id)">Infer Schema</v-btn>
            <v-btn v-if="selectedNode?.data?.type !== 'trigger'" type="button" icon="mdi-content-copy" variant="text" aria-label="Duplicate node" title="Duplicate node (Ctrl/⌘ D)" @click="duplicateNode" />
            <v-btn type="button" icon="mdi-delete-outline" variant="text" color="error" aria-label="Delete node" title="Delete node" @click="removeSelectedNode" />
            <v-btn type="button" icon="mdi-close" variant="text" aria-label="Close node settings" title="Close" @click="closeNodeModal" />
          </div>
        </div>
      </template>
      <NodeInspector ref="nodeInspectorRef" :contacts="contacts" :node="selectedNode" @save="saveNodeConfig" @save-label="saveNodeLabel" />
    </AppRightSidebar>
  </section>
</template>

<style scoped>
.builder-editor-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.workflow-title-input { flex: 1 1 200px; min-width: 160px; max-width: 520px; }
.workflow-title-input :deep(input) { font-size: 1.2rem; font-weight: 650; padding: 0; }

.save-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #f97316;
  margin-right: 8px;
  box-shadow: 0 0 0 2px rgba(249, 115, 22, 0.2);
}

.canvas-frame {
  position: relative;
}

.canvas-action-dock {
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 7;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 10px;
  background: rgba(var(--v-theme-surface), 0.95);
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.1);
  backdrop-filter: blur(6px);
}

.canvas-action-btn {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 8px;
  background: rgb(var(--v-theme-surface));
  color: rgba(var(--v-theme-on-surface), var(--v-high-emphasis-opacity));
  font-size: 0.8rem;
  font-weight: 700;
  line-height: 1;
  min-width: 30px;
  min-height: 30px;
  padding: 0 8px;
}

.canvas-action-btn:hover {
  background: rgb(var(--v-theme-surface-light));
}

.canvas-action-btn-wide {
  min-width: 52px;
}

.node-command-overlay {
  position: absolute;
  inset: 0;
  z-index: 12;
  display: grid;
  place-items: center;
  background: rgba(var(--v-theme-on-surface), 0.28);
}

.node-command-card {
  width: min(560px, calc(100% - 32px));
  max-height: 70%;
  border-radius: 12px;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  background: rgb(var(--v-theme-surface));
  box-shadow: 0 24px 44px rgba(15, 23, 42, 0.18);
  display: grid;
  gap: 10px;
  padding: 12px;
}

.node-command-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.node-command-input {
  min-height: 40px;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 10px;
  padding: 8px 10px;
}

.node-command-list {
  overflow: auto;
  display: grid;
  gap: 8px;
  align-content: start;
}

.node-command-item {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 10px;
  background: rgb(var(--v-theme-surface-light));
  text-align: left;
  padding: 9px 10px;
}

.node-command-item small {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  font-size: 0.72rem;
}

.node-command-item:hover {
  background: rgb(var(--v-theme-surface-light));
}

.node-command-empty {
  margin: 0;
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  padding: 8px 2px;
}

.builder-toolbar .toolbar-actions-primary {
  grid-template-columns: repeat(2, max-content);
}

@media (max-width: 760px) {
  .builder-toolbar .toolbar-actions-primary {
    width: 100%;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .builder-toolbar .toolbar-actions-primary :deep(.v-btn) {
    width: 100%;
  }
}
</style>
