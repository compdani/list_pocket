<template>
  <!-- Nothing until /api/config has answered: mounting the module host, then
       swapping it for the legacy panel, would drop the open conversation. -->
  <component :is="host" v-if="host" />
</template>

<script setup>
import { computed, defineAsyncComponent } from 'vue';
import { useStore } from 'vuex';
import { backendOf } from '../assistant/chat-host.mjs';

// The module host pulls in the installed chat client and is only fetched when
// the flag is on. The legacy panel stays the default for every other value.
const LegacyAssistant = defineAsyncComponent(() => import('./AIAssistant.vue'));
const ModuleAssistant = defineAsyncComponent(() => import('./AIAssistantChat.vue'));

const store = useStore();
const backend = computed(() => backendOf(store.state.serverConfig));
const host = computed(() => {
  if (backend.value === 'module') return ModuleAssistant;
  if (backend.value === 'legacy') return LegacyAssistant;
  return null;
});
</script>
