<template>
  <component
    :is="entry.component"
    v-for="entry in entries"
    :key="entry.key"
    v-bind="context"
  />
</template>

<script setup>
import { computed } from 'vue';
import { pluginRegistry } from './index';

const props = defineProps({
  name: { type: String, required: true },
  context: { type: Object, default: () => ({}) },
});

const entries = computed(() => pluginRegistry.slots[props.name] || []);
</script>
