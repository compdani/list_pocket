<template>
  <section class="subscribers">
    <page-header :title="$t('globals.terms.subscribers')" :count="subscribers.total">
      <template #title>
        {{ $t('globals.terms.subscribers') }}
        <span v-if="currentList">
          &raquo; {{ currentList.name }}
          <span v-if="queryParams.subStatus" class="text-medium-emphasis font-weight-regular text-capitalize">
            ({{ queryParams.subStatus }})
          </span>
        </span>
      </template>
      <template #count>
        <span data-cy="count">{{ subscribers.total }}</span>
      </template>
      <template #actions>
        <v-btn
          v-if="$can('subscribers:manage')"
          type="button"
          color="primary"
          variant="flat"
          class="text-none"
          @click.stop.prevent="showNewForm"
          data-cy="btn-new"
        >
          {{ $t('globals.buttons.new') }}
        </v-btn>
      </template>
    </page-header>

    <section class="subscribers-controls">
      <query-bar
        v-model="queryInput"
        :placeholder="$t('subscribers.queryPlaceholder')"
        :search-label="$t('globals.buttons.search')"
        :disabled="isSearchAdvanced"
        input-cy="search"
        submit-cy="btn-search"
        @update:model-value="onSimpleQueryInput"
        @submit="onSubmit"
      >

            <div v-if="isSearchAdvanced" class="advanced-query mt-2">
              <div v-if="canUseSqlQuery" class="advanced-mode-tabs mb-3">
                <button
                  type="button"
                  class="mode-tab"
                  :class="{ active: advancedMode === 'filters' }"
                  data-cy="btn-mode-filters"
                  @click="setAdvancedMode('filters')"
                >
                  {{ $t('subscribers.filters.title') }}
                </button>
                <button
                  type="button"
                  class="mode-tab"
                  :class="{ active: advancedMode === 'sql' }"
                  data-cy="btn-mode-sql"
                  @click="setAdvancedMode('sql')"
                >
                  SQL
                </button>
              </div>

              <template v-if="advancedMode === 'filters'">
                <p class="text-body-2 text-medium-emphasis mb-3">
                  {{ $t('subscribers.filters.help') }}
                  <a :href="$docsUrl('querying-and-segmentation/')" target="_blank" rel="noopener noreferrer">
                    {{ $t('globals.buttons.learnMore') }}.
                  </a>
                </p>
                <subscriber-filter-builder
                  v-model="filterBuilder"
                  :tag-options="subscriberTagOptions"
                  :list-options="filterListOptions"
                />
              </template>

              <template v-else>
                <v-textarea
                  v-model="queryParams.queryExp"
                  @keydown.enter="onAdvancedQueryEnter"
                  ref="queryExp"
                  placeholder="subscribers.name LIKE '%user%' OR subscribers.status = 'blocklisted'"
                  auto-grow
                  rows="3"
                  density="comfortable"
                  hide-details
                  data-cy="query"
                />
                <span class="text-body-2 text-medium-emphasis">
                  {{ $t('subscribers.advancedQueryHelp') }}.{{ ' ' }}
                  <a :href="$docsUrl('querying-and-segmentation/')" target="_blank"
                    rel="noopener noreferrer">
                    {{ $t('globals.buttons.learnMore') }}.
                  </a>
                </span>
              </template>

              <div class="buttons mt-3">
                <v-btn type="submit" color="primary" prepend-icon="mdi-magnify" data-cy="btn-query">
                  {{ $t('subscribers.query') }}
                </v-btn>
                <v-btn @click.prevent="resetAdvancedSearch" prepend-icon="mdi-cancel" data-cy="btn-query-reset">
                  {{ $t('subscribers.reset') }}
                </v-btn>
              </div>
            </div>

          <div
            v-if="!isSearchAdvanced && activeFilterChips.length"
            class="active-filter-chips mt-2"
            data-cy="active-filter-chips"
          >
            <v-chip
              v-for="(chip, idx) in activeFilterChips"
              :key="`${chip.label}-${idx}`"
              size="small"
              variant="tonal"
              closable
              @click:close="clearFiltersAndRefresh"
            >
              {{ chip.label }}
            </v-chip>
            <button type="button" class="clear-filters-link" @click="clearFiltersAndRefresh">
              {{ $t('subscribers.reset') }}
            </button>
          </div>

          <div v-if="!isSearchAdvanced" class="toggle-advanced">
            <a href="#" @click.prevent="toggleAdvancedSearch" data-cy="btn-advanced-search">
              <v-icon icon="mdi-filter-variant" size="16" />
              {{ $t('subscribers.filters.title') }}
              <span v-if="activeFilterCount" class="filter-count">({{ activeFilterCount }})</span>
            </a>
          </div>
      </query-bar>
    </section>
    <div class="d-flex flex-wrap ga-2 mb-4">
      <v-btn
        variant="text"
        size="small"
        prepend-icon="mdi-cloud-download-outline"
        data-cy="btn-export-subscribers"
        @click="exportSubscribers"
      >
        {{ $t('subscribers.export') }}
      </v-btn>
    </div>
    <bulk-action-bar
      :selected-count="bulk.checked.length"
      :total="subscribers.total"
      :show-select-all="!bulk.all && subscribers.total > subscribers.perPage"
      @select-all="selectAllSubscribers"
    >
      <v-btn
        variant="text"
        size="small"
        prepend-icon="mdi-format-list-bulleted-square"
        data-cy="btn-manage-lists"
        @click="showBulkListForm"
      >
        {{ $t('subscribers.manageLists') }}
      </v-btn>
      <v-btn
        variant="text"
        size="small"
        prepend-icon="mdi-trash-can-outline"
        data-cy="btn-delete-subscribers"
        @click="deleteSubscribers"
      >
        {{ $t('globals.buttons.delete') }}
      </v-btn>
      <v-btn
        variant="text"
        size="small"
        prepend-icon="mdi-account-off-outline"
        data-cy="btn-manage-blocklist"
        @click="blocklistSubscribers"
      >
        {{ $t('subscribers.status.blocklisted') }}
      </v-btn>
    </bulk-action-bar>

    <admin-data-table
        :headers="tableHeaders"
        :items="subscriberRows"
        :items-length="subscribers.total || 0"
        :loading="loading.subscribers"
        :page="queryParams.page"
        :items-per-page="subscribers.perPage || 20"
        :sort-by="tableSortBy"
        :model-value="bulk.checked"
        class="admin-data-table subscribers-table"
        item-value="id"
        return-object
        show-select
        select-strategy="page"
        @update:model-value="updateCheckedSubscribers"
        @update:options="onTableOptionsChange"
        @update:page="onPageChange"
      >
        <template #[`item.email`]="{ item }">
          <div>
            <button type="button" class="link-button" @click.stop.prevent="openSubscriberPage(item)" :class="{ blocklisted: item.status === 'blocklisted' }">
              {{ item.email }}
            </button>
            <copy-text :text="`${item.email}`" hide-text />
            <v-chip v-if="item.status !== 'enabled'" :class="['subscriber-status-chip', item.status]" size="small" variant="tonal" data-cy="blocklisted">
              {{ $t(`subscribers.status.${item.status}`) }}
            </v-chip>
            <div v-if="item.phone" class="subtle-row">
              {{ item.phone }}
            </div>
            <div class="tag-list">
              <router-link v-for="l in item.lists" :key="l.id" :to="`/subscribers/lists/${l.id}`">
                <v-chip :class="['subscriber-list-chip', l.subscriptionStatus]" size="small" variant="tonal">
                  {{ l.name }}
                  <sup v-if="l.optin === 'double' || l.subscriptionStatus === 'unsubscribed'">
                    {{ $t(`subscribers.status.${l.subscriptionStatus}`) }}
                  </sup>
                </v-chip>
              </router-link>
            </div>
          </div>
        </template>

        <template #[`item.name`]="{ item }">
          <div>
            <button type="button" class="link-button" @click.stop.prevent="openSubscriberPage(item)" :class="{ blocklisted: item.status === 'blocklisted' }">
              {{ item.name }}
            </button>
            <copy-text :text="`${item.name}`" hide-text />
          </div>
        </template>

        <template #[`item.lists_count`]="{ item }">
          {{ listCount(item.lists) }}
        </template>

        <template #[`item.created_at`]="{ item }">
          {{ $utils.niceDate(item.createdAt) }}
        </template>

        <template #[`item.updated_at`]="{ item }">
          {{ $utils.niceDate(item.updatedAt) }}
        </template>

        <template #[`item.actions`]="{ item }">
          <div class="action-group">
            <button
              type="button"
              class="action-button"
              data-cy="btn-download"
              :aria-label="$t('subscribers.downloadData')"
              @click.stop.prevent="downloadSubscriber(item)"
            >
              <v-icon icon="mdi-cloud-download-outline" size="18" />
            </button>
            <button
              v-if="$can('subscribers:manage')"
              type="button"
              class="action-button"
              @click.stop.prevent="openSubscriberPage(item)"
              data-cy="btn-edit"
              :aria-label="$t('globals.buttons.edit')"
            >
              <v-icon icon="mdi-pencil-outline" size="18" />
            </button>
            <a
              v-if="$can('subscribers:manage')"
              href="#"
              class="action-button action-button-danger"
              @click.prevent="deleteSubscriber(item)"
              data-cy="btn-delete"
              :aria-label="$t('globals.buttons.delete')"
            >
              <v-icon icon="mdi-trash-can-outline" size="18" />
            </a>
          </div>
        </template>

        <template #no-data>
          <empty-placeholder
            v-if="!loading.subscribers"
            icon="mdi-account-multiple-outline"
            :label="$t('globals.messages.emptyState')"
            :action-label="$can('subscribers:manage') ? $t('globals.buttons.new') : ''"
            @action="showNewForm"
          />
        </template>
      </admin-data-table>

    <v-dialog
      v-model="isBulkListFormVisible"
      max-width="560"
      scrollable
    >
      <subscriber-bulk-list
        :num-subscribers="numSelectedSubscribers"
        @finished="bulkChangeLists"
        @close="isBulkListFormVisible = false"
      />
    </v-dialog>

    <v-dialog
      :model-value="isFormVisible"
      max-width="920"
      scrollable
      @update:model-value="handleDialogModelUpdate"
    >
      <subscriber-form
        v-if="isFormVisible"
        :key="subscriberFormKey"
        :data="subscriberFormData"
        :is-editing="false"
        @finished="querySubscribers"
        @close="closeForm"
      />
    </v-dialog>
  </section>
