<script setup>
/* eslint-disable */
import { computed, onBeforeUnmount, ref, useTemplateRef, watch } from "vue";
import "@vue-flow/core/dist/style.css";
import "@vue-flow/core/dist/theme-default.css";
import "@vue-flow/controls/dist/style.css";
import "@vue-flow/minimap/dist/style.css";
import ContactsPanel from "../../../../frontend/src/components/ContactsPanel.vue";
import RunHistory from "../../../../frontend/src/components/RunHistory.vue";
import WorkflowBuilderPanel from "../../../../frontend/src/components/WorkflowBuilder.vue";
import { useDashboardData } from "../../../../frontend/src/composables/useDashboardData";
import { usePocketBaseAuth } from "../../../../frontend/src/composables/usePocketBaseAuth";
import {
  armWorkflowWebhookCapture,
  getWorkflowRunDetail,
  getWorkflowWebhookCapture,
  pb,
  publishWorkflow as publishWorkflowRequest,
  runWorkflow as runWorkflowRequest,
  saveWorkflowGraph,
  validateWorkflow as validateWorkflowRequest,
} from "../api";

const auth = usePocketBaseAuth();
const isAuthenticated = auth.isAuthenticated;
const dashboard = useDashboardData();
const builderRef = useTemplateRef("builder");
const saveMessage = ref("Loading workflow...");
const saveState = ref("idle");
const validationErrors = ref([]);
const validationFindings = ref([]);
const pendingSave = ref(null);
const saveInFlight = ref(false);
const capturePollTimer = ref(undefined);
const activeCapture = ref(null);
const selectedRunId = ref("");
const selectedRunDetail = ref(null);
const selectedRunLoading = ref(false);
const refreshTimer = ref(undefined);
const workflows = computed(() => dashboard.data.value.workflows);
const contacts = computed(() => dashboard.data.value.contacts);
const runLogs = computed(() => dashboard.data.value.runLogs);
const activeWorkflow = computed(() => dashboard.data.value.activeWorkflow);
const realtimeCollections = ['workflows', 'workflow_nodes', 'workflow_edges', 'workflow_runs', 'node_runs', 'subscribers'];

const nodeLibrary = [
  { label: "Trigger", type: "trigger", description: "Webhook match and security" },
  { label: "Transform", type: "transform", description: "Use run + previous context" },
  { label: "Condition", type: "condition", description: "Branch by context path" },
  { label: "Event Start", type: "event_start", description: "Anchor a named event date" },
  { label: "Wait", type: "wait_until", description: "Pause until a computed time" },
  { label: "HTTP", type: "http_request", description: "Call external service" },
  { label: "PocketBase", type: "pb_update", description: "Query or update records" },
];

watch(
  workflows,
  (nextWorkflows) => {
    if (!nextWorkflows.length) {
      return;
    }

    if (!dashboard.currentWorkflowId.value) {
      dashboard.currentWorkflowId.value = nextWorkflows[0].id;
    }
  },
  { immediate: true }
);

watch(
  [activeWorkflow, runLogs],
  ([workflow, runs]) => {
    if (dashboard.loading.value || dashboard.error.value) {
      selectedRunId.value = '';
      selectedRunDetail.value = null;
      return;
    }

    if (!workflow) {
      selectedRunId.value = "";
      selectedRunDetail.value = null;
      return;
    }

    const matchingRuns = runs.filter((run) => run.workflowId === workflow.workflow.id);
    if (!matchingRuns.length) {
      selectedRunId.value = "";
      selectedRunDetail.value = null;
      return;
    }

    if (!selectedRunId.value || !matchingRuns.some((run) => run.id === selectedRunId.value)) {
      void selectRun(matchingRuns[0].id);
    }
  },
  { immediate: true }
);

watch(runLogs, () => {
  if (selectedRunId.value && !dashboard.loading.value && !dashboard.error.value) {
    void loadRunDetail(selectedRunId.value);
  }
});

watch(
  isAuthenticated,
  (authenticated) => {
    if (authenticated) {
      void dashboard.refresh(dashboard.currentWorkflowId.value);
      void startRealtimeSubscriptions();
      return;
    }

    stopRealtimeSubscriptions();
  },
  { immediate: true }
);

function logout() {
  auth.logout();
  stopRealtimeSubscriptions();
  stopCapturePolling();
  saveState.value = "idle";
  saveMessage.value = "Admin session required to edit workflows.";
  validationErrors.value = [];
  validationFindings.value = [];
  selectedRunId.value = "";
  selectedRunDetail.value = null;
}

