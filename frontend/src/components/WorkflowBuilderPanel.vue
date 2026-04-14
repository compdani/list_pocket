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
import { useBuilderState } from "../composables/useBuilderState";
import NodeInspector from "./NodeInspector.vue";
import AppRightSidebar from "./AppRightSidebar.vue";
import WorkflowNodeCard from "./WorkflowNodeCard.vue";

const props = defineProps({
  contacts: { type: Array, default: () => [] },
  nodeLibrary: { type: Array, default: () => [] },
  onSaveRequest: { type: Function, default: null },
  saveMessage: { type: String, default: "" },
  saveState: { type: String, default: "idle" },
  validationErrors: { type: Array, default: () => [] },
  validationFindings: { type: Array, default: () => [] },
  workflow: { type: Object, default: null },
});

const emit = defineEmits(["captureSchema", "createWorkflow", "deleteWorkflow", "publish", "run", "save", "updateWorkflowName", "validate"]);

const builder = useBuilderState(computed(() => props.workflow));
const { fitView, setCenter, setViewport, viewport, zoomIn, zoomOut } = useVueFlow();
const committedSignature = ref("");
const initializedWorkflowId = ref("");
const isDirty = ref(false);
const nodeInspectorRef = ref(null);
const nodeTypes = { workflow: markRaw(WorkflowNodeCard) };
const restoredViewport = ref({ x: 0, y: 0, zoom: 1 });
const hasRestoredViewport = ref(false);
const showNodeCommand = ref(false);
const commandSearch = ref("");

const selectedNode = computed(() => builder.nodes.value.find((node) => node.id === builder.selectedNodeId.value));
const selectedEdge = computed(() => builder.edges.value.find((edge) => edge.id === builder.selectedEdgeId.value));
const selectedNodeDescription = computed(() => selectedNode.value?.data?.description ?? "");
const currentSignature = computed(() => builder.graphSignature());
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
  { immediate: true, deep: true }
);

watch(
  () => props.saveState,
  (state) => {
    if (state === "saved") {
      committedSignature.value = currentSignature.value;
      isDirty.value = false;
    }

    if (state === "error") {
      isDirty.value = true;
    }
  }
);

watch(currentSignature, (signature) => {
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
  window.removeEventListener("keydown", onWindowKeydown);
});

