<template>
  <v-dialog v-model="isVisible" @update:model-value="close" max-width="1100">
    <v-card class="preview-card">
      <v-card-title class="preview-title">
        <span class="preview-title__text">{{ title }}</span>
      </v-card-title>
      <v-card-text class="preview-content">
        <v-overlay v-if="isLoading" contained persistent>
          <v-progress-circular indeterminate />
        </v-overlay>
        <div class="preview-stage">
          <iframe
            id="iframe"
            name="iframe"
            ref="iframe"
            :title="title"
            :srcdoc="previewHTML"
            @load="onLoaded"
            sandbox="allow-scripts"
            class="preview-iframe"
          />
        </div>
      </v-card-text>
      <v-card-actions class="preview-actions">
        <v-btn @click="close">
          {{ $t('globals.buttons.close') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script>
import { uris } from '../constants';
import { pb } from '../api';

export default {
  name: 'CampaignPreview',

  props: {
    isPost: { type: Boolean, default: false },

    // Template or campaign ID.
    id: { type: [String, Number], default: '' },
    title: { type: String, default: '' },

    // campaign | template.
    type: { type: String, default: '' },

    // campaign | tx.
    templateType: { type: String, default: '' },

    archiveMeta: { type: String, default: null },

    body: { type: String, default: '' },
    preheader: { type: [String, null], default: null },
    contentType: { type: String, default: '' },
    templateId: { type: [String, Number, null], default: null },
    isArchive: { type: Boolean, default: false },
  },

  data() {
    return {
      isVisible: true,
      isLoading: true,
      previewLoaded: false,
      previewHTML: '',
    };
  },

  methods: {
    close() {
      this.$emit('close');
      this.isVisible = false;
    },

    // On iframe load, kill the spinner.
    onLoaded() {
      if (this.previewLoaded) {
        this.isLoading = false;
      }
    },

    async loadPreview() {
      this.isLoading = true;
      this.previewLoaded = false;

      const { token } = pb.authStore;
      const options = {
        method: this.isPost ? 'POST' : 'GET',
        headers: {
          Accept: 'text/html',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
      };

      if (this.isPost) {
        const form = new URLSearchParams();
        if (this.templateId) {
          form.set('template_id', this.templateId);
        }
        if (this.contentType) {
          form.set('content_type', this.contentType);
        }
        if (this.templateType) {
          form.set('template_type', this.templateType);
        }
        if (this.archiveMeta) {
          form.set('archive_meta', this.archiveMeta);
        }
        if (this.body) {
          form.set('body', this.body);
        }
        if (this.preheader !== null && this.preheader !== undefined) {
          form.set('preheader', this.preheader);
        }
        options.body = form;
      }

      try {
        const response = await fetch(this.previewURL, {
          ...options,
          credentials: 'omit',
        });
        const html = await response.text();
        this.previewHTML = response.ok
          ? html
          : `<pre>${this.escapeHTML(html || `${response.status} ${response.statusText}`)}</pre>`;

        this.previewLoaded = true;
        this.isLoading = false;
      } catch (err) {
        const message = err && err.response ? JSON.stringify(err.response) : String(err);
        this.previewHTML = `<pre>${this.escapeHTML(message)}</pre>`;
        this.previewLoaded = true;
        this.isLoading = false;
      }
    },

    escapeHTML(value = '') {
      return String(value)
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;');
    },
  },

  computed: {
    previewURL() {
      let uri = 'about:blank';

      if (this.type === 'campaign') {
        uri = this.isArchive ? uris.previewCampaignArchive : uris.previewCampaign;
      } else if (this.type === 'template') {
        if (this.id) {
          uri = uris.previewTemplate;
        } else {
          uri = uris.previewRawTemplate;
        }
      }

      return uri.replace(':id', this.id);
    },
  },

  mounted() {
    this.$nextTick(() => {
      this.loadPreview();
    });
  },
};
</script>

<style scoped>
.preview-card {
  border-radius: 20px;
  overflow: hidden;
}

.preview-title {
  align-items: center;
  background:
    linear-gradient(180deg, rgba(var(--v-theme-surface), 1) 0%, rgba(var(--v-theme-surface), 0.96) 100%);
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  display: flex;
  min-height: 68px;
  padding: 0 24px;
}

.preview-title__text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-content {
  background:
    radial-gradient(circle at top, rgba(var(--v-theme-primary), 0.06), transparent 35%),
    linear-gradient(180deg, #f6f7fb 0%, #eef1f6 100%);
  min-height: 70vh;
  padding: 24px !important;
  position: relative;
}

.preview-stage {
  align-items: flex-start;
  display: flex;
  justify-content: center;
  min-height: calc(70vh - 48px);
}

.preview-iframe {
  background: #fff;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 18px;
  box-shadow: 0 24px 60px rgba(15, 23, 42, 0.12);
  height: calc(70vh - 48px);
  max-width: 900px;
  width: 100%;
}

.preview-actions {
  border-top: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  justify-content: flex-end;
  padding: 16px 20px;
}

@media (max-width: 959px) {
  .preview-content {
    min-height: 78vh;
    padding: 12px !important;
  }

  .preview-stage {
    min-height: calc(78vh - 24px);
  }

  .preview-iframe {
    border-radius: 12px;
    height: calc(78vh - 24px);
  }
}
</style>
