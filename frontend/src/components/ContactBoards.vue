<template>
  <div data-cy="contact-boards">
    <div class="d-flex align-center mb-1">
      <span class="text-caption text-medium-emphasis">{{ $t('boards.contact.title') }}</span>
      <v-spacer />
      <v-btn
        v-if="canEdit"
        :aria-label="$t('boards.add.title')"
        icon="mdi-plus"
        size="x-small"
        variant="text"
        data-cy="contact-add-to-board"
        @click="isAddVisible = true"
      />
    </div>
    <p v-if="places.length === 0" class="text-caption text-medium-emphasis mb-0">{{ $t('boards.contact.none') }}</p>
    <div v-for="place in places" :key="place.board" class="d-flex align-center ga-1 mb-2">
      <v-select
        :model-value="place.stage"
        :items="stagesOf(place.board)"
        :label="place.boardName"
        :readonly="!canEdit"
        item-title="name"
        item-value="key"
        density="compact"
        variant="outlined"
        hide-details
        :data-cy="`contact-board-${place.board}`"
        @update:model-value="move(place.board, $event)"
      />
      <v-btn
        v-if="canEdit"
        :aria-label="$t('boards.contact.remove', { name: place.boardName })"
        icon="mdi-close"
        size="x-small"
        variant="text"
        :data-cy="`contact-board-remove-${place.board}`"
        @click="remove(place.board)"
      />
    </div>
    <add-to-board-dialog
      v-if="canEdit"
      v-model="isAddVisible"
      :preset-subscribers="[{ id: subscriberId }]"
      @added="load"
    />
  </div>
</template>

<script setup>
import {
  computed, getCurrentInstance, onMounted, ref, watch,
} from 'vue';
import {
  getBoards, getContactBoards, moveBoardCard, removeBoardCard,
} from '../api';
import AddToBoardDialog from './AddToBoardDialog.vue';

const props = defineProps({
  subscriberId: { type: String, required: true },
});
const emit = defineEmits(['updated']);

const { proxy } = getCurrentInstance();
const places = ref([]);
const boards = ref([]);
const isAddVisible = ref(false);

const canEdit = computed(() => proxy.$can('boards:manage'));

function showError(err) {
  proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
}

function stagesOf(key) {
  return boards.value.find((b) => b.key === key)?.stages || [];
}

async function load() {
  try {
    const [mine, all] = await Promise.all([getContactBoards(props.subscriberId), getBoards()]);
    places.value = Array.isArray(mine) ? mine : [];
    boards.value = Array.isArray(all) ? all : [];
    emit('updated', places.value);
  } catch (err) {
    showError(err);
  }
}

async function move(board, stage) {
  try {
    await moveBoardCard(board, props.subscriberId, { stage });
  } catch (err) {
    showError(err);
  }
  await load();
}

async function remove(board) {
  try {
    await removeBoardCard(board, props.subscriberId);
  } catch (err) {
    showError(err);
  }
  await load();
}

watch(() => props.subscriberId, load);
onMounted(load);
</script>
