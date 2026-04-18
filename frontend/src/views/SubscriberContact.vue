<template>
  <section class="subscriber-contact-page">
    <header class="page-header mb-4">
      <v-row align="center" class="ma-0">
        <v-col cols="12" md="8" class="px-0">
          <div class="header-top-row">
            <v-btn
              variant="text"
              prepend-icon="mdi-arrow-left"
              class="text-none px-0"
              @click="goBack"
            >
              {{ $t('globals.terms.subscribers') }}
            </v-btn>
          </div>
          <h1 class="text-h5 font-weight-semibold mb-1">
            {{ subscriber.name || subscriber.email || $t('subscribers.newSubscriber') }}
          </h1>
          <p class="text-medium-emphasis mb-0" v-if="subscriber.id">
            {{ $t('globals.fields.id') }}: {{ subscriber.id }}
          </p>
        </v-col>
      </v-row>
    </header>

    <div v-if="isLoading" class="state-wrap">
      <v-progress-circular indeterminate color="primary" />
    </div>

    <div v-else-if="loadError" class="state-wrap">
      <v-alert type="error" variant="tonal" class="mb-3">
        {{ loadError }}
      </v-alert>
      <v-btn color="primary" variant="flat" @click="loadSubscriber">
        {{ $t('globals.buttons.retry') }}
      </v-btn>
    </div>

    <v-row v-else class="contact-layout" align="start">
      <v-col cols="12" lg="5" class="left-pane">
        <subscriber-form
          :data="subscriber"
          :is-editing="true"
          :is-page-mode="true"
          :hide-activity-tab="true"
          :close-on-save="false"
          :close-label="$t('globals.buttons.close')"
          @finished="refreshSubscriber"
          @close="goBack"
        />
      </v-col>

      <v-col cols="12" lg="7" class="right-pane">
        <v-card class="timeline-card" elevation="0">
          <v-card-title class="d-flex align-center justify-space-between">
            <span>{{ $t('subscribers.activity') }}</span>
            <v-btn size="small" variant="text" prepend-icon="mdi-refresh" @click="refreshActivity">
              {{ $t('globals.buttons.refresh') }}
            </v-btn>
          </v-card-title>
          <v-divider />
          <v-card-text>
            <subscriber-activity :subscriber-id="subscriber.id" :key="activityKey" />
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </section>
</template>

<script>
import SubscriberForm from './SubscriberForm.vue';
import SubscriberActivity from '../components/SubscriberActivity.vue';

export default {
  name: 'SubscriberContact',
  components: {
    SubscriberForm,
    SubscriberActivity,
  },
  data() {
    return {
      isLoading: false,
      loadError: '',
      subscriber: {
        lists: [],
        attribs: {},
      },
      activityKey: 0,
    };
  },
  methods: {
    async loadSubscriber() {
      const id = String(this.$route.params.id || '').trim();
      if (!id) {
        this.$router.replace({ name: 'subscribers' });
        return;
      }

      this.isLoading = true;
      this.loadError = '';
      try {
        const data = await this.$api.getSubscriber(id);
        this.subscriber = {
          ...data,
          lists: Array.isArray(data.lists) ? data.lists : [],
          attribs: data.attribs && typeof data.attribs === 'object' ? data.attribs : {},
        };
      } catch (e) {
        this.loadError = e?.message || this.$t('globals.messages.errorFetching');
      } finally {
        this.isLoading = false;
      }
    },

    async refreshSubscriber() {
      await this.loadSubscriber();
    },

    refreshActivity() {
      this.activityKey += 1;
    },

    goBack() {
      if (window.history.length > 1) {
        this.$router.back();
        return;
      }
      this.$router.push({ name: 'subscribers' });
    },
  },
  mounted() {
    this.loadSubscriber();
  },
  watch: {
    '$route.params.id'() {
      this.loadSubscriber();
    },
  },
};
</script>

<style scoped>
.subscriber-contact-page {
  --contact-border: #dce5f2;
}

.header-top-row {
  align-items: center;
  display: flex;
  gap: 8px;
}

.contact-layout {
  margin: 0;
}

.left-pane,
.right-pane {
  padding-top: 0;
}

.timeline-card {
  background: #fff;
  border: 1px solid var(--contact-border);
  border-radius: 16px;
}

.state-wrap {
  align-items: center;
  display: flex;
  flex-direction: column;
  gap: 12px;
  justify-content: center;
  min-height: 240px;
}

@media (max-width: 1264px) {
  .left-pane,
  .right-pane {
    padding-left: 0;
    padding-right: 0;
  }
}
</style>
