<template>
  <div class="notes" data-cy="contact-notes">
    <div v-if="canEdit" class="notes-new">
      <v-textarea
        v-model="draft"
        :label="$t('crm.notes.placeholder')"
        rows="2"
        auto-grow
        density="compact"
        variant="outlined"
        hide-details
        maxlength="20000"
        data-cy="note-input"
        @keydown.enter.meta.prevent="add"
        @keydown.enter.ctrl.prevent="add"
      />
      <div class="d-flex justify-end mt-2">
        <v-btn
          color="primary"
          variant="flat"
          size="small"
          class="text-none"
          :disabled="!draft.trim()"
          :loading="saving"
          data-cy="note-add"
          @click="add"
        >
          {{ $t('crm.notes.add') }}
        </v-btn>
      </div>
    </div>

    <p v-if="!loading && notes.length === 0" class="text-body-2 text-medium-emphasis py-6 text-center mb-0">
      {{ $t('crm.notes.empty') }}
    </p>
    <article v-for="note in notes" :key="note.id" :class="['note', { 'note--pinned': note.pinned }]" data-cy="note">
      <div class="note-head text-caption text-medium-emphasis">
        <span>{{ note.authorName || '—' }} · {{ $utils.niceDate(note.created, true) }}</span>
        <div v-if="canEdit" class="note-actions">
          <v-btn
            :aria-label="$t(note.pinned ? 'crm.notes.unpin' : 'crm.notes.pin')"
            :icon="note.pinned ? 'mdi-pin' : 'mdi-pin-outline'"
            :color="note.pinned ? 'primary' : undefined"
            variant="text"
            size="x-small"
            @click="togglePin(note)"
          />
          <v-btn
            :aria-label="$t('globals.buttons.delete')"
            icon="mdi-trash-can-outline"
            variant="text"
            size="x-small"
            @click="remove(note)"
          />
        </div>
      </div>
      <div class="note-body">{{ note.body }}</div>
    </article>
  </div>
</template>

<script setup>
import {
  computed, getCurrentInstance, onMounted, ref, watch,
} from 'vue';
import {
  createContactNote, deleteContactNote, getContactNotes, updateContactNote,
} from '../api';

const props = defineProps({
  subscriberId: { type: String, required: true },
});

const { proxy } = getCurrentInstance();
const notes = ref([]);
const draft = ref('');
const loading = ref(false);
const saving = ref(false);

const canEdit = computed(() => proxy.$can('subscribers:manage'));

function showError(err) {
  proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
}

async function load() {
  loading.value = true;
  try {
    const data = await getContactNotes(props.subscriberId);
    notes.value = Array.isArray(data) ? data : [];
  } catch (err) {
    showError(err);
  } finally {
    loading.value = false;
  }
}

async function add() {
  if (!draft.value.trim() || saving.value) {
    return;
  }
  saving.value = true;
  try {
    await createContactNote(props.subscriberId, draft.value);
    draft.value = '';
    await load();
  } catch (err) {
    showError(err);
  } finally {
    saving.value = false;
  }
}

async function togglePin(note) {
  try {
    await updateContactNote(props.subscriberId, note.id, { pinned: !note.pinned });
    await load();
  } catch (err) {
    showError(err);
  }
}

function remove(note) {
  proxy.$utils.confirm(proxy.$t('crm.notes.deleteConfirm'), async () => {
    try {
      await deleteContactNote(props.subscriberId, note.id);
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
.notes-new {
  margin-bottom: 16px;
}

.note {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 10px;
  margin-bottom: 10px;
  padding: 10px 12px;
}

.note--pinned {
  border-color: rgba(var(--v-theme-primary), 0.5);
}

.note-head {
  align-items: center;
  display: flex;
  justify-content: space-between;
  min-height: 28px;
}

.note-actions {
  display: flex;
  gap: 2px;
}

.note-body {
  font-size: 14px;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
</style>
