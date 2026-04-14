<script setup>
/* eslint-disable */
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import RunHistory from "../components/RunHistory.vue";
import WorkflowBuilderPanel from "../components/WorkflowBuilderPanel.vue";
import {
  armWorkflowWebhookCapture,
  cancelWorkflowRun as cancelWorkflowRunRequest,
  createWorkflow as createWorkflowRequest,
  deleteWorkflow as deleteWorkflowRequest,
  getWorkflowDashboard,
  getWorkflowRunDetail,
  getWorkflowWebhookCapture,
  isAuthenticated as hasAuth,
  pb,
  publishWorkflow as publishWorkflowRequest,
  runWorkflow as runWorkflowRequest,
  saveWorkflowGraph,
  validateWorkflow as validateWorkflowRequest,
} from "../api";

const builderRef = ref(null);
const route = useRoute();
const router = useRouter();
const authenticated = ref(hasAuth());
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
const dashboard = ref({
  workflows: [],
  activeWorkflow: null,
  contacts: [],
  companies: [],
  runLogs: [],
});
const loading = ref(true);
const error = ref("");
const currentWorkflowId = ref(typeof route.params.workflowId === "string" ? route.params.workflowId : "");
const realtimeCollections = ["workflows", "workflow_nodes", "workflow_edges", "workflow_runs", "node_runs", "subscribers"];
const nodeLibrary = [
  { label: "Trigger", type: "trigger" },
  { label: "Transform", type: "transform" },
  { label: "Condition", type: "condition" },
  { label: "Event Start", type: "event_start" },
  { label: "Wait", type: "wait_until" },
  { label: "HTTP", type: "http_request" },
  { label: "PocketBase", type: "pb_update" },
  { label: "Transactional Email", type: "send_transactional_email" },
  { label: "Launch Campaign", type: "campaign_launch" },
];

const workflows = computed(() => dashboard.value.workflows || []);
const contacts = computed(() => dashboard.value.contacts || []);
const runLogs = computed(() => dashboard.value.runLogs || []);
const activeWorkflow = computed(() => dashboard.value.activeWorkflow);

const RUN_HISTORY_COLLAPSED_KEY = "workflow-builder:run-history-collapsed";
const runHistoryCollapsed = ref(true);

const runHistoryCount = computed(() => {
  const id = activeWorkflow.value?.workflow?.id;
  if (!id) {
    return runLogs.value.length;
  }
  const matching = runLogs.value.filter((run) => run.workflowId === id);
  return matching.length;
});

onMounted(() => {
  try {
    const stored = window.localStorage.getItem(RUN_HISTORY_COLLAPSED_KEY);
    if (stored === "0") {
      runHistoryCollapsed.value = false;
    } else if (stored === "1") {
      runHistoryCollapsed.value = true;
    }
  } catch (_error) {
    /* ignore */
  }
});

function toggleRunHistory() {
  runHistoryCollapsed.value = !runHistoryCollapsed.value;
  try {
    window.localStorage.setItem(RUN_HISTORY_COLLAPSED_KEY, runHistoryCollapsed.value ? "1" : "0");
  } catch (_error) {
    /* ignore */
  }
}

function setRunHistoryExpanded(expanded) {
  const nextCollapsed = !expanded;
  if (runHistoryCollapsed.value === nextCollapsed) {
    return;
  }
  toggleRunHistory();
}

watch(
  workflows,
  (nextWorkflows) => {
    if (!nextWorkflows.length) {
      return;
    }

    if (!currentWorkflowId.value) {
      currentWorkflowId.value = nextWorkflows[0].id;
    }
  },
  { immediate: true }
);

watch(
  () => route.params.workflowId,
  async (workflowId) => {
    const nextWorkflowId = typeof workflowId === "string" ? workflowId : "";
    if (nextWorkflowId === currentWorkflowId.value) {
      return;
    }

    currentWorkflowId.value = nextWorkflowId;
    validationErrors.value = [];
    validationFindings.value = [];
    saveState.value = "idle";
    saveMessage.value = nextWorkflowId ? "Loading workflow..." : "No workflow selected";
    await refreshDashboard(nextWorkflowId);
  }
);

