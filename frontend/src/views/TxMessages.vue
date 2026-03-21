<template>
  <section class="tx-messages-page">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h4">
          Transactional Messages
          <span v-if="txMessages.total">({{ txMessages.total }})</span>
        </h1>
        <p class="text-medium-emphasis mt-2">
          Sent transactional message history with recipient association and engagement.
        </p>
      </div>
    </header>

    <div class="filters-bar">
      <v-text-field
        v-model="query"
        label="Search by subject, template, or recipient"
        variant="outlined"
        density="comfortable"
        hide-details
        prepend-inner-icon="mdi-magnify"
        class="search-field"
        @keyup.enter="fetchMessages(1)"
      />
      <v-btn color="primary" prepend-icon="mdi-magnify" @click="fetchMessages(1)">
        Search
      </v-btn>
      <v-btn variant="text" @click="resetFilters">
        Reset
      </v-btn>
    </div>

    <div class="table-wrap">
      <v-data-table
        :headers="tableHeaders"
        :items="txMessages.results || []"
        :loading="loading.txMessages"
        class="admin-data-table"
        item-value="id"
        hide-default-footer
      >
        <template #[`item.subject`]="{ item }">
          <button type="button" class="link-button" @click="openMessage(item)">
            {{ item.subject }}
          </button>
          <div class="text-caption text-medium-emphasis mt-1">
            {{ item.templateName || 'No template label' }}
          </div>
        </template>

        <template #[`item.subscriberEmail`]="{ item }">
          <div>{{ item.subscriberEmail }}</div>
          <div v-if="item.subscriberId" class="text-caption text-medium-emphasis">
            linked subscriber
          </div>
          <div v-else class="text-caption text-medium-emphasis">
            external-only
          </div>
        </template>

        <template #[`item.status`]="{ item }">
          <v-chip size="small" :color="statusColor(item.status)" variant="tonal">
            {{ item.status }}
          </v-chip>
        </template>

        <template #[`item.views`]="{ item }">
          <span class="metric">{{ item.views || 0 }}</span>
        </template>

        <template #[`item.clicks`]="{ item }">
          <span class="metric">{{ item.clicks || 0 }}</span>
        </template>

        <template #[`item.sentAt`]="{ item }">
          {{ item.sentAt ? $utils.niceDate(item.sentAt) : 'Pending' }}
        </template>

        <template #[`item.createdAt`]="{ item }">
          {{ $utils.niceDate(item.createdAt) }}
        </template>

        <template #[`item.actions`]="{ item }">
          <v-btn icon="mdi-eye-outline" size="x-small" variant="text" @click="openMessage(item)" />
        </template>
      </v-data-table>

      <empty-placeholder v-if="!loading.txMessages && (txMessages.results || []).length === 0" />
    </div>

    <div v-if="pageCount > 1" class="table-pagination">
      <v-pagination
        v-model="page"
        :length="pageCount"
        rounded="circle"
        @update:model-value="fetchMessages"
      />
    </div>

    <v-dialog v-model="detailOpen" max-width="1100">
      <v-card v-if="activeMessage">
        <v-card-title class="d-flex align-center justify-space-between ga-4 flex-wrap">
          <div>
            <div class="text-h6">{{ activeMessage.subject }}</div>
            <div class="text-body-2 text-medium-emphasis">
              {{ activeMessage.subscriberEmail }} • {{ activeMessage.status }}
            </div>
          </div>
          <v-btn icon="mdi-close" variant="text" @click="detailOpen = false" />
        </v-card-title>

        <v-card-text>
          <div class="detail-metrics">
            <v-sheet class="metric-card" border rounded>
              <div class="text-overline">Views</div>
              <div class="text-h5">{{ activeMessage.views || 0 }}</div>
            </v-sheet>
            <v-sheet class="metric-card" border rounded>
              <div class="text-overline">Clicks</div>
              <div class="text-h5">{{ activeMessage.clicks || 0 }}</div>
            </v-sheet>
            <v-sheet class="metric-card" border rounded>
              <div class="text-overline">Template</div>
              <div class="text-body-1">{{ activeMessage.templateName || 'N/A' }}</div>
            </v-sheet>
          </div>

          <div v-if="activeMessage.error" class="mt-6">
            <div class="text-subtitle-2 mb-2">Delivery Error</div>
            <v-alert type="error" variant="tonal">
              {{ activeMessage.error }}
            </v-alert>
          </div>

          <div class="mt-6">
            <div class="text-subtitle-2 mb-2">Clicked Links</div>
            <v-table density="comfortable">
              <thead>
                <tr>
                  <th>URL</th>
                  <th class="text-right">Clicks</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="link in activeMessage.linkStats || []" :key="link.url">
                  <td class="link-cell">{{ link.url }}</td>
                  <td class="text-right">{{ link.count }}</td>
                </tr>
                <tr v-if="!activeMessage.linkStats || activeMessage.linkStats.length === 0">
                  <td colspan="2" class="text-medium-emphasis">No tracked link clicks recorded.</td>
                </tr>
              </tbody>
            </v-table>
          </div>

          <div class="mt-6">
            <div class="text-subtitle-2 mb-2">Rendered Body</div>
            <div class="message-preview" v-html="activeMessage.body" />
          </div>
        </v-card-text>
      </v-card>
    </v-dialog>
  </section>