async function saveWorkflow(payload, mode) {
  if (saveInFlight.value) {
    pendingSave.value = { payload, mode };
    saveState.value = "dirty";
    saveMessage.value = mode === "auto" ? "Changes queued for autosave..." : "Changes queued for save...";
    return;
  }

  saveInFlight.value = true;
  saveState.value = "saving";
  saveMessage.value = mode === "auto" ? "Autosaving..." : "Saving workflow...";

  try {
    dashboard.replace(await saveWorkflowGraph(payload.workflow.id, {
      nodes: payload.nodes,
      edges: payload.edges,
    }));
    saveState.value = "saved";
    saveMessage.value = `Saved ${new Date().toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}`;
    validationErrors.value = [];
    validationFindings.value = [];
  } catch (error) {
    saveState.value = "error";
    saveMessage.value = `Unsaved: ${error instanceof Error ? error.message : "Failed to save workflow"}`;
    dashboard.error.value = error instanceof Error ? error.message : "Failed to save workflow";
  } finally {
    saveInFlight.value = false;
    if (pendingSave.value) {
      const queued = pendingSave.value;
      pendingSave.value = null;
      await saveWorkflow(queued.payload, queued.mode);
    }
  }
}

async function publishWorkflow(workflowId) {
  saveState.value = "saving";
  saveMessage.value = "Publishing workflow...";
  try {
    dashboard.replace(await publishWorkflowRequest(workflowId));
    saveState.value = "saved";
    saveMessage.value = "Workflow published";
    validationErrors.value = [];
    validationFindings.value = [];
  } catch (error) {
    saveState.value = "error";
    saveMessage.value = error instanceof Error ? error.message : "Failed to publish workflow";
    dashboard.error.value = error instanceof Error ? error.message : "Failed to publish workflow";
    const message = error instanceof Error ? error.message : "";
    validationErrors.value = message.includes(": ") ? message.split(": ").slice(1).join(": ").split("; ") : [];
  }
}

async function runWorkflow(workflowId) {
  const trigger = activeWorkflow.value?.nodes.find((node) => node.type === "trigger");
  const triggerMode = String(trigger?.config?.mode ?? "manual");
  if (triggerMode === "webhook") {
    await armWebhookCapture(workflowId, "test_run");
    return;
  }

  saveState.value = "saving";
  saveMessage.value = "Queueing test run...";
  try {
    dashboard.replace(await runWorkflowRequest(workflowId));
    saveState.value = "saved";
    saveMessage.value = "Test run queued";
  } catch (error) {
    saveState.value = "error";
    saveMessage.value = error instanceof Error ? error.message : "Failed to queue test run";
    dashboard.error.value = error instanceof Error ? error.message : "Failed to queue test run";
  }
}

async function armWebhookCapture(workflowId, mode, nodeId = "") {
  stopCapturePolling();
  saveState.value = "saving";
  saveMessage.value = mode === "infer_schema" ? "Waiting for webhook to infer schema..." : "Waiting for webhook test payload...";

  try {
    activeCapture.value = await armWorkflowWebhookCapture(workflowId, mode);
    saveMessage.value = `${mode === "infer_schema" ? "Send a webhook to" : "Trigger a webhook test at"} ${activeCapture.value.endpoint}`;
    startCapturePolling(mode, nodeId);
  } catch (error) {
    saveState.value = "error";
    saveMessage.value = error instanceof Error ? error.message : "Failed to arm webhook capture";
  }
}

function startCapturePolling(mode, nodeId) {
  stopCapturePolling();
  capturePollTimer.value = window.setInterval(async () => {
    if (!activeCapture.value) {
      stopCapturePolling();
      return;
    }

    try {
      const next = await getWorkflowWebhookCapture(activeCapture.value.id);
      activeCapture.value = next;

      if (next.status === "waiting") {
        return;
      }

      stopCapturePolling();

      if (next.status === "captured" && mode === "infer_schema") {
        if (nodeId && next.payloadJson && next.schemaJson) {
          builderRef.value?.applyNodeConfigValues(nodeId, {
            samplePayload: next.payloadJson,
            payloadSchema: next.schemaJson,
          });
        }
        saveState.value = "saved";
        saveMessage.value = "Schema inferred from incoming webhook";
        return;
      }

      if (next.status === "executed" && mode === "test_run") {
        await dashboard.refresh();
        saveState.value = "saved";
        saveMessage.value = "Webhook test run executed";
        return;
      }

      saveState.value = "error";
      saveMessage.value = next.error || `Webhook capture ${next.status}`;
    } catch (error) {
      stopCapturePolling();
      saveState.value = "error";
      saveMessage.value = error instanceof Error ? error.message : "Failed to poll webhook capture";
    }
  }, 1500);
}