watch(
  [activeWorkflow, runLogs],
  ([workflow, runs]) => {
    if (loading.value || error.value) {
      selectedRunId.value = "";
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
  if (selectedRunId.value && !loading.value && !error.value) {
    void loadRunDetail(selectedRunId.value);
  }
});

watch(
  authenticated,
  (next) => {
    if (next) {
      void refreshDashboard(currentWorkflowId.value);
      void startRealtimeSubscriptions();
      return;
    }

    stopRealtimeSubscriptions();
  },
  { immediate: true }
);

async function refreshDashboard(workflowId = currentWorkflowId.value) {
  if (!authenticated.value) {
    loading.value = false;
    return;
  }

  loading.value = true;

  try {
    const payload = await getWorkflowDashboard(workflowId);
    dashboard.value = payload;
    currentWorkflowId.value = payload.activeWorkflow?.workflow?.id || workflowId || "";
    syncWorkflowRoute(currentWorkflowId.value);
    error.value = "";
    saveState.value = "idle";
    saveMessage.value = payload.activeWorkflow ? "Workflow loaded" : "No workflow selected";
  } catch (nextError) {
    error.value = nextError instanceof Error ? nextError.message : "Failed to load dashboard";
    saveState.value = "error";
    saveMessage.value = error.value;
  } finally {
    loading.value = false;
  }
}

function replaceDashboard(next) {
  dashboard.value = next;
  currentWorkflowId.value = next.activeWorkflow?.workflow?.id || "";
  syncWorkflowRoute(currentWorkflowId.value);
  error.value = "";
  loading.value = false;
  saveState.value = "idle";
}

function updateWorkflowName(value) {
  const nextName = value;
  if (!dashboard.value.activeWorkflow?.workflow) {
    return;
  }

  dashboard.value = {
    ...dashboard.value,
    workflows: (dashboard.value.workflows || []).map((workflow) => (
      workflow.id === dashboard.value.activeWorkflow.workflow.id
        ? { ...workflow, name: nextName }
        : workflow
    )),
    activeWorkflow: {
      ...dashboard.value.activeWorkflow,
      workflow: {
        ...dashboard.value.activeWorkflow.workflow,
        name: nextName,
      },
    },
  };
}

function syncWorkflowRoute(workflowId) {
  const nextWorkflowId = workflowId || undefined;
  const currentRouteWorkflowId = typeof route.params.workflowId === "string" ? route.params.workflowId : undefined;
  if (currentRouteWorkflowId === nextWorkflowId) {
    return;
  }

  router.replace({
    name: "workflowBuilder",
    params: nextWorkflowId ? { workflowId: nextWorkflowId } : {},
  });
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
    replaceDashboard(await saveWorkflowGraph(payload.workflow.id, {
      name: payload.workflow.name,
      nodes: payload.nodes,
      edges: payload.edges,
    }));
    saveState.value = "saved";
    saveMessage.value = `Saved ${new Date().toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}`;
    validationErrors.value = [];
    validationFindings.value = [];
  } catch (nextError) {
    saveState.value = "error";
    saveMessage.value = `Unsaved: ${nextError instanceof Error ? nextError.message : "Failed to save workflow"}`;
    error.value = nextError instanceof Error ? nextError.message : "Failed to save workflow";
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
    replaceDashboard(await publishWorkflowRequest(workflowId));
    saveState.value = "saved";
    saveMessage.value = "Workflow published";
    validationErrors.value = [];
    validationFindings.value = [];
  } catch (nextError) {
    saveState.value = "error";
    saveMessage.value = nextError instanceof Error ? nextError.message : "Failed to publish workflow";
    error.value = nextError instanceof Error ? nextError.message : "Failed to publish workflow";
    const message = nextError instanceof Error ? nextError.message : "";
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
    const runPayload = {};
    if (triggerMode === "tag_added" || triggerMode === "tag_removed") {
      const demoContactId = String(trigger?.config?.demoContactId ?? "").trim();
      if (demoContactId) {
        runPayload.contactId = demoContactId;
      }
    }
    replaceDashboard(await runWorkflowRequest(workflowId, runPayload));
    saveState.value = "saved";
    saveMessage.value = "Test run queued";
  } catch (nextError) {
    saveState.value = "error";
    saveMessage.value = nextError instanceof Error ? nextError.message : "Failed to queue test run";
    error.value = nextError instanceof Error ? nextError.message : "Failed to queue test run";
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
  } catch (nextError) {
    saveState.value = "error";
    saveMessage.value = nextError instanceof Error ? nextError.message : "Failed to arm webhook capture";
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
        await refreshDashboard();
        saveState.value = "saved";
        saveMessage.value = "Webhook test run executed";
        return;
      }

      saveState.value = "error";
      saveMessage.value = next.error || `Webhook capture ${next.status}`;
    } catch (nextError) {
      stopCapturePolling();
      saveState.value = "error";
      saveMessage.value = nextError instanceof Error ? nextError.message : "Failed to poll webhook capture";
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
    await refreshDashboard(currentWorkflowId.value);
    if (selectedRunId.value) {
      await loadRunDetail(selectedRunId.value);
    }
  }, 150);
}

