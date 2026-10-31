<template>
  <div class="thread" data-cy="conversation-thread">
    <div v-if="loading && !thread" class="thread-state">
      <v-progress-circular indeterminate color="primary" />
    </div>
    <div v-else-if="loadError" class="thread-state">
      <v-alert type="error" variant="tonal" density="comfortable">{{ loadError }}</v-alert>
      <v-btn variant="text" class="text-none mt-2" @click="load">{{ $t('globals.buttons.retry') }}</v-btn>
    </div>

    <template v-else-if="thread">
      <div ref="scroller" class="thread-messages" role="log" :aria-label="$t('conversations.title')">
        <p v-if="thread.messages.length === 0" class="thread-state text-medium-emphasis">
          {{ $t('conversations.noMessages') }}
        </p>
        <section v-for="day in days" :key="day.key">
          <div class="thread-day"><span>{{ day.label }}</span></div>
          <article
            v-for="message in day.messages"
            :key="`${message.channel}-${message.id}`"
            :class="['msg', `msg--${message.direction}`]"
            data-cy="conversation-message"
          >
            <div class="msg-bubble">
              <div v-if="message.subject" class="msg-subject">{{ message.subject }}</div>
              <div class="msg-body">{{ message.body || message.kind }}</div>

              <template v-if="canExpand(message)">
                <v-btn
                  size="x-small"
                  variant="text"
                  class="text-none msg-toggle"
                  :loading="emails[message.id]?.loading"
                  :append-icon="emails[message.id]?.open ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                  @click="toggleEmail(message)"
                >
                  {{ $t(emails[message.id]?.open ? 'conversations.hideEmail' : 'conversations.showEmail') }}
                </v-btn>
                <div v-if="emails[message.id]?.open" class="msg-email">
                  <email-body-frame
                    v-if="emails[message.id].html"
                    :html="emails[message.id].html"
                    :title="message.subject || $t('conversations.channel.email')"
                  />
                  <pre v-else-if="emails[message.id].text" class="msg-email-text">{{ emails[message.id].text }}</pre>
                  <p v-else class="text-caption text-medium-emphasis mb-0">{{ $t('conversations.noBody') }}</p>
                  <div v-if="emails[message.id].attachments.length" class="msg-attachments">
                    <a
                      v-for="file in emails[message.id].attachments"
                      :key="file.id"
                      :href="attachmentUrl(file)"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="msg-attachment"
                    >
                      <v-icon icon="mdi-paperclip" size="14" />
                      {{ file.filename || file.originalName || file.id }}
                    </a>
                  </div>
                </div>
              </template>
            </div>
            <div class="msg-meta text-caption text-medium-emphasis">
              <v-icon :icon="channelIcon(message.channel)" size="13" />
              <span>{{ timeOf(message) }}</span>
              <span v-if="message.direction === 'outbound' && message.kind === 'send'">· {{ $t('conversations.campaign') }}</span>
              <span v-else-if="message.direction === 'outbound' && message.status">· {{ statusLabel(message.status) }}</span>
              <v-icon v-if="message.hasAttachments" icon="mdi-paperclip" size="13" :aria-label="$t('conversations.attachments')" />
              <v-btn
                v-if="canExpand(message) && $can('inbox:manage')"
                size="x-small"
                variant="text"
                class="text-none"
                @click="markSpam(message)"
              >
                {{ $t('conversations.markSpam') }}
              </v-btn>
            </div>
          </article>
        </section>
      </div>

      <div class="composer" data-cy="conversation-composer">
        <div v-if="options.length > 1" class="composer-channels" role="tablist">
          <v-chip
            v-for="option in options"
            :key="option.channel"
            size="small"
            :variant="channel === option.channel ? 'flat' : 'outlined'"
            :color="channel === option.channel ? 'primary' : undefined"
            :prepend-icon="channelIcon(option.channel)"
            role="tab"
            :aria-selected="channel === option.channel"
            @click="channel = option.channel"
          >
            {{ channelLabel(option.channel) }}
          </v-chip>
        </div>

        <v-alert
          v-if="blockedText"
          type="info"
          variant="tonal"
          density="compact"
          class="text-body-2"
          data-cy="conversation-reply-blocked"
        >
          {{ blockedText }}
        </v-alert>
        <template v-else-if="current">
          <div class="text-caption text-medium-emphasis composer-to">
            {{ $t('conversations.reply.to', { to: current.to }) }}
            <template v-if="windowHint"> · {{ windowHint }}</template>
          </div>
          <div v-if="channel === 'email'" class="composer-email">
            <v-text-field
              v-model="subject"
              :label="$t('conversations.reply.subject')"
              density="compact"
              variant="outlined"
              hide-details
              maxlength="500"
              data-cy="conversation-subject"
            />
            <v-select
              :model-value="format"
              :items="formatItems"
              :label="$t('workflows.email.format')"
              density="compact"
              variant="outlined"
              hide-details
              class="composer-format"
              data-cy="conversation-format"
              @update:model-value="changeFormat"
            />
          </div>
          <p v-if="showComposed" class="composer-summary text-body-2" :class="{ 'text-medium-emphasis': !composedSummary }" data-cy="conversation-summary">
            {{ composedSummary || $t('workflows.email.empty') }}
          </p>
          <v-textarea
            v-else
            v-model="draft"
            :label="$t('conversations.reply.placeholder')"
            rows="2"
            max-rows="8"
            auto-grow
            density="compact"
            variant="outlined"
            hide-details
            :maxlength="maxLength"
            data-cy="conversation-reply"
            @keydown.enter.meta.prevent="send"
            @keydown.enter.ctrl.prevent="send"
          />
          <div class="composer-actions">
            <span class="text-caption text-medium-emphasis">{{ showComposed ? '' : $t('conversations.reply.hint') }}</span>
            <v-spacer />
            <v-btn
              v-if="channel === 'email'"
              variant="text"
              size="small"
              class="text-none mr-1"
              prepend-icon="mdi-email-edit-outline"
              data-cy="conversation-compose"
              @click="openCompose()"
            >
              {{ $t(hasDraft ? 'workflows.email.edit' : 'workflows.email.write') }}
            </v-btn>
            <v-btn
              color="primary"
              variant="flat"
              size="small"
              class="text-none"
              prepend-icon="mdi-send"
              :disabled="!canSend"
              :loading="sending"
              data-cy="conversation-send"
              @click="send"
            >
              {{ $t('conversations.reply.send') }}
            </v-btn>
          </div>
        </template>
      </div>

      <email-compose-modal
        v-if="composing"
        :input="composeInput"
        :format="composeFormat"
        reply
        @close="composing = false"
        @save="saveCompose"
      />
    </template>
  </div>
