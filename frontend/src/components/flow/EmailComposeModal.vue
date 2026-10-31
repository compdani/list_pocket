<template>
  <v-dialog :model-value="true" fullscreen persistent :retain-focus="false" transition="dialog-bottom-transition">
    <v-card class="compose" data-cy="email-compose">
      <header class="compose-head">
        <v-btn icon="mdi-close" size="small" variant="text" :aria-label="$t('globals.buttons.close')" data-cy="email-compose-close" @click="close" />
        <strong class="compose-title">{{ $t(reply ? 'conversations.reply.emailTitle' : 'workflows.email.title') }}</strong>
        <v-spacer />
        <v-btn v-if="previewing && !readonly" variant="text" class="text-none" prepend-icon="mdi-arrow-left" data-cy="email-compose-back" @click="previewing = false">
          {{ $t('workflows.email.backToEdit') }}
        </v-btn>
        <v-btn v-if="!readonly" color="primary" variant="flat" class="text-none" data-cy="email-compose-done" @click="done">
          {{ $t('workflows.email.done') }}
        </v-btn>
      </header>

      <div class="compose-body">
        <v-text-field
          v-model="draft.subject"
          :label="$t('workflows.email.subject')"
          :readonly="readonly"
          density="compact"
          variant="outlined"
          hide-details
          class="mb-3"
          data-cy="email-compose-subject"
        />
        <v-alert v-if="approvalNeeded" type="info" variant="tonal" density="compact" class="mb-3 text-body-2" data-cy="email-compose-approval">
          {{ $t('workflows.email.needsApproval') }}
        </v-alert>

        <!-- Kept mounted during the preview, because the visual editors hold their own state. -->
        <div v-if="!readonly" v-show="!previewing" class="compose-editor">
          <editor ref="editor" v-model="draft" embedded :content-types="formats" @preview="showPreview" />
        </div>

        <section v-if="previewing" data-cy="email-compose-preview">
          <h2 class="compose-preview-title">{{ $t('workflows.email.previewTitle') }}</h2>
          <p class="text-caption text-medium-emphasis mb-3">{{ $t(reply ? 'conversations.reply.previewHelp' : 'workflows.email.previewHelp') }}</p>
          <v-progress-linear v-if="loading" indeterminate />
          <v-alert v-else-if="previewError" type="error" variant="tonal" density="compact" data-cy="email-compose-preview-error">{{ previewError }}</v-alert>
          <email-body-frame v-else :html="previewHTML" :title="draft.subject || $t('workflows.email.title')" />
        </section>
      </div>
    </v-card>
  </v-dialog>
</template>

<script setup>
import {
  computed, getCurrentInstance, nextTick, onMounted, ref,
} from 'vue';
import Editor from '../Editor.vue';
import EmailBodyFrame from '../EmailBodyFrame.vue';
import {
  EMAIL_FORMATS, applyDraft, composeDraft, needsSendApproval, plainPreviewHTML,
} from '../../utils/emailCompose.mjs';
import { agentPost } from '../../agent/client';

const props = defineProps({
  input: { type: Object, default: () => ({}) },
  // A format picked outside the modal, which the editor converts to with its usual prompt.
  format: { type: String, default: '' },
  readonly: { type: Boolean, default: false },
  // A reply goes out when it's sent, with nothing to fill in or approve.
  reply: { type: Boolean, default: false },
});
const emit = defineEmits(['close', 'save']);

const { proxy } = getCurrentInstance();
const editor = ref(null);
const draft = ref(composeDraft(props.input));
const start = JSON.stringify(applyDraft(props.input, draft.value));
const previewing = ref(false);
const loading = ref(false);
const previewError = ref('');
const previewHTML = ref('');

const formats = computed(() => Object.fromEntries(EMAIL_FORMATS.map((f) => [f.value, f.title || proxy.$t(f.label)])));
const current = () => applyDraft(props.input, draft.value);
const approvalNeeded = computed(() => !props.reply && needsSendApproval(current()));

async function showPreview() {
  if (previewing.value && !props.readonly) {
    previewing.value = false;
    return;
  }
  previewing.value = true;
  loading.value = true;
  previewError.value = '';
  try {
    const { contentType, body } = draft.value;
    const out = await agentPost('flows/email-preview', { content_type: contentType, body });
    previewHTML.value = out.html || plainPreviewHTML(out.text);
  } catch (err) {
    previewError.value = err.message;
  } finally {
    loading.value = false;
  }
}

function done() {
  editor.value?.syncGrapesHtml();
  emit('save', current());
}

function close() {
  if (props.readonly || JSON.stringify(current()) === start) {
    emit('close');
    return;
  }
  proxy.$utils.confirm(proxy.$t('workflows.email.discardTitle'), () => emit('close'));
}

onMounted(async () => {
  if (props.readonly) {
    showPreview();
    return;
  }
  if (props.format && props.format !== draft.value.contentType) {
    await nextTick();
    editor.value.contentTypeSel = props.format;
  }
});
</script>

<style scoped>
.compose {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.compose-head {
  align-items: center;
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  display: flex;
  flex: none;
  gap: 8px;
  padding: 8px 16px 8px 8px;
}

.compose-title {
  font-size: 15px;
}

.compose-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  overflow-y: auto;
  padding: 12px 16px 16px;
}

.compose-body > * {
  flex: none;
}

.compose-body > .compose-editor {
  flex: 1 1 0;
  min-height: 420px;
}

.compose-preview-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 2px;
}
</style>

<style>
/* The rich text editor's menus and dialogs open on the page, behind a fullscreen modal otherwise. */
.tox-tinymce-aux {
  z-index: 3000 !important;
}
</style>
