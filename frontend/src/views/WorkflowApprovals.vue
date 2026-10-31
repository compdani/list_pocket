<template>
  <section data-cy="flow-approvals">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h5">{{ $t('workflows.approvals') }}</h1>
        <p class="text-body-2 text-medium-emphasis mb-0">{{ $t('workflows.approvalsHelp') }}</p>
      </div>
    </header>
    <v-skeleton-loader v-if="loading && !items.length" type="card@2" />
    <div v-else-if="!items.length" class="text-center text-medium-emphasis pa-12">
      <v-icon icon="mdi-check-circle-outline" size="40" class="mb-2" />
      <p class="mb-0">{{ $t('workflows.noApprovals') }}</p>
    </div>
    <approval-card v-for="item in items" :key="item.id" :approval="item" show-run-link @decided="load" />
  </section>
</template>

<script setup>
import { getCurrentInstance, onBeforeUnmount, onMounted, ref } from 'vue';
import { getFlowApprovals } from '../api';
import ApprovalCard from '../components/flow/ApprovalCard.vue';

const REFRESH_MS = 15000;

const { proxy } = getCurrentInstance();
const items = ref([]);
const loading = ref(false);
let timer = null;

async function load({ quiet = false } = {}) {
  if (!quiet) loading.value = true;
  try {
    const data = await getFlowApprovals();
    // Keep what someone's typing: a refresh only adds and removes cards, by ID.
    items.value = Array.isArray(data) ? data : [];
  } catch (err) {
    if (!quiet) proxy.$utils.toast(err.message, 'error');
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  load();
  timer = setInterval(() => load({ quiet: true }), REFRESH_MS);
});
onBeforeUnmount(() => clearInterval(timer));
</script>
