<template>
  <v-dialog :model-value="true" max-width="820" scrollable @update:model-value="$emit('close')">
    <v-card data-cy="agent-context-preview">
      <v-card-title class="d-flex align-center ga-2">
        <v-icon :icon="icon" size="18" />
        <span class="text-truncate">{{ title }}</span>
        <v-spacer />
        <v-btn :aria-label="$t('globals.buttons.close')" icon="mdi-close" size="small" variant="text" @click="$emit('close')" />
      </v-card-title>
      <v-card-subtitle>{{ $t('agent.previewHelp') }}</v-card-subtitle>
      <v-card-text>
        <v-progress-linear v-if="loading" indeterminate color="primary" />
        <v-alert v-else-if="error" type="error" variant="tonal" density="compact">{{ error }}</v-alert>
        <template v-else-if="data">
          <!-- An img can't run anything an image holds, including SVG scripts. -->
          <img v-if="isImage" :src="dataUrl" :alt="title" class="preview-img" data-cy="agent-preview-image" />
          <object v-else-if="isPdf" :data="dataUrl" type="application/pdf" class="preview-pdf" :aria-label="title">
            <a :href="dataUrl" :download="title">{{ $t('agent.downloadFile', { name: title }) }}</a>
          </object>
          <pre v-else class="preview-text" data-cy="agent-preview-text">{{ text }}</pre>
          <p v-if="data.size" class="text-caption text-medium-emphasis mt-2 mb-0">{{ formatBytes(data.size) }}</p>
        </template>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import { agentPost } from '../../agent/client';
import { CONTEXT_ICONS, formatBytes } from '../../agent/parts.mjs';

const props = defineProps({
  item: { type: Object, required: true },
});
defineEmits(['close']);

const data = ref(null);
const loading = ref(true);
const error = ref('');

const icon = computed(() => CONTEXT_ICONS[props.item.type] || 'mdi-paperclip');
const title = computed(() => data.value?.title || props.item.label || props.item.id);
const isImage = computed(() => data.value?.binary && String(data.value.mime).startsWith('image/'));
const isPdf = computed(() => data.value?.binary && data.value.mime === 'application/pdf');
const dataUrl = computed(() => `data:${data.value.mime};base64,${data.value.content}`);
const text = computed(() => (data.value?.data !== undefined ? JSON.stringify(data.value.data, null, 2) : data.value?.content));

onMounted(async () => {
  try {
    data.value = await agentPost('agent/preview', { type: props.item.type, id: props.item.id, label: props.item.label });
  } catch (err) {
    error.value = err.message;
  } finally {
    loading.value = false;
  }
});
</script>

<style scoped>
.preview-img {
  display: block;
  max-height: 60vh;
  max-width: 100%;
}

.preview-pdf {
  height: 60vh;
  width: 100%;
}

.preview-text {
  background: rgb(var(--v-theme-surface-light));
  border-radius: 8px;
  font-size: 12px;
  margin: 0;
  max-height: 60vh;
  overflow: auto;
  padding: 10px 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
