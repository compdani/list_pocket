<template>
  <section data-cy="whatsapp-channel-posts">
    <page-header :title="$t('whatsapp.posts.title')" />

    <v-alert type="warning" variant="tonal" density="comfortable" class="mb-4">
      {{ $t('whatsapp.posts.notice') }}
    </v-alert>
    <v-alert v-if="pacingWarning" type="warning" variant="tonal" density="comfortable" class="mb-4" data-cy="whatsapp-post-pacing">
      {{ pacingWarning }}
    </v-alert>

    <v-card variant="outlined" class="mb-6">
      <v-card-text>
        <v-select
          v-model="form.channel_id"
          :items="channels"
          item-title="name"
          item-value="id"
          :label="$t('whatsapp.posts.channel')"
          :loading="loadingChannels"
          :error-messages="channelsError"
          :no-data-text="$t('whatsapp.posts.noChannels')"
          variant="outlined"
          density="comfortable"
          class="mb-2"
          data-cy="whatsapp-post-channel"
        />
        <v-select
          v-model="form.kind"
          :items="kinds"
          :label="$t('whatsapp.posts.kind')"
          variant="outlined"
          density="comfortable"
          class="mb-2"
          data-cy="whatsapp-post-kind"
        />
        <v-text-field
          v-if="needsMedia"
          v-model="form.media_url"
          :label="$t('whatsapp.posts.mediaUrl')"
          variant="outlined"
          density="comfortable"
          class="mb-2"
          data-cy="whatsapp-post-media"
        />
        <v-text-field
          v-if="form.kind === 'link'"
          v-model="form.title"
          :label="$t('whatsapp.posts.linkTitle')"
          variant="outlined"
          density="comfortable"
          class="mb-2"
          data-cy="whatsapp-post-title"
        />
        <v-textarea
          v-model="form.body"
          :label="needsMedia ? $t('whatsapp.posts.caption') : $t('whatsapp.posts.body')"
          :hint="form.kind === 'link' ? $t('whatsapp.posts.linkHelp') : ''"
          persistent-hint
          rows="3"
          auto-grow
          variant="outlined"
          density="comfortable"
          class="mb-2"
          data-cy="whatsapp-post-body"
        />
        <v-text-field
          v-model="form.send_at"
          :label="$t('whatsapp.posts.sendAt')"
          :hint="$t('whatsapp.posts.sendAtHelp')"
          persistent-hint
          type="datetime-local"
          variant="outlined"
          density="comfortable"
          class="mb-4"
          data-cy="whatsapp-post-send-at"
        />
        <div class="d-flex flex-wrap ga-2">
          <v-btn color="primary" :loading="saving" data-cy="whatsapp-post-schedule" @click="save(true)">
            {{ form.send_at ? $t('whatsapp.posts.schedule') : $t('whatsapp.posts.postNow') }}
          </v-btn>
          <v-btn variant="tonal" :loading="saving" data-cy="whatsapp-post-draft" @click="save(false)">
            {{ $t('whatsapp.posts.saveDraft') }}
          </v-btn>
          <v-btn v-if="editingId" variant="text" @click="resetForm">{{ $t('globals.buttons.cancel') }}</v-btn>
        </div>
      </v-card-text>
    </v-card>

    <empty-placeholder v-if="!loading && posts.length === 0" />

    <v-card v-for="post in posts" :key="post.id" variant="outlined" class="mb-3" :data-cy="`whatsapp-post-${post.id}`">
      <v-card-text>
        <div class="d-flex flex-wrap align-center ga-2 mb-2">
          <v-chip size="small" :color="statusColor(post.status)" data-cy="whatsapp-post-status">
            {{ $t(`whatsapp.posts.status.${post.status}`) }}
          </v-chip>
          <span class="text-body-2">{{ post.channel_name || post.channel_id }}</span>
          <span v-if="post.send_at" class="text-caption text-medium-emphasis">{{ formatTime(post.send_at) }}</span>
        </div>
        <p v-if="post.kind === 'link'" class="text-body-2 font-weight-medium mb-1">{{ post.title }}</p>
        <p class="text-body-2 mb-2 post-body">{{ post.body || post.media_url }}</p>
        <v-alert v-if="post.error" type="error" variant="tonal" density="compact" class="mb-2" :text="post.error" />
        <p v-if="post.status === 'failed'" class="form-help mb-2">{{ $t('whatsapp.posts.retryHelp') }}</p>
        <div class="d-flex flex-wrap ga-2">
          <v-btn
            v-if="post.status === 'draft' || post.status === 'scheduled'"
            size="small"
            variant="tonal"
            @click="edit(post)"
          >
            {{ $t('globals.buttons.edit') }}
          </v-btn>
          <v-btn v-if="post.status === 'failed'" size="small" variant="tonal" color="primary" data-cy="whatsapp-post-retry" @click="retry(post)">
            {{ $t('whatsapp.posts.retry') }}
          </v-btn>
          <v-btn v-if="post.status !== 'sending'" size="small" variant="text" color="error" @click="remove(post)">
            {{ $t('globals.buttons.delete') }}
          </v-btn>
        </div>
      </v-card-text>
    </v-card>
  </section>
