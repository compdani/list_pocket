<template>
  <section class="media-files">
    <div class="media-header">
      <div>
        <h1 class="text-h6 media-title">
          {{ heading }}
          <span v-if="items.length > 0">({{ items.length }})</span>
          <span class="text-medium-emphasis"> / {{ providerLabel }}</span>
        </h1>
        <p class="media-subtitle">
          {{ isModal ? 'Pick an asset to insert into your content.' : 'Browse and manage uploaded assets.' }}
        </p>
      </div>
    </div>

    <v-progress-linear
      v-if="isProcessing || isLoading"
      indeterminate
      color="primary"
      class="my-3"
    />

    <section class="media-shell gallery">
      <v-row class="media-toolbar" dense>
        <v-col cols="12" md="9">
          <v-form @submit.prevent="onQueryMedia" class="search">
            <div class="media-search-row">
              <v-text-field
                v-model="queryParams.query"
                name="query"
                prepend-inner-icon="mdi-magnify"
                variant="outlined"
                density="comfortable"
                hide-details
                ref="query"
                data-cy="query"
              />
              <v-btn type="submit" color="primary" icon="mdi-magnify" data-cy="btn-query" />
            </div>
          </v-form>
        </v-col>
        <v-col v-if="$can('media:manage')" cols="12" md="auto">
          <v-btn
            @click="onToggleForm"
            prepend-icon="mdi-file-upload-outline"
            color="primary"
            variant="tonal"
            class="media-upload-toggle"
            data-cy="btn-toggle-upload"
          >
            {{ $t('media.upload') }}
          </v-btn>
        </v-col>
      </v-row>

      <v-expand-transition>
        <div v-if="$can('media:manage') && showUploadForm" class="upload-panel">
          <v-form @submit.prevent="onSubmit" data-cy="upload">
            <div class="upload-panel__header">
              <div>
                <h2 class="upload-panel__title">{{ $t('media.upload') }}</h2>
                <p class="upload-panel__help">{{ $t('media.uploadHelp') }}</p>
              </div>
            </div>

            <v-file-input
              v-model="form.files"
              :label="$t('media.upload')"
              :hint="null"
              variant="outlined"
              density="comfortable"
              prepend-icon="mdi-file-upload-outline"
              accept=".png,.jpg,.jpeg,.gif,.svg"
              multiple
              show-size
              class="upload-input"
            />

            <div class="upload-chip-list" v-if="uploadFiles.length > 0">
              <v-chip
                v-for="(f, i) in uploadFiles"
                :key="`${f.name}-${i}`"
                size="small"
                class="upload-chip"
                closable
                @click:close="removeUploadFile(i)"
              >
                {{ f.name }}
              </v-chip>
            </div>

            <div class="upload-panel__actions">
              <v-btn
                type="submit"
                color="primary"
                prepend-icon="mdi-file-upload-outline"
                :disabled="uploadFiles.length === 0"
                :loading="isProcessing"
              >
                {{ $tc('media.upload') }}
              </v-btn>
            </div>
          </v-form>
        </div>
      </v-expand-transition>

      <!-- Pagination -->
      <div v-if="total > perPage" class="pagination-wrapper mt-5">
        <v-pagination
          :length="pageCount"
          :model-value="queryParams.page"
          rounded="circle"
          total-visible="7"
          @update:model-value="onPageChange"
        />
      </div>

      <div v-if="isLoading" class="text-center py-6">
        <v-progress-circular indeterminate color="primary" />
      </div>
      <div v-else-if="items.length > 0" class="grid">
        <div v-for="item in items" :key="item.id" class="item">
          <div class="thumb">
            <a @click="(e) => onMediaSelect(item, e)" :href="item.url" target="_blank" rel="noopener noreferer"
              class="thumb-link">
              <div class="thumb-container">
                <img v-if="item.thumbUrl" :src="item.thumbUrl" :title="item.filename" :alt="item.filename" />
                <div v-else class="thumb-placeholder">
                  <span class="file-ext">
                    {{ item.filename.split(".").pop().toUpperCase() }}
                  </span>
                </div>
              </div>
            </a>
            <div class="actions">
              <a href="#" @click.prevent="$utils.confirm(null, () => onDeleteMedia(item.id))" data-cy="btn-delete"
                :aria-label="$t('globals.buttons.delete')" class="delete-btn">
                <v-icon icon="mdi-trash-can-outline" size="small" />
              </a>
            </div>
          </div>
          <div class="info">
            <p class="filename" :title="item.filename">{{ item.filename }}</p>
            <p class="date">{{ $utils.niceDate(item.createdAt, false) }}</p>
          </div>
        </div>
      </div>

      <!-- Empty State -->
      <div v-else-if="!isLoading">
        <empty-placeholder />
      </div>

      <!-- Pagination -->
      <div v-if="total > perPage" class="pagination-wrapper mt-5">
        <v-pagination
          :length="pageCount"
          :model-value="queryParams.page"
          rounded="circle"
          total-visible="7"
          @update:model-value="onPageChange"
        />
      </div>
    </section>
  </section>
</template>

