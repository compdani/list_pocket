<template>
  <section class="inbox">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h4">
          {{ $t('menu.inbox') }}
          <span v-if="total > 0">({{ total }})</span>
        </h1>
      </div>
    </header>

    <!-- Filters -->
    <v-card class="mb-4" elevation="0" variant="outlined">
      <v-card-text>
        <v-row dense>
          <v-col cols="12" md="4">
            <v-text-field
              v-model="filters.search"
              label="Search (from / subject)"
              prepend-inner-icon="mdi-magnify"
              density="compact"
              clearable
              hide-details
              @update:model-value="onFilterChange"
            />
          </v-col>
          <v-col cols="12" md="3">
            <v-select
              v-model="filters.spam_status"
              :items="spamStatusOptions"
              label="Status"
              density="compact"
              clearable
              hide-details
              @update:model-value="onFilterChange"
            />
          </v-col>
          <v-col cols="12" md="2" class="d-flex align-center">
            <v-btn
              variant="outlined"
              density="compact"
              prepend-icon="mdi-shield-check-outline"
              @click="showSpamRules = true"
            >
              Spam Rules
            </v-btn>
          </v-col>
        </v-row>
      </v-card-text>
    </v-card>

    <!-- Table -->
    <v-data-table-server
      :headers="tableHeaders"
      :items="emails"
      :items-length="total"
      :loading="loading"
      :page="page"
      :items-per-page="perPage"
      class="inbox-table"
      item-value="record_id"
      hide-default-footer
      @update:options="onTableOptionsChange"
      @click:row="(_, { item }) => openEmail(item)"
    >
      <template #item.spam_status="{ item }">
        <v-chip
          v-if="item.spam_status"
          :color="spamChipColor(item.spam_status)"
          size="small"
          label
        >
          {{ item.spam_status }}
        </v-chip>
        <span v-else class="text-medium-emphasis text-caption">clean</span>
      </template>

      <template #item.has_attachments="{ item }">
        <v-icon v-if="item.has_attachments" size="small" color="primary">mdi-paperclip</v-icon>
      </template>

      <template #item.received_at="{ item }">
        {{ $utils.formatDate(item.received_at) }}
      </template>

      <template #item.subscriber_name="{ item }">
        <router-link
          v-if="item.subscriber_id"
          :to="{ name: 'subscriber', params: { id: item.subscriber_id } }"
          @click.stop
        >
          {{ item.subscriber_name || item.subscriber_email || item.subscriber_id }}
        </router-link>
        <span v-else class="text-medium-emphasis">—</span>
      </template>

      <template #item.actions="{ item }">
        <v-menu>
          <template #activator="{ props }">
            <v-btn v-bind="props" icon="mdi-dots-vertical" variant="text" size="small" @click.stop />
          </template>
          <v-list density="compact">
            <v-list-item
              prepend-icon="mdi-shield-alert-outline"
              title="Mark as Suspected Spam"
              @click="markSpam(item, 'suspected')"
            />
            <v-list-item
              prepend-icon="mdi-shield-off-outline"
              title="Mark as Spam"
              @click="markSpam(item, 'spam')"
            />
            <v-list-item
              prepend-icon="mdi-shield-remove-outline"
              title="Mark as Confirmed Spam"
              @click="markSpam(item, 'confirmed_spam')"
            />
            <v-divider />
            <v-list-item
              prepend-icon="mdi-shield-check-outline"
              title="Clear Spam Status"
              @click="markSpam(item, '')"
            />
          </v-list>
        </v-menu>
      </template>

      <template #bottom>
        <div class="table-footer d-flex align-center justify-space-between pa-2">
          <span class="text-caption text-medium-emphasis">
            Showing {{ emails.length }} of {{ total }}
          </span>
          <v-pagination
            v-if="total > perPage"
            v-model="page"
            :length="Math.ceil(total / perPage)"
            density="compact"
            @update:model-value="fetchEmails"
          />
        </div>
      </template>
    </v-data-table-server>

    <!-- Email Detail Dialog -->
    <v-dialog v-model="detailDialog" max-width="900" scrollable>
      <v-card v-if="selectedEmail">
        <v-card-title class="d-flex align-center justify-space-between">
          <span class="text-truncate mr-2">{{ selectedEmail.subject || '(no subject)' }}</span>
          <v-btn icon="mdi-close" variant="text" @click="detailDialog = false" />
        </v-card-title>
        <v-card-subtitle>
          <div><strong>From:</strong> {{ selectedEmail.from_address }}</div>
          <div v-if="selectedEmail.to_address"><strong>To:</strong> {{ selectedEmail.to_address }}</div>
          <div v-if="selectedEmail.cc"><strong>CC:</strong> {{ selectedEmail.cc }}</div>
          <div v-if="selectedEmail.reply_to"><strong>Reply-To:</strong> {{ selectedEmail.reply_to }}</div>
          <div><strong>Received:</strong> {{ $utils.formatDate(selectedEmail.received_at) }}</div>
          <div v-if="selectedEmail.spam_status">
            <strong>Spam Status:</strong>
            <v-chip :color="spamChipColor(selectedEmail.spam_status)" size="x-small" class="ml-1" label>
              {{ selectedEmail.spam_status }}
            </v-chip>
          </div>
        </v-card-subtitle>
        <v-divider />
        <v-card-text>
          <!-- HTML body -->
          <div v-if="selectedEmail.body_html" class="email-body">
            <!-- eslint-disable-next-line vue/no-v-html -->
            <div v-html="selectedEmail.body_html" class="email-html-body" />
          </div>
          <!-- Plain text fallback -->
          <pre v-else-if="selectedEmail.body_text" class="email-body-text">{{ selectedEmail.body_text }}</pre>
          <p v-else class="text-medium-emphasis">(no body)</p>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-btn
            variant="text"
            prepend-icon="mdi-shield-alert-outline"
            @click="markSpam(selectedEmail, 'suspected'); detailDialog = false"
          >
            Suspected Spam
          </v-btn>
          <v-btn
            variant="text"
            color="error"
            prepend-icon="mdi-shield-off-outline"
            @click="markSpam(selectedEmail, 'spam'); detailDialog = false"
          >
            Mark Spam
          </v-btn>
          <v-spacer />
          <v-btn variant="text" @click="detailDialog = false">Close</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Spam Rules Dialog -->
    <v-dialog v-model="showSpamRules" max-width="700" scrollable>
      <v-card>
        <v-card-title>
          Spam Rules
          <v-btn icon="mdi-close" variant="text" style="float:right" @click="showSpamRules = false" />
        </v-card-title>
        <v-card-text>
          <v-data-table
            :headers="spamRuleHeaders"
            :items="spamRules"
            :loading="loadingRules"
            density="compact"
            hide-default-footer
          >
            <template #item.is_active="{ item }">
              <v-icon :color="item.is_active ? 'success' : 'error'" size="small">
                {{ item.is_active ? 'mdi-check-circle' : 'mdi-close-circle' }}
              </v-icon>
            </template>
            <template #item.actions="{ item }">
              <v-btn
                icon="mdi-trash-can-outline"
                variant="text"
                size="small"
                color="error"
                @click="deleteSpamRule(item)"
              />
            </template>
          </v-data-table>
          <p v-if="!loadingRules && spamRules.length === 0" class="text-medium-emphasis text-center mt-4">
            No spam rules learned yet. Mark emails as spam to train the filter.
          </p>
        </v-card-text>
      </v-card>
    </v-dialog>
  </section>