onMounted(() => {
  window.addEventListener("keydown", onWindowKeydown);
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

function closeNodeModal() {
  builder.clearSelection();
}

function removeSelectedEdge() {
  builder.removeSelectedEdge();
}

function updateSelectedEdgeField(field, event) {
  builder.updateSelectedEdgeField(field, event.target?.value ?? "");
}

function focusFinding(finding) {
  if (finding.targetType === "node" && finding.targetId) {
    builder.selectNode(finding.targetId);
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
  if (isTypingTarget) {
    return;
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
    await props.onSaveRequest(payload, mode);
    return;
  }

  emit("save", payload, mode);
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
  const nodeId = builder.addNode(type);
  if (!nodeId) {
    return;
  }

  builder.selectNode(nodeId);
  await focusNode(nodeId);
}

defineExpose({
  addNode,
  applyNodeConfigValues,
  openNodeCommand,
});
</script>

<template>
  <section class="builder-shell">
    <div class="builder-toolbar">
      <div class="builder-heading">
        <div class="builder-editor-bar">
          <input
            class="workflow-title-input"
            type="text"
            :value="workflow?.workflow.name ?? ''"
            placeholder="Untitled workflow"
            :disabled="!workflow"
            @input="emit('updateWorkflowName', $event.target.value)"
          />
          <span class="save-indicator" :data-state="(isDirty && saveState !== 'saving' ? 'dirty' : saveState) ?? 'idle'">
            <span v-if="isDirty && saveState !== 'saving'" class="save-dot" aria-hidden="true" />
            {{ saveMessage || (isDirty ? "Unsaved changes" : "Up to date") }}
          </span>
        </div>
        <div class="builder-meta">
          <span class="builder-meta-chip">{{ workflow?.workflow.status ?? "draft" }}</span>
          <span class="builder-meta-chip">v{{ workflow?.workflow.version ?? 1 }}</span>
          <span class="builder-meta-chip">{{ workflow?.workflow.triggerType ?? "manual" }} trigger</span>
          <span v-if="webhookEndpoint" class="builder-meta-chip builder-meta-chip-wide">{{ webhookEndpoint }}</span>
        </div>
      </div>
      <div class="toolbar-actions toolbar-actions-compact toolbar-actions-primary">
        <button class="ghost-button" :disabled="saveState === 'saving'" @click="saveWorkflow('manual')">
          <span v-if="isDirty && saveState !== 'saving'" class="save-dot" aria-label="Unsaved changes" />
          {{ saveState === 'saving' ? "Saving..." : "Save" }}
        </button>
        <button class="primary-button" :disabled="!workflow" @click="workflow && emit('run', workflow.workflow.id)">Run Test</button>
      </div>
      <div class="toolbar-actions toolbar-actions-compact toolbar-actions-secondary">
        <button class="ghost-button" type="button" :disabled="!workflow" @click="emit('createWorkflow')">
          New workflow
        </button>
        <button class="ghost-button" :disabled="!selectedEdge" @click="removeSelectedEdge">Remove Edge</button>
        <button class="ghost-button" :disabled="!workflow" @click="workflow && emit('validate', workflow.workflow.id)">Validate</button>
        <button class="ghost-button" :disabled="!workflow" @click="workflow && emit('publish', workflow.workflow.id)">Publish</button>
        <button class="danger-button" :disabled="!workflow || saveState === 'saving'" @click="workflow && emit('deleteWorkflow', workflow.workflow.id)">
          Delete
        </button>
      </div>
      <details class="toolbar-actions-mobile-more">
        <summary>More actions</summary>
        <div class="toolbar-actions-mobile-menu">
          <button class="ghost-button" type="button" :disabled="!workflow" @click="emit('createWorkflow')">
            New workflow
          </button>
          <button class="ghost-button" :disabled="!selectedEdge" @click="removeSelectedEdge">Remove Edge</button>
          <button class="ghost-button" :disabled="!workflow" @click="workflow && emit('validate', workflow.workflow.id)">Validate</button>
          <button class="ghost-button" :disabled="!workflow" @click="workflow && emit('publish', workflow.workflow.id)">Publish</button>
          <button class="danger-button" :disabled="!workflow || saveState === 'saving'" @click="workflow && emit('deleteWorkflow', workflow.workflow.id)">
            Delete
          </button>
        </div>
      </details>
    </div>

    <div v-if="workflow" class="builder-body builder-body-single">
      <div class="canvas-frame">
        <div class="canvas-action-dock" role="toolbar" aria-label="Canvas actions">
          <button type="button" class="canvas-action-btn canvas-action-btn-wide" title="Add node (A)" @click="openNodeCommand">
            + Add node
          </button>
          <button type="button" class="canvas-action-btn canvas-action-btn-wide" title="Center selected node" @click="centerOnSelection">
            Center
          </button>
          <button type="button" class="canvas-action-btn canvas-action-btn-wide" title="Reset saved view" @click="clearSavedViewport">
            Reset
          </button>
        </div>
        <VueFlow
          :node-types="nodeTypes"
          :nodes="decoratedNodes"
          :edges="decoratedEdges"
          :default-viewport="restoredViewport"
          :fit-view-on-init="!hasRestoredViewport"
          :min-zoom="0.2"
          :max-zoom="1.2"
          :pan-on-scroll="true"
          :pan-on-drag="true"
          :zoom-on-double-click="false"
          :zoom-on-scroll="true"
          :selection-on-drag="false"
          class="workflow-canvas"
          @connect="builder.connectNodes"
          @edge-click="({ edge }) => builder.selectEdge(edge.id)"
          @node-click="({ node }) => builder.selectNode(node.id)"
          @node-drag-stop="onNodeDragStop"
        >
          <MiniMap position="bottom-right" :pannable="true" />
          <Controls position="bottom-left" />
          <Background :gap="22" :size="1.2" :pattern-color="'#d9dee7'" />
        </VueFlow>

        <div v-if="showNodeCommand" class="node-command-overlay" @click.self="closeNodeCommand">
          <div class="node-command-card">
            <div class="node-command-header">
              <strong>Add node</strong>
              <button type="button" class="canvas-action-btn" @click="closeNodeCommand">x</button>
            </div>
            <input
              v-model="commandSearch"
              class="node-command-input"
              placeholder="Search nodes (A)"
              autofocus
            />
            <div class="node-command-list">
              <button
                v-for="node in filteredNodeLibrary"
                :key="node.type"
                type="button"
                class="node-command-item"
                @click="addNodeFromCommand(node.type)"
              >
                <span>{{ node.label }}</span>
                <small>{{ node.type }}</small>
              </button>
              <p v-if="!filteredNodeLibrary.length" class="node-command-empty">No matching node types.</p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-if="workflow && (validationErrors?.length || selectedEdge)" class="builder-secondary-panels">
      <aside v-if="validationErrors?.length" class="panel validation-panel detail-panel">
        <div class="panel-header">
          <h2>Validation</h2>
          <p>Resolve these findings before publish.</p>
        </div>
        <button
          v-for="finding in validationFindings"
          :key="`${finding.code}:${finding.targetId}:${finding.message}`"
          class="validation-item"
          type="button"
          @click="focusFinding(finding)"
        >
          <span class="validation-severity" :data-severity="finding.severity">{{ finding.severity }}</span>
          {{ finding.message }}
        </button>
      </aside>

      <aside v-if="selectedEdge" class="panel edge-panel detail-panel">
        <div class="panel-header">
          <h2>Transition</h2>
          <p>{{ selectedEdge.source }} -> {{ selectedEdge.target }}</p>
        </div>
        <label class="form-field">
          <span>Branch Label</span>
          <input :value="String(selectedEdge.label ?? '')" @input="updateSelectedEdgeField('branch', $event)" />
        </label>
        <label class="form-field">
          <span>Condition</span>
          <textarea rows="4" :value="selectedEdgeExpression" @input="updateSelectedEdgeField('expression', $event)" />
        </label>
        <button type="button" class="danger-button" @click="removeSelectedEdge">Delete Edge</button>
      </aside>
    </div>

    <section v-if="!workflow" class="panel detail-panel builder-empty-state">
      <div class="panel-header">
        <h2>No workflow selected</h2>
        <p>Create or select a workflow to start editing the graph.</p>
      </div>
    </section>

    <AppRightSidebar
      :model-value="showNodeModal"
      :width="700"
      @update:model-value="(next) => !next && closeNodeModal()"
      @close="closeNodeModal"
    >
      <template #header>
        <div class="modal-header mt-2 px-2">
          <div>
            <span class="builder-eyebrow">Node Settings</span>
            <h2>{{ selectedNodeLabel }}</h2>
            <p v-if="selectedNodeDescription" class="field-help">{{ selectedNodeDescription }}</p>
          </div>
          <div class="node-modal-actions">
            <button
              v-if="canCaptureSelectedNodeSchema"
              type="button"
              class="ghost-button"
              @click="workflow && selectedNode && emit('captureSchema', workflow.workflow.id, selectedNode.id)"
            >
              Infer Schema
            </button>
            <button type="button" class="danger-button" @click="removeSelectedNode">Delete Node</button>
            <button type="button" class="ghost-button" @click="closeNodeModal">Close</button>
          </div>
        </div>
      </template>

      <NodeInspector
        ref="nodeInspectorRef"
        :contacts="contacts"
        :node="selectedNode"
        @save="saveNodeConfig"
        @save-label="saveNodeLabel"
      />
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

.workflow-title-input {
  flex: 1 1 200px;
  min-width: 140px;
  max-width: min(520px, 100%);
  min-height: 34px;
  padding: 6px 10px;
  border: 1px solid #ccd5e2;
  border-radius: 8px;
  background: #fff;
  color: #0f172a;
  font-size: 0.98rem;
  font-weight: 600;
}

.workflow-title-input:disabled {
  opacity: 0.6;
}

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
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.95);
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.1);
  backdrop-filter: blur(6px);
}

