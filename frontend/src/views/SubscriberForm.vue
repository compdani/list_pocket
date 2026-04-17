<template>
  <v-form @submit.prevent="onSubmit">
    <div class="admin-dialog-card modal-card content">
      <header class="admin-dialog-head modal-card-head">
        <div class="dialog-meta-row">
          <p v-if="isEditing" class="entity-meta has-text-grey is-size-7">
            {{ $t('globals.fields.id') }}: <span data-cy="id"><copy-text :text="`${data.id}`" /></span>
            {{ $t('globals.fields.uuid') }}: <copy-text :text="data.uuid" />
          </p>
          <span v-if="isEditing" class="status-pill" :class="data.status">
            {{ $t(`subscribers.status.${data.status}`) }}
          </span>
        </div>

        <h4 v-if="isEditing" class="dialog-title">
          {{ data.name }}
        </h4>
        <h4 v-else class="dialog-title">
          {{ $t('subscribers.newSubscriber') }}
        </h4>
      </header>

      <section class="admin-dialog-body modal-card-body">
        <v-row class="mb-1">
          <v-col cols="12" md="4">
            <v-text-field
              ref="focus"
              v-model="form.email"
              :label="$t('subscribers.email')"
              maxlength="200"
              name="email"
              :placeholder="$t('subscribers.email')"
              required
              type="email"
              variant="outlined"
              density="comfortable"
            />
          </v-col>

          <v-col cols="12" md="4">
            <v-text-field
              v-model="form.phone"
              label="Phone"
              maxlength="64"
              name="phone"
              placeholder="Phone"
              type="tel"
              variant="outlined"
              density="comfortable"
            />
          </v-col>

          <v-col cols="12" md="4">
            <v-select
              v-model="form.status"
              :items="statusOptions"
              item-title="title"
              item-value="value"
              :label="$t('globals.fields.status')"
              name="status"
              required
              variant="outlined"
              density="comfortable"
            />
            <p class="form-help">{{ $t('subscribers.blocklistedHelp') }}</p>
          </v-col>
        </v-row>

        <v-row class="mb-2">
          <v-col cols="12" md="6">
            <v-text-field
              v-model="form.firstName"
              label="First name"
              maxlength="200"
              name="first_name"
              placeholder="First name"
              type="text"
              variant="outlined"
              density="comfortable"
            />
          </v-col>
          <v-col cols="12" md="6">
            <v-text-field
              v-model="form.lastName"
              label="Last name"
              maxlength="200"
              name="last_name"
              placeholder="Last name"
              type="text"
              variant="outlined"
              density="comfortable"
            />
          </v-col>
        </v-row>

        <v-tabs
          v-model="activeTab"
          color="primary"
          density="comfortable"
          class="settings-tabs mb-4"
          grow
        >
          <v-tab value="lists">
            {{ $t('globals.terms.lists') }}
          </v-tab>
          <v-tab value="subscriptions" :disabled="!data.lists || data.lists.length === 0">
            {{ `${$tc('globals.terms.subscriptions', 2)} (${data.lists ? data.lists.length : 0})` }}
          </v-tab>
          <v-tab value="bounces" :disabled="bounces.length === 0">
            {{ `${$t('globals.terms.bounces')} (${bounces.length})` }}
          </v-tab>
          <v-tab value="activity" :disabled="!isEditing">
            {{ $t('subscribers.activity') }}
          </v-tab>
        </v-tabs>

        <v-window v-model="activeTab" :touch="false" class="tab-window">
          <v-window-item value="lists">
            <section class="tab-panel">
              <v-select
                :model-value="selectedListIds"
                :items="availableLists"
                :label="$t('subscribers.lists')"
                item-title="name"
                item-value="listValue"
                multiple
                chips
                closable-chips
                variant="outlined"
                density="comfortable"
                @update:model-value="onListsChange"
              />
              <p class="form-help">{{ $t('subscribers.listsHelp') }}</p>

              <div class="lists-actions">
                <div class="lists-actions-main">
                  <v-checkbox
                    v-model="form.preconfirm"
                    :disabled="!hasOptinList"
                    :label="$t('subscribers.preconfirm')"
                    density="comfortable"
                    hide-details
                  />
                  <p class="form-help mt-1">{{ $t('subscribers.preconfirmHelp') }}</p>
                </div>

                <v-btn
                  v-if="$can('subscribers:manage') && isEditing"
                  type="button"
                  color="primary"
                  variant="text"
                  prepend-icon="mdi-email-outline"
                  :disabled="!hasOptinList"
                  class="optin-action"
                  @click.prevent="sendOptinConfirmation"
                >
                  {{ $t('subscribers.sendOptinConfirm') }}
                </v-btn>
              </div>
            </section>
          </v-window-item>

          <v-window-item value="subscriptions">
            <section v-if="data.lists && data.lists.length > 0" class="tab-panel">
              <h5 class="mb-3">{{ `${$tc('globals.terms.subscriptions', 2)} (${data.lists.length})` }}</h5>
              <div class="table-wrap">
                <table class="dialog-table">
                  <thead>
                    <tr>
                      <th>{{ $tc('globals.terms.list', 1) }}</th>
                      <th>{{ $t('subscribers.emailStatus') }}</th>
                      <th>{{ $t('subscribers.smsStatus') }}</th>
                      <th>{{ $t('globals.fields.createdAt') }}</th>
                      <th>{{ $t('globals.fields.updatedAt') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="item in data.lists" :key="item.id">
                      <td>
                        <router-link :to="`/lists/${item.id}`">
                          {{ item.name }}
                        </router-link>
                        <div class="subtle-row">
                          <span class="status-pill neutral">
                            {{ $t(`lists.optins.${item.optin}`) }}
                          </span>
                        </div>
                      </td>
                      <td>
                        <span class="status-pill neutral">
                          {{ $t(`subscribers.status.${item.subscriptionStatus}`) }}
                        </span>
                        <div v-if="item.optin === 'double' && item.subscriptionMeta.optinIp" class="subtle-row">
                          {{ item.subscriptionMeta.optinIp }}
                        </div>
                      </td>
                      <td>
                        <span class="status-pill neutral">
                          {{ $t(`subscribers.status.${item.subscriptionSmsStatus || item.subscriptionStatus}`) }}
                        </span>
                      </td>
                      <td>{{ $utils.niceDate(item.subscriptionCreatedAt, true) }}</td>
                      <td>{{ $utils.niceDate(item.subscriptionCreatedAt, true) }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </section>
          </v-window-item>

          <v-window-item value="bounces">
            <section v-if="bounces.length > 0" class="tab-panel bounces">
              <div class="panel-head">
                <h5>{{ `${$t('globals.terms.bounces')} (${bounces.length})` }}</h5>
                <v-btn
                  v-if="isBounceVisible"
                  type="button"
                  color="error"
                  variant="text"
                  class="danger-action"
                  @click.prevent="deleteBounces"
                >
                  {{ $t('globals.buttons.delete') }}
                </v-btn>
              </div>

              <div class="table-wrap">
                <table class="dialog-table">
                  <thead>
                    <tr>
                      <th>{{ $tc('globals.terms.campaign', 1) }}</th>
                      <th>{{ $t('globals.fields.createdAt') }}</th>
                      <th>{{ $t('globals.fields.type') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="bounce in bounces" :key="bounce.id">
                      <td>
                        <router-link
                          v-if="bounce.campaign"
                          :to="{ name: 'bounces', query: { campaign_id: bounce.campaign.id } }"
                        >
                          {{ bounce.campaign.name }}
                        </router-link>
                      </td>
                      <td>{{ $utils.niceDate(bounce.createdAt, true) }}</td>
                      <td>
                        <a href="#" @click.prevent="toggleMeta(bounce.id)">
                          {{ bounce.source }}
                        </a>
                        <pre v-if="visibleMeta[bounce.id]" class="meta-block">{{ bounce.meta }}</pre>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </section>
          </v-window-item>

          <v-window-item value="activity">
            <section v-if="isEditing && data.id" class="tab-panel activity">
              <h5 class="mb-3">{{ $t('subscribers.activity') }}</h5>
              <subscriber-activity :subscriber-id="data.id" />
            </section>
          </v-window-item>
        </v-window>

        <v-combobox
          v-model="form.tags"
          :items="subscriberTagOptions"
          :aria-label="$t('globals.terms.tags')"
          :label="$t('globals.terms.tags')"
          :placeholder="$t('globals.terms.tags')"
          multiple
          chips
          closable-chips
          variant="outlined"
          density="comfortable"
          class="mt-6 mb-2"
        />

        <v-textarea
          v-model="form.strAttribs"
          :label="$t('globals.terms.attribs')"
          name="attribs"
          variant="outlined"
          auto-grow
          rows="5"
          class="mb-1"
        />
        <p class="form-help">{{ $t('subscribers.attribsHelp') }} {{ egAttribs }}</p>
        <a :href="$docsUrl('concepts/')" target="_blank" rel="noopener noreferrer" class="is-size-7">
          {{ $t('globals.buttons.learnMore') }}
        </a>
      </section>

      <footer class="admin-dialog-foot modal-card-foot">
        <v-btn type="button" variant="outlined" class="dialog-action" @click="$emit('close')">
          {{ $t('globals.buttons.close') }}
        </v-btn>
        <v-btn
          v-if="$can('subscribers:manage')"
          color="primary"
          variant="flat"
          class="dialog-action"
          :disabled="loading.subscribers"
          :loading="loading.subscribers"
          type="submit"
        >
          {{ $t('globals.buttons.save') }}
        </v-btn>
      </footer>
    </div>
  </v-form>
</template>

<script setup>
import {
  computed,
  getCurrentInstance,
  nextTick,
  onMounted,
  ref,
  toRef,
  watch,
} from 'vue';
import { useStore } from 'vuex';
import CopyText from '../components/CopyText.vue';
import SubscriberActivity from '../components/SubscriberActivity.vue';

const props = defineProps({
  data: {
    type: Object,
    default: () => ({ lists: [] }),
  },
  isEditing: Boolean,
});

const emit = defineEmits(['finished', 'close']);
const { proxy } = getCurrentInstance();
const store = useStore();

const data = toRef(props, 'data');
const isEditing = toRef(props, 'isEditing');

const form = ref({
  lists: [],
  strAttribs: '{}',
  status: 'enabled',
  preconfirm: false,
  tags: [],
});

const focus = ref(null);
const isBounceVisible = ref(false);
const bounces = ref([]);
const visibleMeta = ref({});
const activeTab = ref('lists');
const subscriberTagOptions = ref([]);

const egAttribs = '{"job": "developer", "location": "Mars", "has_rocket": true}';

const lists = computed(() => store.state.lists);
const loading = computed(() => store.state.loading);

const statusOptions = computed(() => [
  { title: proxy.$t('subscribers.status.enabled'), value: 'enabled' },
  { title: proxy.$t('subscribers.status.blocklisted'), value: 'blocklisted' },
]);

const availableLists = computed(() => (
  Array.isArray(lists.value && lists.value.results)
    ? lists.value.results.map((list) => ({
      ...list,
      listValue: String(list.id),
    }))
    : []
));

const selectedListIds = computed(() => (
  Array.isArray(form.value.lists)
    ? form.value.lists.map((list) => (
      String(list.id)
    ))
    : []
));

function normalizeTags(values = []) {
  const seen = new Set();
  return (Array.isArray(values) ? values : [])
    .map((tag) => String(tag || '').trim())
    .filter((tag) => {
      const key = tag.toLowerCase();
      if (!key || seen.has(key)) {
        return false;
      }
      seen.add(key);
      return true;
    });
}

async function loadSubscriberTagOptions() {
  const tags = await proxy.$api.getSubscriberTagsCatalog();
  subscriberTagOptions.value = normalizeTags(tags);
}

async function persistSubscriberTagOptions() {
  const tags = normalizeTags(form.value.tags);
  form.value.tags = tags;
  if (tags.length === 0) {
    return;
  }
  await proxy.$api.ensureSubscriberTags(tags, subscriberTagOptions.value);
  subscriberTagOptions.value = normalizeTags([...subscriberTagOptions.value, ...tags]);
}

const hasOptinList = computed(() => (
  Array.isArray(form.value.lists) && form.value.lists.some((l) => l.optin === 'double')
));

function initForm() {
  const baseForm = {
    id: null,
    email: '',
    phone: '',
    firstName: '',
    lastName: '',
    lists: [],
    strAttribs: '{}',
    status: 'enabled',
    preconfirm: false,
    tags: [],
  };

  if (!props.isEditing) {
    form.value = baseForm;
    bounces.value = [];
    visibleMeta.value = {};
    return;
  }

  const source = props.data || {};
  form.value = {
    ...baseForm,
    ...source,
    email: source.email || '',
    phone: source.phone || '',
    firstName: source.firstName || '',
    lastName: source.lastName || '',
    status: source.status || 'enabled',
    lists: Array.isArray(source.lists) ? [...source.lists] : [],
    strAttribs: JSON.stringify(source.attribs || {}, null, 4),
    tags: Array.isArray((source.attribs || {}).tags) ? [...source.attribs.tags] : [],
  };
  bounces.value = [];
  visibleMeta.value = {};
  activeTab.value = 'lists';

  if (form.value.id) {
    getBounces();
  }
}

function toggleMeta(id) {
  visibleMeta.value = {
    ...visibleMeta.value,
    [id]: !visibleMeta.value[id],
  };
}

function onListsChange(selectedIDs) {
  const normalizedIDs = Array.isArray(selectedIDs) ? selectedIDs.map((id) => String(id)) : [];
  const listMap = new Map(availableLists.value.map((list) => [list.listValue, list]));
  form.value.lists = normalizedIDs
    .map((id) => listMap.get(id))
    .filter(Boolean);
}

function deleteBounces() {
  proxy.$utils.confirm(
    null,
    () => {
      proxy.$api.deleteSubscriberBounces(form.value.id).then(() => {
        getBounces();
        const subscriberName = [form.value.firstName, form.value.lastName].filter(Boolean).join(' ') || form.value.email;
        proxy.$utils.toast(proxy.$t('globals.messages.deleted', { name: subscriberName }));
      });
    },
  );
}

function getBounces() {
  proxy.$api.getSubscriberBounces(form.value.id).then((response) => {
    bounces.value = response;
  });
}

function onSubmit() {
  if (isEditing.value) {
    updateSubscriber();
    return;
  }

  createSubscriber();
}

async function createSubscriber() {
  let attribs = {};
  if (form.value.strAttribs) {
    attribs = validateAttribs(form.value.strAttribs);
    if (!attribs) {
      return;
    }
  }

  if (form.value.tags.length > 0) {
    attribs.tags = [...form.value.tags];
  } else {
    delete attribs.tags;
  }
  await persistSubscriberTagOptions();

  const payload = {
    email: form.value.email,
    phone: form.value.phone,
    first_name: form.value.firstName,
    last_name: form.value.lastName,
    status: form.value.status,
    attribs,
    preconfirm_subscriptions: form.value.preconfirm,

    // List IDs.
    list_record_ids: form.value.lists
      .map((l) => l.id)
      .filter((id) => typeof id === 'string' && id.length > 0),
  };

  proxy.$api.createSubscriber(payload).then((response) => {
    emit('finished');
    emit('close');
    proxy.$utils.toast(proxy.$t('globals.messages.created', { name: response.name }));
  });
}

async function updateSubscriber() {
  let attribs = {};
  if (form.value.strAttribs) {
    attribs = validateAttribs(form.value.strAttribs);
    if (!attribs) {
      return;
    }
  }

  if (form.value.tags.length > 0) {
    attribs.tags = [...form.value.tags];
  } else {
    delete attribs.tags;
  }
  await persistSubscriberTagOptions();

  const payload = {
    id: form.value.id,
    record_id: form.value.id,
    email: form.value.email,
    phone: form.value.phone,
    first_name: form.value.firstName,
    last_name: form.value.lastName,
    status: form.value.status,
    preconfirm_subscriptions: form.value.preconfirm,
    attribs,

    // List IDs.
    list_record_ids: form.value.lists
      .map((l) => l.id)
      .filter((id) => typeof id === 'string' && id.length > 0),
  };

  proxy.$api.updateSubscriber(payload).then((response) => {
    emit('finished');
    emit('close');
    proxy.$utils.toast(proxy.$t('globals.messages.updated', { name: response.name }));
  });
}

function sendOptinConfirmation() {
  proxy.$api.sendSubscriberOptin(form.value.id).then(() => {
    proxy.$utils.toast(proxy.$t('subscribers.sentOptinConfirm'));
  });
}

function validateAttribs(str) {
  // Parse and validate attributes JSON.
  let attribs = {};
  try {
    attribs = JSON.parse(str);
  } catch (e) {
    proxy.$utils.toast(
      `${proxy.$t('subscribers.invalidJSON')}: ${e.toString()}`,
      'is-danger',
      3000,
    );
    return null;
  }

  if (attribs instanceof Array) {
    proxy.$utils.toast('Attributes should be a map {} and not an array []', 'is-danger', 3000);
    return null;
  }

  return attribs;
}

watch(data, () => {
  initForm();
}, { deep: true });

watch(isEditing, () => {
  initForm();
});

onMounted(() => {
  initForm();
  loadSubscriberTagOptions();

  nextTick(() => {
    if (focus.value && typeof focus.value.focus === 'function') {
      focus.value.focus();
    }
  });
});
</script>

<style scoped>
.admin-dialog-card {
  background: #fff;
  border: 1px solid #dce5f2;
  border-radius: 16px;
  box-shadow: 0 18px 45px rgba(15, 23, 42, 0.18);
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 48px);
  overflow: hidden;
  width: min(920px, calc(100vw - 32px));
}

.admin-dialog-head {
  background: linear-gradient(180deg, #ffffff 0%, #f8fbff 100%);
  border-bottom: 1px solid #ebf1fb;
  display: block;
  padding: 18px 20px;
}

.dialog-meta-row {
  align-items: flex-start;
  display: flex;
  justify-content: space-between;
}

.entity-meta {
  margin: 0;
}

.dialog-title {
  margin: 8px 0 0;
}

.admin-dialog-body {
  overflow: auto;
  padding: 24px 20px;
}

.admin-dialog-foot {
  background: #fff;
  border-top: 1px solid #ebf1fb;
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  padding: 16px 20px 20px;
}

.form-field {
  margin-bottom: 18px;
}

.form-help {
  color: #667085;
  font-size: 0.9rem;
  margin-top: 4px;
}

.tab-panel {
  background: #f8fbff;
  border: 1px solid #e7eefb;
  border-radius: 12px;
  padding: 14px;
}

.tab-window {
  overflow: visible;
}

.lists-actions {
  align-items: flex-start;
  display: flex;
  gap: 12px;
  justify-content: space-between;
  margin-top: 8px;
}

.lists-actions-main {
  flex: 1;
  min-width: 0;
}

.optin-action {
  align-self: flex-start;
}

.panel-head {
  align-items: center;
  display: flex;
  justify-content: space-between;
  margin-bottom: 12px;
}

.panel-head h5 {
  margin: 0;
}

.table-wrap {
  border: 1px solid #e4ebf7;
  border-radius: 12px;
  overflow-x: auto;
}

.dialog-table {
  border-collapse: collapse;
  width: 100%;
}

.dialog-table th {
  background: #f8fbff;
  color: #334155;
  font-weight: 600;
}

.dialog-table th,
.dialog-table td {
  border-bottom: 1px solid #eaecf0;
  padding: 12px 10px;
  text-align: left;
  vertical-align: top;
}

.subtle-row {
  color: #667085;
  font-size: 0.85rem;
  margin-top: 6px;
}

.meta-block {
  background: #f8fafc;
  border: 1px solid #e4e7ec;
  border-radius: 8px;
  margin-top: 8px;
  padding: 10px;
  white-space: pre-wrap;
}

.status-pill {
  background: #eff6ff;
  border-radius: 999px;
  color: #0f5bd8;
  display: inline-block;
  font-size: 0.85rem;
  font-weight: 600;
  padding: 6px 10px;
}

.status-pill.enabled {
  background: #eaf8ef;
  color: #177245;
}

.status-pill.blocklisted {
  background: #fff1f0;
  color: #c2413a;
}

.status-pill.neutral {
  background: #f5f7fb;
  color: #475467;
}

:deep(.v-field) {
  border-radius: 12px;
}

.dialog-action {
  height: 44px;
  min-width: 120px;
}

.settings-tabs {
  background: #f8fbff;
  border: 1px solid #d8dfec;
  border-radius: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 20px;
  padding: 6px;
  min-height: fit-content;
}

:deep(.settings-tabs .v-slide-group__content) {
  gap: 8px;
}

:deep(.settings-tabs .v-tab) {
  border-radius: 10px;
  color: #667085;
  font-size: 0.95rem;
  font-weight: 500;
  text-transform: none;
}

:deep(.settings-tabs .v-tab.v-tab--selected) {
  background: #fff;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.08);
  color: #0f5bd8;
  font-weight: 600;
}

@media (max-width: 640px) {
  .admin-dialog-head {
    padding: 16px;
  }

  .admin-dialog-body {
    padding: 16px;
  }

  .admin-dialog-foot {
    flex-direction: column-reverse;
    padding: 12px 16px 16px;
  }

  .dialog-action {
    min-width: 100%;
  }

  .dialog-meta-row {
    align-items: flex-start;
    flex-direction: column;
    gap: 10px;
  }

  .lists-actions {
    flex-direction: column;
  }

  .optin-action {
    width: 100%;
  }
}
</style>
