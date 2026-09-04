<template>
  <!-- Two-way Data-Binding -->
  <div class="editor">
    <v-toolbar class="mb-1">
      <v-select v-model="contentTypeSel" :disabled="isContentTypeSelectDisabled" :label="$t('campaigns.format')"
        :items="Object.entries(contentTypes).map(([key, name]) => ({ value: key, title: name }))" item-title="title"
        item-value="value" name="content_type" hide-details class="mx-2" variant="outlined" density="compact" />
      <v-spacer />

      <v-btn v-if="isContentTypeLocked" color="warning" prepend-icon="mdi-lock-open-variant-outline"
        :disabled="disabled" @click="onUnlockContentTypeSelect">
        {{ $t('campaigns.unlockFormat') }}
      </v-btn>
      <v-btn v-if="!isVisualTplSelector && self.contentType === 'visual'" @click="onShowVisualTplSelector"
        variant="text" prepend-icon="mdi-file-find-outline" data-cy="btn-select-visual-tpl">
        {{ $t('campaigns.importVisualTemplate') }}
      </v-btn>
      <v-btn @click="onTogglePreview" color="primary" prepend-icon="mdi-file-find-outline" data-cy="btn-preview"
        aria-keyshortcuts="F9">
        <span class="has-kbd">{{ $t('campaigns.preview') }} <span class="kbd">F9</span></span>
      </v-btn>
      <v-btn color="primary" variant="outlined" prepend-icon="mdi-creation-outline" :disabled="disabled" @click="isAIDialogOpen = true">
        {{ $t('campaigns.aiBuilder') }}
      </v-btn>

      <template #extension v-if="self.contentType !== 'visual'">
        <v-select v-model="templateId" :label="$tc('globals.terms.template')" :placeholder="$t('globals.terms.none')"
          :items="validTemplates" item-title="name" item-value="id" name="template" :disabled="disabled" clearable hide-details variant="outlined" density="compact" class="mx-2" />
      </template>
      <template #extension v-if="isVisualTplSelector">

          <v-select v-model="visualTemplateId" @update:model-value="() => isVisualTplDisabled = false"
            :label="$tc('globals.terms.template')" :placeholder="$t('globals.terms.none')" :items="validTemplates"
            item-title="name" item-value="id" name="template" :disabled="disabled" clearable class="mx-2"
            hide-details variant="outlined" density="compact" />

          <v-btn :disabled="disabled || isVisualTplDisabled || !visualTemplateId" @click="onImportVisualTpl"
            color="primary" prepend-icon="mdi-content-save-outline" data-cy="btn-save-visual-tpl"
            :loading="loading.templates">
            {{ $t('globals.terms.import') }}
          </v-btn>
      </template>
    </v-toolbar>



    <richtext-editor v-if="self.contentType === 'richtext'" v-model="self.body" :disabled="disabled"
      key="editor-richtext" />

    <!-- visual editor //-->
    <visual-editor v-if="self.contentType === 'visual'" :source="self.bodySource" @change="onVisualEditorChange"
      height="65vh" ref="visualEditor" />

    <grapes-mjml-editor v-if="self.contentType === 'grapes_mjml'" ref="grapesEditor" :source="self.bodySource"
      :data="self.body" height="65vh" @change="onGrapesEditorChange" />

    <!-- raw html editor //-->
    <code-editor lang="html" v-if="self.contentType === 'html'" v-model="self.body" key="editor-html" />

    <!-- markdown editor //-->
    <code-editor lang="markdown" v-if="self.contentType === 'markdown'" v-model="self.body" key="editor-markdown" />

    <!-- plain text //-->
    <v-textarea v-if="self.contentType === 'plain'" v-model="self.body" name="content" ref="plainEditor"
      class="plain-editor" />

    <!-- campaign preview //-->
    <campaign-preview v-if="isPreviewing" is-post @close="onTogglePreview" type="campaign" :id="id" :title="title"
      :content-type="self.contentType" :template-id="templateId" :body="self.body" :preheader="preheader" />

    <ai-campaign-builder-dialog
      v-model="isAIDialogOpen"
      :context="aiContext"
      :current="aiCurrent"
      :session-key="aiSessionKey"
      @apply="onApplyAIResult"
    />
  </div>
</template>

<script>
import { defineAsyncComponent } from 'vue';
import { html as beautifyHTML } from 'js-beautify';
import TurndownService from 'turndown';
import { mapState } from 'vuex';

import CampaignPreview from './CampaignPreview.vue';
import GrapesMjmlEditor from './GrapesMjmlEditor.vue';
import markdownToVisualBlock from './editor';
import AICampaignBuilderDialog from './AICampaignBuilderDialog.vue';

