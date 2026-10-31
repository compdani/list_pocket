<template>
  <section :class="['inbox-page', { 'inbox-page--thread': selectedKey }]">
    <aside class="inbox-list" :aria-label="$t('conversations.title')">
      <header class="inbox-list-head">
        <div class="d-flex align-center justify-space-between">
          <h1 class="text-h6">{{ $t('conversations.title') }}</h1>
          <v-btn
            :to="{ name: 'inboundEmail' }"
            size="x-small"
            variant="text"
            class="text-none"
          >
            {{ $t('conversations.emailLog') }}
          </v-btn>
        </div>
        <v-text-field
          v-model="search"
          :label="$t('conversations.search')"
          prepend-inner-icon="mdi-magnify"
          density="compact"
          variant="outlined"
          hide-details
          clearable
          class="mt-2"
          data-cy="conversation-search"
        />
        <div class="inbox-filters">
          <v-chip
            v-for="option in viewOptions"
            :key="option"
            size="small"
            :variant="view === option ? 'flat' : 'outlined'"
            :color="view === option ? 'primary' : undefined"
            @click="view = option"
          >
            {{ $t(`conversations.view.${option}`) }}
            <span v-if="option === 'unread' && unread > 0" class="ml-1">({{ unread }})</span>
          </v-chip>
        </div>
        <v-select
          v-model="channelFilter"
          :items="channelItems"
          density="compact"
          variant="outlined"
          hide-details
          class="mt-2"
          :aria-label="$t('conversations.allChannels')"
        />
      </header>

      <div class="inbox-rows" role="list">
        <div v-if="loading && results.length === 0" class="pa-3">
          <v-skeleton-loader type="list-item-two-line@6" />
        </div>
        <p v-else-if="results.length === 0" class="text-body-2 text-medium-emphasis text-center pa-8 mb-0">
          {{ $t('conversations.empty') }}
        </p>
        <button
          v-for="row in results"
          :key="row.key"
          type="button"
          role="listitem"
          :class="['inbox-row', { 'inbox-row--active': row.key === selectedKey, 'inbox-row--unread': row.unread }]"
          data-cy="conversation-row"
          @click="select(row.key)"
        >
          <contact-avatar :name="row.name" :fallback="row.email || row.address" />
          <span class="inbox-row-main">
            <span class="inbox-row-top">
              <span class="inbox-row-name">{{ nameOf(row) }}</span>
              <span class="inbox-row-time">{{ shortStamp(row.lastAt) }}</span>
            </span>
            <span v-if="row.subject" class="inbox-row-subject">{{ row.subject }}</span>
            <span class="inbox-row-preview">
              <v-icon :icon="channelIcon(row.channel)" size="13" />
              <span v-if="row.direction === 'outbound'">{{ $t('conversations.you') }}:</span>
              {{ row.preview }}
            </span>
          </span>
          <span v-if="row.unread" class="inbox-row-dot" aria-hidden="true" />
        </button>
        <div v-if="results.length < total" class="d-flex justify-center pa-3">
          <v-btn size="small" variant="text" class="text-none" :loading="loadingMore" @click="loadMore">
            {{ $t('conversations.loadMore') }}
          </v-btn>
        </div>
      </div>
    </aside>

    <main class="inbox-thread">
      <template v-if="selectedKey">
        <header class="inbox-thread-head">
          <v-btn
            class="inbox-back"
            :aria-label="$t('conversations.back')"
            icon="mdi-arrow-left"
            variant="text"
            size="small"
            @click="select('')"
          />
          <contact-avatar :name="contact?.name" :fallback="contact?.email || headAddress" />
          <div class="inbox-thread-title">
            <div class="text-subtitle-1 font-weight-medium text-truncate">
              {{ contact?.name || contact?.email || headAddress }}
            </div>
            <div class="text-caption text-medium-emphasis text-truncate">
              <template v-if="contact">{{ [contact.email, contact.phone].filter(Boolean).join(' · ') }}</template>
              <template v-else>{{ $t('conversations.notContact') }}</template>
            </div>
          </div>
          <v-btn
            v-if="$can('inbox:manage')"
            size="small"
            variant="text"
            class="text-none"
            :prepend-icon="selectedClosed ? 'mdi-archive-arrow-up-outline' : 'mdi-check-circle-outline'"
            data-cy="conversation-close"
            @click="toggleClosed"
          >
            {{ $t(selectedClosed ? 'conversations.reopen' : 'conversations.close') }}
          </v-btn>
        </header>
        <div class="inbox-thread-body">
          <conversation-thread
            ref="threadRef"
            :conversation-key="selectedKey"
            @loaded="onThreadLoaded"
            @changed="refreshList"
          />
        </div>
      </template>
      <div v-else class="inbox-empty text-medium-emphasis">
        <v-icon icon="mdi-forum-outline" size="48" class="mb-3" />
        <p class="mb-0">{{ $t('conversations.select') }}</p>
      </div>
    </main>

    <aside v-if="selectedKey" class="inbox-side" :aria-label="$t('crm.tab.details')">
      <template v-if="contact">
        <div class="text-center mb-4">
          <contact-avatar :name="contact.name" :fallback="contact.email" :size="56" />
          <div class="text-subtitle-2 mt-2">{{ contact.name || contact.email }}</div>
          <v-btn
            :to="{ name: 'subscriber', params: { id: contact.id } }"
            size="small"
            variant="text"
            class="text-none"
            append-icon="mdi-open-in-new"
          >
            {{ $t('conversations.viewContact') }}
          </v-btn>
        </div>
        <contact-crm-fields :subscriber-id="contact.id" @updated="refreshList" />
        <h2 class="inbox-side-title">{{ $t('crm.tab.tasks') }}</h2>
        <contact-tasks :subscriber-id="contact.id" />
        <h2 class="inbox-side-title">{{ $t('crm.tab.notes') }}</h2>
        <contact-notes :subscriber-id="contact.id" />
      </template>
      <div v-else class="text-center text-body-2 text-medium-emphasis pa-4">
        <p>{{ $t('conversations.notContact') }}</p>
        <v-btn
          v-if="$can('subscribers:manage')"
          :to="{ name: 'subscribers' }"
          size="small"
          variant="outlined"
          class="text-none"
        >
          {{ $t('conversations.addContact') }}
        </v-btn>
      </div>
    </aside>
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, onBeforeUnmount, onMounted, ref, watch,
} from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useStore } from 'vuex';
import { getConversations, setConversationClosed } from '../api';
import ContactAvatar from '../components/ContactAvatar.vue';
import ContactCrmFields from '../components/ContactCrmFields.vue';
import ContactNotes from '../components/ContactNotes.vue';
import ContactTasks from '../components/ContactTasks.vue';
import ConversationThread from '../components/ConversationThread.vue';
import { channelIcon, conversationName, shortStamp } from '../utils/crm.mjs';
import { pluginChannels } from '../utils/channels';