</template>

<script setup>
import {
  computed, defineAsyncComponent, getCurrentInstance, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch,
} from 'vue';
import {
  getConversationThread,
  getInboundEmailAttachmentDownloadUrl,
  getInboundEmailByID,
  getInboundEmailReplyAttachments,
  markConversationRead,
  sendConversationReply,
  sendWhatsAppReply,
  updateInboundEmailSpamStatus,
} from '../api';
import EmailBodyFrame from './EmailBodyFrame.vue';
import {
  channelIcon, defaultReplyChannel, groupByDay, parseStamp, replySubject,
} from '../utils/crm.mjs';
import { EMAIL_FORMATS, bodySummary } from '../utils/emailCompose.mjs';

// It brings every email editor with it, so it loads when it opens.
const EmailComposeModal = defineAsyncComponent(() => import('./flow/EmailComposeModal.vue'));

const WHATSAPP = 'ch:whatsapp';
const REFRESH_MS = 30000;

const props = defineProps({
  conversationKey: { type: String, required: true },
});
const emit = defineEmits(['changed', 'loaded']);

const { proxy } = getCurrentInstance();
const thread = ref(null);
const loading = ref(false);
const loadError = ref('');
const scroller = ref(null);
const emails = reactive({});
const channel = ref('');
const subject = ref('');
const draft = ref('');
const sending = ref(false);
// A formatted email is kept apart from the text box, so its HTML can't reach an SMS.
const composed = ref(null);
const composing = ref(false);
const composeInput = ref({});
const composeFormat = ref('');
let timer = null;

const days = computed(() => groupByDay(thread.value?.messages || [], proxy.$i18n?.locale));
const options = computed(() => thread.value?.replyOptions || []);
const current = computed(() => options.value.find((o) => o.channel === channel.value));
const maxLength = computed(() => (channel.value === WHATSAPP ? 4096 : 10000));

const canReplyOnChannel = computed(() => (
  channel.value === WHATSAPP ? proxy.$can('whatsapp:reply') : proxy.$can('inbox:manage')
));

const blockedText = computed(() => {
  const option = current.value;
  if (!option) {
    return '';
  }
  if (!option.available) {
    return proxy.$t(`conversations.reply.${option.reason}`);
  }
  return canReplyOnChannel.value ? '' : proxy.$t('conversations.reply.noPermission');
});

