<script setup>
/* eslint-disable */
import { computed } from "vue";
import { Handle, Position } from "@vue-flow/core";

const props = defineProps({
  id: { type: String, required: true },
  data: { type: Object, required: true },
});

const badgeLabel = computed(() => {
  switch (props.data.type) {
    case "trigger":
      return "Trigger";
    case "transform":
      return "Script";
    case "condition":
      return "If / Else";
    case "event_start":
      return "Event";
    case "http_request":
      return "HTTP";
    case "pb_query":
      return "Query";
    case "pb_update":
      return "CRM";
    case "wait_until":
      return "Wait";
    default:
      return "Node";
  }
});

const isTrigger = computed(() => props.data.type === "trigger");
const isTerminal = computed(() => props.data.type === "pb_update");
const isCondition = computed(() => props.data.type === "condition");
</script>

<template>
  <div class="workflow-node-card" :data-kind="data.type">
    <Handle v-if="!isTrigger" type="target" :position="Position.Top" />
    <span class="workflow-node-badge">{{ badgeLabel }}</span>
    <strong>{{ data.label }}</strong>
    <span>{{ data.contactMode }}</span>
    <template v-if="isCondition">
      <div class="branch-handle-labels">
        <span>Yes</span>
        <span>No</span>
      </div>
      <Handle id="yes" type="source" :position="Position.Bottom" :style="{ left: '32%' }" />
      <Handle id="no" type="source" :position="Position.Bottom" :style="{ left: '68%' }" />
    </template>
    <Handle v-else-if="!isTerminal" type="source" :position="Position.Bottom" />
  </div>
</template>