</template>

<script>
import dayjs from 'dayjs';
import {
  createWhatsAppChannelPost,
  deleteWhatsAppChannelPost,
  getWhatsAppChannelPosts,
  getWhatsAppChannels,
  retryWhatsAppChannelPost,
  updateWhatsAppChannelPost,
} from '../api';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import PageHeader from '../components/PageHeader.vue';

const blankForm = () => ({
  channel_id: null, kind: 'text', body: '', media_url: '', title: '', send_at: '',
});

export default {
  components: { EmptyPlaceholder, PageHeader },

  data() {
    return {
      channels: [],
      posts: [],
      form: blankForm(),
      editingId: null,
      loading: true,
      loadingChannels: true,
      channelsError: '',
      saving: false,
      pacingWarning: '',
    };
  },

  computed: {
    kinds() {
      return ['text', 'image', 'video', 'link'].map((value) => ({
        title: this.$t(`whatsapp.posts.kinds.${value}`),
        value,
      }));
    },

    needsMedia() {
      return this.form.kind === 'image' || this.form.kind === 'video';
    },
  },

  mounted() {
    this.loadChannels();
    this.loadPosts();
  },

  methods: {
    errorText(err) {
      return (err.response && err.response.message) || err.message;
    },

    statusColor(status) {
      return { sent: 'success', failed: 'error', scheduled: 'info' }[status];
    },

    // send_at is stored in UTC with no zone marker.
    formatTime(value) {
      return dayjs(`${value.replace(' ', 'T')}Z`).format('YYYY-MM-DD HH:mm');
    },

    async loadChannels() {
      this.loadingChannels = true;
      try {
        this.channels = (await getWhatsAppChannels()) || [];
      } catch (err) {
        this.channelsError = this.errorText(err);
      } finally {
        this.loadingChannels = false;
      }
    },

    async loadPosts() {
      this.loading = true;
      try {
        this.posts = (await getWhatsAppChannelPosts()) || [];
      } catch (err) {
        this.$utils.toast(this.errorText(err), 'error');
      } finally {
        this.loading = false;
      }
    },

    resetForm() {
      this.form = blankForm();
      this.editingId = null;
    },

    edit(post) {
      this.editingId = post.id;
      this.form = {
        channel_id: post.channel_id,
        kind: post.kind,
        body: post.body,
        media_url: post.media_url,
        title: post.title,
        send_at: post.send_at ? dayjs(`${post.send_at.replace(' ', 'T')}Z`).format('YYYY-MM-DDTHH:mm') : '',
      };
    },

    async save(schedule) {
      const channel = this.channels.find((c) => c.id === this.form.channel_id);
      const payload = {
        ...this.form,
        channel_name: channel ? channel.name : '',
        schedule,
        send_at: this.form.send_at ? dayjs(this.form.send_at).toISOString() : '',
      };
      this.saving = true;
      try {
        const out = this.editingId
          ? await updateWhatsAppChannelPost(this.editingId, payload)
          : await createWhatsAppChannelPost(payload);
        this.pacingWarning = out.pacing_warning ? this.$t('whatsapp.posts.pacing', { n: out.posts_that_day }) : '';
        this.resetForm();
        await this.loadPosts();
      } catch (err) {
        this.$utils.toast(this.errorText(err), 'error');
      } finally {
        this.saving = false;
      }
    },

    async retry(post) {
      try {
        await retryWhatsAppChannelPost(post.id);
        await this.loadPosts();
      } catch (err) {
        this.$utils.toast(this.errorText(err), 'error');
      }
    },

    remove(post) {
      this.$utils.confirm(this.$t('whatsapp.posts.deleteConfirm'), async () => {
        try {
          await deleteWhatsAppChannelPost(post.id);
          await this.loadPosts();
        } catch (err) {
          this.$utils.toast(this.errorText(err), 'error');
        }
      });
    },
  },
};
</script>

<style scoped>
.post-body {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
