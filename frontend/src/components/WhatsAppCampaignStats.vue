<template>
  <div v-if="total > 0" class="d-flex flex-wrap ga-2 mb-4" data-cy="whatsapp-campaign-stats">
    <v-chip v-for="row in rows" :key="row.key" size="small" variant="outlined" :data-cy="`whatsapp-stat-${row.key}`">
      {{ $t(`whatsapp.status.${row.key}`) }}: {{ row.count }}
    </v-chip>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import { getWhatsAppCampaignStats } from '../api';

const props = defineProps({
  campaignId: { type: String, required: true },
});

const counts = ref({});

const rows = computed(() => ['sent', 'delivered', 'read', 'failed']
  .map((key) => ({ key, count: counts.value[key] || 0 })));
const total = computed(() => rows.value.reduce((sum, row) => sum + row.count, 0));

onMounted(async () => {
  try {
    counts.value = (await getWhatsAppCampaignStats(props.campaignId)) || {};
  } catch {
    // Stats are extra detail, so the campaign form works without them.
  }
});
</script>
