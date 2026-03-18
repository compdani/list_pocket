<script setup>
/* eslint-disable */
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";
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
import WorkflowNodeCard from "./WorkflowNodeCard.vue";

const props = defineProps({
  onSaveRequest: { type: Function, default: null },
  saveMessage: { type: String, default: "" },
  saveState: { type: String, default: "idle" },
  validationErrors: { type: Array, default: () => [] },
  validationFindings: { type: Array, default: () => [] },
  workflow: { type: Object, default: null },
});

const emit = defineEmits(["captureSchema", "deleteWorkflow", "publish", "run", "save", "validate"]);

const builder = useBuilderState(computed(() => props.workflow));
const { setCenter } = useVueFlow();
const autosaveTimer = ref(undefined);
const committedSignature = ref("");
const initializedWorkflowId = ref("");
const isDirty = ref(false);
const nodeTypes = { workflow: WorkflowNodeCard };

const selectedNode = computed(() => builder.nodes.value.find((node) => node.id === builder.selectedNodeId.value));
const selectedEdge = computed(() => builder.edges.value.find((edge) => edge.id === builder.selectedEdgeId.value));
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

watch(
  () => props.workflow,
  async () => {
    if (autosaveTimer.value) {
      window.clearTimeout(autosaveTimer.value);
      autosaveTimer.value = undefined;
    }

    await nextTick();

    if (!props.workflow) {
      committedSignature.value = currentSignature.value;
      initializedWorkflowId.value = "";
      isDirty.value = false;
      return;
    }

    if (!initializedWorkflowId.value || initializedWorkflowId.value !== props.workflow.workflow.id) {
      initializedWorkflowId.value = props.workflow.workflow.id;
      committedSignature.value = currentSignature.value;
      isDirty.value = false;
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
  if (!isDirty.value) {
    return;
  }

  if (autosaveTimer.value) {
    window.clearTimeout(autosaveTimer.value);
  }

  autosaveTimer.value = window.setTimeout(() => {
    saveWorkflow("auto");
  }, 800);
});

onBeforeUnmount(() => {
  if (autosaveTimer.value) {
    window.clearTimeout(autosaveTimer.value);
  }
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

async function saveWorkflow(mode = "manual") {
  if (!props.workflow) {
    return;
  }

  if (autosaveTimer.value) {
    window.clearTimeout(autosaveTimer.value);
    autosaveTimer.value = undefined;
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
});
</script>

<template>
  <section class="builder-shell">
    <div class="builder-toolbar">
      <div class="builder-heading">
        <span class="builder-eyebrow">Workflow Runs</span>
        <div class="builder-title-row">
          <h1>{{ workflow?.workflow.name ?? "Workflow Builder" }}</h1>
          <span class="save-indicator" :data-state="(isDirty && saveState !== 'saving' ? 'dirty' : saveState) ?? 'idle'">
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
      <div class="toolbar-actions toolbar-actions-compact">
        <button class="ghost-button" :disabled="saveState === 'saving'" @click="saveWorkflow('manual')">
          {{ saveState === 'saving' ? "Saving..." : "Save" }}
        </button>
        <button class="ghost-button" :disabled="!selectedEdge" @click="removeSelectedEdge">Remove Edge</button>
        <button class="ghost-button" :disabled="!workflow" @click="workflow && emit('validate', workflow.workflow.id)">Validate</button>
        <button class="ghost-button" :disabled="!workflow" @click="workflow && emit('publish', workflow.workflow.id)">Publish</button>
        <button class="danger-button" :disabled="!workflow || saveState === 'saving'" @click="workflow && emit('deleteWorkflow', workflow.workflow.id)">
          Delete
        </button>
        <button class="primary-button" :disabled="!workflow" @click="workflow && emit('run', workflow.workflow.id)">Run Test</button>
      </div>
    </div>

    <div v-if="workflow" class="builder-body builder-body-single">
      <div class="canvas-frame">
        <VueFlow
          :node-types="nodeTypes"
          :nodes="decoratedNodes"
          :edges="decoratedEdges"
          fit-view-on-init
          class="workflow-canvas"
          @connect="builder.connectNodes"
          @edge-click="({ edge }) => builder.selectEdge(edge.id)"
          @node-click="({ node }) => builder.selectNode(node.id)"
          @node-drag-stop="({ node }) => builder.updateNodePosition(node.id, node.position.x, node.position.y)"
        >
          <MiniMap />
          <Controls />
          <Background :gap="22" :size="1.2" :pattern-color="'#d9dee7'" />
        </VueFlow>
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

    <section v-else class="panel detail-panel builder-empty-state">
      <div class="panel-header">
        <h2>No workflow selected</h2>
        <p>Create or select a workflow to start editing the graph.</p>
      </div>
    </section>

    <div v-if="showNodeModal" class="modal-backdrop" @click="closeNodeModal">
      <div class="modal-shell" @click.stop>
        <div class="modal-header">
          <div>
            <span class="builder-eyebrow">Node Settings</span>
            <h2>{{ selectedNodeLabel }}</h2>
          </div>
          <button type="button" class="ghost-button" @click="closeNodeModal">Close</button>
        </div>

        <NodeInspector
          :node="selectedNode"
          @capture-schema="workflow && selectedNode && emit('captureSchema', workflow.workflow.id, selectedNode.id)"
          @remove="removeSelectedNode"
          @save="saveNodeConfig"
          @save-label="saveNodeLabel"
        />
      </div>
    </div>
  </section>
</template>
