<template>
  <section class="workflow-hub">
    <v-card class="workflow-hero" variant="outlined">
      <div class="workflow-hero-copy">
        <span class="workflow-eyebrow">Automation Workspace</span>
        <h1 class="title is-3">Workflows in the Admin</h1>
        <p>
          Manage automations next to subscribers, campaigns, and lists. Review workflow health here,
          then jump into the builder when you need to edit logic.
        </p>
      </div>

      <div class="workflow-hero-actions">
        <v-btn type="button" color="primary" variant="flat" @click="openBuilder()">Open Builder</v-btn>
      </div>
    </v-card>

    <div class="workflow-stats">
      <v-card class="workflow-stat-card" variant="outlined">
        <span class="workflow-stat-label">Workflows</span>
        <strong>{{ stats.totalWorkflows }}</strong>
        <p>{{ stats.publishedWorkflows }} published</p>
      </v-card>

      <v-card class="workflow-stat-card" variant="outlined">
        <span class="workflow-stat-label">Runs</span>
        <strong>{{ stats.totalRuns }}</strong>
        <p>{{ stats.runningRuns }} active right now</p>
      </v-card>

      <v-card class="workflow-stat-card" variant="outlined">
        <span class="workflow-stat-label">Subscribers in Scope</span>
        <strong>{{ stats.totalContacts }}</strong>
        <p>{{ stats.webhookWorkflows }} webhook-driven workflows</p>
      </v-card>

      <v-card class="workflow-stat-card" variant="outlined">
        <span class="workflow-stat-label">Active Draft</span>
        <strong>{{ activeWorkflowName }}</strong>
        <p>{{ activeWorkflowTrigger }}</p>
      </v-card>
    </div>

    <div class="workflow-grid">
      <v-card class="workflow-panel" variant="outlined">
        <div class="workflow-panel-header">
          <div>
            <span class="workflow-eyebrow">Library</span>
            <h2>Workflow Catalog</h2>
          </div>
          <v-btn type="button" variant="outlined" @click="refreshDashboard">Refresh</v-btn>
        </div>

        <div v-if="loading" class="workflow-empty-state">
          Loading workflow overview...
        </div>

        <div v-else-if="dashboard.workflows.length === 0" class="workflow-empty-state">
          No workflows found yet.
        </div>

        <div v-else class="workflow-card-list">
          <v-card
            v-for="workflow in dashboard.workflows"
            :key="workflow.id"
            class="workflow-record-card"
            :class="{ 'workflow-record-card-active': workflow.id === selectedWorkflowId }"
            variant="outlined"
            role="button"
            tabindex="0"
            @click="selectWorkflow(workflow.id)"
            @keydown.enter.prevent="selectWorkflow(workflow.id)"
            @keydown.space.prevent="selectWorkflow(workflow.id)"
          >
            <div class="workflow-record-topline">
              <strong>{{ workflow.name }}</strong>
              <span class="workflow-status-pill" :class="`workflow-status-${workflow.status}`">
                {{ workflow.status }}
              </span>
            </div>
            <p>{{ workflow.triggerType }} trigger · v{{ workflow.version }}</p>
          </v-card>
        </div>
      </v-card>

      <v-card class="workflow-panel" variant="outlined">
        <div class="workflow-panel-header">
          <div>
            <span class="workflow-eyebrow">Recent Activity</span>
            <h2>Workflow Runs</h2>
          </div>
          <v-btn type="button" variant="outlined" @click="openBuilder()">Open Builder</v-btn>
        </div>

        <div v-if="dashboard.runLogs.length === 0" class="workflow-empty-state">
          No recent runs to review.
        </div>

        <div v-else class="workflow-run-list">
          <v-card v-for="run in recentRuns" :key="run.id" class="workflow-run-card" variant="outlined">
            <div class="workflow-run-topline">
              <strong>{{ run.workflowName }}</strong>
              <span class="workflow-run-status" :class="`workflow-run-${run.status}`">
                {{ run.status }}
              </span>
            </div>
            <p>{{ run.summary }}</p>
            <div class="workflow-run-meta">
              <span>{{ run.contactName || 'Subscriber context unavailable' }}</span>
              <span>{{ $utils.niceDate(run.startedAt) }}</span>
            </div>
          </v-card>
        </div>
      </v-card>
    </div>

    <v-card class="workflow-panel workflow-focus-panel" variant="outlined">
      <div class="workflow-panel-header">
        <div>
          <span class="workflow-eyebrow">Focus</span>
          <h2>{{ activeWorkflowName }}</h2>
        </div>
        <div class="workflow-focus-actions">
          <v-btn type="button" color="error" variant="outlined" :disabled="!selectedWorkflowId" @click="confirmDeleteWorkflow()">Delete Workflow</v-btn>
          <v-btn type="button" color="primary" variant="flat" @click="openBuilder()">Edit in Builder</v-btn>
        </div>
      </div>

      <div class="workflow-focus-grid">
        <v-card class="workflow-focus-card" variant="outlined">
          <span class="workflow-focus-label">Trigger</span>
          <strong>{{ activeWorkflowTrigger }}</strong>
        </v-card>
        <v-card class="workflow-focus-card" variant="outlined">
          <span class="workflow-focus-label">Nodes</span>
          <strong>{{ activeNodeCount }}</strong>
        </v-card>
        <v-card class="workflow-focus-card" variant="outlined">
          <span class="workflow-focus-label">Edges</span>
          <strong>{{ activeEdgeCount }}</strong>
        </v-card>
      </div>
    </v-card>
  </section>