</template>

<script>
import { mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';

export default {
  name: 'TxMessagesPage',

  components: {
    EmptyPlaceholder,
  },

  data() {
    return {
      query: '',
      page: 1,
      perPage: 20,
      detailOpen: false,
      activeMessage: null,
    };
  },

  computed: {
    ...mapState(['txMessages', 'loading']),

    tableHeaders() {
      return [
        { title: 'Subject', key: 'subject' },
        { title: 'Recipient', key: 'subscriberEmail' },
        { title: 'Status', key: 'status' },
        { title: 'Views', key: 'views', align: 'end' },
        { title: 'Clicks', key: 'clicks', align: 'end' },
        { title: 'Sent', key: 'sentAt' },
        { title: 'Created', key: 'createdAt' },
        { title: '', key: 'actions', sortable: false, align: 'end', width: 80 },
      ];
    },

    pageCount() {
      if (!this.txMessages.perPage || !this.txMessages.total) {
        return 1;
      }
      return Math.max(1, Math.ceil(this.txMessages.total / this.txMessages.perPage));
    },
  },

  methods: {
    statusColor(status) {
      switch (status) {
        case 'sent':
          return 'success';
        case 'failed':
          return 'error';
        default:
          return 'warning';
      }
    },

    async fetchMessages(targetPage = this.page) {
      this.page = targetPage;
      await this.$api.getTxMessages({
        query: this.query,
        page: this.page,
        per_page: this.perPage,
      });
    },

    async openMessage(item) {
      this.activeMessage = await this.$api.getTxMessage(item.id);
      this.detailOpen = true;
    },

    resetFilters() {
      this.query = '';
      this.fetchMessages(1);
    },
  },

  mounted() {
    this.fetchMessages();
  },
};
</script>

<style scoped>
.tx-messages-page {
  display: grid;
  gap: 24px;
}

.filters-bar {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}

.search-field {
  min-width: min(100%, 420px);
}

.table-wrap {
  background: white;
  border: 1px solid #dce5f2;
  border-radius: 20px;
  overflow: hidden;
}

.table-pagination {
  display: flex;
  justify-content: center;
}

.metric {
  font-variant-numeric: tabular-nums;
}

.detail-metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.metric-card {
  padding: 16px;
}

.message-preview {
  border: 1px solid #dce5f2;
  border-radius: 16px;
  padding: 20px;
  background: #fff;
  overflow: auto;
}

.link-cell {
  max-width: 680px;
  overflow-wrap: anywhere;
}

@media (max-width: 900px) {
  .detail-metrics {
    grid-template-columns: 1fr;
  }
}
</style>