const windowHint = computed(() => {
  const closes = current.value?.windowClosesAt;
  return closes ? proxy.$t('conversations.reply.windowOpen', { at: proxy.$utils.niceDate(closes, true) }) : '';
});

const format = computed(() => composed.value?.content_type || 'plain');
const formatItems = computed(() => EMAIL_FORMATS.map((f) => ({ value: f.value, title: f.title || proxy.$t(f.label) })));
const showComposed = computed(() => channel.value === 'email' && Boolean(composed.value));
const composedSummary = computed(() => bodySummary(composed.value || {}));
const hasDraft = computed(() => Boolean(showComposed.value ? composedSummary.value : draft.value.trim()));

const canSend = computed(() => {
  if (!hasDraft.value || sending.value) {
    return false;
  }
  return channel.value !== 'email' || Boolean(subject.value.trim());
});

function openCompose(target = format.value) {
  // An empty reply opens on the format. An existing one is converted, which the editor confirms.
  const body = composed.value ? composed.value.body : draft.value;
  composeInput.value = {
    subject: subject.value,
    body,
    content_type: body.trim() ? format.value : target,
    body_source: composed.value?.body_source,
  };
  composeFormat.value = body.trim() && target !== format.value ? target : '';
  composing.value = true;
}

function changeFormat(target) {
  if (target !== format.value) {
    openCompose(target);
  }
}

function saveCompose(input) {
  subject.value = input.subject || '';
  draft.value = input.content_type ? '' : (input.body || '');
  composed.value = input.content_type ? {
    content_type: input.content_type, body: input.body || '', body_source: input.body_source,
  } : null;
  composing.value = false;
}

function channelLabel(id) {
  const name = id === WHATSAPP ? 'whatsapp' : id;
  const key = `conversations.channel.${name}`;
  const text = proxy.$t(key);
  return text === key ? String(id).replace(/^ch:/, '') : text;
}

function statusLabel(status) {
  const key = `conversations.status.${status}`;
  const text = proxy.$t(key);
  return text === key ? status : text;
}

function timeOf(message) {
  const date = parseStamp(message.occurredAt);
  return date ? date.toLocaleTimeString(proxy.$i18n?.locale, { hour: 'numeric', minute: '2-digit' }) : '';
}

// Only stored inbound emails have a full body to show.
function canExpand(message) {
  return message.channel === 'email' && message.direction === 'inbound';
}

function attachmentUrl(file) {
  return file.downloadUrl || getInboundEmailAttachmentDownloadUrl(file.id);
}

async function toggleEmail(message) {
  const state = emails[message.id];
  if (state?.fetched) {
    state.open = !state.open;
    return;
  }
  emails[message.id] = {
    loading: true, open: false, fetched: false, html: '', text: '', attachments: [],
  };
  const next = emails[message.id];
  try {
    const email = await getInboundEmailByID(message.id);
    next.html = email.bodyHtml || '';
    next.text = email.bodyText || '';
    if (message.hasAttachments) {
      const files = await getInboundEmailReplyAttachments(message.id);
      next.attachments = Array.isArray(files) ? files : (files?.data || []);
    }
    next.fetched = true;
    next.open = true;
  } catch (err) {
    proxy.$utils.toast(err.message, 'error');
    delete emails[message.id];
    return;
  } finally {
    if (emails[message.id]) {
      next.loading = false;
    }
  }
}

function scrollToEnd() {
  nextTick(() => {
    if (scroller.value) {
      scroller.value.scrollTop = scroller.value.scrollHeight;
    }
  });
}

async function load({ quiet = false } = {}) {
  const key = props.conversationKey;
  if (!quiet) {
    loading.value = true;
    loadError.value = '';
  }
  try {
    const data = await getConversationThread(key);
    if (key !== props.conversationKey) {
      return;
    }
    const grew = (data.messages?.length || 0) !== (thread.value?.messages?.length || 0);
    thread.value = data;
    emit('loaded', data);
    if (!options.value.some((o) => o.channel === channel.value)) {
      channel.value = defaultReplyChannel(data.replyOptions, data.messages);
    }
    if (!subject.value) {
      subject.value = replySubject(data.messages);
    }
    if (grew) {
      scrollToEnd();
    }
  } catch (err) {
    if (!quiet) {
      loadError.value = err.message;
    }
  } finally {
    loading.value = false;
  }
}

async function markRead() {
  try {
    await markConversationRead(props.conversationKey);
    emit('changed');
  } catch {
    // Being read isn't worth interrupting anyone over. It'll stay marked unread.
  }
}

