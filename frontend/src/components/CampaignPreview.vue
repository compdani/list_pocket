<template>
  <div>
    <b-modal scroll="keep" @close="close" :aria-modal="true" :active="isVisible">
      <div>
        <div class="modal-card" style="width: auto">
          <header class="modal-card-head">
            <h4>{{ title }}</h4>
          </header>
        </div>
        <section expanded class="modal-card-body preview">
          <b-loading :active="isLoading" :is-full-page="false" />
          <iframe id="iframe" name="iframe" ref="iframe" :title="title" :srcdoc="previewHTML"
            @load="onLoaded" sandbox="allow-scripts" />
        </section>
        <footer class="modal-card-foot has-text-right">
          <b-button @click="close">
            {{ $t('globals.buttons.close') }}
          </b-button>
        </footer>
      </div>
    </b-modal>
  </div>
</template>

<script>
import { uris } from '../constants';
import { pb } from '../api';

export default {
  name: 'CampaignPreview',

  props: {
    isPost: { type: Boolean, default: false },

    // Template or campaign ID.
    id: { type: Number, default: 0 },
    title: { type: String, default: '' },

    // campaign | template.
    type: { type: String, default: '' },

    // campaign | tx.
    templateType: { type: String, default: '' },

    archiveMeta: { type: String, default: null },

    body: { type: String, default: '' },
    contentType: { type: String, default: '' },
    templateId: { type: [Number, null], default: null },
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
          ...(token ? { Authorization: token } : {}),
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
        options.body = form;
      }

      try {
        const response = await fetch(this.previewURL, {
          ...options,
          credentials: 'omit',
        });
        const html = await response.text();
        this.previewHTML = html;

        this.previewLoaded = true;
        this.isLoading = false;
      } catch (err) {
        const message = err && err.response ? JSON.stringify(err.response) : String(err);
        this.previewHTML = message;
        this.previewLoaded = true;
        this.isLoading = false;
      }
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
