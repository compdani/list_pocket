<template>
  <v-form @submit.prevent="onSubmit">
    <div class="admin-dialog-card modal-card content template-dialog-card">
      <header class="admin-dialog-head modal-card-head">
        <div class="dialog-meta-row">
          <p v-if="isEditing" class="entity-meta has-text-grey is-size-7">
            {{ $t('globals.fields.id') }}:
            <span data-cy="id"><copy-text :text="`${data.id}`" /></span>
          </p>

          <v-btn
            type="button"
            color="primary"
            variant="tonal"
            prepend-icon="mdi-file-find-outline"
            class="preview-button"
            @click="onTogglePreview"
          >
            {{ $t('templates.preview') }} (F9)
          </v-btn>
        </div>

        <h4 v-if="isEditing" class="dialog-title">
          {{ data.name }}
        </h4>
        <h4 v-else class="dialog-title">
          {{ $t('templates.newTemplate') }}
        </h4>
      </header>

      <section class="admin-dialog-body modal-card-body template-dialog-body">
        <v-alert
          v-if="formError"
          type="error"
          variant="tonal"
          class="mb-4"
        >
          {{ formError }}
        </v-alert>

        <v-row class="mb-1">
          <v-col cols="12" md="8">
            <v-text-field
              ref="focus"
              v-model="form.name"
              :label="$t('globals.fields.name')"
              maxlength="200"
              name="name"
              :placeholder="$t('globals.fields.name')"
              required
              variant="outlined"
              density="comfortable"
            />
          </v-col>

          <v-col cols="12" md="4">
            <v-select
              v-model="form.type"
              :items="typeOptions"
              item-title="title"
              item-value="value"
              :label="$t('globals.fields.type')"
              :disabled="isEditing"
              required
              variant="outlined"
              density="comfortable"
            />
          </v-col>
        </v-row>

        <v-row v-if="form.type === 'tx'" class="mb-1">
          <v-col cols="12">
            <v-text-field
              v-model="form.subject"
              :label="$t('templates.subject')"
              maxlength="200"
              name="subject"
              :placeholder="$t('templates.subject')"
              required
              variant="outlined"
              density="comfortable"
            />
          </v-col>
        </v-row>

        <v-row v-if="form.type === 'tx'" class="mb-1">
          <v-col cols="12" md="4">
            <v-select
              v-model="editorMode"
              :items="editorModeOptions"
              item-title="title"
              item-value="value"
              label="Editor"
              variant="outlined"
              density="comfortable"
            />
          </v-col>
        </v-row>

        <div class="editor-shell">
          <div class="editor-shell-head">
            <div class="editor-label">
              {{ currentEditorLabel }}
            </div>
            <v-btn
              v-if="form.type === 'tx'"
              type="button"
              size="small"
              variant="text"
              prepend-icon="mdi-code-braces"
              @click="insertContentPlaceholder"
            >
              Insert Content Placeholder
            </v-btn>
          </div>

          <div class="editor-surface">
            <visual-editor
              v-if="useVisualEditor"
              name="body"
              :source="form.bodySource"
              height="62vh"
              @change="onChangeVisualEditor"
            />

            <grapes-mjml-editor
              v-else-if="useGrapesEditor"
              :source="form.bodySource"
              :data="form.body"
              height="62vh"
              @change="onChangeVisualEditor"
            />

            <richtext-editor
              v-else-if="useRichtextEditor"
              v-model="form.body"
              :preserve-go-template="true"
            />

            <code-editor
              v-else
              v-model="form.body"
              lang="html"
              name="body"
            />
          </div>
        </div>

        <p class="form-help mt-3">
          <template v-if="form.type === 'campaign'">
            {{ $t('templates.placeholderHelp', { placeholder: egPlaceholder }) }}
          </template>
          <a target="_blank" rel="noopener noreferer" href="https://listmonk.app/docs/templating">
            {{ $t('globals.buttons.learnMore') }}
          </a>
        </p>
      </section>

      <footer class="admin-dialog-foot modal-card-foot">
        <v-btn
          type="button"
          variant="outlined"
          class="dialog-action"
          @click="$emit('close')"
        >
          {{ $t('globals.buttons.close') }}
        </v-btn>
        <v-btn
          v-if="$can('templates:manage')"
          color="primary"
          variant="flat"
          class="dialog-action"
          data-cy="btn-save"
          :disabled="loading.templates"
          :loading="loading.templates"
          type="submit"
        >
          {{ $t('globals.buttons.save') }}
        </v-btn>
      </footer>
    </div>
  </v-form>

  <campaign-preview
    v-if="previewItem"
    is-post
    type="template"
    :title="previewItem.name"
    :template-type="previewItem.type"
    :body="form.body"
    @close="onTogglePreview"
  />
