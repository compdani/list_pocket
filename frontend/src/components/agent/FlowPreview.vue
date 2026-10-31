<template>
  <div class="flow-preview" data-cy="agent-flow-preview">
    <VueFlow
      :id="flowId"
      :nodes="flow.nodes"
      :edges="flow.edges"
      :node-types="nodeTypes"
      :nodes-draggable="false"
      :nodes-connectable="false"
      :elements-selectable="false"
      :min-zoom="0.2"
      :max-zoom="1.2"
      fit-view-on-init
    >
      <Background :gap="22" :size="1.2" />
    </VueFlow>
  </div>
</template>

<script setup>
import { computed, markRaw } from 'vue';
import '@vue-flow/core/dist/style.css';
import '@vue-flow/core/dist/theme-default.css';
import { Background } from '@vue-flow/background';
import { VueFlow } from '@vue-flow/core';
import FlowNode from '../flow/FlowNode.vue';
import { ensureLayout, tidyLayout, toFlow } from '../../utils/flowGraph.mjs';

const props = defineProps({
  definition: { type: Object, required: true },
  diff: { type: Object, default: () => ({}) },
  current: { type: Object, default: null },
});

const nodeTypes = { flow: markRaw(FlowNode) };
const flowId = `preview-${Math.random().toString(36).slice(2)}`;

const flow = computed(() => {
  const shown = JSON.parse(JSON.stringify(props.definition));
  shown.nodes ||= [];
  // Keep nodes where they are in the draft, so a change is easy to spot.
  const placed = Object.fromEntries((props.current?.nodes || []).map((n) => [n.id, n.ui]));
  shown.nodes.forEach((n) => {
    if (placed[n.id]) n.ui = placed[n.id];
  });
  if (shown.nodes.every((n) => !n.ui)) tidyLayout(shown);
  else ensureLayout(shown);
  return toFlow(shown, { diff: props.diff });
});
</script>

<style scoped>
.flow-preview {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 8px;
  height: 52vh;
  overflow: hidden;
}
</style>