</template>

<script setup>
import { ref, reactive, onMounted, watch } from 'vue';
import {
  getInboundEmailInbox,
  getInboundEmailByID,
  updateInboundEmailSpamStatus,
  getInboundSpamRules,
  deleteInboundSpamRule,
} from '../api/index.js';

const emails = ref([]);
const total = ref(0);
const loading = ref(false);
const page = ref(1);
const perPage = 50;

const detailDialog = ref(false);
const selectedEmail = ref(null);
const showSpamRules = ref(false);
const spamRules = ref([]);
const loadingRules = ref(false);

const filters = reactive({
  search: '',
  spam_status: '',
});

const spamStatusOptions = [
  { title: 'Clean (no spam)', value: '' },
  { title: 'All emails', value: 'all' },
  { title: 'Suspected Spam', value: 'suspected' },
  { title: 'Spam', value: 'spam' },
  { title: 'Confirmed Spam', value: 'confirmed_spam' },
];

const tableHeaders = [
  { title: 'From', key: 'from_address', sortable: false },
  { title: 'Subject', key: 'subject', sortable: false },
  { title: 'Subscriber', key: 'subscriber_name', sortable: false },
  { title: 'Received', key: 'received_at', sortable: false },
  { title: 'Status', key: 'spam_status', sortable: false },
  { title: '', key: 'has_attachments', sortable: false, width: '40px' },
  { title: '', key: 'actions', sortable: false, width: '48px' },
];

const spamRuleHeaders = [
  { title: 'Type', key: 'type' },
  { title: 'Value', key: 'value' },
  { title: 'Level', key: 'spam_level' },
  { title: 'Hits', key: 'hit_count' },
  { title: 'Active', key: 'is_active' },
  { title: '', key: 'actions', sortable: false, width: '48px' },
];

async function fetchEmails() {
  loading.value = true;
  try {
    const params = {
      limit: perPage,
      offset: (page.value - 1) * perPage,
    };
    if (filters.search) params.search = filters.search;
    if (filters.spam_status !== '') params.spam_status = filters.spam_status;

    const resp = await getInboundEmailInbox(params);
    emails.value = resp.data?.data?.results || [];
    total.value = resp.data?.data?.total || 0;
  } finally {
    loading.value = false;
  }
}

async function openEmail(item) {
  const resp = await getInboundEmailByID(item.record_id);
  selectedEmail.value = resp.data?.data || item;
  detailDialog.value = true;
}

async function markSpam(item, status) {
  await updateInboundEmailSpamStatus(item.record_id, status);
  await fetchEmails();
}

async function fetchSpamRules() {
  loadingRules.value = true;
  try {
    const resp = await getInboundSpamRules({ limit: 200 });
    spamRules.value = resp.data?.data?.results || [];
  } finally {
    loadingRules.value = false;
  }
}

async function deleteSpamRule(rule) {
  await deleteInboundSpamRule(rule.record_id);
  await fetchSpamRules();
}

function onFilterChange() {
  page.value = 1;
  fetchEmails();
}

function onTableOptionsChange({ page: p }) {
  if (p) page.value = p;
  fetchEmails();
}

function spamChipColor(status) {
  switch (status) {
    case 'confirmed_spam': return 'error';
    case 'spam': return 'warning';
    case 'suspected': return 'info';
    default: return 'default';
  }
}

watch(showSpamRules, (val) => {
  if (val) fetchSpamRules();
});

onMounted(fetchEmails);
</script>

<style scoped>
.email-html-body {
  max-width: 100%;
  overflow-x: auto;
  font-size: 0.9rem;
}
.email-body-text {
  white-space: pre-wrap;
  font-size: 0.9rem;
  font-family: inherit;
}
.inbox-table {
  cursor: pointer;
}
</style>
