<template>
  <div class="subscriber-activity">
    <div v-if="isLoading" class="d-flex justify-center align-center py-12">
      <v-progress-circular indeterminate color="primary" />
    </div>

    <div v-else>
      <v-row class="mb-6">
        <v-col cols="12" sm="4">
          <v-card class="text-center">
            <v-card-text>
              <p class="text-uppercase text-caption font-weight-bold">Total</p>
              <p class="text-h4">{{ total }}</p>
            </v-card-text>
          </v-card>
        </v-col>
        <v-col cols="12" sm="4">
          <v-card class="text-center">
            <v-card-text>
              <p class="text-uppercase text-caption font-weight-bold">Shown</p>
              <p class="text-h4">{{ events.length }}</p>
            </v-card-text>
          </v-card>
        </v-col>
        <v-col cols="12" sm="4">
          <v-card class="text-center">
            <v-card-text>
              <p class="text-uppercase text-caption font-weight-bold">Inbound</p>
              <p class="text-h4">{{ inboundCount }}</p>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>

      <div v-if="events.length > 0" class="timeline-stack">
        <v-card
          v-for="(event, idx) in events"
          :key="`${event.eventType}-${event.occurredAt}-${idx}`"
          class="timeline-item"
          elevation="0"
        >
          <v-card-text>
            <div class="timeline-item-head">
              <div class="d-flex align-center ga-2">
                <v-chip size="small" label :color="eventTypeColor(event.eventType)">
                  {{ eventTypeLabel(event.eventType) }}
                </v-chip>
                <v-chip size="small" variant="outlined" label>
                  {{ event.channel }}
                </v-chip>
              </div>
              <span class="text-caption text-medium-emphasis">{{ formatActivityTimestamp(event.occurredAt) }}</span>
            </div>

            <p class="mb-1 font-weight-medium">{{ eventTitle(event) }}</p>
            <p v-if="eventSubtitle(event)" class="mb-2 text-body-2 text-medium-emphasis">{{ eventSubtitle(event) }}</p>

            <div
              v-if="event.eventType === 'inbound_email_reply' && emailAttachments(event).length > 0"
              class="d-flex flex-wrap ga-2 mb-2"
            >
              <a
                v-for="(attachment, attachmentIdx) in emailAttachments(event)"
                :key="`${event.occurredAt}-${attachmentLabel(attachment)}-${attachmentIdx}`"
                :href="attachmentLink(attachment)"
                class="attachment-link"
                target="_blank"
                rel="noopener noreferrer"
              >
                {{ attachmentLabel(attachment) }}
              </a>
            </div>

            <details>
              <summary class="text-caption text-medium-emphasis">Details</summary>
              <pre class="event-metadata">{{ prettyMetadata(event.metadata) }}</pre>
            </details>
          </v-card-text>
        </v-card>

        <div v-if="hasMore" class="d-flex justify-center py-4">
          <v-btn :loading="isLoadingMore" variant="outlined" @click="loadMore">Load more</v-btn>
        </div>
      </div>

      <div v-else class="text-center text-grey py-12">
        <p>{{ $t('globals.messages.emptyState') }}</p>
      </div>
    </div>
  </div>
</template>