async function startRealtimeSubscriptions() {
  stopRealtimeSubscriptions();

  await Promise.all(realtimeCollections.map((collectionName) =>
    pb.collection(collectionName).subscribe("*", () => {
      queueDashboardRefresh();
    })));
}

function stopRealtimeSubscriptions() {
  realtimeCollections.forEach((collectionName) => {
    pb.collection(collectionName).unsubscribe("*");
  });

  if (refreshTimer.value) {
    window.clearTimeout(refreshTimer.value);
    refreshTimer.value = undefined;
  }
}

async function selectWorkflow(workflowId) {
  currentWorkflowId.value = workflowId;
  validationErrors.value = [];
  validationFindings.value = [];
  saveState.value = "idle";
  saveMessage.value = "Loading workflow...";
  syncWorkflowRoute(workflowId);
}

async function loadRunDetail(runId) {
  selectedRunLoading.value = true;
  try {
    selectedRunDetail.value = await getWorkflowRunDetail(runId);
  } catch (nextError) {
    error.value = nextError instanceof Error ? nextError.message : "Failed to load run detail";
  } finally {
    selectedRunLoading.value = false;
  }
}

async function selectRun(runId) {
  selectedRunId.value = runId;
  await loadRunDetail(runId);
}

async function cancelRun(runId) {
  if (!runId || !window.confirm("Stop this run? Waiting or queued work will be cancelled.")) {
    return;
  }

  try {
    selectedRunLoading.value = true;
    selectedRunDetail.value = await cancelWorkflowRunRequest(runId);
    saveMessage.value = "Run cancelled";
    saveState.value = "saved";
    queueDashboardRefresh();
  } catch (nextError) {
    error.value = nextError instanceof Error ? nextError.message : "Failed to cancel run";
    saveMessage.value = error.value;
    saveState.value = "error";
  } finally {
    selectedRunLoading.value = false;
  }
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
  } catch (nextError) {
    saveState.value = "error";
    saveMessage.value = nextError instanceof Error ? nextError.message : "Validation failed";
    error.value = nextError instanceof Error ? nextError.message : "Validation failed";
  }
}

async function createWorkflow() {
  saveState.value = "saving";
  saveMessage.value = "Creating workflow...";
  try {
    replaceDashboard(await createWorkflowRequest());
    validationErrors.value = [];
    validationFindings.value = [];
    saveState.value = "saved";
    saveMessage.value = "Workflow created";
  } catch (nextError) {
    saveState.value = "error";
    saveMessage.value = nextError instanceof Error ? nextError.message : "Failed to create workflow";
    error.value = nextError instanceof Error ? nextError.message : "Failed to create workflow";
  }
}

async function deleteWorkflow(workflowId) {
  if (!workflowId || !window.confirm("Delete this workflow? This also removes its runs and graph history.")) {
    return;
  }

  saveState.value = "saving";
  saveMessage.value = "Deleting workflow...";

  try {
    const payload = await deleteWorkflowRequest(workflowId);
    validationErrors.value = [];
    validationFindings.value = [];
    activeCapture.value = null;
    selectedRunId.value = "";
    selectedRunDetail.value = null;

    if (payload.activeWorkflow?.workflow?.id) {
      replaceDashboard(payload);
      saveState.value = "saved";
      saveMessage.value = "Workflow deleted";
      return;
    }

    dashboard.value = payload;
    currentWorkflowId.value = "";
    error.value = "";
    loading.value = false;
    saveState.value = "idle";
    saveMessage.value = "Workflow deleted";
    await router.replace({ name: "workflows" });
  } catch (nextError) {
    saveState.value = "error";
    saveMessage.value = nextError instanceof Error ? nextError.message : "Failed to delete workflow";
    error.value = nextError instanceof Error ? nextError.message : "Failed to delete workflow";
  }
}

onBeforeUnmount(() => {
  stopRealtimeSubscriptions();
  stopCapturePolling();
});
</script>

