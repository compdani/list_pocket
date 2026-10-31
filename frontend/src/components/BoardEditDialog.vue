<template>
  <v-dialog :model-value="modelValue" max-width="620" scrollable @update:model-value="$emit('update:modelValue', $event)">
    <v-card v-if="board" data-cy="board-edit">
      <v-card-title>{{ $t('boards.edit') }}</v-card-title>
      <v-card-text>
        <v-text-field
          v-model="name"
          :label="$t('boards.name')"
          density="compact"
          variant="outlined"
          maxlength="200"
          data-cy="board-edit-name"
          @blur="saveName"
          @keydown.enter="saveName"
        />

        <h3 class="text-subtitle-2 mb-2">{{ $t('boards.stages') }}</h3>
        <div v-for="(stage, i) in board.stages" :key="stage.key" class="stage-row" :data-cy="`board-edit-stage-${stage.key}`">
          <v-text-field
            :model-value="stage.name"
            :label="$t('boards.stageName')"
            density="compact"
            variant="outlined"
            hide-details
            maxlength="200"
            data-cy="board-edit-stage-name"
            @change="saveStage(stage, { name: $event.target.value })"
          />
          <v-select
            :model-value="stage.color"
            :items="colorItems"
            :label="$t('boards.stageColor')"
            density="compact"
            variant="outlined"
            hide-details
            class="stage-color"
            @update:model-value="saveStage(stage, { color: $event })"
          >
            <template #selection="{ item }">
              <v-chip size="x-small" label :color="item.value">{{ item.title }}</v-chip>
            </template>
          </v-select>
          <v-btn :aria-label="$t('boards.stageUp', { name: stage.name })" icon="mdi-chevron-up" size="x-small" variant="text" :disabled="i === 0" :data-cy="`board-stage-up-${stage.key}`" @click="shift(stage, -1)" />
          <v-btn :aria-label="$t('boards.stageDown', { name: stage.name })" icon="mdi-chevron-down" size="x-small" variant="text" :disabled="i === board.stages.length - 1" :data-cy="`board-stage-down-${stage.key}`" @click="shift(stage, 1)" />
          <v-btn :aria-label="$t('boards.stageDelete', { name: stage.name })" icon="mdi-trash-can-outline" size="x-small" variant="text" :disabled="board.stages.length === 1" :data-cy="`board-stage-delete-${stage.key}`" @click="startDelete(stage)" />

          <div v-if="deleting === stage.key" class="stage-delete">
            <v-select
              v-if="stage.total > 0"
              v-model="moveTo"
              :items="otherStages(stage)"
              :label="$t('boards.stageDeleteMove', { n: stage.total })"
              item-title="name"
              item-value="key"
              density="compact"
              variant="outlined"
              hide-details
              data-cy="board-stage-move-to"
            />
            <span v-else class="text-body-2">{{ $t('boards.stageDeleteConfirm', { name: stage.name }) }}</span>
            <v-btn size="small" variant="text" @click="deleting = ''">{{ $t('globals.buttons.cancel') }}</v-btn>
            <v-btn size="small" variant="flat" color="error" :disabled="stage.total > 0 && !moveTo" data-cy="board-stage-delete-confirm" @click="confirmDelete(stage)">
              {{ $t('globals.buttons.delete') }}
            </v-btn>
          </div>
        </div>

        <div class="d-flex ga-2 mt-3">
          <v-text-field
            v-model="newStage"
            :label="$t('boards.stageAdd')"
            density="compact"
            variant="outlined"
            hide-details
            maxlength="200"
            data-cy="board-add-stage-name"
            @keydown.enter="addStage"
          />
          <v-btn variant="tonal" class="text-none" prepend-icon="mdi-plus" :disabled="!newStage.trim()" data-cy="board-add-stage" @click="addStage">
            {{ $t('boards.stageAdd') }}
          </v-btn>
        </div>
      </v-card-text>
      <v-card-actions>
        <v-btn color="error" variant="text" class="text-none" prepend-icon="mdi-trash-can-outline" data-cy="board-delete" @click="removeBoard">
          {{ $t('globals.buttons.delete') }}
        </v-btn>
        <v-spacer />
        <v-btn data-cy="board-edit-close" @click="$emit('update:modelValue', false)">{{ $t('globals.buttons.close') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup>
import {
  computed, getCurrentInstance, ref, watch,
} from 'vue';
import {
  createBoardStage, deleteBoard, deleteBoardStage, reorderBoardStages, updateBoard, updateBoardStage,
} from '../api';
import { STAGE_COLORS, shiftKey } from '../utils/boards.mjs';

const props = defineProps({
  modelValue: Boolean,
  board: { type: Object, default: null },
});
const emit = defineEmits(['update:modelValue', 'changed', 'deleted']);

const { proxy } = getCurrentInstance();
const name = ref('');
const newStage = ref('');
const deleting = ref('');
const moveTo = ref(null);

const colorItems = computed(() => STAGE_COLORS.map((value) => ({ value, title: proxy.$t(`boards.stageColor.${value}`) })));

function otherStages(stage) {
  return props.board.stages.filter((s) => s.key !== stage.key);
}

async function run(action) {
  try {
    emit('changed', await action());
  } catch (err) {
    proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
    name.value = props.board.name;
  }
}

function saveName() {
  const next = name.value.trim();
  if (next !== props.board.name) run(() => updateBoard(props.board.key, { name: next }));
}

function saveStage(stage, change) {
  run(() => updateBoardStage(props.board.key, stage.key, change));
}

function shift(stage, delta) {
  const keys = props.board.stages.map((s) => s.key);
  run(() => reorderBoardStages(props.board.key, shiftKey(keys, stage.key, delta)));
}

function startDelete(stage) {
  deleting.value = stage.key;
  moveTo.value = null;
}

async function confirmDelete(stage) {
  await run(() => deleteBoardStage(props.board.key, stage.key, stage.total > 0 ? moveTo.value : ''));
  deleting.value = '';
}

async function addStage() {
  const stageName = newStage.value.trim();
  if (!stageName) return;
  await run(() => createBoardStage(props.board.key, { name: stageName }));
  newStage.value = '';
}

function removeBoard() {
  proxy.$utils.confirm(proxy.$t('boards.deleteConfirm', { name: props.board.name }), async () => {
    try {
      await deleteBoard(props.board.key);
      emit('deleted');
    } catch (err) {
      proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
    }
  });
}

watch(() => props.modelValue, (open) => {
  if (!open) return;
  name.value = props.board?.name || '';
  newStage.value = '';
  deleting.value = '';
}, { immediate: true });
</script>

<style scoped>
.stage-row {
  align-items: center;
  display: grid;
  gap: 4px 6px;
  grid-template-columns: minmax(0, 1fr) 130px auto auto auto;
  margin-bottom: 10px;
}

.stage-delete {
  align-items: center;
  display: flex;
  gap: 8px;
  grid-column: 1 / -1;
}

@media (max-width: 560px) {
  .stage-row {
    grid-template-columns: minmax(0, 1fr) auto auto auto;
  }

  .stage-row > :first-child,
  .stage-color {
    grid-column: 1 / -1;
  }
}
</style>
