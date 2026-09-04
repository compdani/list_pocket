<template>
  <v-dialog
    :model-value="confirmState.open"
    max-width="480"
    persistent
    scrollable
    @update:model-value="onModel"
    @keydown.esc="onCancel"
  >
    <v-card>
      <v-card-title class="text-h6">
        {{ confirmState.title || $t('globals.messages.confirm') }}
      </v-card-title>
      <v-card-text>
        <p class="mb-0" style="white-space: pre-wrap;">{{ confirmState.message }}</p>
        <v-text-field
          v-if="confirmState.mode === 'prompt'"
          v-model="confirmState.inputValue"
          class="mt-4"
          :placeholder="confirmState.inputPlaceholder"
          :label="confirmState.inputPlaceholder"
          hide-details="auto"
          variant="outlined"
          density="comfortable"
          autofocus
          @keydown.enter.prevent="onConfirm"
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" @click="onCancel">
          {{ $t('globals.buttons.cancel') }}
        </v-btn>
        <v-btn color="primary" variant="flat" @click="onConfirm">
          {{ confirmState.confirmText || $t('globals.buttons.ok') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup>
import { confirmState, settleConfirm } from '../utils/confirmDialog';

function onCancel() {
  settleConfirm(confirmState.mode === 'prompt' ? null : false);
}

function onConfirm() {
  if (confirmState.mode === 'prompt') {
    settleConfirm(confirmState.inputValue);
    return;
  }
  settleConfirm(true);
}

function onModel(open) {
  if (!open && confirmState.open) {
    onCancel();
  }
}
</script>
