<template>
  <div class="subscriber-activity">
    <div v-if="isLoading" class="d-flex justify-center align-center py-12">
      <v-progress-circular indeterminate color="primary" />
    </div>

    <div v-else>
      <!-- Summary Stats -->
      <v-row class="mb-6">
        <v-col cols="12" sm="4">
          <v-card class="text-center">
            <v-card-text>
              <p class="text-uppercase text-caption font-weight-bold">
                {{ $t('subscribers.activity.summarySent') }}
              </p>
              <p class="text-h4">{{ countSent }}</p>
            </v-card-text>
          </v-card>
        </v-col>
        <v-col cols="12" sm="4">
          <v-card class="text-center">
            <v-card-text>
              <p class="text-uppercase text-caption font-weight-bold">
                {{ $t('campaigns.views') }}
              </p>
              <p class="text-h4">{{ totalViews }}</p>
            </v-card-text>
          </v-card>
        </v-col>
        <v-col cols="12" sm="4">
          <v-card class="text-center">
            <v-card-text>
              <p class="text-uppercase text-caption font-weight-bold">
                {{ $t('campaigns.clicks') }}
              </p>
              <p class="text-h4">{{ totalClicks }}</p>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>

      <!-- Campaign sends (send ledger) -->
      <div class="mb-6">
        <h5 class="text-h5 mb-4">
          {{ $t('subscribers.activity.campaignSends') }}
        </h5>

        <v-data-table
          v-if="activity.campaignSends && activity.campaignSends.length > 0"
          :items="activity.campaignSends"
          :headers="campaignSendsHeaders"
          hover
          sort-by="updated"
          sort-order="desc"
          :page-size="10"
          class="campaign-sends-table"
        >
          <template #item.name="{ item }">
            <div v-if="item.uuid">
              <router-link :to="{ name: 'campaign', params: { id: item.id } }">
                {{ item.name }}
              </router-link>
              <p class="text-caption text-grey">{{ item.subject }}</p>
            </div>
            <div v-else>
              <em class="text-grey">{{ $t('subscribers.activity.campaignDeleted') }}</em>
            </div>
          </template>
          <template #item.status="{ item }">
            <v-chip label size="small" :color="sendStatusColor(item.status)">
              {{ sendStatusLabel(item.status) }}
            </v-chip>
          </template>
          <template #item.updated="{ item }">
            <span v-if="item.updated">
              {{ formatActivityTimestamp(item.updated) }}
            </span>
          </template>
        </v-data-table>
        <div v-else class="text-center text-grey py-12">
          <p>{{ $t('globals.messages.emptyState') }}</p>
        </div>
      </div>

      <!-- Campaign Views Section -->
      <div class="mb-6">
        <h5 class="text-h5 mb-4">
          {{ $t('campaigns.views') }}
        </h5>

        <v-data-table
          v-if="activity.campaignViews && activity.campaignViews.length > 0"
          :items="activity.campaignViews"
          :headers="campaignViewsHeaders"
          hover
          sort-by="lastViewedAt"
          sort-order="desc"
          :page-size="10"
          class="campaign-views-table"
        >
          <template #item.name="{ item }">
            <div v-if="item.uuid">
              <router-link :to="{ name: 'campaign', params: { id: item.id } }">
                {{ item.name }}
              </router-link>
              <p class="text-caption text-grey">{{ item.subject }}</p>
            </div>
            <div v-else>
              <em class="text-grey">{{ $t('subscribers.activity.campaignDeleted') }}</em>
            </div>
          </template>
          <template #item.viewCount="{ item }">
            <v-chip label small>{{ item.viewCount }}</v-chip>
          </template>
          <template #item.lastViewedAt="{ item }">
            <span v-if="item.lastViewedAt">
              {{ formatActivityTimestamp(item.lastViewedAt) }}
            </span>
          </template>
        </v-data-table>
        <div v-else class="text-center text-grey py-12">
          <p>{{ $t('globals.messages.emptyState') }}</p>
        </div>
      </div>

      <!-- Link Clicks Section -->
      <div class="mb-6">
        <h5 class="text-h5 mb-4 mt-6">
          {{ $t('campaigns.clicks') }}
        </h5>

        <v-data-table
          v-if="activity.linkClicks && activity.linkClicks.length > 0"
          :items="activity.linkClicks"
          :headers="linkClicksHeaders"
          hover
          sort-by="lastClickedAt"
          sort-order="desc"
          :page-size="10"
          class="link-clicks-table"
        >
          <template #item.url="{ item }">
            <a :href="item.url" target="_blank" rel="noopener noreferrer" class="link-click-url">
              {{ item.url }}
            </a>
          </template>
          <template #item.campaignName="{ item }">
            <div v-if="item.campaignUuid">
              <router-link :to="{ name: 'campaign', params: { id: item.campaignId } }">
                {{ item.campaignSubject || item.campaignName }}
              </router-link>
            </div>
            <div v-else>
              &mdash;
            </div>
          </template>
          <template #item.clickCount="{ item }">
            <v-chip label small>{{ item.clickCount }}</v-chip>
          </template>
          <template #item.lastClickedAt="{ item }">
            <span v-if="item.lastClickedAt">
              {{ formatActivityTimestamp(item.lastClickedAt) }}
            </span>
          </template>
        </v-data-table>
        <div v-else class="text-center text-grey py-12">
          <p>{{ $t('globals.messages.emptyState') }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import dayjs from 'dayjs';

export default {
  props: {
    subscriberId: {
      type: [Number, String],
      required: true,
    },
  },

  data() {
    return {
      isLoading: false,
      activity: {
        campaignSends: [],
        campaignViews: [],
        linkClicks: [],
      },
      campaignSendsHeaders: [
        { key: 'name', title: this.$tc('globals.terms.campaign', 1), sortable: true },
        { key: 'status', title: this.$t('subscribers.activity.sendStatus'), sortable: true },
        { key: 'updated', title: this.$t('globals.fields.updatedAt'), sortable: true },
      ],
      campaignViewsHeaders: [
        { key: 'name', title: this.$tc('globals.terms.campaign', 1), sortable: true },
        { key: 'viewCount', title: this.$t('campaigns.views'), sortable: true, align: 'end' },
        { key: 'lastViewedAt', title: this.$t('globals.fields.createdAt'), sortable: true },
      ],
      linkClicksHeaders: [
        { key: 'url', title: this.$t('globals.terms.url'), sortable: true },
        { key: 'campaignName', title: this.$tc('globals.terms.campaign', 1), sortable: true },
        { key: 'clickCount', title: this.$t('campaigns.clicks'), sortable: true, align: 'end' },
        { key: 'lastClickedAt', title: this.$t('globals.fields.createdAt'), sortable: true },
      ],
    };
  },

  computed: {
    countSent() {
      if (!this.activity.campaignSends || !this.activity.campaignSends.length) {
        return 0;
      }
      return this.activity.campaignSends.filter((r) => r.status === 'sent').length;
    },

    totalViews() {
      if (!this.activity.campaignViews) return 0;
      return this.activity.campaignViews.reduce((sum, v) => sum + (v.viewCount || 0), 0);
    },

    totalClicks() {
      if (!this.activity.linkClicks) return 0;
      return this.activity.linkClicks.reduce((sum, c) => sum + (c.clickCount || 0), 0);
    },
  },

  mounted() {
    this.getActivity();
  },

  watch: {
    subscriberId() {
      this.getActivity();
    },
  },

  methods: {
    sendStatusColor(status) {
      switch (status) {
        case 'sent':
          return 'success';
        case 'inflight':
          return 'warning';
        case 'pending':
          return 'surface-variant';
        case 'skipped':
          return 'grey';
        default:
          return 'default';
      }
    },

    sendStatusLabel(status) {
      if (!status) {
        return '—';
      }
      const key = `subscribers.activity.sendStatus.${status}`;
      const translated = this.$t(key);
      return translated === key ? status : translated;
    },

    formatActivityTimestamp(value) {
      if (!value) {
        return '';
      }

      return dayjs(value).format('MM-DD-YY hh:mm A');
    },

    getActivity() {
      if (!this.subscriberId) {
        this.activity = {
          campaignSends: [],
          campaignViews: [],
          linkClicks: [],
        };
        this.isLoading = false;
        return;
      }

      this.isLoading = true;
      this.$api.getSubscriberActivity(this.subscriberId).then((data) => {
        this.activity = {
          campaignSends: data.campaignSends || [],
          campaignViews: data.campaignViews || [],
          linkClicks: data.linkClicks || [],
        };
        this.isLoading = false;
      }).catch(() => {
        this.isLoading = false;
      });
    },
  },
};
</script>