const PAGE_SIZE = 50;
const REFRESH_MS = 30000;
const SEARCH_DELAY_MS = 300;

const { proxy } = getCurrentInstance();
const route = useRoute();
const router = useRouter();
const store = useStore();

const viewOptions = ['open', 'unread', 'closed', 'all'];
const results = ref([]);
const total = ref(0);
const unread = ref(0);
const loading = ref(false);
const loadingMore = ref(false);
const view = ref('open');
const channelFilter = ref('');
const search = ref('');
const thread = ref(null);
const threadRef = ref(null);
const closedOverride = ref(null);
let timer = null;
let searchTimer = null;
let requestSeq = 0;

const selectedKey = computed(() => String(route.query.c || ''));
const contact = computed(() => thread.value?.contact || null);
const selectedRow = computed(() => results.value.find((r) => r.key === selectedKey.value));
const headAddress = computed(() => thread.value?.address || selectedRow.value?.address || '');
const selectedClosed = computed(() => (
  closedOverride.value === null ? Boolean(selectedRow.value?.closed) : closedOverride.value
));

const channelItems = computed(() => {
  const plugin = pluginChannels(store.state.serverConfig?.channels)
    .filter((c) => c.id !== 'ch:whatsapp')
    .map((c) => ({ title: c.label || c.id, value: c.id }));
  return [
    { title: proxy.$t('conversations.allChannels'), value: '' },
    { title: proxy.$t('conversations.channel.email'), value: 'email' },
    { title: proxy.$t('conversations.channel.sms'), value: 'sms' },
    { title: proxy.$t('conversations.channel.whatsapp'), value: 'ch:whatsapp' },
    ...plugin,
  ];
});

function nameOf(row) {
  return conversationName(row);
}

function query(offset) {
  const params = { view: view.value, limit: PAGE_SIZE, offset };
  if (channelFilter.value) {
    params.channel = channelFilter.value;
  }
  if (search.value && search.value.trim()) {
    params.search = search.value.trim();
  }
  return params;
}

async function fetchList({ quiet = false } = {}) {
  requestSeq += 1;
  const seq = requestSeq;
  if (!quiet) {
    loading.value = true;
  }
  try {
    // A refresh keeps however many rows have been loaded so far.
    const params = { ...query(0), limit: Math.min(Math.max(results.value.length, PAGE_SIZE), 100) };
    const data = await getConversations(params);
    if (seq !== requestSeq) {
      return;
    }
    results.value = data.results || [];
    total.value = data.total || 0;
    unread.value = data.unread || 0;
  } catch (err) {
    if (!quiet) {
      proxy.$utils.toast(err.message, 'error');
    }
  } finally {
    if (seq === requestSeq) {
      loading.value = false;
    }
  }
}

function refreshList() {
  return fetchList({ quiet: true });
}

