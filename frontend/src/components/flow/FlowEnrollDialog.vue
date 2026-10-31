<template>
  <v-dialog :model-value="modelValue" max-width="560" @update:model-value="$emit('update:modelValue', $event)">
    <v-card data-cy="flow-enroll">
      <v-card-title>{{ $t('workflows.enroll.title') }}</v-card-title>
      <v-card-text>
        <v-autocomplete
          v-if="!workflowId"
          v-model="chosenWorkflow"
          :items="workflows"
          :loading="loadingWorkflows"
          :label="$t('workflows.enroll.workflow')"
          :no-data-text="$t('workflows.enroll.noPublished')"
          item-title="name"
          item-value="id"
          density="comfortable"
          variant="outlined"
          data-cy="flow-enroll-workflow"
        />

        <p v-if="presetSubscribers.length" class="text-body-2 mb-3" data-cy="flow-enroll-count">
          {{ $t('workflows.enroll.selected', { n: presetSubscribers.length }) }}
        </p>
        <subscriber-picker
          v-else
          v-model="chosen"
          :active="modelValue"
          data-cy="flow-enroll-subscribers"
        />

        <p class="text-caption text-medium-emphasis mb-0">{{ $t('workflows.enroll.help') }}</p>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn @click="$emit('update:modelValue', false)">{{ $t('globals.buttons.cancel') }}</v-btn>
        <v-btn color="primary" variant="flat" :disabled="!canSubmit" :loading="submitting" data-cy="flow-enroll-submit" @click="submit">
          {{ $t('workflows.enroll.submit') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup>
import {
  computed, getCurrentInstance, ref, watch,
} from 'vue';
import { enrollFlowSubscribers, getFlows } from '../../api';
import SubscriberPicker from '../SubscriberPicker.vue';

const props = defineProps({
  modelValue: Boolean,
  // Set when the workflow is already known. Otherwise the dialog asks for one.
  workflowId: { type: String, default: '' },
  // Set when the subscribers are already known. Otherwise the dialog asks for them.
  presetSubscribers: { type: Array, default: () => [] },
});
const emit = defineEmits(['update:modelValue', 'enrolled']);

const { proxy } = getCurrentInstance();
const workflows = ref([]);
const loadingWorkflows = ref(false);
const chosenWorkflow = ref(null);
const chosen = ref([]);
const submitting = ref(false);

const targetWorkflow = computed(() => props.workflowId || chosenWorkflow.value);
const targets = computed(() => (props.presetSubscribers.length ? props.presetSubscribers : chosen.value));
const canSubmit = computed(() => Boolean(targetWorkflow.value) && targets.value.length > 0);
function errorText(err) {
  return (err.response && err.response.message) || err.message;
}

async function loadWorkflows() {
  loadingWorkflows.value = true;
  try {
    const list = await getFlows();
    workflows.value = (Array.isArray(list) ? list : []).filter((wf) => wf.published_version > 0);
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  } finally {
    loadingWorkflows.value = false;
  }
}

function summary(out) {
  const parts = [proxy.$t('workflows.enroll.started', { n: out.started })];
  if (out.active) parts.push(proxy.$t('workflows.enroll.skippedActive', { n: out.active }));
  if (out.blocklisted) parts.push(proxy.$t('workflows.enroll.skippedBlocklisted', { n: out.blocklisted }));
  if (out.missing) parts.push(proxy.$t('workflows.enroll.skippedMissing', { n: out.missing }));
  if (out.failed) parts.push(proxy.$t('workflows.enroll.failed', { n: out.failed, error: out.error }));
  return parts.join(' ');
}

async function submit() {
  submitting.value = true;
  try {
    const out = await enrollFlowSubscribers(targetWorkflow.value, targets.value.map((s) => String(s.id)));
    proxy.$utils.toast(summary(out), out.failed || !out.started ? 'error' : undefined);
    emit('enrolled', out);
    if (out.started) emit('update:modelValue', false);
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  } finally {
    submitting.value = false;
  }
}

watch(() => props.modelValue, (open) => {
  if (!open) return;
  chosen.value = [];
  chosenWorkflow.value = null;
  if (!props.workflowId) loadWorkflows();
});
</script>