<template>
  <main v-if="authenticated" class="c-shell workflow-route">
    <section class="workspace-main workspace-main-wide workflow-editor-shell">
      <div class="workflow-editor-shell-top">
        <p v-if="error" class="workflow-editor-error" role="alert">
          Live API unavailable. {{ error }}
        </p>
      </div>

      <WorkflowBuilderPanel
        :contacts="contacts"
        :node-library="nodeLibrary"
        ref="builderRef"
        :key="activeWorkflow?.workflow?.id ?? 'empty'"
        :on-save-request="saveWorkflow"
        :save-message="saveMessage"
        :save-state="saveState"
        :validation-errors="validationErrors"
        :validation-findings="validationFindings"
        :workflow="activeWorkflow"
        @capture-schema="(workflowId, nodeId) => armWebhookCapture(workflowId, 'infer_schema', nodeId)"
        @create-workflow="createWorkflow"
        @delete-workflow="deleteWorkflow"
        @publish="publishWorkflow"
        @run="runWorkflow"
        @save="saveWorkflow"
        @update-workflow-name="updateWorkflowName"
        @validate="validateWorkflow"
      />

      <v-expansion-panels
        class="workflow-run-history-dock"
        :model-value="runHistoryCollapsed ? [] : [0]"
        variant="accordion"
        @update:model-value="setRunHistoryExpanded(Boolean($event?.length))"
      >
        <v-expansion-panel>
          <v-expansion-panel-title>
            <span class="workflow-run-history-toggle-label">Run history</span>
            <v-chip v-if="runHistoryCount" class="workflow-run-history-toggle-count" size="small">{{ runHistoryCount }}</v-chip>
          </v-expansion-panel-title>
          <v-expansion-panel-text class="workflow-run-history-body">
            <RunHistory
              :active-workflow-id="activeWorkflow?.workflow.id"
              :runs="runLogs"
              :selected-run-detail="selectedRunDetail"
              :selected-run-id="selectedRunId"
              :selected-run-loading="selectedRunLoading"
              compact
              @cancel-run="cancelRun"
              @select-run="selectRun"
            />
          </v-expansion-panel-text>
        </v-expansion-panel>
      </v-expansion-panels>
    </section>
  </main>

  <main v-else class="c-shell workflow-route">
    <v-card class="login-card" variant="outlined">
      <div class="panel-header">
        <span class="eyebrow">Workflow Control Plane</span>
        <h1>Admin session required.</h1>
        <p>Sign in to the admin first, then reopen the builder.</p>
      </div>
    </v-card>
  </main>
</template>

<style>
.workflow-route {
  --wf-border: #d8dde6;
  --wf-muted: #7b8796;
  --wf-panel: rgba(255, 255, 255, 0.94);
}

.workflow-route button,
.workflow-route input,
.workflow-route select,
.workflow-route textarea {
  font: inherit;
}

.workflow-route .c-shell {
  min-height: 100vh;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
}



.workflow-route .workflow-route .panel,
.workflow-route .builder-shell,
.workflow-route .login-card {
  border: 1px solid var(--wf-border);
  border-radius: 16px;
  background: var(--wf-panel);
  box-shadow: 0 12px 36px rgba(15, 23, 42, 0.06);
}

.workflow-route .workspace-topbar,
.workflow-route .builder-toolbar,
.workflow-route .modal-header,
.workflow-route .run-row,
.workflow-route .workspace-session {
  display: flex;
  gap: 12px;
}

.workflow-route .workspace-session strong,
.workflow-route .panel-header h2,
.workflow-route .workspace-topbar h1 {
  margin: 0;
}

.workflow-route .workspace-section,
.workflow-route .panel-header,
.workflow-route .workspace-session,
.workflow-route .workspace-main,
.workflow-route .builder-shell,
.workflow-route .detail-panel,
.workflow-route .contact-card,
.workflow-route .run-card,
.workflow-route .login-card,
.workflow-route .builder-heading,
.workflow-route .form-grid,
.workflow-route .record-grid div,
.workflow-route .trace-step-card,
.workflow-route .trace-logs {
  display: grid;
  gap: 6px;
}

