<script setup>
/* eslint-disable */
import { computed } from "vue";

const props = defineProps({
  activeWorkflowId: { type: String, default: "" },
  runs: { type: Array, default: () => [] },
  selectedRunDetail: { type: Object, default: null },
  selectedRunId: { type: String, default: "" },
  selectedRunLoading: { type: Boolean, default: false },
  compact: { type: Boolean, default: false },
});

const emit = defineEmits(["cancelRun", "selectRun"]);

const visibleRuns = computed(() => {
  if (!props.activeWorkflowId) {
    return props.runs;
  }

  const matching = props.runs.filter((run) => run.workflowId === props.activeWorkflowId);
  return matching.length ? matching : props.runs;
});

function formatJson(value) {
  return JSON.stringify(value, null, 2);
}

const canCancelSelectedRun = computed(() => ["queued", "waiting"].includes(String(props.selectedRunDetail?.status ?? "")));
</script>

<template>
  <v-card class="panel panel-dark" :class="{ 'run-history-card--compact': compact }" flat>
    <v-card-title v-if="!compact" class="panel-header">
      <div>
        <h2>Run History</h2>
        <p>Open an execution to inspect each node run, payload transition, and log line.</p>
      </div>
    </v-card-title>

    <v-card-text>
      <div class="run-history-layout">
        <v-list class="run-list">
          <v-btn
            v-for="run in visibleRuns"
            :key="run.id"
            class="run-card run-card-button"
            :class="{ 'run-card-active': run.id === selectedRunId }"
            variant="text"
            @click="emit('selectRun', run.id)"
          >
            <div class="run-row">
              <strong>{{ run.workflowName }}</strong>
              <span class="status-pill" :data-status="run.status">{{ run.status }}</span>
            </div>
            <div class="run-row subtle">
              <span>{{ run.contactName || "No contact" }}</span>
              <span>{{ run.startedAt }}</span>
            </div>
            <p>{{ run.summary }}</p>
          </v-btn>
        </v-list>

        <div class="run-trace-panel">
          <div v-if="selectedRunLoading" class="run-trace-empty">Loading execution trace...</div>
            <div v-else-if="selectedRunDetail" class="run-trace-detail">
              <div class="run-trace-summary">
                <div class="run-row">
                  <strong>{{ selectedRunDetail.workflowName }}</strong>
                  <span class="status-pill" :data-status="selectedRunDetail.status">{{ selectedRunDetail.status }}</span>
                </div>
                <div class="run-row subtle">
                  <span>{{ selectedRunDetail.contactName || "No contact" }}</span>
                  <span>{{ selectedRunDetail.startedAt }}<template v-if="selectedRunDetail.endedAt"> -> {{ selectedRunDetail.endedAt }}</template></span>
                </div>
                <p>{{ selectedRunDetail.summary }}</p>
                <v-btn v-if="canCancelSelectedRun" @click="emit('cancelRun', selectedRunDetail.id)" color="error" class="run-cancel-button">
                  Stop Run
                </v-btn>
              </div>

              <v-expansion-panels variant="accordion">
                <v-expansion-panel class="trace-json-block">
                  <v-expansion-panel-title>Trigger Payload</v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <pre>{{ formatJson(selectedRunDetail.triggerPayload) }}</pre>
                  </v-expansion-panel-text>
                </v-expansion-panel>
              </v-expansion-panels>

              <div class="trace-step-list">
                <v-card v-for="nodeRun in selectedRunDetail.nodeRuns" :key="nodeRun.id" class="trace-step-card mb-3">
                  <v-card-text>
                    <div class="run-row">
                      <strong>{{ nodeRun.nodeLabel || nodeRun.nodeType }}</strong>
                      <span class="status-pill" :data-status="nodeRun.status">{{ nodeRun.status }}</span>
                    </div>
                    <div class="run-row subtle">
                      <span>{{ nodeRun.nodeType }}</span>
                      <span>{{ nodeRun.startedAt }}<template v-if="nodeRun.endedAt"> -> {{ nodeRun.endedAt }}</template></span>
                    </div>
                    <p v-if="nodeRun.error" class="trace-error">{{ nodeRun.error }}</p>
                    <v-expansion-panels variant="accordion">
                      <v-expansion-panel class="trace-json-block">
                        <v-expansion-panel-title>Input</v-expansion-panel-title>
                        <v-expansion-panel-text>
                          <pre>{{ formatJson(nodeRun.input) }}</pre>
                        </v-expansion-panel-text>
                      </v-expansion-panel>
                      <v-expansion-panel class="trace-json-block">
                        <v-expansion-panel-title>Output</v-expansion-panel-title>
                        <v-expansion-panel-text>
                          <pre>{{ formatJson(nodeRun.output) }}</pre>
                        </v-expansion-panel-text>
                      </v-expansion-panel>
                    </v-expansion-panels>
                    <div v-if="nodeRun.logs.length" class="trace-logs">
                      <strong>Logs</strong>
                      <span v-for="log in nodeRun.logs" :key="log">{{ log }}</span>
                    </div>
                  </v-card-text>
                </v-card>
              </div>
            </div>
          <div v-else class="run-trace-empty">Select a run to inspect its execution trace.</div>
        </div>
      </div>
    </v-card-text>
  </v-card>
</template>

<style scoped>
.run-history-card--compact :deep(.v-card-text) {
  padding-top: 8px;
}
</style>