function stopCapturePolling() {
  if (capturePollTimer.value) {
    window.clearInterval(capturePollTimer.value);
    capturePollTimer.value = undefined;
  }
}

function queueDashboardRefresh() {
  if (refreshTimer.value) {
    window.clearTimeout(refreshTimer.value);
  }

  refreshTimer.value = window.setTimeout(async () => {
    refreshTimer.value = undefined;
    await dashboard.refresh(dashboard.currentWorkflowId.value);
    if (selectedRunId.value) {
      await loadRunDetail(selectedRunId.value);
    }
  }, 150);
}

async function startRealtimeSubscriptions() {
  stopRealtimeSubscriptions();

  await Promise.all(realtimeCollections.map((collectionName) =>
    pb.collection(collectionName).subscribe('*', () => {
      queueDashboardRefresh();
    })));
}

function stopRealtimeSubscriptions() {
  realtimeCollections.forEach((collectionName) => {
    pb.collection(collectionName).unsubscribe('*');
  });

  if (refreshTimer.value) {
    window.clearTimeout(refreshTimer.value);
    refreshTimer.value = undefined;
  }
}

function addNode(type) {
  builderRef.value?.addNode(type);
}

async function selectWorkflow(workflowId) {
  dashboard.currentWorkflowId.value = workflowId;
  validationErrors.value = [];
  validationFindings.value = [];
  saveState.value = "idle";
  saveMessage.value = "Loading workflow...";
  await dashboard.refresh(workflowId);
}

async function loadRunDetail(runId) {
  selectedRunLoading.value = true;
  try {
    selectedRunDetail.value = await getWorkflowRunDetail(runId);
  } catch (error) {
    dashboard.error.value = error instanceof Error ? error.message : "Failed to load run detail";
  } finally {
    selectedRunLoading.value = false;
  }
}

async function selectRun(runId) {
  selectedRunId.value = runId;
  await loadRunDetail(runId);
}

async function validateWorkflow(workflowId) {
  saveState.value = "saving";
  saveMessage.value = "Validating workflow...";
  try {
    const result = await validateWorkflowRequest(workflowId);
    validationErrors.value = result.errors;
    validationFindings.value = result.findings;
    saveState.value = result.valid ? "saved" : "error";
    saveMessage.value = result.valid ? "Workflow is valid" : "Validation failed";
  } catch (error) {
    saveState.value = "error";
    saveMessage.value = error instanceof Error ? error.message : "Validation failed";
    dashboard.error.value = error instanceof Error ? error.message : "Validation failed";
  }
}

onBeforeUnmount(() => {
  stopRealtimeSubscriptions();
});
</script>

<template>
  <section class="workflow-integrated">
    <template v-if="isAuthenticated">
      <section class="workflow-admin-toolbar panel">
        <p v-if="dashboard.error" class="workflow-admin-note toolbar-note">
          Live API unavailable. Showing fallback data. {{ dashboard.error }}
        </p>

        <div class="toolbar-block">
          <span class="toolbar-label">Workflows</span>
          <div class="workflow-selector-list">
            <button
              v-for="workflow in workflows"
              :key="workflow.id"
              type="button"
              class="workflow-selector-button"
              :class="{ 'workflow-selector-button-active': workflow.id === activeWorkflow?.workflow.id }"
              @click="selectWorkflow(workflow.id)"
            >
              <strong>{{ workflow.name }}</strong>
              <span>{{ workflow.triggerType }} · v{{ workflow.version }}</span>
            </button>
          </div>
        </div>

        <div class="toolbar-block toolbar-block-compact">
          <span class="toolbar-label">Quick Add</span>
          <div class="quick-add-list">
            <button
              v-for="node in nodeLibrary"
              :key="node.label"
              type="button"
              class="quick-add-button"
              @click="addNode(node.type)"
            >
              <strong>{{ node.label }}</strong>
              <span>{{ node.description }}</span>
            </button>
          </div>
        </div>
      </section>

      <WorkflowBuilderPanel
        ref="builder"
        :on-save-request="saveWorkflow"
        :save-message="saveMessage"
        :save-state="saveState"
        :validation-errors="validationErrors"
        :validation-findings="validationFindings"
        :workflow="activeWorkflow"
        @capture-schema="(workflowId, nodeId) => armWebhookCapture(workflowId, 'infer_schema', nodeId)"
        @publish="publishWorkflow"
        @run="runWorkflow"
        @save="saveWorkflow"
        @validate="validateWorkflow"
      />

      <section class="bottom-grid">
        <ContactsPanel :contacts="contacts" />
        <RunHistory
          :active-workflow-id="activeWorkflow?.workflow.id"
          :runs="runLogs"
          :selected-run-detail="selectedRunDetail"
          :selected-run-id="selectedRunId"
          :selected-run-loading="selectedRunLoading"
          @select-run="selectRun"
        />
      </section>
    </template>

    <section v-else class="workflow-auth-panel panel">
      <div class="panel-header">
        <h2>Admin session required</h2>
        <p>Sign in to the admin first, then reopen the workflow builder.</p>
      </div>
      <router-link class="primary-button workflow-login-link" :to="{ name: 'dashboard' }">
        Return to Admin
      </router-link>
    </section>
  </section>
