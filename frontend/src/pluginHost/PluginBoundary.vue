<template>
  <v-alert
    v-if="error"
    type="error"
    variant="tonal"
    density="comfortable"
    class="my-2"
    :title="$t('plugins.componentFailed', { name: pluginId })"
    :text="error"
  />
  <v-alert
    v-else-if="!allowed"
    type="warning"
    variant="tonal"
    density="comfortable"
    class="my-2"
    :text="$t('plugins.noPermission')"
  />
  <slot v-else />
</template>

<script setup>
import {
  computed, getCurrentInstance, onErrorCaptured, ref,
} from 'vue';

const props = defineProps({
  pluginId: { type: String, required: true },
  permission: { type: String, default: '' },
});

const error = ref('');
const { proxy } = getCurrentInstance();

const allowed = computed(() => !props.permission || proxy.$can(props.permission));

// Contains a plugin's rendering errors so the rest of the page keeps working.
onErrorCaptured((err) => {
  error.value = String((err && err.message) || err);
  // eslint-disable-next-line no-console
  console.error(`plugin ${props.pluginId}:`, err);
  return false;
});
</script>
