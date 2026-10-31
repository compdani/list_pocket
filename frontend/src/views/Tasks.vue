<template>
  <section class="tasks-page">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h5">{{ $t('crm.tasks.title') }}</h1>
        <p class="text-body-2 text-medium-emphasis mb-0">{{ $t('crm.tasks.hint') }}</p>
      </div>
    </header>

    <div class="tasks-filters">
      <v-btn-toggle v-model="who" mandatory density="compact" variant="outlined" divided color="primary">
        <v-btn value="me" class="text-none" data-cy="tasks-mine">{{ $t('crm.tasks.mine') }}</v-btn>
        <v-btn value="" class="text-none" data-cy="tasks-everyone">{{ $t('crm.tasks.everyone') }}</v-btn>
      </v-btn-toggle>
      <v-btn-toggle v-model="status" mandatory density="compact" variant="outlined" divided color="primary">
        <v-btn value="open" class="text-none">{{ $t('crm.tasks.open') }}</v-btn>
        <v-btn value="done" class="text-none">{{ $t('crm.tasks.done') }}</v-btn>
      </v-btn-toggle>
    </div>

    <v-skeleton-loader v-if="loading && tasks.length === 0" type="list-item@6" />
    <div v-else-if="tasks.length === 0" class="tasks-empty text-medium-emphasis">
      <v-icon icon="mdi-check-circle-outline" size="40" class="mb-2" />
      <p class="mb-0">{{ $t(status === 'done' ? 'crm.tasks.noneDone' : 'crm.tasks.noneOpen') }}</p>
    </div>

    <template v-else-if="status === 'open'">
      <section v-for="group in groups" :key="group.key" class="tasks-group" :data-cy="`tasks-${group.key}`">
        <h2 :class="['tasks-group-title', { 'text-error': group.key === 'overdue' }]">
          {{ $t(group.label) }} <span class="text-medium-emphasis">({{ group.tasks.length }})</span>
        </h2>
        <contact-task-row
          v-for="task in group.tasks"
          :key="task.id"
          :task="task"
          show-contact
          @toggle="toggle"
          @remove="remove"
        />
      </section>
    </template>
    <section v-else class="tasks-group">
      <contact-task-row
        v-for="task in tasks"
        :key="task.id"
        :task="task"
        show-contact
        @toggle="toggle"
        @remove="remove"
      />
    </section>
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, onMounted, ref, watch,
} from 'vue';
import { deleteContactTask, getAllContactTasks, updateContactTask } from '../api';
import ContactTaskRow from '../components/ContactTaskRow.vue';
import { groupTasks } from '../utils/crm.mjs';

const { proxy } = getCurrentInstance();
const tasks = ref([]);
const loading = ref(false);
const who = ref('me');
const status = ref('open');

const groups = computed(() => {
  const grouped = groupTasks(tasks.value);
  return [
    { key: 'overdue', label: 'crm.tasks.overdue', tasks: grouped.overdue },
    { key: 'today', label: 'crm.tasks.today', tasks: grouped.today },
    { key: 'upcoming', label: 'crm.tasks.upcoming', tasks: grouped.upcoming },
    { key: 'noDate', label: 'crm.tasks.noDate', tasks: grouped.noDate },
  ].filter((group) => group.tasks.length > 0);
});

function showError(err) {
  proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
}

async function load() {
  loading.value = true;
  try {
    const params = { status: status.value };
    if (who.value) {
      params.assignee = who.value;
    }
    const data = await getAllContactTasks(params);
    tasks.value = Array.isArray(data) ? data : [];
  } catch (err) {
    showError(err);
  } finally {
    loading.value = false;
  }
}

async function toggle(task) {
  try {
    await updateContactTask(task.subscriber, task.id, { status: task.status === 'done' ? 'open' : 'done' });
    await load();
  } catch (err) {
    showError(err);
  }
}

function remove(task) {
  proxy.$utils.confirm(proxy.$t('crm.tasks.deleteConfirm'), async () => {
    try {
      await deleteContactTask(task.subscriber, task.id);
      await load();
    } catch (err) {
      showError(err);
    }
  });
}

watch([who, status], load);
onMounted(load);
</script>

<style scoped>
.tasks-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 16px;
}

.tasks-group {
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 12px;
  margin-bottom: 16px;
  padding: 12px 16px;
}

.tasks-group-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 4px;
}

.tasks-empty {
  align-items: center;
  display: flex;
  flex-direction: column;
  padding: 64px 16px;
}
</style>