</template>

<script>
export default {
  name: 'WorkflowView',

  data() {
    return {
      loading: true,
      dashboard: {
        workflows: [],
        activeWorkflow: null,
        contacts: [],
        companies: [],
        runLogs: [],
      },
      selectedWorkflowId: '',
    };
  },

  computed: {
    stats() {
      const workflows = Array.isArray(this.dashboard.workflows) ? this.dashboard.workflows : [];
      const runLogs = Array.isArray(this.dashboard.runLogs) ? this.dashboard.runLogs : [];
      const contacts = Array.isArray(this.dashboard.contacts) ? this.dashboard.contacts : [];

      return {
        totalWorkflows: workflows.length,
        publishedWorkflows: workflows.filter((item) => item.status === 'published').length,
        webhookWorkflows: workflows.filter((item) => item.triggerType === 'webhook').length,
        totalRuns: runLogs.length,
        runningRuns: runLogs.filter((item) => item.status === 'running' || item.status === 'waiting').length,
        totalContacts: contacts.length,
      };
    },

    recentRuns() {
      const runLogs = Array.isArray(this.dashboard.runLogs) ? this.dashboard.runLogs : [];
      return runLogs.slice(0, 6);
    },

    activeWorkflowName() {
      if (this.dashboard.activeWorkflow && this.dashboard.activeWorkflow.workflow) {
        return this.dashboard.activeWorkflow.workflow.name;
      }
      return 'No workflow selected';
    },

    activeWorkflowTrigger() {
      if (this.dashboard.activeWorkflow && this.dashboard.activeWorkflow.workflow) {
        return `${this.dashboard.activeWorkflow.workflow.triggerType} trigger`;
      }
      return 'Open the builder to start a draft';
    },

    activeNodeCount() {
      return this.dashboard.activeWorkflow && Array.isArray(this.dashboard.activeWorkflow.nodes)
        ? this.dashboard.activeWorkflow.nodes.length
        : 0;
    },

    activeEdgeCount() {
      return this.dashboard.activeWorkflow && Array.isArray(this.dashboard.activeWorkflow.edges)
        ? this.dashboard.activeWorkflow.edges.length
        : 0;
    },
  },

  methods: {
    async refreshDashboard(workflowId = this.selectedWorkflowId) {
      this.loading = true;
      try {
        const payload = await this.$api.getWorkflowDashboard(workflowId || undefined);
        this.dashboard = {
          workflows: Array.isArray(payload.workflows) ? payload.workflows : [],
          activeWorkflow: payload.activeWorkflow || null,
          contacts: Array.isArray(payload.contacts) ? payload.contacts : [],
          companies: Array.isArray(payload.companies) ? payload.companies : [],
          runLogs: Array.isArray(payload.runLogs) ? payload.runLogs : [],
        };

        if (!this.selectedWorkflowId && this.dashboard.activeWorkflow && this.dashboard.activeWorkflow.workflow) {
          this.selectedWorkflowId = this.dashboard.activeWorkflow.workflow.id;
        }
      } catch (err) {
        this.$utils.toast(err && err.message ? err.message : 'Failed to load workflows', 'is-danger');
      } finally {
        this.loading = false;
      }
    },

    selectWorkflow(workflowId) {
      this.selectedWorkflowId = workflowId;
      this.refreshDashboard(workflowId);
    },

    async deleteSelectedWorkflow() {
      if (!this.selectedWorkflowId) {
        return;
      }

      try {
        const payload = await this.$api.deleteWorkflow(this.selectedWorkflowId);
        this.dashboard = {
          workflows: Array.isArray(payload.workflows) ? payload.workflows : [],
          activeWorkflow: payload.activeWorkflow || null,
          contacts: Array.isArray(payload.contacts) ? payload.contacts : [],
          companies: Array.isArray(payload.companies) ? payload.companies : [],
          runLogs: Array.isArray(payload.runLogs) ? payload.runLogs : [],
        };
        this.selectedWorkflowId = this.dashboard.activeWorkflow?.workflow?.id || this.dashboard.workflows[0]?.id || '';
      } catch (err) {
        this.$utils.toast(err && err.message ? err.message : 'Failed to delete workflow', 'is-danger');
      }
    },

    confirmDeleteWorkflow() {
      const name = this.dashboard.activeWorkflow?.workflow?.name || 'this workflow';
      this.$utils.confirm(`Delete ${name}? This also removes its runs and graph history.`, this.deleteSelectedWorkflow);
    },

    openBuilder(workflowId = this.selectedWorkflowId || this.dashboard.activeWorkflow?.workflow?.id || this.dashboard.workflows[0]?.id) {
      this.$router.push(
        workflowId
          ? { name: 'workflowBuilder', params: { workflowId } }
          : { name: 'workflowBuilder' },
      );
    },
  },

  created() {
    this.$events.$on('page.refresh', this.refreshDashboard);
  },

  mounted() {
    this.refreshDashboard();
  },

  beforeUnmount() {
    this.$events.$off('page.refresh', this.refreshDashboard);
  },
};
</script>

