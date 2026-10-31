<template>
  <section class="board-page">
    <v-btn :to="{ name: 'boards' }" variant="text" size="small" class="text-none mb-1" prepend-icon="mdi-arrow-left" exact>
      {{ $t('boards.back') }}
    </v-btn>
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h5" data-cy="board-title">{{ board?.name }}</h1>
        <p class="text-body-2 text-medium-emphasis mb-0">
          {{ $t(isEmpty ? 'boards.howTo' : 'boards.hint') }}
        </p>
      </div>
      <div v-if="canEdit && board" class="header-actions d-flex ga-2">
        <v-btn variant="outlined" class="text-none" prepend-icon="mdi-cog-outline" data-cy="board-edit-open" @click="isEditVisible = true">
          {{ $t('boards.edit') }}
        </v-btn>
        <v-btn color="primary" variant="flat" class="text-none" prepend-icon="mdi-plus" data-cy="board-add-contacts" @click="isAddVisible = true">
          {{ $t('boards.addContacts') }}
        </v-btn>
      </div>
    </header>

    <v-skeleton-loader v-if="loading && !board" type="card@3" />
    <div v-else-if="board" class="board-columns">
      <section
        v-for="stage in board.stages"
        :key="stage.key"
        :class="['board-col', { 'board-col--over': over.stage === stage.key }]"
        :data-cy="`board-stage-${stage.key}`"
        @dragover.prevent="onDragOver(stage, $event)"
        @dragleave="onDragLeave(stage, $event)"
        @drop.prevent="onDrop(stage)"
      >
        <header class="board-col-head">
          <v-chip size="small" label :color="stage.color">{{ stage.name }}</v-chip>
          <span class="text-caption text-medium-emphasis">{{ stage.total }}</span>
        </header>
        <div class="board-cards">
          <p v-if="stage.cards.length === 0" class="text-caption text-medium-emphasis text-center py-4 mb-0">
            {{ $t('boards.card.empty') }}
          </p>
          <template v-for="(card, i) in stage.cards" :key="card.subscriber">
            <div v-if="showMarker(stage, i)" class="board-drop-marker" />
            <article
              :class="['board-card', { 'board-card--dragging': dragging?.subscriber === card.subscriber }]"
              :draggable="canEdit"
              :data-subscriber="card.subscriber"
              data-cy="board-card"
              @dragstart="onDragStart(card, $event)"
              @dragend="onDragEnd"
            >
              <contact-avatar :name="card.name" :fallback="card.email" :size="30" />
              <div class="board-card-main">
                <router-link :to="{ name: 'subscriber', params: { id: card.subscriber } }" class="board-card-name">
                  {{ card.name || emailOf(card) || card.subscriber }}
                </router-link>
                <div v-if="card.company" class="text-caption text-medium-emphasis text-truncate">{{ card.company }}</div>
                <div class="board-card-meta text-caption text-medium-emphasis">
                  <span v-if="card.ownerName" class="text-truncate">
                    <v-icon icon="mdi-account-tie-outline" size="13" /> {{ card.ownerName }}
                  </span>
                  <span v-if="card.openTasks > 0">
                    <v-icon icon="mdi-clipboard-text-outline" size="13" /> {{ card.openTasks }}
                  </span>
                </div>
              </div>
              <!-- Dragging needs a mouse, so this is how keyboards and touch screens move a card. -->
              <v-menu v-if="canEdit">
                <template #activator="{ props: menuProps }">
                  <v-btn
                    v-bind="menuProps"
                    :aria-label="$t('boards.card.moveMenu', { name: card.name || emailOf(card) })"
                    icon="mdi-dots-vertical"
                    variant="text"
                    size="x-small"
                    data-cy="board-card-menu"
                  />
                </template>
                <v-list density="compact">
                  <v-list-item v-if="i > 0" :title="$t('boards.card.up')" data-cy="board-card-up" @click="move(card, stage.key, i - 1)" />
                  <v-list-item v-if="i < stage.cards.length - 1" :title="$t('boards.card.down')" data-cy="board-card-down" @click="move(card, stage.key, i + 1)" />
                  <v-list-subheader v-if="board.stages.length > 1">{{ $t('boards.card.moveTo') }}</v-list-subheader>
                  <v-list-item
                    v-for="other in stagesExcept(stage)"
                    :key="other.key"
                    :title="other.name"
                    :data-cy="`board-card-move-${other.key}`"
                    @click="move(card, other.key, 0)"
                  />
                  <v-divider />
                  <v-list-item :title="$t('boards.card.remove')" data-cy="board-card-remove" @click="remove(card)" />
                </v-list>
              </v-menu>
            </article>
          </template>
          <div v-if="showMarker(stage, stage.cards.length)" class="board-drop-marker" />
          <p v-if="stage.total > stage.cards.length" class="text-caption text-medium-emphasis text-center mt-2 mb-0">
            {{ $t('boards.card.showing', { shown: stage.cards.length, total: stage.total }) }}
          </p>
        </div>
      </section>
    </div>

    <template v-if="canEdit && board">
      <add-to-board-dialog v-model="isAddVisible" :preset-board="board" @added="load" />
      <board-edit-dialog v-model="isEditVisible" :board="board" @changed="onChanged" @deleted="onDeleted" />
    </template>
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, onMounted, ref, watch,
} from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  getBoard, moveBoardCard, removeBoardCard,
} from '../api';
import AddToBoardDialog from '../components/AddToBoardDialog.vue';
import BoardEditDialog from '../components/BoardEditDialog.vue';
import ContactAvatar from '../components/ContactAvatar.vue';
import { dropIndex, moveCard } from '../utils/boards.mjs';
import { visibleEmail } from '../utils/crm.mjs';

