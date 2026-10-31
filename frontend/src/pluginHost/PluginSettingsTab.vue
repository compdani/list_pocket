<template>
  <div class="settings-section">
    <v-progress-linear v-if="isLoading" indeterminate color="primary" class="mb-4" />
    <schema-form
      v-if="!isLoading"
      v-model="values"
      :fields="fields"
      :secrets="secrets"
      :disabled="isSaving"
    />
    <v-btn
      color="primary"
      :loading="isSaving"
      :disabled="isLoading"
      :data-cy="`plugin-settings-save-${pluginId}`"
      @click="save"
    >
      {{ $t('globals.buttons.save') }}
    </v-btn>
  </div>
</template>

<script setup>
import { getCurrentInstance, onMounted, ref } from 'vue';
import { getPluginSettings, updatePluginSettings } from '../api';
import SchemaForm from './SchemaForm.vue';

const props = defineProps({
  pluginId: { type: String, required: true },
  fields: { type: Array, default: () => [] },
});

const { proxy } = getCurrentInstance();
const values = ref({});
const secrets = ref({});
const isLoading = ref(true);
const isSaving = ref(false);

async function load() {
  isLoading.value = true;
  try {
    const data = await getPluginSettings(props.pluginId);
    values.value = data.values || {};
    secrets.value = data.secrets || {};
  } catch (err) {
    proxy.$utils.toast(err.message, 'error');
  } finally {
    isLoading.value = false;
  }
}

async function save() {
  isSaving.value = true;
  try {
    await updatePluginSettings(props.pluginId, values.value);
    proxy.$utils.toast(proxy.$t('globals.messages.saved', { name: props.pluginId }));
    await load();
  } catch (err) {
    proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
  } finally {
    isSaving.value = false;
  }
}

onMounted(load);
</script>