async function loadMore() {
  loadingMore.value = true;
  try {
    const data = await getConversations(query(results.value.length));
    results.value = [...results.value, ...(data.results || [])];
    total.value = data.total || 0;
  } catch (err) {
    proxy.$utils.toast(err.message, 'error');
  } finally {
    loadingMore.value = false;
  }
}

function select(key) {
  const next = { ...route.query };
  if (key) {
    next.c = key;
  } else {
    delete next.c;
  }
  router.push({ query: next });
}

function onThreadLoaded(data) {
  thread.value = data;
}

async function toggleClosed() {
  const next = !selectedClosed.value;
  try {
    await setConversationClosed(selectedKey.value, next);
    closedOverride.value = next;
    await refreshList();
  } catch (err) {
    proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
  }
}

watch(selectedKey, () => {
  thread.value = null;
  closedOverride.value = null;
});

watch([view, channelFilter], () => {
  results.value = [];
  fetchList();
});

watch(search, () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    results.value = [];
    fetchList();
  }, SEARCH_DELAY_MS);
});

onMounted(() => {
  fetchList();
  timer = setInterval(refreshList, REFRESH_MS);
});

onBeforeUnmount(() => {
  clearInterval(timer);
  clearTimeout(searchTimer);
});
</script>

<style scoped>
.inbox-page {
  --inbox-border: rgba(var(--v-border-color), var(--v-border-opacity));
  background: rgb(var(--v-theme-surface));
  border: 1px solid var(--inbox-border);
  border-radius: 12px;
  display: grid;
  grid-template-columns: minmax(280px, 340px) minmax(0, 1fr);
  height: calc(100vh - 112px);
  min-height: 520px;
  overflow: hidden;
}

.inbox-page--thread {
  grid-template-columns: minmax(280px, 340px) minmax(0, 1fr) minmax(280px, 320px);
}

.inbox-list {
  border-right: 1px solid var(--inbox-border);
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.inbox-list-head {
  border-bottom: 1px solid var(--inbox-border);
  flex: none;
  padding: 12px;
}

.inbox-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}

.inbox-rows {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.inbox-row {
  align-items: flex-start;
  background: none;
  border: 0;
  border-bottom: 1px solid var(--inbox-border);
  color: inherit;
  cursor: pointer;
  display: flex;
  font: inherit;
  gap: 10px;
  padding: 10px 12px;
  text-align: left;
  width: 100%;
}

.inbox-row:hover {
  background: rgb(var(--v-theme-surface-light));
}

.inbox-row:focus-visible {
  outline: 2px solid rgb(var(--v-theme-primary));
  outline-offset: -2px;
}

.inbox-row--active {
  background: rgba(var(--v-theme-primary), 0.12);
}

.inbox-row-main {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.inbox-row-top {
  align-items: baseline;
  display: flex;
  gap: 8px;
  justify-content: space-between;
}

.inbox-row-name,
.inbox-row-subject,
.inbox-row-preview {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.inbox-row-name {
  font-size: 14px;
}

.inbox-row-time {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  flex: none;
  font-size: 12px;
}

.inbox-row-subject {
  font-size: 13px;
}

.inbox-row-preview {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  font-size: 13px;
}

.inbox-row--unread .inbox-row-name,
.inbox-row--unread .inbox-row-subject {
  font-weight: 600;
}

.inbox-row-dot {
  background: rgb(var(--v-theme-primary));
  border-radius: 50%;
  flex: none;
  height: 8px;
  margin-top: 6px;
  width: 8px;
}

.inbox-thread {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
}

.inbox-thread-head {
  align-items: center;
  border-bottom: 1px solid var(--inbox-border);
  display: flex;
  flex: none;
  gap: 10px;
  padding: 10px 16px;
}

.inbox-thread-title {
  flex: 1;
  min-width: 0;
}

.inbox-thread-body {
  display: flex;
  flex: 1;
  min-height: 0;
}

.inbox-back {
  display: none;
}

.inbox-empty {
  align-items: center;
  display: flex;
  flex: 1;
  flex-direction: column;
  justify-content: center;
}

.inbox-side {
  border-left: 1px solid var(--inbox-border);
  min-height: 0;
  overflow-y: auto;
  padding: 16px;
}

.inbox-side-title {
  font-size: 13px;
  font-weight: 600;
  margin: 20px 0 8px;
}

@media (max-width: 1280px) {
  .inbox-page--thread {
    grid-template-columns: minmax(260px, 320px) minmax(0, 1fr);
  }

  .inbox-side {
    display: none;
  }
}

/* One pane at a time on a phone. */
@media (max-width: 760px) {
  .inbox-page,
  .inbox-page--thread {
    grid-template-columns: minmax(0, 1fr);
    height: calc(100vh - 96px);
  }

  .inbox-page--thread .inbox-list,
  .inbox-page:not(.inbox-page--thread) .inbox-thread {
    display: none;
  }

  .inbox-list {
    border-right: 0;
  }

  .inbox-back {
    display: inline-flex;
  }
}
</style>