</template>

<style>
.workflow-integrated {
  display: grid;
  gap: 20px;
}

.workflow-integrated .workflow-admin-toolbar,
.workflow-integrated .panel,
.workflow-integrated .builder-shell {
  background: #fff;
  border: 1px solid #dde5f0;
  border-radius: 20px;
  box-shadow: 0 14px 35px rgba(15, 23, 42, 0.05);
}

.workflow-integrated .toolbar-note {
  margin: 0;
}

.workflow-integrated .toolbar-block,
.workflow-integrated .panel-header,
.workflow-integrated .workspace-session,
.workflow-integrated .builder-heading,
.workflow-integrated .detail-panel,
.workflow-integrated .contact-card,
.workflow-integrated .run-card,
.workflow-integrated .run-trace-detail,
.workflow-integrated .trace-step-card,
.workflow-integrated .validation-panel,
.workflow-integrated .form-grid,
.workflow-integrated .workflow-selector-button,
.workflow-integrated .quick-add-button {
  display: grid;
  gap: 6px;
}

.workflow-integrated .eyebrow,
.workflow-integrated .builder-eyebrow,
.workflow-integrated .toolbar-label,
.workflow-integrated .field-help {
  color: #64748b;
  font-size: 0.74rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.workflow-integrated .builder-toolbar h1,
.workflow-integrated .panel-header h2,
.workflow-integrated .modal-header h2 {
  margin: 0;
}

.workflow-integrated .workflow-admin-note,
.workflow-integrated .panel-header p,
.workflow-integrated .builder-toolbar p,
.workflow-integrated .contact-meta,
.workflow-integrated .run-row.subtle,
.workflow-integrated .quick-add-button span,
.workflow-integrated .workflow-selector-button span,
.workflow-integrated .workflow-node-card span {
  color: #6b7280;
  margin: 0;
}

.workflow-integrated .workflow-admin-toolbar {
  display: grid;
  gap: 18px;
  padding: 20px;
}

.workflow-integrated .workflow-selector-list,
.workflow-integrated .quick-add-list,
.workflow-integrated .contact-list,
.workflow-integrated .run-list {
  display: grid;
  gap: 10px;
}

.workflow-integrated .workflow-selector-list {
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
}

.workflow-integrated .quick-add-list {
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
}

.workflow-integrated .workflow-selector-button,
.workflow-integrated .quick-add-button,
.workflow-integrated .ghost-button,
.workflow-integrated .primary-button,
.workflow-integrated .danger-button {
  border-radius: 14px;
  cursor: pointer;
  font: inherit;
  transition: background-color 140ms ease, border-color 140ms ease, box-shadow 140ms ease, color 140ms ease;
}

.workflow-integrated .workflow-selector-button,
.workflow-integrated .quick-add-button {
  background: #f8fbff;
  border: 1px solid #dce7f8;
  padding: 14px 16px;
  text-align: left;
}

.workflow-integrated .workflow-selector-button-active {
  background: #edf4ff;
  border-color: #9dbcf3;
  box-shadow: inset 0 0 0 1px rgba(15, 91, 216, 0.18);
}

.workflow-integrated .quick-add-button:hover,
.workflow-integrated .workflow-selector-button:hover,
.workflow-integrated .ghost-button:hover {
  background: #eef4fb;
}

.workflow-integrated .builder-shell,
.workflow-integrated .panel {
  padding: 18px;
}

.workflow-integrated .builder-shell {
  display: grid;
  gap: 16px;
}

.workflow-integrated .builder-toolbar,
.workflow-integrated .workflow-integrated .modal-header {
  align-items: flex-start;
  display: flex;
  gap: 16px;
  justify-content: space-between;
}

.workflow-integrated .toolbar-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: flex-end;
}