const RichtextEditor = defineAsyncComponent(() => import('./RichtextEditor.vue'));
const VisualEditor = defineAsyncComponent(() => import('./VisualEditor.vue'));
const CodeEditor = defineAsyncComponent(() => import('./CodeEditor.vue'));

const turndown = new TurndownService();

export default {
  components: {
    CampaignPreview,
    RichtextEditor,
    'code-editor': CodeEditor,
    'visual-editor': VisualEditor,
    'grapes-mjml-editor': GrapesMjmlEditor,
    'ai-campaign-builder-dialog': AICampaignBuilderDialog,
  },

  emits: ['update:modelValue', 'ai-generated'],

  props: {
    contentTypes: { type: Object, default: () => ({}) },
    id: { type: [String, Number], default: '' },
    title: { type: String, default: '' },
    disabled: { type: Boolean, default: false },
    templates: { type: Array, default: null },
    preheader: { type: String, default: '' },
    subject: { type: String, default: '' },
    // `messenger` signals the campaign channel (e.g. 'email' or 'quo') so the
    // template picker can show only templates compatible with that channel.
    messenger: { type: String, default: 'email' },

    // modelValue is provided by the parent component.
    // Throughout the editor, `this.self` references that reactive object.
    modelValue: {
      type: Object,
      default: () => ({
        body: '',
        bodySource: null,
        contentType: '',
        templateId: null,
      }),
    },
  },

  data() {
    return {
      isPreviewing: false,
      isVisualTplSelector: false,
      isVisualTplDisabled: false,
      contentTypeSel: this.$props.modelValue.contentType,
      templateId: null,
      visualTemplateId: null,
      isTypeSelectorLocked: true,
      isAIDialogOpen: false,
    };
  },

  methods: {
    onContentTypeChange(to, from) {
      if (!this.self.body.trim()) {
        this.convertContentType(to, from);
        return;
      }

      // Ask for confirmation as pretty much all conversions are lossy.
      this.$utils.confirm(
        this.$t('campaigns.confirmSwitchFormat'),
        () => {
          this.convertContentType(to, from);
        },
        () => {
          // Cancelled. Reset the <select> to the last value.
          this.contentTypeSel = from;
        },
      );
    },

    isGuardedEditorSwitch(from, to) {
      return (from === 'visual' || from === 'grapes_mjml') && from !== to;
    },

    isGuardedType(type) {
      return type === 'visual' || type === 'grapes_mjml';
    },

    onUnlockContentTypeSelect() {
      this.isTypeSelectorLocked = false;
    },

    htmlToMjml(html) {
      const source = String(html ?? '').trim();
      const fallback = '<div></div>';
      return `<mjml><mj-body><mj-section><mj-column><mj-raw>${source || fallback}</mj-raw></mj-column></mj-section></mj-body></mjml>`;
    },

    convertContentType(to, from) {
      if (from === 'grapes_mjml' && from !== to) {
        this.syncGrapesHtml();
      }

      let body = this.self.body ?? '';
      let bodySource = null;

      // Skip UI update (markdown => richtext, html requires a backenbd call).
      let skip = false;

      // If `from` is HTML content, strip out `<body>..` etc. and keep the beautified HTML.
      let isHTML = false;
      if (from === 'richtext' || from === 'html' || from === 'visual' || from === 'grapes_mjml') {
        const d = document.createElement('div');
        d.innerHTML = body;
        body = this.beautifyHTML(d.innerHTML.trim());
        isHTML = true;
      }

      // HTML => Non-HTML.
      if (isHTML) {
        switch (to) {
          case 'plain': {
            const d = document.createElement('div');
            d.innerHTML = body;
            body = this.trimLines(d.innerText.trim(), true);
            break;
          }

          case 'markdown': {
            body = turndown.turndown(body).replace(/\n\n+/ig, '\n\n');
            break;
          }

          case 'visual':
            {
              const md = turndown.turndown(body).replace(/\n\n+/ig, '\n\n');
              bodySource = JSON.stringify(markdownToVisualBlock(md));
              break;
            }

          case 'grapes_mjml':
            bodySource = this.htmlToMjml(body);
            break;

          default:
            // Switching between HTML formats, no need to do anything further
            // as body is already beautified.
            // richtext|html => visual, the contents are simply lost.
            break;
        }

        // Markdown to HTML requires a backend call.
      } else if (from === 'markdown' && (to === 'richtext' || to === 'html')) {
        skip = true;
        this.$api.convertCampaignContent({
          id: 1, body, from, to,
        }).then((data) => {
          this.$nextTick(() => {
            // Both type + body should be updated in one cycle to avoid firing
            // multiple events.
            this.self.contentType = to;
            this.self.body = this.beautifyHTML(data.trim());
          });
        });

        // Plain to an HTML type, change plain line breaks to HTML breaks.
      } else if (from === 'plain' && (to === 'richtext' || to === 'html')) {
        body = body.replace(/\n/ig, '<br>\n');
      } else if (to === 'visual') {
        bodySource = JSON.stringify(markdownToVisualBlock(body));
      } else if (to === 'grapes_mjml') {
        bodySource = this.htmlToMjml(body);
      }

      // =======================================================================
      // Reset the campaign template ID if its converted to or from visual template.
      if (to === 'visual' || from === 'visual' || to === 'grapes_mjml' || from === 'grapes_mjml') {
        this.templateId = null;
        this.self.templateId = null;
      }

      // =======================================================================
      // Apply the conversion on the editor UI.
      if (!skip) {
        this.$nextTick(() => {
          // Both type + body should be updated in one cycle to avoid firing
          // multiple events.
          this.self.contentType = to;
          this.self.body = body;
          this.self.bodySource = bodySource;
          if (this.isGuardedType(to)) {
            this.isTypeSelectorLocked = true;
          }
        });
      }
    },

    onTogglePreview() {
      this.syncGrapesHtml();
      this.isPreviewing = !this.isPreviewing;
    },

    onKeyboardShortcut(e) {
      // On F9, toggle the preview.
      if (e.key === 'F9') {
        this.onTogglePreview();
        e.preventDefault();
      }

      // On Ctrl+S, trigger save.
      if (e.ctrlKey && e.key === 's') {
        this.$events.$emit('campaign.update');
        e.preventDefault();
      }
    },

    onVisualEditorChange({ body, source }) {
      this.self.body = body;
      this.self.bodySource = source;
    },

    onGrapesEditorChange({ body, source }) {
      if (typeof body === 'string') {
        this.self.body = body;
      }
      this.self.bodySource = source;
    },

    syncGrapesHtml() {
      if (this.self.contentType !== 'grapes_mjml') {
        return;
      }
      const compiled = this.$refs.grapesEditor?.getCompiledContent?.();
      if (!compiled) {
        return;
      }
      if (compiled.source) {
        this.self.bodySource = compiled.source;
      }
      if (compiled.body) {
        this.self.body = compiled.body;
      }
    },

    beautifyHTML(str) {
      // Pad all tags with linebreaks.
      let s = this.trimLines(str.replace(/(<(?!(\/)?a|span)([^>]+)>)/ig, '\n$1\n'), true);
      // Remove extra linebreaks.
      s = s.replace(/\n+/g, '\n');

      return beautifyHTML(s, {
        indent_size: 4,
        indent_char: ' ',
        max_preserve_newlines: 2,
        inline: ['h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'b', 'strong', 'span', 'em', 'i', 'code', 'a'],
      }).trim();
    },

    trimLines(str, removeEmptyLines) {
      const out = str.split('\n');
      for (let i = 0; i < out.length; i += 1) {
        const line = out[i].trim();
        if (removeEmptyLines) {
          out[i] = line;
        } else if (line === '') {
          out[i] = '';
        }
      }

      return out.join('\n').replace(/\n\s*\n\s*\n/g, '\n\n');
    },

    onShowVisualTplSelector() {
      this.isVisualTplSelector = true;
      this.setDefaultTemplate();
    },

    onApplyAIResult(result) {
      if (!result || typeof result !== 'object') {
        return;
      }

      if (result.contentType && this.contentTypes[result.contentType]) {
        this.self.contentType = result.contentType;
        this.contentTypeSel = result.contentType;
        if (this.isGuardedType(result.contentType)) {
          this.isTypeSelectorLocked = true;
        }
      }

      const nextType = result.contentType || this.self.contentType;
      if (typeof result.body === 'string') {
        if (nextType === 'visual' || nextType === 'grapes_mjml') {
          this.self.bodySource = result.body;
          if (nextType === 'visual') {
            try {
              this.$refs.visualEditor?.render?.(JSON.parse(result.body || '{}'));
            } catch {
              // Keep the source in state even if it's not valid JSON.
            }
          }
          if (nextType === 'grapes_mjml') {
            this.$refs.grapesEditor?.loadMjml?.(result.body);
            this.syncGrapesHtml();
          }
        } else {
          this.self.body = result.body;
          this.self.bodySource = '';
        }
      }

      this.$emit('ai-generated', {
        subject: result.subject || '',
        preheader: result.preheader || '',
      });
    },

    onImportVisualTpl() {
      if (!this.visualTemplateId) {
        return;
      }

      this.$utils.confirm(
        this.$t('campaigns.confirmOverwriteContent'),
        () => {
          // Fetch the template body from the server.
          this.$api.getTemplate(this.visualTemplateId).then((data) => {
            this.self.body = data.body;
            this.self.bodySource = data.bodySource;
            this.isVisualTplDisabled = true;

            this.$refs.visualEditor.render(JSON.parse(data.bodySource));
          });
        },
      );
    },

    setDefaultTemplate() {
      if (this.self.contentType === 'visual') {
        this.visualTemplateId = this.validTemplates[0]?.id || null;
      } else {
        if (this.templateId) {
          return;
        }

        const defaultTemplate = this.validTemplates.find((t) => t.isDefault === true);
        this.templateId = defaultTemplate?.id || this.validTemplates[0]?.id || null;
      }
    },
  },

  mounted() {
    // Set initial content type for the selector.
    this.contentTypeSel = this.modelValue.contentType;
    this.templateId = this.modelValue.templateId;

    window.addEventListener('keydown', this.onKeyboardShortcut);

    this.$events.$on('campaign.preview', () => {
      this.isPreviewing = true;
    });
  },

  beforeUnmount() {
    window.removeEventListener('keydown', this.onKeyboardShortcut);
    this.$events.$off('campaign.preview');
  },

  computed: {
    ...mapState(['serverConfig', 'loading']),

    // This references the incoming `modelValue` prop.
    self: {
      get() {
        return this.modelValue;
      },

      // Any direct replacement of the object is emitted to the parent.
      set(val) {
        this.$emit('update:modelValue', val);
      },
    },

    // Returns the list of valid (visual vs. normal) templates for the template dropdown.
    validTemplates() {
      const templates = Array.isArray(this.templates) ? this.templates : [];
      const hasContentPlaceholder = (body = '') => /\{\{(\s+)?template\s+?"content"(\s+)?\.(\s+)?}}/.test(String(body || ''));

      // SMS campaigns only accept plain-text templates (`campaign_sms`).
      if (this.messenger === 'quo') {
        return templates.filter((t) => t.type === 'campaign_sms' && hasContentPlaceholder(t.body));
      }

      if (this.self.contentType === 'visual') {
        return templates.filter((t) => t.type === 'campaign_visual');
      }

      if (this.self.contentType === 'grapes_mjml') {
        return templates.filter((t) => t.type === 'campaign_grapes_mjml');
      }

      // For text editors, allow any campaign template type
      // as long as it exposes the {{ template "content" . }} insertion point.
      // SMS templates are intentionally excluded from email campaigns since
      // they lack the HTML wrappers expected by the email pipeline.
      if (this.self.contentType === 'richtext'
        || this.self.contentType === 'html'
        || this.self.contentType === 'markdown'
        || this.self.contentType === 'plain') {
        return templates.filter((t) => (
          (t.type === 'campaign' || t.type === 'campaign_visual' || t.type === 'campaign_grapes_mjml')
          && hasContentPlaceholder(t.body)
        ));
      }

      return templates.filter((t) => (t.type === 'campaign'));
    },

    isContentTypeLocked() {
      return this.isGuardedType(this.self.contentType) && this.isTypeSelectorLocked;
    },

    isContentTypeSelectDisabled() {
      return this.disabled || this.isContentTypeLocked;
    },

    aiContext() {
      return {
        origin: 'campaign_editor',
        campaignType: 'regular',
        templateType: '',
        contentType: this.self.contentType,
        editorMode: this.self.contentType,
      };
    },

    aiCurrent() {
      const contentType = this.self.contentType;
      const isVisual = contentType === 'visual';
      const isGrapes = contentType === 'grapes_mjml';
      const body = (isVisual || isGrapes) ? (this.self.bodySource || '') : (this.self.body || '');

      return {
        subject: this.subject || '',
        preheader: this.preheader || '',
        body,
        templateId: this.templateId || '',
      };
    },

    aiSessionKey() {
      return `campaign:${this.id || 'new'}`;
    },
  },

  watch: {
    modelValue: {
      deep: true,
      handler(val) {
        if (!val) {
          return;
        }

        if (val.contentType && val.contentType !== this.contentTypeSel) {
          this.contentTypeSel = val.contentType;
          if (this.isGuardedType(val.contentType)) {
            this.isTypeSelectorLocked = true;
          }
        }

        if (val.templateId !== this.templateId) {
          this.templateId = val.templateId;
        }
      },
    },

    validTemplates() {
      // When the filtered list of validTemplates changes (visual vs. regular),
      // select the appropriate 'default' in the template select list.
      this.setDefaultTemplate();
    },

    contentTypeSel(to, from) {
      // Show the conversion prompt if the value in the dropdown isn't the same
      // as the current selection. This happens when eg: contentTypeSel = html -> visual happens
      // in the selector, the prompt is shown, and Cancel is clicked,
      // at which point, contentTypeSel = html again, which triggers this event.
      if (from !== to && to !== this.self.contentType) {
        this.onContentTypeChange(to, from);
      }
    },

    templateId(to) {
      if (this.self.templateId === to) {
        return;
      }

      this.self.templateId = to;
    },
  },
};

</script>