</template>

<script>
import { mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import PageHeader from '../components/PageHeader.vue';
import QueryBar from '../components/QueryBar.vue';
import BulkActionBar from '../components/BulkActionBar.vue';
import AdminDataTable from '../components/AdminDataTable.vue';
import SubscriberFilterBuilder from '../components/SubscriberFilterBuilder.vue';
import { uris } from '../constants';
import {
  countActiveFilterRules,
  describeFilterRule,
  emptyFilterGroup,
  flattenFilterRules,
  hydrateFilterBuilder,
  serializeSubscriberFilters,
} from '../utils/subscriberFilters';
import SubscriberBulkList from './SubscriberBulkList.vue';
import SubscriberForm from './SubscriberForm.vue';
import CopyText from '../components/CopyText.vue';
import { events } from '../utils/events';

export default {
  components: {
    SubscriberForm,
    SubscriberBulkList,
    SubscriberFilterBuilder,
    CopyText,
    EmptyPlaceholder,
    PageHeader,
    QueryBar,
    BulkActionBar,
    AdminDataTable,
  },

  data() {
    return {
      // Current subscriber item for create overlay.
      curItem: null,
      isSearchAdvanced: false,
      advancedMode: 'filters',
      isFormVisible: false,
      isBulkListFormVisible: false,
      filterBuilder: emptyFilterGroup(),
      appliedFiltersJson: null,
      subscriberTagOptions: [],

      // Table bulk row selection states.
      bulk: {
        checked: [],
        all: false,
      },

      queryInput: '',

      // Query params to filter the getSubscribers() API call.
      queryParams: {
        // Search query expression.
        queryExp: '',
        search: '',

        // ID of the list the current subscriber view is filtered by.
        listRecordID: null,
        page: 1,
        orderBy: 'id',
        order: 'desc',
        subStatus: null,
      },
    };
  },

  methods: {
    subscriberValue(subscriber) {
      if (!subscriber) {
        return '';
      }

      return String(subscriber.id || '');
    },

    // Count the lists from which a subscriber has not unsubscribed.
    listCount(lists) {
      return lists.reduce((defVal, item) => (defVal + (item.subscriptionStatus !== 'unsubscribed' ? 1 : 0)), 0);
    },

    toggleAdvancedSearch() {
      this.isSearchAdvanced = true;
      this.advancedMode = this.canUseSqlQuery && this.queryParams.queryExp ? 'sql' : 'filters';
      if (this.advancedMode === 'filters') {
        this.queryParams.queryExp = '';
        this.filterBuilder = hydrateFilterBuilder(this.appliedFiltersJson);
      }
      this.loadSubscriberTagOptions();
    },

    resetAdvancedSearch() {
      this.isSearchAdvanced = false;
      this.advancedMode = 'filters';
      this.filterBuilder = emptyFilterGroup();
      this.appliedFiltersJson = null;
      this.queryParams.queryExp = '';
      this.queryParams.page = 1;
      this.querySubscribers();
      this.$nextTick(() => {
        if (this.$refs.query && typeof this.$refs.query.focus === 'function') {
          this.$refs.query.focus();
        }
      });
    },

    setAdvancedMode(mode) {
      if (mode === 'sql' && !this.canUseSqlQuery) {
        return;
      }
      if (mode === this.advancedMode) {
        return;
      }
      // Mutually exclusive: clear the other mode when switching.
      if (mode === 'sql') {
        this.filterBuilder = emptyFilterGroup();
        this.appliedFiltersJson = null;
      } else {
        this.queryParams.queryExp = '';
      }
      this.advancedMode = mode;
    },

    clearFiltersAndRefresh() {
      this.filterBuilder = emptyFilterGroup();
      this.appliedFiltersJson = null;
      this.queryParams.queryExp = '';
      this.queryParams.page = 1;
      this.querySubscribers();
    },

    loadSubscriberTagOptions() {
      this.$api.getSubscriberTagsCatalog().then((tags) => {
        this.subscriberTagOptions = tags || [];
      }).catch(() => {
        this.subscriberTagOptions = [];
      });
    },

    currentFiltersPayload() {
      if (this.advancedMode === 'sql' && this.queryParams.queryExp) {
        return null;
      }
      // Prefer in-progress builder when advanced filters panel is open; otherwise applied.
      if (this.isSearchAdvanced && this.advancedMode === 'filters') {
        return serializeSubscriberFilters(this.filterBuilder);
      }
      return this.appliedFiltersJson;
    },

    // Mark all subscribers in the query as selected.
    selectAllSubscribers() {
      this.bulk.all = true;
    },

    onTableCheck() {
      // Disable bulk.all selection if there are no rows checked in the table.
      if (this.bulk.checked.length !== this.subscribers.total) {
        this.bulk.all = false;
      }
    },

    isSubscriberChecked(id) {
      return this.bulk.checked.some((item) => this.subscriberValue(item) === id);
    },

    toggleSubscriberSelection(subscriber, checked) {
      const subscriberID = this.subscriberValue(subscriber);
      if (checked) {
        if (!this.isSubscriberChecked(subscriberID)) {
          this.bulk.checked = [...this.bulk.checked, subscriber];
        }
      } else {
        this.bulk.checked = this.bulk.checked.filter((item) => this.subscriberValue(item) !== subscriberID);
      }
      this.onTableCheck();
    },

    toggleCurrentPageSubscribers(checked) {
      if (checked) {
        const checkedMap = new Map(this.bulk.checked.map((item) => [this.subscriberValue(item), item]));
        this.subscriberRows.forEach((row) => {
          checkedMap.set(this.subscriberValue(row), row);
        });
        this.bulk.checked = [...checkedMap.values()];
      } else {
        const currentIDs = new Set(this.subscriberRows.map((row) => this.subscriberValue(row)));
        this.bulk.checked = this.bulk.checked.filter((item) => !currentIDs.has(this.subscriberValue(item)));
      }
      this.onTableCheck();
    },

    // Navigate to dedicated contact page.
    openSubscriberPage(sub) {
      const id = this.subscriberValue(sub);
      if (!id) {
        return;
      }
      this.$router.push({ name: 'subscriber', params: { id } });
    },

    // Show the new list form.
    showNewForm() {
      this.curItem = {};
      this.isFormVisible = true;
    },

    showBulkListForm() {
      this.isBulkListFormVisible = true;
    },

    closeForm() {
      this.isFormVisible = false;
    },

    handleDialogModelUpdate(value) {
      this.isFormVisible = value;
    },

    onPageChange(p) {
      this.querySubscribers({ page: p });
    },

    onSort(field, direction) {
      this.querySubscribers({ orderBy: field, order: direction });
    },

    nextSortDirection(field) {
      return this.queryParams.orderBy === field && this.queryParams.order === 'asc' ? 'desc' : 'asc';
    },

    onTableOptionsChange(options) {
      const [sort] = options.sortBy || [];
      const nextPage = options.page || 1;
      const nextOrderBy = sort?.key || this.queryParams.orderBy;
      const nextOrder = sort?.order || this.queryParams.order;

      if (
        nextPage === this.queryParams.page
        && nextOrderBy === this.queryParams.orderBy
        && nextOrder === this.queryParams.order
      ) {
        return;
      }

      this.querySubscribers({
        page: nextPage,
        orderBy: nextOrderBy,
        order: nextOrder,
      });
    },

    updateCheckedSubscribers(rows) {
      this.bulk.checked = rows;
      this.onTableCheck();
    },

    onSimpleQueryInput(v) {
      const q = String(v || '').replace(/'/, "''").trim();
      this.queryParams.page = 1;
      this.queryParams.search = q.toLowerCase();
    },

    // Ctrl + Enter on the advanced query searches.
    onAdvancedQueryEnter(e) {
      if (e.ctrlKey) {
        this.onSubmit();
      }
    },

    onSubmit() {
      this.querySubscribers({ page: 1 });
      // Collapse the builder after applying filters so chips remain visible.
      if (this.isSearchAdvanced && this.advancedMode === 'filters') {
        this.isSearchAdvanced = false;
      }
    },

    // Search / query subscribers.
    querySubscribers(params) {
      this.queryParams = { ...this.queryParams, ...params };

      const filtersJson = this.currentFiltersPayload();
      if (this.isSearchAdvanced && this.advancedMode === 'filters') {
        this.appliedFiltersJson = filtersJson;
      }

      const qp = {
        list_record_id: this.queryParams.listRecordID,
        search: this.queryParams.search,
        query: this.advancedMode === 'sql' ? this.queryParams.queryExp : '',
        filters: filtersJson,
        page: this.queryParams.page,
        subscription_status: this.queryParams.subStatus,
        order_by: this.queryParams.orderBy,
        order: this.queryParams.order,
      };

      if (qp.query) {
        delete qp.filters;
      } else {
        delete qp.query;
      }
      if (!qp.filters) {
        delete qp.filters;
      }

      const filteredQP = Object.fromEntries(
        Object.entries(qp).filter(([, value]) => value !== '' && value !== null && value !== undefined),
      );

      this.$nextTick(() => {
        this.$api.getSubscribers(filteredQP).then(() => {
          this.bulk.checked = [];
        });
      });
    },

    deleteSubscriber(sub) {
      this.$utils.confirm(
        null,
        () => {
          this.$api.deleteSubscriber(sub.id).then(() => {
            this.querySubscribers();

            this.$utils.toast(this.$t('globals.messages.deleted', { name: sub.name }));
          });
        },
      );
    },

    blocklistSubscribers() {
      let fn = null;
      if (!this.bulk.all && this.bulk.checked.length > 0) {
        // If 'all' is not selected, blocklist subscribers by IDs.
        fn = () => {
          const subscriberRecordIDs = this.bulk.checked
            .map((s) => this.subscriberValue(s))
            .filter((id) => typeof id === 'string' && id.length > 0);
          this.$api.blocklistSubscribers({ subscriber_record_ids: subscriberRecordIDs })
            .then(() => this.querySubscribers());
        };
      } else {
        // 'All' is selected, blocklist by query.
        fn = () => {
          this.$api.blocklistSubscribersByQuery({
            search: this.queryParams.search,
            query: this.queryParams.queryExp,
            filters: this.appliedFiltersJson ? JSON.parse(this.appliedFiltersJson) : null,
            list_record_ids: this.queryParams.listRecordID ? [this.queryParams.listRecordID] : null,
            subscription_status: this.queryParams.subStatus,
          }).then(() => this.querySubscribers());
        };
      }

      this.$utils.confirm(this.$t('subscribers.confirmBlocklist', { num: this.numSelectedSubscribers }), fn);
    },

    exportSubscribers() {
      const num = !this.bulk.all && this.bulk.checked.length > 0
        ? this.bulk.checked.length : this.subscribers.total;

      this.$utils.confirm(this.$t('subscribers.confirmExport', { num }), () => {
        const q = new URLSearchParams();

        if (this.queryParams.queryExp) {
          q.append('query', this.queryParams.queryExp);
        } else {
          if (this.queryParams.search) {
            q.append('search', this.queryParams.search);
          }
          if (this.appliedFiltersJson) {
            q.append('filters', this.appliedFiltersJson);
          }
        }

        if (this.queryParams.listRecordID) {
          q.append('list_record_id', this.queryParams.listRecordID);
        }

        if (this.queryParams.subStatus) {
          q.append('subscription_status', this.queryParams.subStatus);
        }

        // Export selected subscribers.
        if (!this.bulk.all && this.bulk.checked.length > 0) {
          this.bulk.checked.forEach((s) => q.append('subscriber_record_id', this.subscriberValue(s)));
        }

        this.downloadWithAuth(`${uris.exportSubscribers}?${q.toString()}`, 'subscribers-export.csv');
      });
    },

    async downloadSubscriber(subscriber) {
      const subscriberID = this.subscriberValue(subscriber);
      await this.downloadWithAuth(`/mailapi/subscribers/${subscriberID}/export`, `subscriber-${subscriberID}.json`);
    },

    async downloadWithAuth(url, fallbackFilename) {
      try {
        const token = this.$api.getAuthToken();
        const response = await fetch(url, {
          headers: token ? { Authorization: token } : {},
          credentials: 'omit',
        });

        if (!response.ok) {
          const payload = await response.json().catch(() => null);
          throw new Error(payload && payload.message ? payload.message : 'Download failed');
        }

        const blob = await response.blob();
        const disposition = response.headers.get('content-disposition') || '';
        const filenameMatch = disposition.match(/filename\*?=(?:UTF-8''|")?([^";]+)/i);
        const filename = filenameMatch ? decodeURIComponent(filenameMatch[1].replace(/"/g, '')) : fallbackFilename;
        const objectUrl = window.URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = objectUrl;
        link.download = filename;
        document.body.appendChild(link);
        link.click();
        link.remove();
        window.URL.revokeObjectURL(objectUrl);
      } catch (err) {
        this.$utils.toast(err && err.message ? err.message : 'Download failed', 'is-danger');
      }
    },

    deleteSubscribers() {
      let fn = null;
      if (!this.bulk.all && this.bulk.checked.length > 0) {
        // If 'all' is not selected, delete subscribers by IDs.
        fn = () => {
          const subscriberRecordIDs = this.bulk.checked
            .map((s) => this.subscriberValue(s))
            .filter((id) => typeof id === 'string' && id.length > 0);
          this.$api.deleteSubscribers({ subscriber_record_id: subscriberRecordIDs })
            .then(() => {
              this.querySubscribers();

              this.$utils.toast(this.$t('subscribers.subscribersDeleted', { num: this.numSelectedSubscribers }));
            });
        };
      } else {
        // 'All' is selected, delete by query.
        fn = () => {
          this.$api.deleteSubscribersByQuery({
            // If the query expression is empty, explicitly pass `all=true`
            // so that the backend deletes all records in the DB with an empty query string.
            all: this.queryParams.queryExp.trim() === ''
              && this.queryParams.search.trim() === ''
              && !this.appliedFiltersJson,
            search: this.queryParams.search,
            query: this.queryParams.queryExp,
            filters: this.appliedFiltersJson ? JSON.parse(this.appliedFiltersJson) : null,
            list_record_ids: this.queryParams.listRecordID ? [this.queryParams.listRecordID] : null,
            subscription_status: this.queryParams.subStatus,
          }).then(() => {
            this.querySubscribers();

            this.$utils.toast(this.$t(
              'subscribers.subscribersDeleted',
              { num: this.numSelectedSubscribers },
            ));
          });
        };
      }

      this.$utils.confirm(this.$t('subscribers.confirmDelete', { num: this.numSelectedSubscribers }), fn);
    },

    bulkChangeLists(action, preconfirm, lists) {
      const data = {
        action,
        query: this.queryParams.queryExp,
        search: this.queryParams.search,
        filters: this.appliedFiltersJson ? JSON.parse(this.appliedFiltersJson) : null,
        list_record_ids: this.queryParams.listRecordID ? [this.queryParams.listRecordID] : null,
        target_list_record_ids: lists
          .map((l) => l.id)
          .filter((id) => typeof id === 'string' && id.length > 0),
      };

      if (preconfirm) {
        data.status = 'confirmed';
      }

      let fn = null;
      if (!this.bulk.all && this.bulk.checked.length > 0) {
        // If 'all' is not selected, perform by IDs.
        fn = this.$api.addSubscribersToLists;
        data.subscriber_record_ids = this.bulk.checked
          .map((s) => this.subscriberValue(s))
          .filter((id) => typeof id === 'string' && id.length > 0);
      } else {
        // 'All' is selected, perform by query.
        data.query = this.queryParams.queryExp;
        data.subscription_status = this.queryParams.subStatus;
        fn = this.$api.addSubscribersToListsByQuery;
      }

      fn(data).then(() => {
        this.querySubscribers();
        this.$utils.toast(this.$t('subscribers.listChangeApplied'));
      });
    },
  },

  computed: {
    ...mapState(['subscribers', 'lists', 'loading']),

    subscriberRows() {
      return this.subscribers.results ?? [];
    },

    tableHeaders() {
      return [
        { title: this.$t('subscribers.email'), key: 'email', sortable: true },
        { title: this.$t('globals.fields.name'), key: 'name', sortable: true },
        { title: this.$t('globals.terms.lists'), key: 'lists_count', sortable: false },
        { title: this.$t('globals.fields.createdAt'), key: 'created_at', sortable: true },
        { title: this.$t('globals.fields.updatedAt'), key: 'updated_at', sortable: true },
        {
          title: '',
          key: 'actions',
          sortable: false,
          align: 'end',
        },
      ];
    },

    tableSortBy() {
      return [{
        key: this.queryParams.orderBy,
        order: this.queryParams.order,
      }];
    },

    subscriberPageCount() {
      if (!this.subscribers.perPage || !this.subscribers.total) {
        return 1;
      }
      return Math.max(1, Math.ceil(this.subscribers.total / this.subscribers.perPage));
    },

    numSelectedSubscribers() {
      if (this.bulk.all) {
        return this.subscribers.total;
      }
      return this.bulk.checked.length;
    },

    subscriberFormKey() {
      if (!this.isFormVisible) {
        return 'subscriber-form-hidden';
      }

      return `new-${this.curItem?.id || 'new'}`;
    },

    subscriberFormData() {
      const item = this.curItem && typeof this.curItem === 'object' ? this.curItem : {};
      const normalized = { ...item };
      normalized.lists = Array.isArray(item.lists) ? item.lists : [];
      normalized.attribs = item.attribs && typeof item.attribs === 'object' ? item.attribs : {};
      return normalized;
    },

    // Returns the list that the subscribers are being filtered by in.
    currentList() {
      if (!this.queryParams.listRecordID || !this.lists.results) {
        return null;
      }

      return this.lists.results.find((l) => (
        String(l.id) === this.queryParams.listRecordID
      ));
    },

    canUseSqlQuery() {
      return this.$can('subscribers:sql_query');
    },

    filterListOptions() {
      const results = this.lists?.results || [];
      return results.map((l) => ({
        title: l.name,
        value: String(l.id),
      }));
    },

    listNameById() {
      return Object.fromEntries(this.filterListOptions.map((l) => [l.value, l.title]));
    },

    activeFilterCount() {
      if (this.appliedFiltersJson) {
        try {
          return countActiveFilterRules(JSON.parse(this.appliedFiltersJson));
        } catch {
          return 0;
        }
      }
      return 0;
    },

    activeFilterChips() {
      if (!this.appliedFiltersJson) {
        return [];
      }
      try {
        const rules = flattenFilterRules(JSON.parse(this.appliedFiltersJson));
        return rules.map((rule) => ({
          label: describeFilterRule(rule, this.listNameById),
        }));
      } catch {
        return [];
      }
    },
  },

  created() {
    events.$on('page.refresh', this.querySubscribers);
  },

  beforeUnmount() {
    events.$off('page.refresh', this.querySubscribers);
  },

  mounted() {
    if (this.$route.params.listID) {
      this.queryParams.listRecordID = this.$route.params.listID;
    }
    if (this.$route.query.subscription_status) {
      this.queryParams.subStatus = this.$route.query.subscription_status;
    }

    this.loadSubscriberTagOptions();
    // Get subscribers on load.
    this.querySubscribers();
  },
};
</script>

<style scoped>
.subscribers {
  --subscribers-border: #dce5f2;
  --subscribers-border-strong: #c7d5ea;
  --subscribers-surface-soft: #f6f9ff;
}

.subscribers-controls {
  margin-bottom: 8px;
}

.query-card {
  background: linear-gradient(180deg, #ffffff 0%, #f6f9ff 100%);
  border: 1px solid var(--subscribers-border);
  border-radius: 16px;
  box-shadow: 0 8px 20px rgba(15, 76, 129, 0.05);
}

.query-card-body {
  padding: 16px;
}

.advanced-mode-tabs {
  display: inline-flex;
  gap: 4px;
  background: #eef3fb;
  border-radius: 10px;
  padding: 4px;
}

.mode-tab {
  background: transparent;
  border: 0;
  border-radius: 8px;
  color: #475569;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  padding: 6px 12px;
}

.mode-tab.active {
  background: #fff;
  box-shadow: 0 1px 2px rgba(15, 76, 129, 0.12);
  color: #0f4c81;
}

.active-filter-chips {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.clear-filters-link {
  background: none;
  border: 0;
  color: #0f4c81;
  cursor: pointer;
  font-size: 13px;
  text-decoration: underline;
}

.filter-count {
  color: #64748b;
  font-weight: 500;
}

.query-form {
  display: block;
}

.query-main-row {
  align-items: center;
  display: flex;
  gap: 12px;
}

.query-input {
  flex: 1;
}

.query-input :deep(.v-field) {
  border-radius: 12px;
}

.query-submit {
  border-radius: 12px;
  min-height: 44px;
  min-width: 44px;
}

.advanced-query {
  margin-top: 10px;
}

.subscribers-table {
  background: #fff;
  border: 1px solid var(--subscribers-border);
  border-radius: 16px;
  overflow: hidden;
}

.subscribers-table :deep(thead th) {
  background: var(--subscribers-surface-soft);
  border-bottom: 1px solid var(--subscribers-border-strong) !important;
  color: #334155;
  font-weight: 600;
}

.subscribers-table :deep(tbody td) {
  padding-bottom: 16px !important;
  padding-top: 16px !important;
  vertical-align: top;
}

.subscribers-table :deep(tbody tr:hover) {
  background: #f8fbff;
}

.subscribers-table :deep(.v-data-table__tr--selected) {
  background: #edf5ff;
}

.admin-table {
  background: #fff;
  border-radius: 16px;
  overflow: hidden;
}

.checkbox-col {
  width: 44px;
}

.actions-col,
.actions {
  text-align: right;
  white-space: nowrap;
}

.action-group {
  align-items: center;
  display: inline-flex;
  gap: 6px;
  justify-content: flex-end;
}

.action-button {
  align-items: center;
  background: #f5f7fb;
  border: 1px solid #dbe2ef;
  border-radius: 10px;
  color: #0f5bd8;
  display: inline-flex;
  height: 36px;
  justify-content: center;
  text-decoration: none;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
  width: 36px;
}

.action-button:hover {
  background: #e8f0ff;
  border-color: #bfd1fb;
  color: #0a47a7;
}

.action-button:focus-visible {
  box-shadow: 0 0 0 3px rgba(15, 91, 216, 0.18);
  outline: none;
}

.action-button-danger {
  color: #cc3b2f;
}

.action-button-danger:hover {
  background: #fff0ee;
  border-color: #f4c4bc;
  color: #a92a1f;
}

.action-button :deep(.v-icon) {
  font-size: 18px;
}

.link-button,
.icon-button {
  background: none;
  border: 0;
  cursor: pointer;
  padding: 0;
}

.link-button {
  color: inherit;
  font: inherit;
}

.sort-button {
  background: none;
  border: 0;
  cursor: pointer;
  font: inherit;
  padding: 0;
}

.table-wrap {
  overflow-x: auto;
}

.admin-dialog-frame {
  background: transparent;
  box-shadow: none;
  overflow: visible;
}

.admin-overlay {
  padding: 16px;
  z-index: 2400;
}

.table-pagination {
  align-items: center;
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  margin-bottom: 16px;
}

.page-indicator {
  background: #0f5bd7;
  border-radius: 6px;
  color: #fff;
  min-width: 40px;
  padding: 10px 12px;
  text-align: center;
}

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

.subscriber-status-chip.blocklisted {
  background: #fff0ee;
  color: #a92a1f;
}

.subscriber-status-chip.bounced {
  background: #fff5e8;
  color: #8a4b00;
}

.subscriber-status-chip.unsubscribed {
  background: #eef2f8;
  color: #475569;
}

.subscriber-list-chip.unconfirmed {
  background: #fff5e8;
  color: #8a4b00;
}

.subscriber-list-chip.confirmed,
.subscriber-list-chip.enabled {
  background: #e8f7ee;
  color: #21693f;
}

.subscriber-list-chip.unsubscribed {
  background: #eef2f8;
  color: #475569;
}

@media (max-width: 960px) {
  .query-main-row {
    align-items: stretch;
    flex-direction: column;
  }

  .query-submit {
    width: 100%;
  }

  .table-pagination {
    justify-content: center;
  }
}
</style>