.workflow-integrated .save-indicator {
  align-items: center;
  background: #f8fafc;
  border: 1px solid #dbe2ea;
  border-radius: 999px;
  color: #475569;
  display: inline-flex;
  font-size: 0.88rem;
  min-height: 40px;
  padding: 0 14px;
}

.workflow-integrated .save-indicator[data-state="saved"] {
  background: #ddf6e7;
  border-color: #b6e6c7;
  color: #0f6a34;
}

.workflow-integrated .save-indicator[data-state="error"],
.workflow-integrated .validation-severity[data-severity="error"] {
  background: #fde7e3;
  border-color: #f8c9c0;
  color: #b42318;
}

.workflow-integrated .save-indicator[data-state="dirty"],
.workflow-integrated .validation-severity[data-severity="warning"] {
  background: #fff4d6;
  border-color: #efd79c;
  color: #8a5b00;
}

.workflow-integrated .ghost-button,
.workflow-integrated .primary-button,
.workflow-integrated .danger-button {
  align-items: center;
  display: inline-flex;
  justify-content: center;
  min-height: 40px;
  padding: 0 14px;
  text-decoration: none;
}

.workflow-integrated .ghost-button {
  background: #f5f7fb;
  border: 1px solid #dbe2ef;
  color: #0f172a;
}

.workflow-integrated .primary-button {
  background: #0f5bd8;
  border: 1px solid #0f5bd8;
  color: #fff;
}

.workflow-integrated .danger-button {
  background: #fff1ef;
  border: 1px solid #f3c4bc;
  color: #b42318;
}

.workflow-integrated .builder-body,
.workflow-integrated .bottom-grid,
.workflow-integrated .run-history-layout {
  display: grid;
  gap: 16px;
}

.workflow-integrated .builder-body {
  grid-template-columns: 320px minmax(0, 1fr);
}

.workflow-integrated .builder-details {
  display: grid;
  gap: 16px;
}

.workflow-integrated .detail-panel {
  align-content: start;
}