.workflow-route .workspace-section-label,
.workflow-route .builder-eyebrow,
.workflow-route .eyebrow,
.workflow-route .field-help,
.workflow-route .builder-summary-label {
  color: var(--wf-muted);
  font-size: 0.76rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.workflow-route .workflow-nav-list,
.workflow-route .library-list,
.workflow-route .contact-list,
.workflow-route .run-list,
.workflow-route .builder-details,
.workflow-route .trace-step-list,
.workflow-route .map-field-list {
  display: grid;
  gap: 8px;
}

.workflow-route .run-history-layout {
  display: grid;
  grid-template-columns: minmax(260px, 320px) minmax(0, 1fr);
  gap: 14px;
}

.workflow-route .workspace-main-wide {
  max-width: 100%;
  margin: 0 auto;
  width: 100%;
}

/* Grid: error strip (optional) + editor + run history. Tight padding — session lives in app bar. */
.workflow-route .workflow-editor-shell-top {
  min-height: 0;
}

.workflow-route .workflow-editor-error {
  margin: 0;
  padding: 6px 10px;
  border-radius: 8px;
  background: #fdebec;
  color: #b42318;
  font-size: 0.84rem;
}

.workflow-route .workspace-main.workflow-editor-shell {
  display: grid;
  grid-template-rows: auto minmax(280px, 1fr) auto;
  gap: 8px;
  align-content: stretch;
  min-height: calc(100dvh - 96px);
  max-width: none;
  width: 100%;
  flex: 1 1 auto;
}

.workflow-route .workflow-editor-shell > .builder-shell {
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 8px 10px;
  gap: 8px;
}

.workflow-route .workflow-editor-shell .builder-toolbar {
  flex-wrap: wrap;
  row-gap: 6px;
  padding: 0;
  align-items: flex-start;
}

.workflow-route .workflow-editor-shell .toolbar-actions.toolbar-actions-compact {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 8px;
  justify-content: flex-end;
}

.workflow-route .workflow-editor-shell .save-indicator {
  padding: 6px 10px;
  font-size: 0.78rem;
}

.workflow-route .workflow-editor-shell .builder-meta-chip {
  min-height: 24px;
  padding: 0 8px;
  font-size: 0.76rem;
}

.workflow-route .workflow-editor-shell .builder-body {
  flex: 1 1 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.workflow-route .workflow-editor-shell .builder-body-single {
  min-height: 0;
}

.workflow-route .workflow-editor-shell .canvas-frame {
  flex: 1 1 0;
  display: flex;
  flex-direction: column;
  align-items: stretch;
  min-height: min(280px, 40vh);
}

.workflow-route .workflow-editor-shell .workflow-canvas {
  flex: 1 1 0;
  min-height: 0;
  height: 100%;
}

.workflow-route .workflow-editor-shell .vue-flow {
  flex: 1 1 auto;
  width: 100%;
  min-height: 280px;
  height: 100%;
}

.workflow-route .workflow-run-history-dock {
  flex: 0 0 auto;
  border: 1px solid var(--wf-border);
  border-radius: 12px;
  background: var(--wf-panel);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.06);
  overflow: hidden;
}

.workflow-route .workflow-run-history-toggle {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: none;
  background: #f8fafc;
  cursor: pointer;
  font: inherit;
  font-weight: 600;
  color: #334155;
}

.workflow-route .workflow-run-history-dock:not(.workflow-run-history-dock--collapsed) .workflow-run-history-toggle {
  border-bottom: 1px solid #e2e8f0;
}

.workflow-route .workflow-run-history-toggle-label {
  flex: 1;
  text-align: left;
}

.workflow-route .workflow-run-history-toggle-count {
  font-size: 0.72rem;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 999px;
  background: #e2e8f0;
  color: #475569;
}

.workflow-route .workflow-run-history-body {
  max-height: min(38vh, 440px);
  overflow: auto;
  padding: 0 10px 12px;
}

.workflow-route .workflow-run-history-body .run-history-card--compact {
  border: none !important;
  box-shadow: none !important;
  background: transparent !important;
}

.workflow-route .builder-controls {
  display: grid;
  gap: 12px;
  padding: 16px 18px;
}

.workflow-route .builder-controls-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.workflow-route .builder-controls-copy {
  display: grid;
  gap: 4px;
}

.workflow-route .builder-controls-copy h2 {
  margin: 0;
  font-size: 1.45rem;
}

.workflow-route .workflow-name-input {
  width: min(420px, 100%);
  min-height: 46px;
  padding: 10px 14px;
  border: 1px solid #ccd5e2;
  border-radius: 14px;
  background: #fff;
  color: #0f172a;
  font-size: 1.4rem;
  font-weight: 700;
}

.workflow-route .builder-controls-row {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}

.workflow-route .builder-controls-row-compact {
  align-items: flex-start;
}

.workflow-route .builder-controls-label {
  color: var(--wf-muted);
  font-size: 0.76rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  padding-top: 10px;
}

.workflow-route .workflow-chip-list,
.workflow-route .quick-add-list {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.workflow-route .workflow-chip-list-scroll {
  flex-wrap: nowrap;
  overflow-x: auto;
  overflow-y: hidden;
  padding-bottom: 2px;
  scrollbar-width: thin;
}

.workflow-route .workflow-chip,
.workflow-route .quick-add-chip {
  border: 1px solid #dbe2ea;
  border-radius: 999px;
  background: #f8fafc;
  color: #1f2937;
  cursor: pointer;
  padding: 9px 14px;
  white-space: nowrap;
}

.workflow-route .workflow-chip-active {
  background: #111827;
  border-color: #111827;
  color: #fff;
}

.workflow-route .workflow-nav-item,
.workflow-route .library-button {
  width: 100%;
  text-align: left;
  border: 1px solid transparent;
  border-radius: 14px;
  cursor: pointer;
}

.workflow-route .run-card-button,
.workflow-route .validation-item {
  padding: 10px 12px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
}

.workflow-route .builder-toolbar p,
.workflow-route .panel-header p,
.workflow-route .contact-meta,
.workflow-route .run-row.subtle,
.workflow-route .workflow-node-card span,
.workflow-route .topbar-note {
  color: #64748b;
  margin: 0;
}

.workflow-route .workflow-chip:hover,
.workflow-route .quick-add-chip:hover {
  background: #f3f6f9;
}

.workflow-route .workspace-main,
.workflow-route .builder-shell {
  padding: 14px;
  align-content: start;
  gap: 14px;
}

.workflow-route .builder-toolbar {
  align-items: flex-start;
  justify-content: space-between;
  flex-wrap: wrap;
  padding: 4px 2px 0;
}

.workflow-route .builder-heading {
  min-width: 0;
  flex: 1 1 420px;
}

.workflow-route .workspace-topbar {
  align-items: center;
  justify-content: space-between;
  padding: 2px 2px 0;
}

.workflow-route .workspace-session {
  justify-items: end;
  text-align: right;
}

.workflow-route .toolbar-actions {
  display: grid;
  grid-template-columns: repeat(3, max-content);
  justify-content: flex-end;
  gap: 10px;
  align-content: start;
}

.workflow-route .toolbar-actions-compact {
  flex: 0 0 auto;
  max-width: none;
}

.workflow-route .builder-title-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.workflow-route .builder-meta {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.workflow-route .builder-meta-chip {
  display: inline-flex;
  align-items: center;
  min-height: 30px;
  padding: 0 10px;
  border-radius: 999px;
  background: #f4f7fb;
  border: 1px solid #dde5ee;
  color: #526173;
  font-size: 0.84rem;
}

.workflow-route .builder-meta-chip-wide {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
}

.workflow-route .save-indicator,
.workflow-route .status-pill,
.workflow-route .validation-severity,
.workflow-route .workflow-node-badge {
  border-radius: 999px;
}

.workflow-route .save-indicator {
  align-self: flex-start;
  padding: 10px 14px;
  background: #eef3f8;
  color: #51606f;
  white-space: nowrap;
}

.workflow-route .save-indicator[data-state="saving"],
.workflow-route .validation-severity[data-severity="warning"] {
  background: #fff4d6;
  color: #9a6700;
}

.workflow-route .save-indicator[data-state="saved"],
.workflow-route .status-pill[data-status="success"] {
  background: #e7f7ef;
  color: #0f766e;
}

.workflow-route .save-indicator[data-state="error"],
.workflow-route .status-pill[data-status="failed"],
.workflow-route .validation-severity[data-severity="error"] {
  background: #fdebec;
  color: #b42318;
}

.workflow-route .save-indicator[data-state="dirty"] {
  background: #fef0c7;
  color: #a16207;
}

.workflow-route .builder-body,
.workflow-route .bottom-grid {
  display: grid;
  gap: 16px;
  width: 100%;
}

.workflow-route .builder-empty-state {
  min-height: 140px;
  align-content: center;
}

.workflow-route .builder-body {
  grid-template-columns: 320px minmax(0, 1fr);
  min-height: 680px;
}

.workflow-route .builder-body-single {
  grid-template-columns: minmax(0, 1fr);
  min-width: 0;
}

.workflow-route .bottom-grid {
  grid-template-columns: minmax(0, 1fr);
}

.workflow-route .modal-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 14px;
  border-bottom: 1px solid #e6ebf2;
}

.workflow-route .modal-header h2 {
  font-size: 1.55rem;
}

.workflow-route .modal-header > div:first-child {
  min-width: 0;
}

.workflow-route .node-modal-actions,
.workflow-route .template-picker-actions {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
}

.workflow-route .node-inspector {
  border: none;
  box-shadow: none;
  background: transparent;
  padding: 0;
}

.workflow-route .inspector-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.workflow-route .form-field-card,
.workflow-route .context-help {
  display: grid;
  gap: 10px;
  padding: 16px;
  border: 1px solid #e2e8f0;
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.75);
}

.workflow-route .form-field-card-wide {
  grid-column: 1 / -1;
}

.workflow-route .form-field-header {
  display: grid;
  gap: 6px;
}

.workflow-route .form-field-label {
  font-size: 0.98rem;
  font-weight: 700;
  color: #0f172a;
}

.workflow-route .field-help {
  margin: 0;
  color: #64748b;
  font-size: 0.9rem;
  line-height: 1.45;
  letter-spacing: normal;
  text-transform: none;
}

.workflow-route .context-help-header {
  display: grid;
  gap: 4px;
}

.workflow-route .context-help-header p {
  margin: 0;
  color: #64748b;
  font-size: 0.9rem;
}

.workflow-route .context-token-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.workflow-route .detail-panel,
.workflow-route .panel {
  padding: 16px;
}

.workflow-route .record-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.workflow-route .record-grid div,
.workflow-route .contact-card,
.workflow-route .trace-step-card,
.workflow-route .run-trace-panel,
.workflow-route .trace-json-block {
  padding: 12px;
  border-radius: 14px;
  background: #f6f8fb;
}

.workflow-route .canvas-frame {
  min-height: 680px;
  width: 100%;
  min-width: 0;
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid #e2e8f1;
  background: #f6f8fb;
}

.workflow-route .workflow-canvas {
  height: 100%;
  width: 100%;
  min-width: 0;
  background: linear-gradient(180deg, rgba(249, 251, 253, 0.98), rgba(245, 248, 251, 0.98));
}

.workflow-route .vue-flow {
  min-height: 640px;
  width: 100%;
}

.workflow-route .workflow-node-card {
  min-width: 180px;
  padding: 10px 12px;
  border-radius: 14px;
  border: 1px solid #dbe2ea;
  background: #fff;
  box-shadow: 0 6px 18px rgba(15, 23, 42, 0.08);
  display: grid;
  gap: 4px;
}

.workflow-route .workflow-node-badge {
  justify-self: start;
  padding: 2px 8px;
  background: #edf7f3;
  color: #0f766e;
  font-size: 0.72rem;
  font-weight: 600;
}

.workflow-route .workflow-node-card[data-kind="transform"] .workflow-node-badge {
  background: #eef4ff;
  color: #3b5ccc;
}

.workflow-route .workflow-node-card[data-kind="condition"] .workflow-node-badge {
  background: #edf9ed;
  color: #15803d;
}

.workflow-route .workflow-node-card[data-kind="event_start"] .workflow-node-badge {
  background: #fff1f2;
  color: #be123c;
}

.workflow-route .workflow-node-card[data-kind="pb_update"] .workflow-node-badge {
  background: #fff4d6;
  color: #9a6700;
}

.workflow-route .workflow-node-card[data-kind="send_transactional_email"] .workflow-node-badge {
  background: #fef3e6;
  color: #b45309;
}

.workflow-route .workflow-node-card[data-kind="campaign_launch"] .workflow-node-badge {
  background: #e0f2fe;
  color: #0f5b99;
}

.workflow-route .workflow-node-card[data-kind="wait_until"] .workflow-node-badge {
  background: #f3e8ff;
  color: #7c3aed;
}

.workflow-route .branch-handle-labels {
  display: flex;
  justify-content: space-between;
  margin-top: 2px;
  padding: 0 18px 8px;
  font-size: 0.72rem;
  font-weight: 600;
  color: #5f6b7a;
}

.workflow-route .vue-flow__node.selected .workflow-node-card,
.workflow-route .flow-node-invalid .workflow-node-card {
  box-shadow: 0 0 0 3px rgba(59, 92, 204, 0.14), 0 6px 18px rgba(15, 23, 42, 0.08);
}

.workflow-route .flow-node-invalid .workflow-node-card {
  box-shadow: 0 0 0 3px rgba(180, 35, 24, 0.18), 0 6px 18px rgba(15, 23, 42, 0.08);
}

.workflow-route .vue-flow__handle {
  width: 10px;
  height: 10px;
  background: #cbd5e1;
  border: 2px solid #fff;
}

.workflow-route .vue-flow__controls {
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.08);
  border-radius: 10px;
  overflow: hidden;
  border: 1px solid #dde5ef;
}

