<template>
  <div class="crm-fields" data-cy="contact-crm-fields">
    <v-select
      :model-value="profile.owner"
      :items="ownerItems"
      :label="$t('crm.owner')"
      :readonly="!canEdit"
      density="compact"
      variant="outlined"
      hide-details
      data-cy="contact-owner"
      @update:model-value="save({ owner: $event })"
    />
    <v-text-field
      v-model="company"
      :label="$t('crm.company')"
      :readonly="!canEdit"
      density="compact"
      variant="outlined"
      hide-details
      maxlength="200"
      data-cy="contact-company"
      @blur="saveCompany"
      @keydown.enter="saveCompany"
    />
  </div>
</template>

<script setup>
import {
  computed, getCurrentInstance, onMounted, ref, watch,
} from 'vue';
import { getContactProfile, getCRMAssignees, updateContactProfile } from '../api';

const props = defineProps({
  subscriberId: { type: String, required: true },
});
const emit = defineEmits(['updated']);

const { proxy } = getCurrentInstance();
const profile = ref({ owner: '', company: '' });
const assignees = ref([]);
const company = ref('');

const canEdit = computed(() => proxy.$can('subscribers:manage'));
const ownerItems = computed(() => [
  { title: proxy.$t('crm.owner.none'), value: '' },
  ...assignees.value.map((user) => ({ title: user.name, value: user.id })),
]);

function apply(data) {
  profile.value = data;
  company.value = data.company || '';
  emit('updated', data);
}

async function load() {
  try {
    const [data, users] = await Promise.all([getContactProfile(props.subscriberId), getCRMAssignees()]);
    assignees.value = Array.isArray(users) ? users : [];
    apply(data);
  } catch (err) {
    proxy.$utils.toast(err.message, 'error');
  }
}

async function save(change) {
  try {
    apply(await updateContactProfile(props.subscriberId, change));
  } catch (err) {
    proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
    await load();
  }
}

function saveCompany() {
  if (canEdit.value && company.value.trim() !== (profile.value.company || '')) {
    save({ company: company.value });
  }
}

watch(() => props.subscriberId, load);
onMounted(load);
</script>

<style scoped>
.crm-fields {
  display: grid;
  gap: 12px;
}
</style>