.workflow-integrated .record-grid {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.workflow-integrated .record-grid div {
  background: #f8fbff;
  border: 1px solid #dce7f8;
  border-radius: 14px;
  display: grid;
  gap: 6px;
  padding: 12px;
}

.workflow-integrated .record-grid span,
.workflow-integrated .workflow-node-badge,
.workflow-integrated .status-pill {
  font-size: 0.76rem;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.workflow-integrated .validation-item,
.workflow-integrated .run-card-button {
  background: #f8fbff;
  border: 1px solid #dce7f8;
  border-radius: 14px;
  cursor: pointer;
  padding: 14px;
  text-align: left;
}

.workflow-integrated .validation-item {
  display: grid;
  gap: 8px;
}

.workflow-integrated .validation-severity,
.workflow-integrated .status-pill {
  border-radius: 999px;
  display: inline-flex;
  justify-content: center;
  padding: 4px 10px;
  width: fit-content;
}

.workflow-integrated .canvas-frame {
  background: #f7fafc;
  border: 1px solid #dde5f0;
  border-radius: 18px;
  min-height: 640px;
  overflow: hidden;
}

.workflow-integrated .workflow-canvas {
  height: 100%;
  min-height: 640px;
}

.workflow-integrated .modal-backdrop {
  align-items: center;
  background: rgba(15, 23, 42, 0.45);
  display: flex;
  inset: 0;
  justify-content: center;
  padding: 24px;
  position: fixed;
  z-index: 2600;
}

.workflow-integrated .modal-shell {
  background: #fff;
  border: 1px solid #dde5f0;
  border-radius: 20px;
  box-shadow: 0 20px 55px rgba(15, 23, 42, 0.16);
  max-height: calc(100vh - 48px);
  max-width: 760px;
  overflow: auto;
  padding: 20px;
  width: min(760px, 100%);
}

.workflow-integrated .modal-header {
  margin-bottom: 16px;
}

.workflow-integrated .form-grid,
.workflow-integrated .map-field-list,
.workflow-integrated .trace-step-list,
.workflow-integrated .trace-logs {
  display: grid;
  gap: 12px;
}

.workflow-integrated .form-field {
  display: grid;
  gap: 8px;
}

.workflow-integrated .form-field input,
.workflow-integrated .form-field select,
.workflow-integrated .form-field textarea,
.workflow-integrated .map-field-row input {
  background: #fff;
  border: 1px solid #ccd5e2;
  border-radius: 12px;
  font: inherit;
  min-height: 42px;
  padding: 10px 12px;
}

.workflow-integrated .map-field-row {
  display: grid;
  gap: 8px;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
}

.workflow-integrated .context-help {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 14px;
  display: grid;
  gap: 8px;
  padding: 14px;
}

.workflow-integrated .context-token {
  color: #475569;
  font-family: "IBM Plex Mono", "SFMono-Regular", monospace;
  font-size: 0.82rem;
}

.workflow-integrated .bottom-grid {
  grid-template-columns: minmax(260px, 360px) minmax(0, 1fr);
}

.workflow-integrated .panel-dark {
  background: #fbfcfe;
}

.workflow-integrated .contact-card,
.workflow-integrated .trace-step-card {
  background: #f8fbff;
  border: 1px solid #dce7f8;
  border-radius: 14px;
  padding: 14px;
}

.workflow-integrated .run-history-layout {
  grid-template-columns: minmax(260px, 320px) minmax(0, 1fr);
}

.workflow-integrated .run-row {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
}

.workflow-integrated .run-card-active {
  background: #edf4ff;
  border-color: #9dbcf3;
}

.workflow-integrated .run-trace-panel,
.workflow-integrated .trace-json-block {
  background: #fff;
  border: 1px solid #dde5f0;
  border-radius: 14px;
  padding: 14px;
}

.workflow-integrated .run-trace-empty {
  color: #64748b;
  padding: 18px;
  text-align: center;
}

.workflow-integrated .trace-json-block pre {
  margin: 12px 0 0;
  overflow: auto;
}

.workflow-integrated .trace-error {
  color: #b42318;
  font-weight: 600;
  margin: 0;
}

.workflow-integrated .workflow-node-card {
  background: #fff;
  border: 1px solid #dce7f8;
  border-radius: 16px;
  box-shadow: 0 10px 24px rgba(15, 23, 42, 0.06);
  display: grid;
  gap: 8px;
  min-width: 220px;
  padding: 14px;
}

.workflow-integrated .workflow-node-badge {
  color: #0f5bd8;
}

.workflow-integrated .vue-flow__node.selected .workflow-node-card,
.workflow-integrated .flow-node-invalid .workflow-node-card {
  border-color: #0f5bd8;
  box-shadow: 0 0 0 2px rgba(15, 91, 216, 0.14);
}

.workflow-integrated .flow-node-invalid .workflow-node-card {
  border-color: #b42318;
  box-shadow: 0 0 0 2px rgba(180, 35, 24, 0.12);
}

.workflow-integrated .status-pill[data-status="success"] {
  background: #ddf6e7;
  color: #0f6a34;
}

.workflow-integrated .status-pill[data-status="running"],
.workflow-integrated .status-pill[data-status="waiting"],
.workflow-integrated .status-pill[data-status="queued"] {
  background: #dfeeff;
  color: #0f5bd8;
}

.workflow-integrated .status-pill[data-status="failed"] {
  background: #fde7e3;
  color: #b42318;
}

.workflow-integrated .status-pill[data-status="cancelled"] {
  background: #ebeff5;
  color: #475569;
}

.workflow-integrated .workflow-auth-panel {
  padding: 28px;
}

.workflow-integrated .workflow-login-link {
  margin-top: 8px;
  width: fit-content;
}

@media (max-width: 1100px) {
  .workflow-integrated .builder-body,
  .workflow-integrated .bottom-grid,
  .workflow-integrated .run-history-layout {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 840px) {
  .workflow-integrated .builder-toolbar {
    flex-direction: column;
  }

  .workflow-integrated .toolbar-actions {
    justify-content: flex-start;
  }

  .workflow-integrated .map-field-row,
  .workflow-integrated .record-grid {
    grid-template-columns: 1fr;
  }
}
</style>