.workflow-route .vue-flow__controls-button {
  width: 32px;
  height: 32px;
  border: none;
  background: rgba(255, 255, 255, 0.96);
  color: #334155;
}

.workflow-route .vue-flow__minimap {
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid #dde3eb;
  border-radius: 8px;
  box-shadow: 0 8px 18px rgba(15, 23, 42, 0.08);
}

.workflow-route .status-pill,
.workflow-route .validation-severity {
  display: inline-flex;
  justify-content: center;
  width: fit-content;
  padding: 4px 10px;
  font-size: 0.76rem;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.workflow-route .status-pill[data-status="queued"],
.workflow-route .status-pill[data-status="running"],
.workflow-route .status-pill[data-status="waiting"] {
  background: #dfeeff;
  color: #0f5bd8;
}

.workflow-route .status-pill[data-status="cancelled"] {
  background: #ebeff5;
  color: #475569;
}

.workflow-route .run-card-button,
.workflow-route .validation-item {
  cursor: pointer;
  text-align: left;
  border-radius: 14px;
}

.workflow-route .run-card-active {
  background: #eef3f8;
  border-color: #c7d3e2;
}

.workflow-route .run-row {
  align-items: center;
  justify-content: space-between;
}

.workflow-route .trace-error {
  color: #b42318;
  font-weight: 600;
  margin: 0;
}

.workflow-route .trace-json-block pre {
  margin: 12px 0 0;
  overflow: auto;
}

.workflow-route .context-help {
  grid-column: 1 / -1;
  background: linear-gradient(180deg, #f8fbff, #f3f7fc);
}

.workflow-route .demo-contact-hint {
  padding: 10px 14px;
  color: #64748b;
  font-size: 0.9rem;
}

.workflow-route .demo-contact-selection {
  display: inline-flex;
  align-items: center;
  min-height: 26px;
}

.workflow-route .form-field-card .v-input {
  width: 100%;
}

.workflow-route .context-token {
  display: inline-flex;
  align-items: center;
  min-height: 34px;
  padding: 0 12px;
  border-radius: 999px;
  border: 1px solid #d7e0ea;
  background: #fff;
  color: #475569;
  font-family: "IBM Plex Mono", "SFMono-Regular", monospace;
  font-size: 0.82rem;
}

.workflow-route .form-field > input,
.workflow-route .form-field > select,
.workflow-route .form-field > textarea,
.workflow-route .map-field-row > input {
  background: #fff;
  border: 1px solid #ccd5e2;
  border-radius: 12px;
  font: inherit;
  min-height: 42px;
  padding: 10px 12px;
}

.workflow-route .form-field > textarea {
  min-height: 160px;
  resize: vertical;
}

.workflow-route .map-field-row {
  display: grid;
  gap: 8px;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
}

.workflow-route .login-card {
  max-width: 640px;
  margin: 48px auto 0;
}

@media (max-width: 1200px) {
  .workflow-route .builder-body,
  .workflow-route .bottom-grid,
  .workflow-route .run-history-layout {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 760px) {
  .workflow-route .workspace-main.workflow-editor-shell {
    grid-template-rows: auto minmax(220px, 1fr) auto;
    gap: 6px;
    min-height: calc(100dvh - 84px);
  }

  .workflow-route .workflow-editor-shell > .builder-shell {
    padding: 8px;
    gap: 6px;
  }

  .workflow-route .workspace-topbar,
  .workflow-route .builder-toolbar,
  .workflow-route .modal-header,
  .workflow-route .run-row,
  .workflow-route .workspace-session {
    flex-direction: column;
    align-items: flex-start;
  }

  .workflow-route .builder-controls-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .workflow-route .workflow-editor-shell .toolbar-actions {
    grid-template-columns: 1fr;
    justify-content: stretch;
    width: 100%;
  }

  .workflow-route .workflow-editor-shell .builder-toolbar {
    width: 100%;
    gap: 8px;
    justify-content: flex-start;
    align-items: stretch;
  }

  .workflow-route .workflow-editor-shell .builder-heading,
  .workflow-route .workflow-editor-shell .builder-editor-bar,
  .workflow-route .workflow-editor-shell .builder-meta {
    width: 100%;
  }

  .workflow-route .workflow-editor-shell .builder-heading {
    flex: 0 0 auto;
  }

  .workflow-route .workflow-editor-shell .builder-meta {
    gap: 6px;
  }

  .workflow-route .inspector-fields,
  .workflow-route .record-grid,
  .workflow-route .map-field-row {
    grid-template-columns: 1fr;
  }
}
</style>
