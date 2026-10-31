<template>
  <v-avatar :size="size" color="primary" variant="tonal" class="contact-avatar">
    <span :style="{ fontSize: `${Math.round(size * 0.38)}px` }">{{ letters }}</span>
  </v-avatar>
</template>

<script setup>
import { computed } from 'vue';
import { initials } from '../utils/crm.mjs';

const props = defineProps({
  name: { type: String, default: '' },
  fallback: { type: String, default: '' },
  size: { type: Number, default: 36 },
});

const letters = computed(() => initials(props.name, props.fallback));
</script>

<style scoped>
.contact-avatar {
  flex: none;
  font-weight: 600;
}
</style>
