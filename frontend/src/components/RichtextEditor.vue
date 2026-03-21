<template>
  <div class="richtext-editor" v-if="isRichtextReady">
    <tiny-mce v-model="computedValue" :disabled="disabled" :init="richtextConf" license-key="gpl" />

    <!-- Source code editor modal -->
    <v-dialog v-model="isRichtextSourceVisible" max-width="1200">
      <v-card>
        <v-card-text class="pt-0 preview">
          <code-editor lang="html" v-model="richTextSourceBody" key="richtext-source" />
        </v-card-text>
        <v-card-actions class="justify-end">
          <v-btn @click="onFormatRichtextHTML" variant="text">
            {{ $t('campaigns.formatHTML') }}
          </v-btn>
          <v-btn @click="() => { this.isRichtextSourceVisible = false; }" variant="text">
            {{ $t('globals.buttons.close') }}
          </v-btn>
          <v-btn @click="onSaveRichTextSource" color="primary">
            {{ $t('globals.buttons.save') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Insert HTML snippet modal -->
    <v-dialog v-model="isInsertHTMLVisible" max-width="750">
      <v-card>
        <v-card-text class="pt-0 preview">
          <code-editor lang="html" v-model="insertHTMLSnippet" key="richtext-snippet" />
        </v-card-text>
        <v-card-actions class="justify-end">
          <v-btn @click="onFormatRichtextHTMLSnippet" variant="text">
            {{ $t('campaigns.formatHTML') }}
          </v-btn>
          <v-btn @click="() => { this.isInsertHTMLVisible = false; }" variant="text">
            {{ $t('globals.buttons.close') }}
          </v-btn>
          <v-btn @click="onInsertHTML" color="primary">
            {{ $t('globals.buttons.insert') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- image picker modal -->
    <v-dialog v-model="isMediaVisible" max-width="900">
      <v-card>
        <v-card-text class="pt-0">
          <media
            is-modal
            type="pictures"
            @selected="onMediaSelect"
            @close="isMediaVisible = false"
          />
        </v-card-text>
      </v-card>
    </v-dialog>
  </div>
</template>

<script>
import { html } from 'js-beautify';
import { mapState } from 'vuex';

import TinyMce from '@tinymce/tinymce-vue';
import 'tinymce/skins/ui/oxide/skin.css';

import { colors, uris } from '../constants';
import CodeEditor from './CodeEditor.vue';
import Media from '../views/Media.vue';

let tinyMceLoader;
const GO_TEMPLATE_PATTERN = /\{\{[\s\S]*?\}\}/g;

function encodeGoTemplates(html = '') {
  return String(html).replace(GO_TEMPLATE_PATTERN, (match) => (
    match
      .replace(/\{/g, '&#123;')
      .replace(/\}/g, '&#125;')
  ));
}

function decodeGoTemplates(html = '') {
  return String(html)
    .replace(/&#123;|&lbrace;/g, '{')
    .replace(/&#125;|&rbrace;/g, '}');
}

function loadTinyMceRuntime() {
  if (!tinyMceLoader) {
    tinyMceLoader = (async () => {
      const { default: tinymce } = await import('tinymce/tinymce');
      if (typeof window !== 'undefined') {
        window.tinymce = tinymce;
      }
      if (typeof globalThis !== 'undefined') {
        globalThis.tinymce = tinymce;
      }

      await Promise.all([
        import('tinymce/models/dom'),
        import('tinymce/icons/default'),
        import('tinymce/themes/silver'),
        import('tinymce/plugins/anchor'),
        import('tinymce/plugins/autolink'),
        import('tinymce/plugins/autoresize'),
        import('tinymce/plugins/charmap'),
        import('tinymce/plugins/emoticons'),
        import('tinymce/plugins/emoticons/js/emojis'),
        import('tinymce/plugins/fullscreen'),
        import('tinymce/plugins/help'),
        import('tinymce/plugins/image'),
        import('tinymce/plugins/link'),
        import('tinymce/plugins/lists'),
        import('tinymce/plugins/searchreplace'),
        import('tinymce/plugins/table'),
        import('tinymce/plugins/visualblocks'),
        import('tinymce/plugins/visualchars'),
        import('tinymce/plugins/wordcount'),
      ]);

      return tinymce;
    })();
  }

  return tinyMceLoader;
}

// Map of listmonk language codes to corresponding TinyMCE language files.
const LANGS = {
  'cs-cz': 'cs',
  de: 'de',
  es: 'es_419',
  fr: 'fr_FR',
  it: 'it_IT',
  pl: 'pl',
  pt: 'pt_PT',
  'pt-BR': 'pt_BR',
  ro: 'ro',
  tr: 'tr',
};

export default {
  components: {
    Media,
    'tiny-mce': TinyMce,
    'code-editor': CodeEditor,
  },

  emits: ['update:modelValue'],

  props: {
    disabled: { type: Boolean, default: false },
    preserveGoTemplate: { type: Boolean, default: false },
    modelValue: {
      type: String,
      default: '',
    },
  },

  data() {
    return {
      isPreviewing: false,
      isMediaVisible: false,
      isReady: false,
      isRichtextReady: false,
      isRichtextSourceVisible: false,
      isInsertHTMLVisible: false,
      insertHTMLSnippet: '',
      isTrackLink: false,
      richtextConf: {},
      richTextSourceBody: '',
      contentType: '',
    };
  },

  methods: {
    encodeEditorValue(value) {
      return this.preserveGoTemplate ? encodeGoTemplates(value) : value;
    },

    decodeEditorValue(value) {
      return this.preserveGoTemplate ? decodeGoTemplates(value) : value;
    },

    async initRichtextEditor() {
      await loadTinyMceRuntime();
      const { lang } = this.serverConfig;

      this.richtextConf = {
        init_instance_callback: () => { this.isReady = true; },
        urlconverter_callback: this.onEditorURLConvert,

        setup: (editor) => {
          editor.addShortcut('ctrl+s', 'Save content', () => {
            this.$events.$emit('campaign.update', {});
          });
          editor.addShortcut('f9', 'Preview', () => {
            this.$events.$emit('campaign.preview', {});
          });

          editor.on('init', () => {
            editor.focus();
          });

          // Custom HTML editor.
          editor.ui.registry.addButton('html', {
            icon: 'sourcecode',
            tooltip: 'Source code',
            onAction: this.onRichtextViewSource,
          });

          editor.ui.registry.addButton('insert-html', {
            icon: 'code-sample',
            tooltip: 'Insert HTML',
            onAction: this.onOpenInsertHTML,
          });

          editor.on('CloseWindow', () => {
            editor.selection.getNode().scrollIntoView(false);
          });
        },

        license_key: 'gpl',
        browser_spellcheck: true,
        min_height: 500,
        toolbar_sticky: true,
        entity_encoding: 'raw',
        convert_urls: true,
        verify_html: !this.preserveGoTemplate,
        extended_valid_elements: this.preserveGoTemplate ? '*[*]' : undefined,
        plugins: [
          'anchor', 'autoresize', 'autolink', 'charmap', 'emoticons', 'fullscreen',
          'help', 'image', 'link', 'lists', 'searchreplace',
          'table', 'visualblocks', 'visualchars', 'wordcount',
        ],
        toolbar: `undo redo | formatselect styleselect fontsizeselect |
                  bold italic underline strikethrough forecolor backcolor subscript superscript |
                  alignleft aligncenter alignright alignjustify |
                  bullist numlist table image insert-html | outdent indent | link removeformat |
                  html fullscreen help`,
        base_url: `${uris.static}/tinymce`,
        fontsize_formats: '10px 11px 12px 14px 15px 16px 18px 24px 36px',
        skin: false,
        content_css: false,
        content_style: `
          body { font-family: 'Inter', sans-serif; font-size: 15px; }
          img { max-width: 100%; }
          img.img-float-left { float: left; margin: 0 1em 1em 0; }
          img.img-float-right { float: right; margin: 0 0 1em 1em; }
          a { color: ${colors.primary}; }
          table, td { border-color: #ccc;}
        `,

        language: LANGS[lang] || null,
        language_url: LANGS[lang] ? `${uris.static}/tinymce/lang/${LANGS[lang]}.js` : null,

        image_advtab: true,
        image_class_list: [
          { title: 'None', value: '' },
          { title: 'Float left', value: 'img-float-left' },
          { title: 'Float right', value: 'img-float-right' },
        ],

        file_picker_types: 'image',
        file_picker_callback: (callback) => {
          this.isMediaVisible = true;
          this.imageCallack = callback;
        },
      };

      this.isRichtextReady = true;
    },

    onEditorURLConvert(url) {
      let u = url;
      if (this.isTrackLink) {
        u = `${u}@TrackLink`;
      }

      this.isTrackLink = false;
      return u;
    },

    onRichtextViewSource() {
      this.richTextSourceBody = this.decodeEditorValue(this.modelValue);
      this.isRichtextSourceVisible = true;
    },

    onOpenInsertHTML() {
      this.isInsertHTMLVisible = true;
    },

    onInsertHTML() {
      this.isInsertHTMLVisible = false;
      window.tinymce.activeEditor?.execCommand('mceInsertContent', false, this.encodeEditorValue(this.insertHTMLSnippet));
      this.insertHTMLSnippet = '';
    },

    onFormatRichtextHTML() {
      this.richTextSourceBody = this.beautifyHTML(this.richTextSourceBody);
    },

    onFormatRichtextHTMLSnippet() {
      this.insertHTMLSnippet = this.beautifyHTML(this.insertHTMLSnippet);
    },

    onSaveRichTextSource() {
      const decoded = this.decodeEditorValue(this.richTextSourceBody);
      this.computedValue = decoded;
      window.tinymce.activeEditor?.setContent(this.encodeEditorValue(decoded));
      this.richTextSourceBody = '';
      this.isRichtextSourceVisible = false;
    },

    onMediaSelect(media) {
      this.imageCallack(media.url);
      this.isMediaVisible = false;
    },

    beautifyHTML(str) {
      // Pad all tags with linebreaks.
      let s = this.trimLines(str.replace(/(<(?!(\/)?a|span)([^>]+)>)/ig, '\n$1\n'), true);
      // Remove extra linebreaks.
      s = s.replace(/\n+/g, '\n');

      try {
        s = html(s).trim();
      } catch (error) {
        // eslint-disable-next-line no-console
        console.log('error formatting HTML', error);
      }

      return s;
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
  },

  mounted() {
    void this.initRichtextEditor();
  },

  computed: {
    ...mapState(['serverConfig']),

    computedValue: {
      get() {
        return this.encodeEditorValue(this.modelValue);
      },
      set(newValue) {
        this.$emit('update:modelValue', this.decodeEditorValue(newValue));
      },
    },
  },
};
</script>
