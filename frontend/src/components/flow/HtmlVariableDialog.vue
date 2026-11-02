<template>
  <v-dialog :model-value="true" fullscreen persistent :retain-focus="false" transition="dialog-bottom-transition">
    <v-card class="html-dialog" data-cy="flow-html-dialog">
      <header class="html-dialog-head">
        <v-btn icon="mdi-close" size="small" variant="text" :aria-label="$t('globals.buttons.close')" data-cy="flow-html-close" @click="close" />
        <strong class="html-dialog-title">{{ name }}</strong>
        <v-spacer />
        <v-btn v-if="!readonly" color="primary" variant="flat" class="text-none" data-cy="flow-html-done" @click="$emit('save', draft)">
          {{ $t('workflows.email.done') }}
        </v-btn>
      </header>
      <div class="html-dialog-body">
        <richtext-editor v-model="draft" :disabled="readonly" preserve-go-template fill />
      </div>
    </v-card>
  </v-dialog>
</template>

<script setup>
import { getCurrentInstance, ref } from 'vue';
import RichtextEditor from '../RichtextEditor.vue';

const props = defineProps({
  name: { type: String, default: '' },
  modelValue: { type: String, default: '' },
  readonly: { type: Boolean, default: false },
});
const emit = defineEmits(['close', 'save']);

const { proxy } = getCurrentInstance();
const draft = ref(props.modelValue);

function close() {
  if (props.readonly || draft.value === props.modelValue) {
    emit('close');
    return;
  }
  proxy.$utils.confirm(proxy.$t('workflows.set.discardHtml'), () => emit('close'));
}
</script>

<style scoped>
.html-dialog {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.html-dialog-head {
  align-items: center;
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  display: flex;
  flex: none;
  gap: 8px;
  padding: 8px 16px 8px 8px;
}

.html-dialog-title {
  font-size: 15px;
}

.html-dialog-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  padding: 12px 16px 16px;
}

/* A flex item has a height its contents can fill, which the editor's 100% needs. */
.html-dialog-body > * {
  flex: 1;
  min-height: 0;
}
</style>

<style>
/* The rich text editor's menus and dialogs open on the page, behind a fullscreen modal otherwise. */
.tox-tinymce-aux {
  z-index: 3000 !important;
}
</style>
