<template>
  <v-autocomplete
    :model-value="modelValue"
    v-model:search="search"
    :items="options"
    :loading="searching"
    :label="$t('globals.terms.subscribers')"
    :no-data-text="$t('workflows.enroll.noMatches')"
    :item-title="subscriberLabel"
    :item-props="subscriberProps"
    item-value="id"
    multiple
    chips
    closable-chips
    clear-on-select
    no-filter
    return-object
    density="comfortable"
    variant="outlined"
    @update:model-value="$emit('update:modelValue', $event)"
  />
</template>

<script setup>
import {
  computed, getCurrentInstance, onBeforeUnmount, ref, watch,
} from 'vue';
import { searchSubscribers } from '../api';
import { visibleEmail } from '../utils/crm.mjs';

const SEARCH_DELAY_MS = 250;
const SEARCH_LIMIT = 20;

const props = defineProps({
  modelValue: { type: Array, default: () => [] },
  // Searching starts when this turns true, such as when a dialog opens.
  active: { type: Boolean, default: true },
  allowBlocklisted: { type: Boolean, default: false },
});
defineEmits(['update:modelValue']);

const { proxy } = getCurrentInstance();
const results = ref([]);
const search = ref('');
const searching = ref(false);
let searchTimer = null;
let searchSeq = 0;

// What's chosen stays listed, or its chip would lose its label when a new search runs.
const options = computed(() => {
  const ids = new Set(props.modelValue.map((s) => s.id));
  return [...props.modelValue, ...results.value.filter((s) => !ids.has(s.id))];
});

function subscriberLabel(sub) {
  return sub.name || visibleEmail(sub.email) || sub.phone || sub.id;
}

function subscriberProps(sub) {
  const blocked = !props.allowBlocklisted && sub.status === 'blocklisted';
  return {
    subtitle: blocked ? proxy.$t('workflows.enroll.blocklistedOption') : visibleEmail(sub.email) || sub.phone,
    disabled: blocked,
  };
}

async function runSearch(term) {
  searchSeq += 1;
  const seq = searchSeq;
  searching.value = true;
  try {
    const out = await searchSubscribers({ search: term, page: 1, per_page: SEARCH_LIMIT });
    if (seq === searchSeq) results.value = out.results || [];
  } catch (err) {
    if (seq === searchSeq) proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
  } finally {
    if (seq === searchSeq) searching.value = false;
  }
}

watch(() => props.active, (active) => {
  if (!active) return;
  search.value = '';
  runSearch('');
}, { immediate: true });

watch(search, (term) => {
  if (!props.active) return;
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => runSearch((term || '').trim()), SEARCH_DELAY_MS);
});

onBeforeUnmount(() => clearTimeout(searchTimer));
</script>
