<template>
  <section class="boards-page">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h5">{{ $t('boards.title') }}</h1>
        <p class="text-body-2 text-medium-emphasis mb-0">{{ $t('boards.intro') }}</p>
      </div>
      <div v-if="canEdit" class="header-actions">
        <v-btn color="primary" variant="flat" class="text-none" prepend-icon="mdi-plus" data-cy="board-new" @click="openCreate">
          {{ $t('boards.new') }}
        </v-btn>
      </div>
    </header>

    <v-skeleton-loader v-if="loading && boards.length === 0" type="card@3" />
    <p v-else-if="boards.length === 0" class="text-body-2 text-medium-emphasis" data-cy="boards-empty">{{ $t('boards.empty') }}</p>
    <div v-else class="boards-grid">
      <router-link
        v-for="board in boards"
        :key="board.key"
        :to="{ name: 'board', params: { board: board.key } }"
        class="board-tile"
        :data-cy="`board-${board.key}`"
      >
        <div class="text-subtitle-1 font-weight-medium">{{ board.name }}</div>
        <div class="text-caption text-medium-emphasis mb-2">
          {{ $t('boards.stageCount', { n: board.stages.length }) }} · {{ $t('boards.contactCount', { n: contactCount(board) }) }}
        </div>
        <div class="d-flex flex-wrap ga-1">
          <v-chip v-for="stage in board.stages" :key="stage.key" size="x-small" label :color="stage.color">
            {{ stage.name }} {{ stage.total }}
          </v-chip>
        </div>
      </router-link>
    </div>

    <v-dialog v-model="isCreateVisible" max-width="440">
      <v-card>
        <v-card-title>{{ $t('boards.new') }}</v-card-title>
        <v-card-text>
          <v-text-field
            v-model="newName"
            :label="$t('boards.name')"
            density="comfortable"
            variant="outlined"
            hide-details
            autofocus
            maxlength="200"
            data-cy="board-new-name"
            @keydown.enter="create"
          />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="isCreateVisible = false">{{ $t('globals.buttons.cancel') }}</v-btn>
          <v-btn color="primary" variant="flat" :disabled="!newName.trim()" :loading="creating" data-cy="board-new-submit" @click="create">
            {{ $t('boards.new') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, onMounted, ref,
} from 'vue';
import { useRouter } from 'vue-router';
import { createBoard, getBoards } from '../api';

const { proxy } = getCurrentInstance();
const router = useRouter();
const boards = ref([]);
const loading = ref(false);
const isCreateVisible = ref(false);
const newName = ref('');
const creating = ref(false);

const canEdit = computed(() => proxy.$can('boards:manage'));

function contactCount(board) {
  return board.stages.reduce((sum, stage) => sum + stage.total, 0);
}

function openCreate() {
  newName.value = '';
  isCreateVisible.value = true;
}

async function create() {
  if (!newName.value.trim() || creating.value) return;
  creating.value = true;
  try {
    const board = await createBoard({ name: newName.value });
    router.push({ name: 'board', params: { board: board.key } });
  } catch (err) {
    proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
  } finally {
    creating.value = false;
  }
}

async function load() {
  loading.value = true;
  try {
    const list = await getBoards();
    boards.value = Array.isArray(list) ? list : [];
  } catch (err) {
    proxy.$utils.toast(err.message, 'error');
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>

<style scoped>
.boards-grid {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
}

.board-tile {
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 12px;
  color: inherit;
  display: block;
  padding: 14px 16px;
  text-decoration: none;
}

.board-tile:hover {
  border-color: rgb(var(--v-theme-primary));
}
</style>
