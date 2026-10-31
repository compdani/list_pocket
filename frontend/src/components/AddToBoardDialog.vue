<template>
  <v-dialog :model-value="modelValue" max-width="560" @update:model-value="$emit('update:modelValue', $event)">
    <v-card data-cy="add-to-board">
      <v-card-title>{{ $t('boards.add.title') }}</v-card-title>
      <v-card-text>
        <v-select
          v-if="!presetBoard"
          v-model="chosenBoard"
          :items="boards"
          :loading="loading"
          :label="$t('boards.add.board')"
          :no-data-text="$t('boards.add.noBoards')"
          item-title="name"
          item-value="key"
          density="comfortable"
          variant="outlined"
          data-cy="add-to-board-board"
        />
        <v-select
          v-model="chosenStage"
          :items="stages"
          :label="$t('boards.stage')"
          :disabled="stages.length === 0"
          item-title="name"
          item-value="key"
          density="comfortable"
          variant="outlined"
          data-cy="add-to-board-stage"
        />
        <p v-if="presetSubscribers.length" class="text-body-2 mb-0" data-cy="add-to-board-count">
          {{ $t('boards.add.selected', { n: presetSubscribers.length }) }}
        </p>
        <subscriber-picker v-else v-model="chosen" :active="modelValue" allow-blocklisted data-cy="add-to-board-subscribers" />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn @click="$emit('update:modelValue', false)">{{ $t('globals.buttons.cancel') }}</v-btn>
        <v-btn color="primary" variant="flat" :disabled="!canSubmit" :loading="submitting" data-cy="add-to-board-submit" @click="submit">
          {{ $t('boards.add.title') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup>
import {
  computed, getCurrentInstance, ref, watch,
} from 'vue';
import { addBoardCards, getBoards } from '../api';
import SubscriberPicker from './SubscriberPicker.vue';

const props = defineProps({
  modelValue: Boolean,
  // Set when the board is already known. Otherwise the dialog asks for one.
  presetBoard: { type: Object, default: null },
  // Set when the subscribers are already known. Otherwise the dialog asks for them.
  presetSubscribers: { type: Array, default: () => [] },
});
const emit = defineEmits(['update:modelValue', 'added']);

const { proxy } = getCurrentInstance();
const boards = ref([]);
const loading = ref(false);
const chosenBoard = ref(null);
const chosenStage = ref(null);
const chosen = ref([]);
const submitting = ref(false);

const board = computed(() => props.presetBoard || boards.value.find((b) => b.key === chosenBoard.value));
const stages = computed(() => board.value?.stages || []);
const targets = computed(() => (props.presetSubscribers.length ? props.presetSubscribers : chosen.value));
const canSubmit = computed(() => Boolean(board.value && chosenStage.value) && targets.value.length > 0);

function errorText(err) {
  return (err.response && err.response.message) || err.message;
}

async function loadBoards() {
  loading.value = true;
  try {
    const list = await getBoards();
    boards.value = Array.isArray(list) ? list : [];
    if (boards.value.length === 1) chosenBoard.value = boards.value[0].key;
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  } finally {
    loading.value = false;
  }
}

async function submit() {
  submitting.value = true;
  try {
    await addBoardCards(board.value.key, targets.value.map((s) => String(s.id)), chosenStage.value);
    proxy.$utils.toast(proxy.$t('boards.add.done', { name: board.value.name }));
    emit('added');
    emit('update:modelValue', false);
  } catch (err) {
    proxy.$utils.toast(errorText(err), 'error');
  } finally {
    submitting.value = false;
  }
}

// New contacts usually start at the beginning, so the first stage is chosen for them.
watch(stages, (list) => {
  chosenStage.value = list[0]?.key || null;
}, { immediate: true });

watch(() => props.modelValue, (open) => {
  if (!open) return;
  chosen.value = [];
  chosenBoard.value = null;
  chosenStage.value = stages.value[0]?.key || null;
  if (!props.presetBoard) loadBoards();
});
</script>
