<template>
  <section v-if="thread" class="mt-4" data-cy="subscriber-whatsapp">
    <h5 class="text-subtitle-1 mb-2">{{ $t('whatsapp.conversation') }}</h5>
    <v-card variant="outlined">
      <v-card-text>
        <div v-if="messages.length > 0" class="whatsapp-thread mb-3" data-cy="whatsapp-thread">
          <div
            v-for="event in messages"
            :key="event.id"
            :class="['whatsapp-message', event.direction === 'outbound' ? 'whatsapp-message--out' : 'whatsapp-message--in']"
          >
            <div class="text-body-2 whatsapp-body">{{ event.body || event.kind }}</div>
            <div class="text-caption text-medium-emphasis">
              {{ $utils.niceDate(event.occurred_at, true) }}
              <span v-if="event.direction === 'outbound' && event.status"> · {{ statusLabel(event.status) }}</span>
            </div>
          </div>
        </div>
        <p v-else class="text-body-2 text-medium-emphasis">{{ $t('whatsapp.noMessages') }}</p>

        <v-alert
          v-if="blockedReason"
          type="info"
          variant="tonal"
          density="comfortable"
          class="mb-2"
          :text="$t(`whatsapp.reply.${blockedReason}`)"
          data-cy="whatsapp-reply-blocked"
        />
        <template v-else-if="$can('whatsapp:reply')">
          <v-textarea
            v-model="draft"
            :label="$t('whatsapp.reply.label')"
            :hint="windowHint"
            persistent-hint
            rows="2"
            auto-grow
            maxlength="4096"
            variant="outlined"
            density="comfortable"
            class="mb-2"
            data-cy="whatsapp-reply"
          />
          <v-btn color="primary" :disabled="!draft.trim()" :loading="sending" data-cy="whatsapp-reply-send" @click.prevent="send">
            {{ $t('whatsapp.reply.send') }}
          </v-btn>
        </template>
      </v-card-text>
    </v-card>
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, onMounted, ref, watch,
} from 'vue';
import { getWhatsAppThread, sendWhatsAppReply } from '../api';

const props = defineProps({
  subscriberId: { type: String, required: true },
});

const { proxy } = getCurrentInstance();
const thread = ref(null);
const draft = ref('');
const sending = ref(false);

// The API returns newest first, and a conversation reads oldest first.
const messages = computed(() => [...(thread.value?.events || [])].reverse());

const blockedReason = computed(() => {
  const t = thread.value;
  if (!t) {
    return '';
  }
  if (!t.configured) {
    return 'not_configured';
  }
  if (!t.has_phone) {
    return 'no_phone';
  }
  if (t.opted_out) {
    return 'opted_out';
  }
  return t.requires_window && !t.window_open ? 'window_closed' : '';
});

const windowHint = computed(() => {
  const closes = thread.value?.window_closes_at;
  return closes ? proxy.$t('whatsapp.reply.windowOpen', { at: proxy.$utils.niceDate(closes, true) }) : '';
});

function statusLabel(status) {
  const key = `whatsapp.status.${status}`;
  const text = proxy.$t(key);
  return text === key ? status : text;
}

async function load() {
  try {
    thread.value = await getWhatsAppThread(props.subscriberId);
  } catch (err) {
    proxy.$utils.toast(err.message, 'error');
  }
}

async function send() {
  sending.value = true;
  try {
    await sendWhatsAppReply(props.subscriberId, draft.value.trim());
    draft.value = '';
  } catch (err) {
    const reason = err.response && err.response.reason;
    const text = reason ? proxy.$t(`whatsapp.reply.${reason}`) : ((err.response && err.response.message) || err.message);
    proxy.$utils.toast(text, 'error');
  } finally {
    sending.value = false;
  }
  await load();
}

watch(() => props.subscriberId, load);
onMounted(load);
</script>

<style scoped>
.whatsapp-thread {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 360px;
  overflow-y: auto;
}

.whatsapp-message {
  max-width: 80%;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.whatsapp-message--out {
  align-self: flex-end;
  background: rgba(var(--v-theme-primary), 0.08);
}

.whatsapp-message--in {
  align-self: flex-start;
}

.whatsapp-body {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