function replyErrorText(err) {
  const reason = err.response && err.response.reason;
  if (reason) {
    return proxy.$t(`conversations.reply.${reason}`);
  }
  return (err.response && err.response.message) || err.message;
}

async function send() {
  if (!canSend.value) {
    return;
  }
  sending.value = true;
  try {
    const body = draft.value.trim();
    if (channel.value === WHATSAPP) {
      await sendWhatsAppReply(thread.value.contact.id, body);
    } else {
      await sendConversationReply({
        key: props.conversationKey,
        channel: channel.value,
        subject: subject.value.trim(),
        body: showComposed.value ? composed.value.body : body,
        content_type: showComposed.value ? composed.value.content_type : '',
      });
    }
    if (showComposed.value) {
      composed.value = null;
    } else {
      draft.value = '';
    }
    await load({ quiet: true });
    emit('changed');
  } catch (err) {
    proxy.$utils.toast(replyErrorText(err), 'error');
  } finally {
    sending.value = false;
  }
}

async function markSpam(message) {
  try {
    await updateInboundEmailSpamStatus(message.id, 'spam');
    proxy.$utils.toast(proxy.$t('conversations.markedSpam'));
    emit('changed');
    await load({ quiet: true });
  } catch (err) {
    proxy.$utils.toast(err.message, 'error');
  }
}

function reset() {
  thread.value = null;
  channel.value = '';
  subject.value = '';
  draft.value = '';
  composed.value = null;
  composing.value = false;
  Object.keys(emails).forEach((id) => delete emails[id]);
}

async function open() {
  await load();
  if (thread.value) {
    await markRead();
  }
}

watch(() => props.conversationKey, () => {
  reset();
  open();
});

watch(thread, (value, old) => {
  if (value && !old) {
    scrollToEnd();
  }
});

onMounted(() => {
  open();
  timer = setInterval(() => load({ quiet: true }), REFRESH_MS);
});

onBeforeUnmount(() => clearInterval(timer));

defineExpose({ reload: () => load({ quiet: true }) });
</script>

<style scoped>
.thread {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}

.thread-state {
  align-items: center;
  display: flex;
  flex: 1;
  flex-direction: column;
  justify-content: center;
  min-height: 160px;
  padding: 24px;
  text-align: center;
}

.thread-messages {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 16px;
}

.thread-day {
  margin: 12px 0;
  text-align: center;
}

.thread-day span {
  background: rgb(var(--v-theme-surface-light));
  border-radius: 999px;
  font-size: 12px;
  padding: 2px 10px;
}

.msg {
  display: flex;
  flex-direction: column;
  margin-bottom: 12px;
}

.msg--outbound {
  align-items: flex-end;
}

.msg--inbound {
  align-items: flex-start;
}

.msg-bubble {
  background: rgb(var(--v-theme-surface-light));
  border-radius: 14px;
  max-width: min(78%, 640px);
  padding: 8px 12px;
}

.msg:has(.msg-email) .msg-bubble {
  max-width: 100%;
  width: 100%;
}

.msg--inbound .msg-bubble {
  border-bottom-left-radius: 4px;
}

.msg--outbound .msg-bubble {
  background: rgba(var(--v-theme-primary), 0.14);
  border-bottom-right-radius: 4px;
}

.msg-subject {
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 2px;
}

.msg-body {
  font-size: 14px;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.msg-toggle {
  margin: 4px 0 0 -6px;
}

.msg-email {
  margin-top: 8px;
}

.msg-email-text {
  font-family: inherit;
  font-size: 14px;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.msg-attachments {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

.msg-attachment {
  align-items: center;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 999px;
  color: rgb(var(--v-theme-primary));
  display: inline-flex;
  font-size: 12px;
  gap: 4px;
  padding: 2px 10px;
  text-decoration: none;
}

.msg-meta {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 3px;
  min-height: 22px;
  padding: 0 4px;
}

.composer {
  border-top: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  flex: none;
  padding: 12px 16px;
}

.composer-channels {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 8px;
}

.composer-to {
  margin-bottom: 6px;
}

.composer-email {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
}

.composer-email > :first-child {
  flex: 1 1 240px;
}

.composer-format {
  flex: 0 1 190px;
}

.composer-summary {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 4px;
  margin: 0;
  overflow-wrap: anywhere;
  padding: 8px 12px;
}

.composer-actions {
  align-items: center;
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
}

@media (max-width: 600px) {
  .composer-actions .text-caption {
    display: none;
  }

  .composer-actions {
    justify-content: flex-end;
  }
}
</style>
