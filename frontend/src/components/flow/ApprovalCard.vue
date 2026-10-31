<template>
  <article class="approval" data-cy="flow-approval">
    <div class="d-flex align-center flex-wrap ga-2 mb-1">
      <strong>{{ approval.title || $t('workflows.approvalUntitled') }}</strong>
      <v-spacer />
      <router-link v-if="showRunLink && approval.run_id" :to="{ name: 'workflowRun', params: { id: approval.run_id } }" class="text-caption">
        {{ approval.workflow_name || $t('workflows.viewRun') }}
      </router-link>
    </div>
    <p v-if="approval.instructions" class="text-body-2 mb-2 approval-text">{{ approval.instructions }}</p>
    <json-field v-model="edited" :label="$t('workflows.approvalData')" :readonly="!canDecide" :rows="4" class="mb-2" data-cy="flow-approval-data" />
    <div v-if="canDecide" class="d-flex justify-end ga-2">
      <v-btn size="small" variant="text" class="text-none" :loading="busy === 'deny'" data-cy="flow-approval-deny" @click="decide(false)">{{ $t('workflows.deny') }}</v-btn>
      <v-btn size="small" color="primary" variant="flat" class="text-none" :loading="busy === 'approve'" data-cy="flow-approval-approve" @click="decide(true)">{{ $t('workflows.approve') }}</v-btn>
    </div>
  </article>
</template>

<script setup>
import { computed, getCurrentInstance, ref } from 'vue';
import { decideFlowApproval } from '../../api';
import JsonField from './JsonField.vue';

const props = defineProps({
  approval: { type: Object, required: true },
  showRunLink: { type: Boolean, default: false },
});
const emit = defineEmits(['decided']);

const { proxy } = getCurrentInstance();
const edited = ref(props.approval.payload);
const busy = ref('');
const canDecide = computed(() => proxy.$can('workflows:approve') && props.approval.status === 'pending');

async function decide(approved) {
  busy.value = approved ? 'approve' : 'deny';
  try {
    await decideFlowApproval(props.approval.id, approved, approved ? edited.value : null);
    emit('decided');
  } catch (err) {
    proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
  } finally {
    busy.value = '';
  }
}
</script>

<style scoped>
.approval {
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 12px;
  margin-bottom: 12px;
  padding: 14px 16px;
}

.approval-text {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
</style>
