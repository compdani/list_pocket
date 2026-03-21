<script setup>
/* eslint-disable */
const props = defineProps({
  activeWorkflowId: { type: String, default: "" },
  workflows: { type: Array, default: () => [] },
});

const emit = defineEmits(["add", "createWorkflow", "selectWorkflow"]);

const nodeLibrary = [
  { label: "Trigger", type: "trigger", description: "Webhook match and security" },
  { label: "Transform", type: "transform", description: "Use run + previous context" },
  { label: "Condition", type: "condition", description: "Branch by context path" },
  { label: "Event Start", type: "event_start", description: "Anchor a named event date" },
  { label: "Wait", type: "wait_until", description: "Pause until a computed time" },
  { label: "HTTP", type: "http_request", description: "Call external service" },
  { label: "PocketBase", type: "pb_update", description: "Query or update records" },
  { label: "Transactional Email", type: "send_transactional_email", description: "Send tracked transactional email" },
  { label: "Launch Campaign", type: "campaign_launch", description: "Start an existing campaign" },
];
</script>

<template>
  <aside class="workspace-rail">
    <div class="workspace-section">
      <button class="primary-button sidebar-primary-button" type="button" @click="emit('createWorkflow')">
        New Workflow
      </button>
    </div>

    <div class="workspace-section">
      <div class="workflow-nav-list">
        <button
          v-for="workflow in props.workflows"
          :key="workflow.id"
          class="workflow-nav-item"
          :class="{ 'workflow-nav-item-active': workflow.id === props.activeWorkflowId }"
          type="button"
          @click="emit('selectWorkflow', workflow.id)"
        >
          <strong>{{ workflow.name }}</strong>
          <span>{{ workflow.triggerType }} · v{{ workflow.version }}</span>
        </button>
      </div>
    </div>

    <div class="workspace-section">
      <span class="workspace-section-label">Quick Add</span>
      <div class="library-list">
        <button
          v-for="node in nodeLibrary"
          :key="node.label"
          class="library-card library-button"
          type="button"
          @click="emit('add', node.type)"
        >
          <strong>{{ node.label }}</strong>
          <span>{{ node.description }}</span>
        </button>
      </div>
    </div>
  </aside>
</template>