const { proxy } = getCurrentInstance();
const route = useRoute();
const router = useRouter();
const board = ref(null);
const loading = ref(false);
const dragging = ref(null);
const over = ref({ stage: '', index: 0 });
const isAddVisible = ref(false);
const isEditVisible = ref(false);

const canEdit = computed(() => proxy.$can('boards:manage'));
const isEmpty = computed(() => Boolean(board.value) && board.value.stages.every((stage) => stage.total === 0));

function emailOf(card) {
  return visibleEmail(card.email);
}

function stagesExcept(stage) {
  return board.value.stages.filter((s) => s.key !== stage.key);
}

function errorText(err) {
  return (err.response && err.response.message) || err.message;
}

async function load() {
  loading.value = true;
  try {
    board.value = await getBoard(route.params.board);
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  } finally {
    loading.value = false;
  }
}

// The server sends the whole board after an edit. Its cards come with it.
function onChanged(next) {
  board.value = next;
}

function onDeleted() {
  isEditVisible.value = false;
  router.push({ name: 'boards' });
}

async function move(card, stageKey, position) {
  board.value = { ...board.value, stages: moveCard(board.value.stages, card.subscriber, stageKey, position) };
  try {
    await moveBoardCard(board.value.key, card.subscriber, { stage: stageKey, position });
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
    await load();
  }
}

async function remove(card) {
  board.value = { ...board.value, stages: moveCard(board.value.stages, card.subscriber, null) };
  try {
    await removeBoardCard(board.value.key, card.subscriber);
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
    await load();
  }
}

function onDragStart(card, event) {
  dragging.value = card;
  event.dataTransfer.effectAllowed = 'move';
  event.dataTransfer.setData('text/plain', card.subscriber);
}

function onDragEnd() {
  dragging.value = null;
  over.value = { stage: '', index: 0 };
}

// The index doesn't count the dragged card, which is how the server counts positions.
function onDragOver(stage, event) {
  if (!dragging.value) return;
  const midpoints = [...event.currentTarget.querySelectorAll('[data-subscriber]')]
    .filter((el) => el.dataset.subscriber !== dragging.value.subscriber)
    .map((el) => {
      const rect = el.getBoundingClientRect();
      return rect.top + rect.height / 2;
    });
  over.value = { stage: stage.key, index: dropIndex(midpoints, event.clientY) };
}

function onDragLeave(stage, event) {
  if (over.value.stage === stage.key && !event.currentTarget.contains(event.relatedTarget)) {
    over.value = { stage: '', index: 0 };
  }
}

function onDrop(stage) {
  const card = dragging.value;
  const { index } = over.value;
  onDragEnd();
  if (!card) return;
  const inStage = stage.cards.findIndex((c) => c.subscriber === card.subscriber);
  if (inStage !== index) move(card, stage.key, index);
}

// Shows where the card will land, given a slot among all the cards drawn in this stage.
function showMarker(stage, slot) {
  if (!dragging.value || over.value.stage !== stage.key) return false;
  const inStage = stage.cards.findIndex((c) => c.subscriber === dragging.value.subscriber);
  const target = inStage !== -1 && over.value.index >= inStage ? over.value.index + 1 : over.value.index;
  return slot === target;
}

watch(() => route.params.board, (key) => {
  if (key) load();
});
onMounted(load);
</script>

<style scoped>
.board-columns {
  display: grid;
  gap: 12px;
  grid-auto-columns: minmax(240px, 1fr);
  grid-auto-flow: column;
  overflow-x: auto;
  padding-bottom: 8px;
}

.board-col {
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 240px);
  min-height: 320px;
}

.board-col--over {
  border-color: rgb(var(--v-theme-primary));
  box-shadow: inset 0 0 0 1px rgb(var(--v-theme-primary));
}

.board-col-head {
  align-items: center;
  display: flex;
  flex: none;
  justify-content: space-between;
  padding: 10px 12px;
}

.board-cards {
  flex: 1;
  overflow-y: auto;
  padding: 0 8px 8px;
}

.board-card {
  align-items: flex-start;
  background: rgb(var(--v-theme-surface-light));
  border-radius: 8px;
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
  padding: 8px;
}

.board-card[draggable='true'] {
  cursor: grab;
}

.board-card--dragging {
  opacity: 0.45;
}

.board-drop-marker {
  background: rgb(var(--v-theme-primary));
  border-radius: 2px;
  height: 3px;
  margin-bottom: 8px;
}

.board-card-main {
  flex: 1;
  min-width: 0;
}

.board-card-name {
  color: inherit;
  display: block;
  font-size: 14px;
  font-weight: 500;
  overflow: hidden;
  text-decoration: none;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.board-card-name:hover {
  text-decoration: underline;
}

.board-card-meta {
  display: flex;
  gap: 10px;
}
</style>