<style scoped>
.workflow-hub {
  display: grid;
  gap: 24px;
}

.workflow-hero,
.workflow-panel,
.workflow-stat-card {
  background: #fff;
  border: 1px solid #dde5f0;
  border-radius: 20px;
  box-shadow: 0 14px 35px rgba(15, 23, 42, 0.05);
}

.workflow-hero {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  padding: 28px;
  background:
    radial-gradient(circle at top right, rgba(15, 91, 216, 0.08), transparent 28%),
    linear-gradient(180deg, #ffffff, #f8fbff);
}

.workflow-hero-copy {
  display: grid;
  gap: 10px;
  max-width: 720px;
}

.workflow-hero-copy p,
.workflow-run-card p,
.workflow-record-card p {
  color: #6b7280;
  margin: 0;
}

.workflow-eyebrow,
.workflow-stat-label,
.workflow-focus-label {
  color: #64748b;
  font-size: 0.74rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.workflow-hero-actions {
  align-items: flex-start;
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.workflow-stats {
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.workflow-stat-card {
  display: grid;
  gap: 8px;
  padding: 20px;
}

.workflow-stat-card strong,
.workflow-focus-card strong {
  color: #0f172a;
  font-size: 1.85rem;
  line-height: 1.1;
}

.workflow-stat-card p {
  color: #6b7280;
  margin: 0;
}

.workflow-grid {
  display: grid;
  gap: 24px;
  grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.9fr);
}

.workflow-panel {
  display: grid;
  gap: 18px;
  padding: 22px;
}

.workflow-panel-header {
  align-items: start;
  display: flex;
  gap: 16px;
  justify-content: space-between;
}

.workflow-focus-actions {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.workflow-panel-header h2 {
  margin: 4px 0 0;
}

.workflow-card-list,
.workflow-run-list {
  display: grid;
  gap: 12px;
}

.workflow-record-card,
.workflow-run-card {
  background: #f8fbff;
  border: 1px solid #dce7f8;
  border-radius: 16px;
  display: grid;
  gap: 10px;
  padding: 16px;
  text-align: left;
}

.workflow-record-card {
  cursor: pointer;
}

.workflow-record-card-active {
  background: #edf4ff;
  border-color: #9dbcf3;
  box-shadow: inset 0 0 0 1px rgba(15, 91, 216, 0.18);
}

.workflow-record-topline,
.workflow-run-topline,
.workflow-run-meta,
.workflow-focus-grid {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
}

.workflow-status-pill,
.workflow-run-status {
  border-radius: 999px;
  font-size: 0.78rem;
  font-weight: 700;
  padding: 5px 10px;
  text-transform: capitalize;
}

.workflow-status-draft {
  background: #fff4d6;
  color: #8a5b00;
}

.workflow-status-published,
.workflow-run-success {
  background: #ddf6e7;
  color: #0f6a34;
}

.workflow-status-archived,
.workflow-run-cancelled {
  background: #ebeff5;
  color: #475569;
}

.workflow-run-running,
.workflow-run-waiting {
  background: #dfeeff;
  color: #0f5bd8;
}

.workflow-run-failed {
  background: #fde7e3;
  color: #b42318;
}

.workflow-run-meta {
  color: #64748b;
  font-size: 0.88rem;
}

.workflow-focus-panel {
  gap: 20px;
}

.workflow-focus-grid {
  align-items: stretch;
  gap: 16px;
  justify-content: flex-start;
}

.workflow-focus-card {
  background: #f8fbff;
  border: 1px solid #dce7f8;
  border-radius: 16px;
  display: grid;
  gap: 8px;
  min-width: 180px;
  padding: 18px;
}

.workflow-empty-state {
  background: #f8fafc;
  border: 1px dashed #cdd7e3;
  border-radius: 16px;
  color: #64748b;
  padding: 28px;
  text-align: center;
}

@media (max-width: 1100px) {
  .workflow-stats,
  .workflow-grid {
    grid-template-columns: 1fr 1fr;
  }
}

@media (max-width: 840px) {
  .workflow-hero,
  .workflow-panel-header {
    flex-direction: column;
  }

  .workflow-stats,
  .workflow-grid {
    grid-template-columns: 1fr;
  }

  .workflow-focus-grid {
    flex-direction: column;
  }

  .workflow-focus-card {
    min-width: 0;
  }
}
</style>