.canvas-action-btn {
  border: 1px solid #dce4ee;
  border-radius: 8px;
  background: #fff;
  color: #334155;
  font-size: 0.8rem;
  font-weight: 700;
  line-height: 1;
  min-width: 30px;
  min-height: 30px;
  padding: 0 8px;
}

.canvas-action-btn:hover {
  background: #f8fafc;
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
  background: rgba(15, 23, 42, 0.28);
}

.node-command-card {
  width: min(560px, calc(100% - 32px));
  max-height: 70%;
  border-radius: 12px;
  border: 1px solid #d7dfeb;
  background: #fff;
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
  border: 1px solid #d4dde9;
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
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: #f8fafc;
  text-align: left;
  padding: 9px 10px;
}

.node-command-item small {
  color: #64748b;
  font-size: 0.72rem;
}

.node-command-item:hover {
  background: #f1f5f9;
}

.node-command-empty {
  margin: 0;
  color: #64748b;
  padding: 8px 2px;
}

.builder-toolbar .toolbar-actions-primary {
  grid-template-columns: repeat(2, max-content);
}

.builder-toolbar .toolbar-actions-mobile-more {
  display: none;
}

.builder-toolbar .toolbar-actions-mobile-menu {
  display: grid;
  gap: 8px;
  padding-top: 8px;
}

@media (max-width: 760px) {
  .builder-toolbar .toolbar-actions-primary {
    width: 100%;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .builder-toolbar .toolbar-actions-primary .ghost-button,
  .builder-toolbar .toolbar-actions-primary .primary-button {
    width: 100%;
  }

  .builder-toolbar .toolbar-actions-secondary {
    display: none !important;
  }

  .builder-toolbar .toolbar-actions-mobile-more {
    display: block;
    width: 100%;
    border: 1px solid #dde5ef;
    border-radius: 10px;
    background: #f8fafc;
    padding: 6px 8px;
  }

  .builder-toolbar .toolbar-actions-mobile-more > summary {
    cursor: pointer;
    font-weight: 600;
    color: #334155;
    list-style: none;
  }

  .builder-toolbar .toolbar-actions-mobile-more[open] {
    padding-bottom: 8px;
  }

  .builder-toolbar .toolbar-actions-mobile-more > summary::-webkit-details-marker {
    display: none;
  }
}
</style>
