<template>
  <div class="subscriber-activity">
    <div v-if="isLoading" class="d-flex justify-center align-center py-12">
      <v-progress-circular indeterminate color="primary" />
    </div>

    <div v-else>
      <div v-if="events.length > 0" class="timeline-stack">
        <article
          v-for="(event, idx) in events"
          :key="`${event.eventType}-${event.occurredAt}-${idx}`"
          class="timeline-item"
        >
          <span class="timeline-dot" :class="`text-${eventTypeColor(event.eventType)}`">
            <v-icon :icon="eventTypeIcon(event)" size="16" />
          </span>
          <div class="timeline-item-body">
            <div class="timeline-item-head">
              <span class="text-caption font-weight-medium">{{ eventTypeLabel(event) }}</span>
              <span class="text-caption text-medium-emphasis">{{ formatActivityTimestamp(event.occurredAt) }}</span>
            </div>

            <p class="mb-0 timeline-title">{{ eventTitle(event) }}</p>
            <plugin-slot name="subscriber.activity.item" :context="{ event }" />
            <p v-if="eventSubtitle(event)" class="mb-0 text-body-2 text-medium-emphasis">{{ eventSubtitle(event) }}</p>

            <div
              v-if="event.eventType === 'inbound_email_reply' && emailAttachments(event).length > 0"
              class="d-flex flex-wrap ga-2 mt-1"
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
          </div>
        </article>

        <div v-if="hasMore" class="d-flex justify-center py-4">
          <v-btn :loading="isLoadingMore" variant="outlined" @click="loadMore">Load more</v-btn>
        </div>
      </div>

      <div v-else class="text-center text-medium-emphasis py-12">
        <p>{{ $t('globals.messages.emptyState') }}</p>
      </div>
    </div>
  </div>
</template>

<script>
import dayjs from 'dayjs';
import { legacyActivityToTimeline } from './subscriberActivityTransform';
import { channelIcon } from '../utils/crm.mjs';

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

    eventTypeIcon(event) {
      switch (event.eventType) {
        case 'campaign_send':
          return 'mdi-bullhorn-outline';
        case 'campaign_view':
          return 'mdi-eye-outline';
        case 'link_click':
          return 'mdi-cursor-default-click-outline';
        case 'inbound_sms':
          return 'mdi-cellphone-message';
        case 'inbound_email_reply':
          return 'mdi-email-open-outline';
        case 'channel_event':
          return channelIcon(event.channel);
        case 'board_move':
          return 'mdi-view-column-outline';
        default:
          return 'mdi-comment-text-outline';
      }
    },

    eventTypeLabel(event) {
      switch (event.eventType) {
        case 'campaign_send':
          return 'Campaign sent';
        case 'campaign_view':
          return 'Campaign viewed';
        case 'link_click':
          return 'Link clicked';
        case 'inbound_sms':
          return 'SMS received';
        case 'inbound_email_reply':
          return 'Email received';
        case 'channel_event': {
          const direction = (event.metadata || {}).direction;
          const key = direction === 'outbound' ? 'Sent' : 'Received';
          return `${key} · ${String(event.channel || '').replace(/^ch:/, '')}`;
        }
        case 'board_move':
          return 'Board';
        default:
          return event.eventType || 'Event';
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
      if (event.eventType === 'channel_event') {
        return md.body || `${md.direction || ''} ${md.kind || ''}`.trim() || md.address;
      }
      if (event.eventType === 'board_move') {
        if (event.status === 'added') return `Added to ${md.toStage} on ${md.board}`;
        if (event.status === 'removed') return `Removed from ${md.fromStage} on ${md.board}`;
        return `Moved from ${md.fromStage} to ${md.toStage} on ${md.board}`;
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
      if (event.eventType === 'board_move') {
        return (event.actor || {}).label || '';
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

    formatActivityTimestamp(value) {
      if (!value) {
        return '';
      }

      return dayjs(value).format('MMM D, YYYY h:mm A');
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
  position: relative;
}

.timeline-item {
  display: flex;
  gap: 12px;
  padding-bottom: 16px;
  position: relative;
}

/* The line joining one event to the next. */
.timeline-item:not(:last-of-type)::before {
  background: rgba(var(--v-border-color), var(--v-border-opacity));
  bottom: 0;
  content: '';
  left: 15px;
  position: absolute;
  top: 32px;
  width: 1px;
}

.timeline-dot {
  align-items: center;
  background: rgb(var(--v-theme-surface-light));
  border-radius: 50%;
  display: flex;
  flex: none;
  height: 32px;
  justify-content: center;
  width: 32px;
}

.timeline-item-body {
  flex: 1;
  min-width: 0;
}

.timeline-item-head {
  align-items: baseline;
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  justify-content: space-between;
}

.timeline-title {
  font-size: 14px;
  overflow-wrap: anywhere;
}

.attachment-link {
  align-items: center;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 999px;
  color: rgb(var(--v-theme-primary));
  display: inline-flex;
  font-size: 12px;
  font-weight: 600;
  padding: 4px 10px;
  text-decoration: none;
}

.attachment-link:hover {
  background: rgb(var(--v-theme-surface-light));
}
</style>