<script>
import dayjs from 'dayjs';
import { legacyActivityToTimeline } from './subscriberActivityTransform';

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
      isLoadingMore: false,
      events: [],
      total: 0,
      offset: 0,
      limit: 25,
      hasMore: false,
    };
  },

  computed: {
    inboundCount() {
      return this.events.filter((e) => e.eventType === 'inbound_sms' || e.eventType === 'inbound_email_reply').length;
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
    async fetchTimelinePage(offset) {
      try {
        return await this.$api.getSubscriberTimeline(this.subscriberId, {
          limit: this.limit,
          offset,
          sort: 'desc',
        });
      } catch (err) {
        // Backward compatibility: older servers may not expose /timeline yet.
        const status = typeof err?.status === 'number' ? err.status : 0;
        if (status !== 404) {
          throw err;
        }
        const legacy = await this.$api.getSubscriberActivity(this.subscriberId);
        return legacyActivityToTimeline(legacy);
      }
    },

    eventTypeColor(eventType) {
      switch (eventType) {
        case 'campaign_send':
          return 'primary';
        case 'campaign_view':
          return 'success';
        case 'link_click':
          return 'info';
        case 'inbound_sms':
          return 'orange';
        case 'inbound_email_reply':
          return 'deep-purple';
        default:
          return 'default';
      }
    },

    eventTypeLabel(eventType) {
      switch (eventType) {
        case 'campaign_send':
          return 'Campaign Sent';
        case 'campaign_view':
          return 'Campaign View';
        case 'link_click':
          return 'Link Click';
        case 'inbound_sms':
          return 'Inbound SMS';
        case 'inbound_email_reply':
          return 'Inbound Email';
        default:
          return eventType || 'Event';
      }
    },

    eventTitle(event) {
      const md = event.metadata || {};
      if (event.eventType === 'inbound_sms') {
        return md.messageBody || md.fromNumber || 'Inbound SMS received';
      }
      if (event.eventType === 'inbound_email_reply') {
        return md.subject || md.fromAddress || 'Inbound email reply';
      }
      if (event.eventType === 'campaign_send') {
        return md.subject || md.campaignName || 'Campaign send';
      }
      if (event.eventType === 'campaign_view') {
        return md.subject || md.campaignName || 'Campaign view';
      }
      if (event.eventType === 'link_click') {
        return md.url || md.subject || 'Link click';
      }
      return 'Timeline event';
    },

    eventSubtitle(event) {
      const md = event.metadata || {};
      if (event.eventType === 'campaign_send') {
        return [md.campaignName, event.status].filter(Boolean).join(' · ');
      }
      if (event.eventType === 'campaign_view') {
        return `Views: ${md.viewCount || 0}`;
      }
      if (event.eventType === 'link_click') {
        return `Clicks: ${md.clickCount || 0}`;
      }
      if (event.eventType === 'inbound_sms') {
        return md.fromNumber || '';
      }
      if (event.eventType === 'inbound_email_reply') {
        return md.fromAddress || '';
      }
      return '';
    },

    emailAttachments(event) {
      const md = event.metadata || {};
      if (!Array.isArray(md.attachments)) {
        return [];
      }
      return md.attachments.filter((item) => item && typeof item === 'object');
    },

    attachmentLabel(attachment) {
      return attachment.filename
        || attachment.originalName
        || attachment.original_name
        || attachment.fileName
        || attachment.file_name
        || 'Attachment';
    },

    attachmentLink(attachment) {
      return attachment.downloadUrl || attachment.download_url || '#';
    },

    async hydrateAttachmentLinks(events) {
      const inboundEmailEvents = events.filter((event) => event.eventType === 'inbound_email_reply');
      await Promise.all(inboundEmailEvents.map(async (event) => {
        const md = event.metadata || {};
        const hasInline = Array.isArray(md.attachments) && md.attachments.length > 0;
        if (hasInline || !md.inboundEmailReplyId) {
          return;
        }
        try {
          const data = await this.$api.getInboundEmailReplyAttachments(md.inboundEmailReplyId);
          const attachments = Array.isArray(data) ? data : (Array.isArray(data.data) ? data.data : []);
          if (!event.metadata || typeof event.metadata !== 'object') {
            event.metadata = {};
          }
          event.metadata.attachments = attachments;
        } catch {
          // Non-blocking: timeline should still render even if attachment lookup fails.
        }
      }));
    },

    prettyMetadata(metadata) {
      try {
        return JSON.stringify(metadata || {}, null, 2);
      } catch {
        return '{}';
      }
    },

    formatActivityTimestamp(value) {
      if (!value) {
        return '';
      }

      return dayjs(value).format('MM-DD-YY hh:mm A');
    },

    async getActivity() {
      if (!this.subscriberId) {
        this.events = [];
        this.total = 0;
        this.offset = 0;
        this.hasMore = false;
        this.isLoading = false;
        return;
      }

      this.isLoading = true;
      this.offset = 0;
      try {
        const data = await this.fetchTimelinePage(0);
        this.events = Array.isArray(data.events) ? data.events : [];
        await this.hydrateAttachmentLinks(this.events);
        this.total = data.total || 0;
        this.offset = this.events.length;
        this.hasMore = Boolean(data.hasMore);
      } finally {
        this.isLoading = false;
      }
    },

    async loadMore() {
      if (!this.hasMore || this.isLoadingMore) {
        return;
      }
      this.isLoadingMore = true;
      try {
        const data = await this.fetchTimelinePage(this.offset);
        const next = Array.isArray(data.events) ? data.events : [];
        await this.hydrateAttachmentLinks(next);
        this.events = [...this.events, ...next];
        this.offset = this.events.length;
        this.total = data.total || this.total;
        this.hasMore = Boolean(data.hasMore);
      } finally {
        this.isLoadingMore = false;
      }
    },
  },
};
</script>

<style scoped>
.timeline-stack {
  display: grid;
  gap: 12px;
}

.timeline-item {
  border: 1px solid #e8edf7;
  border-radius: 12px;
}

.timeline-item-head {
  align-items: center;
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
}

.event-metadata {
  background: #f8fafc;
  border-radius: 8px;
  margin-top: 8px;
  overflow-x: auto;
  padding: 10px;
  white-space: pre-wrap;
}

.attachment-link {
  align-items: center;
  border: 1px solid #d6deed;
  border-radius: 999px;
  color: #1e4eb3;
  display: inline-flex;
  font-size: 12px;
  font-weight: 600;
  padding: 4px 10px;
  text-decoration: none;
}

.attachment-link:hover {
  background: #eef4ff;
}
</style>
