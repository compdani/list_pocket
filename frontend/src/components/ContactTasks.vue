<template>
  <div data-cy="contact-tasks">
    <div v-if="canEdit" class="tasks-new">
      <v-text-field
        v-model="title"
        :label="$t('crm.tasks.placeholder')"
        density="compact"
        variant="outlined"
        hide-details
        maxlength="500"
        class="tasks-new-title"
        data-cy="task-input"
        @keydown.enter="add"
      />
      <v-text-field
        v-model="due"
        :label="$t('crm.tasks.due')"
        type="datetime-local"
        density="compact"
        variant="outlined"
        hide-details
        class="tasks-new-due"
        data-cy="task-due"
      />
      <v-btn
        color="primary"
        variant="flat"
        class="text-none"
        :disabled="!title.trim()"
        :loading="saving"
        data-cy="task-add"
        @click="add"
      >
        {{ $t('crm.tasks.add') }}
      </v-btn>
    </div>

    <p v-if="!loading && tasks.length === 0" class="text-body-2 text-medium-emphasis py-6 text-center mb-0">
      {{ $t('crm.tasks.empty') }}
    </p>
    <contact-task-row
      v-for="task in tasks"
      :key="task.id"
      :task="task"
      @toggle="toggle"
      @remove="remove"
    />
  </div>
</template>

<script setup>
import {
  computed, getCurrentInstance, onMounted, ref, watch,
} from 'vue';
import {
  createContactTask, deleteContactTask, getContactTasks, updateContactTask,
} from '../api';
import ContactTaskRow from './ContactTaskRow.vue';
import { localInputToRFC3339 } from '../utils/crm.mjs';

const props = defineProps({
  subscriberId: { type: String, required: true },
});

const { proxy } = getCurrentInstance();
const tasks = ref([]);
const title = ref('');
const due = ref('');
const loading = ref(false);
const saving = ref(false);

const canEdit = computed(() => proxy.$can('subscribers:manage'));

function showError(err) {
  proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
}

async function load() {
  loading.value = true;
  try {
    const data = await getContactTasks(props.subscriberId);
    tasks.value = Array.isArray(data) ? data : [];
  } catch (err) {
    showError(err);
  } finally {
    loading.value = false;
  }
}

async function add() {
  if (!title.value.trim() || saving.value) {
    return;
  }
  saving.value = true;
  try {
    await createContactTask(props.subscriberId, { title: title.value, due_at: localInputToRFC3339(due.value) });
    title.value = '';
    due.value = '';
    await load();
  } catch (err) {
    showError(err);
  } finally {
    saving.value = false;
  }
}

async function toggle(task) {
  try {
    await updateContactTask(props.subscriberId, task.id, { status: task.status === 'done' ? 'open' : 'done' });
    await load();
  } catch (err) {
    showError(err);
  }
}

function remove(task) {
  proxy.$utils.confirm(proxy.$t('crm.tasks.deleteConfirm'), async () => {
    try {
      await deleteContactTask(props.subscriberId, task.id);
      await load();
    } catch (err) {
      showError(err);
    }
  });
}

watch(() => props.subscriberId, load);
onMounted(load);
</script>

<style scoped>
.tasks-new {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}

.tasks-new-title {
  flex: 2 1 220px;
}

.tasks-new-due {
  flex: 1 1 190px;
}
</style>
