<template>
  <!-- Nothing until /api/config has answered: choosing the host after a flash
       of the wrong page would lose the draft and the conversation. -->
  <component :is="host" v-if="host" />
</template>

<script setup>
import { computed, defineAsyncComponent } from 'vue';
import { useStore } from 'vuex';
import { backendOf } from '../assistant/chat-host.mjs';

// Both hosts load their own code; the module host pulls in the vendored chat
// client and is only fetched when the flag is on.
const Agent = defineAsyncComponent(() => import('./Agent.vue'));
const AgentChat = defineAsyncComponent(() => import('./AgentChat.vue'));

const store = useStore();
const backend = computed(() => backendOf(store.state.serverConfig));
const host = computed(() => {
  if (backend.value === 'module') return AgentChat;
  if (backend.value === 'legacy') return Agent;
  return null;
});
</script>