</template>

<script>
import { mapState } from 'vuex';
import CampaignPreview from '../components/CampaignPreview.vue';
import CodeEditor from '../components/CodeEditor.vue';
import RichtextEditor from '../components/RichtextEditor.vue';
import VisualEditor from '../components/VisualEditor.vue';
import GrapesMjmlEditor from '../components/GrapesMjmlEditor.vue';
import CopyText from '../components/CopyText.vue';

const baseForm = () => ({
  name: '',
  subject: '',
  type: 'campaign',
  optin: '',
  body: '',
  bodySource: '',
});

export default {
  name: 'TemplateForm',

  components: {
    CampaignPreview,
    CopyText,
    'code-editor': CodeEditor,
    'richtext-editor': RichtextEditor,
    'visual-editor': VisualEditor,
    'grapes-mjml-editor': GrapesMjmlEditor,
  },

  props: {
    data: { type: Object, default: () => ({}) },
    isEditing: { type: Boolean, default: false },
  },

  data() {
    return {
      form: baseForm(),
      formError: '',
      previewItem: null,
      egPlaceholder: '{{ template "content" . }}',
      editorMode: 'html',
    };
  },

  computed: {
    ...mapState(['loading']),

    typeOptions() {
      return [
        { title: this.$tc('templates.typeCampaignHTML'), value: 'campaign' },
        { title: this.$tc('templates.typeCampaignVisual'), value: 'campaign_visual' },
        { title: 'GrapesJS (MJML)', value: 'campaign_grapes_mjml' },
        { title: this.$tc('templates.typeTransactional'), value: 'tx' },
      ];
    },

    editorModeOptions() {
      return [
        { title: this.$t('templates.rawHTML'), value: 'html' },
        { title: 'TinyMCE', value: 'richtext' },
        { title: this.$tc('templates.typeCampaignVisual'), value: 'visual' },
        { title: 'GrapesJS (MJML)', value: 'grapes_mjml' },
      ];
    },

    useVisualEditor() {
      return this.form.type === 'campaign_visual' || (this.form.type === 'tx' && this.editorMode === 'visual');
    },

    useGrapesEditor() {
      return this.form.type === 'campaign_grapes_mjml' || (this.form.type === 'tx' && this.editorMode === 'grapes_mjml');
    },

    useRichtextEditor() {
      return this.form.type === 'tx' && this.editorMode === 'richtext';
    },

    currentEditorLabel() {
      if (this.useVisualEditor) {
        return this.$t('templates.typeCampaignVisual');
      }
      if (this.useGrapesEditor) {
        return 'GrapesJS (MJML)';
      }
      if (this.useRichtextEditor) {
        return 'TinyMCE';
      }
      return this.$t('templates.rawHTML');
    },
  },

  methods: {
    extractErrorMessage(err) {
      if (err && err.response && err.response.message) {
        return err.response.message;
      }

      if (err && err.message) {
        return err.message;
      }

      return 'Something went wrong while processing your request.';
    },

    normalizeForm(data = {}) {
      const hasBodySource = typeof data.bodySource === 'string' && data.bodySource.trim() !== '';
      const initialEditorMode = data.type === 'campaign_grapes_mjml'
        ? 'grapes_mjml'
        : data.type === 'campaign_visual'
          ? 'visual'
          : (data.type === 'tx' && hasBodySource)
            ? 'visual'
            : 'html';
      return {
        ...baseForm(),
        ...data,
        body: data.body ?? '',
        bodySource: data.bodySource ?? '',
        editorMode: initialEditorMode,
      };
    },

    onTogglePreview() {
      this.previewItem = this.previewItem ? null : this.form;
    },

    onPreviewShortcut(e) {
      if (e.key === 'F9') {
        this.onTogglePreview();
        e.preventDefault();
      }
    },

    onSubmit() {
      this.formError = '';

      if (!this.form.name.trim()) {
        this.formError = this.$t('campaigns.fieldInvalidName');
        return;
      }

      if (this.form.type === 'tx' && !this.form.subject.trim()) {
        this.formError = this.$t('globals.messages.missingFields', { name: 'subject' });
        return;
      }

      if (this.isEditing) {
        this.updateTemplate();
        return;
      }

      this.createTemplate();
    },

    createTemplate() {
      const payload = {
        id: this.data.id,
        name: this.form.name,
        type: this.form.type,
        subject: this.form.subject,
        body: this.form.body,
        body_source: this.form.bodySource,
      };

      this.$api.createTemplate(payload).then((resp) => {
        this.$emit('finished');
        this.$emit('close');
        this.$utils.toast(this.$t('globals.messages.created', { name: resp.name }));
      }).catch((err) => {
        this.formError = this.extractErrorMessage(err);
      });
    },

    updateTemplate() {
      const payload = {
        id: this.data.id,
        name: this.form.name,
        type: this.form.type,
        subject: this.form.subject,
        body: this.form.body,
        body_source: this.form.bodySource,
      };

      this.$api.updateTemplate(payload).then((resp) => {
        this.$emit('finished');
        this.$emit('close');
        this.$utils.toast(`'${resp.name}' updated`);
      }).catch((err) => {
        this.formError = this.extractErrorMessage(err);
      });
    },

    onChangeVisualEditor({ source, body }) {
      this.formError = '';
      this.form.body = body;
      this.form.bodySource = source;
    },

    insertContentPlaceholder() {
      const placeholder = '{{ template "content" . }}';
      if (this.form.body.includes(placeholder)) {
        return;
      }
      this.form.body = this.form.body ? `${this.form.body}\n${placeholder}` : placeholder;
    },
  },

  mounted() {
    this.form = this.normalizeForm(this.data);
    this.editorMode = this.form.editorMode || 'html';

    this.$nextTick(() => {
      if (this.$refs.focus?.focus) {
        this.$refs.focus.focus();
      }
    });

    window.addEventListener('keydown', this.onPreviewShortcut);
  },

  beforeUnmount() {
    window.removeEventListener('keydown', this.onPreviewShortcut);
  },

  watch: {
    editorMode(nextMode) {
      if (this.form.type !== 'tx') {
        return;
      }
      if (nextMode !== 'visual' && nextMode !== 'grapes_mjml') {
        this.form.bodySource = '';
      }
    },

    'form.type'(nextType) {
      if (nextType === 'campaign_visual') {
        this.editorMode = 'visual';
      } else if (nextType === 'campaign_grapes_mjml') {
        this.editorMode = 'grapes_mjml';
      } else if (nextType === 'tx') {
        this.editorMode = this.form.bodySource ? 'visual' : 'html';
      } else {
        this.editorMode = 'html';
      }
    },
  },
};
</script>