<script>
import { mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';

export default {
  emits: ['selected', 'close'],

  components: {
    EmptyPlaceholder,
  },

  name: 'Media',

  props: {
    isModal: Boolean,
    type: { type: String, default: '' },
  },

  data() {
    return {
      form: {
        files: [],
      },
      toUpload: 0,
      uploaded: 0,
      showUploadForm: false,
      isPictureLoading: false,
      pictureResults: [],
      pictureTotal: 0,
      picturePerPage: 20,

      queryParams: {
        page: 1,
        query: '',
      },
    };
  },

  methods: {
    mapPictureRecord(record) {
      const fileName = Array.isArray(record.file) ? record.file[0] : record.file;
      const isImage = /\.(avif|gif|jpe?g|png|svg|webp)$/i.test(fileName || '');

      return {
        id: record.id,
        filename: record.original_name || fileName,
        createdAt: record.created,
        url: this.$api.pb.files.getURL(record, fileName),
        thumbUrl: isImage ? this.$api.pb.files.getURL(record, fileName, { thumb: '300x0' }) : '',
      };
    },

    removeUploadFile(i) {
      if (!Array.isArray(this.form.files)) {
        this.form.files = this.uploadFiles;
      }

      this.form.files.splice(i, 1);
    },

    async getMedia() {
      if (this.isLegacyMode) {
        await this.$api.getMedia({
          page: this.queryParams.page,
          query: this.queryParams.query,
        });
        return;
      }

      this.isPictureLoading = true;
      try {
        const res = await this.$api.listPictures({
          page: this.queryParams.page,
          perPage: this.picturePerPage,
          query: this.queryParams.query,
        });
        this.pictureResults = (res.items || []).map((item) => this.mapPictureRecord(item));
        this.pictureTotal = res.totalItems || 0;
        this.picturePerPage = res.perPage || this.picturePerPage;
      } finally {
        this.isPictureLoading = false;
      }
    },

    onToggleForm() {
      this.showUploadForm = !this.showUploadForm;
      this.$utils.setPref('media.upload', this.showUploadForm);
    },

    onQueryMedia() {
      this.queryParams.page = 1;
      this.getMedia();
    },

    onMediaSelect(m, e) {
      // If the component is open in the modal mode, close the modal and
      // fire the selection event.
      // Otherwise, do nothing and let the image open like a normal link.
      if (this.isModal) {
        e.preventDefault();
        this.$emit('selected', m);
        this.$emit('close');
      }
    },

    async onSubmit() {
      const files = this.uploadFiles;
      this.toUpload = files.length;

      if (this.isLegacyMode) {
        for (let i = 0; i < this.toUpload; i += 1) {
          const params = new FormData();
          params.set('file', files[i]);
          this.$api.uploadMedia(params).then(() => {
            this.onUploaded();
          }, () => {
            this.onUploaded();
          });
        }
        return;
      }

      try {
        await Promise.allSettled(files.map((file) => this.$api.uploadPicture(file)));
      } finally {
        this.toUpload = 0;
        this.uploaded = 0;
        this.form.files = [];
        this.getMedia();
      }
    },

    onDeleteMedia(id) {
      const promise = this.isLegacyMode ? this.$api.deleteMedia(id) : this.$api.deletePicture(id);
      promise.then(() => {
        this.getMedia();
      });
    },

    onUploaded() {
      this.uploaded += 1;
      if (this.uploaded >= this.toUpload) {
        this.toUpload = 0;
        this.uploaded = 0;
        this.form.files = [];

        this.getMedia();
      }
    },

    onPageChange(p) {
      this.queryParams.page = p;
      this.getMedia();
    },
  },

  computed: {
    ...mapState(['loading', 'media', 'serverConfig']),

    uploadFiles() {
      if (Array.isArray(this.form.files)) {
        return this.form.files;
      }

      return this.form.files ? [this.form.files] : [];
    },

    isLegacyMode() {
      return this.type === 'legacy-attachment';
    },

    heading() {
      return this.isLegacyMode ? this.$t('media.title') : 'Pictures';
    },

    providerLabel() {
      return this.isLegacyMode ? this.serverConfig.media_provider : 'PocketBase';
    },

    items() {
      return this.isLegacyMode ? (this.media.results || []) : this.pictureResults;
    },

    total() {
      return this.isLegacyMode ? this.media.total : this.pictureTotal;
    },

    perPage() {
      return this.isLegacyMode ? this.media.perPage : this.picturePerPage;
    },

    pageCount() {
      const total = Number(this.total) || 0;
      const perPage = Number(this.perPage) || 1;
      return Math.max(1, Math.ceil(total / perPage));
    },

    isLoading() {
      return this.isLegacyMode ? this.loading.media : this.isPictureLoading;
    },

    isProcessing() {
      if (this.toUpload > 0 && this.uploaded < this.toUpload) {
        return true;
      }
      return false;
    },
  },

  created() {
    this.$events.$on('page.refresh', this.getMedia);
  },

  destroyed() {
    this.$events.$off('page.refresh', this.getMedia);
  },

  mounted() {
    this.getMedia();

    if (this.$utils.getPref('media.upload')) {
      this.showUploadForm = true;
    }
  },
};
</script>