<style scoped>
.admin-dialog-card {
  background: #fff;
  border: 1px solid #dce5f2;
  border-radius: 16px;
  box-shadow: 0 18px 45px rgba(15, 23, 42, 0.18);
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 48px);
  overflow: hidden;
  width: min(1200px, calc(100vw - 32px));
}

.admin-dialog-head {
  background: linear-gradient(180deg, #ffffff 0%, #f8fbff 100%);
  border-bottom: 1px solid #ebf1fb;
  display: block;
  padding: 18px 20px;
  width: 100%;
}

.dialog-meta-row {
  align-items: flex-start;
  display: flex;
  gap: 1rem;
  justify-content: space-between;
  margin-bottom: 8px;
}

.dialog-title {
  margin: 8px 0 0;
}

.template-dialog-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  overflow: auto;
  padding: 24px 20px;
}

.editor-shell {
  background: #f8fbff;
  border: 1px solid #e7eefb;
  border-radius: 12px;
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  padding: 14px;
}

.editor-shell-head {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
}

.editor-label {
  color: #334155;
  font-size: 0.9rem;
  font-weight: 600;
  margin-bottom: 10px;
}

.editor-surface {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.admin-dialog-foot {
  background: #fff;
  border-top: 1px solid #ebf1fb;
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  padding: 16px 20px 20px;
}

.dialog-action {
  height: 44px;
  min-width: 120px;
}

.form-help {
  color: #667085;
  font-size: 0.9rem;
  line-height: 1.5;
}

:deep(.v-field) {
  border-radius: 12px;
}

:deep(.code-editor) {
  background: #fff;
  border-color: #d7e0ee;
  border-radius: 10px;
  height: 62vh;
  min-height: 420px;
}

:deep(.visual-editor-wrapper) {
  background: #fff;
  border-color: #d7e0ee;
  border-radius: 10px;
  height: min(62vh, calc(100vh - 360px));
  min-height: 420px;
  overflow: hidden;
}

@media (max-width: 768px) {
  .admin-dialog-head {
    padding: 16px;
  }

  .template-dialog-body {
    padding: 16px;
  }

  .editor-shell {
    min-height: 360px;
    padding: 12px;
  }

  :deep(.code-editor) {
    height: 52vh;
    min-height: 300px;
  }

  .dialog-meta-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .admin-dialog-foot {
    flex-direction: column-reverse;
    padding: 12px 16px 16px;
  }

  .preview-button {
    width: 100%;
  }

  .dialog-action {
    width: 100%;
  }
}
</style>
