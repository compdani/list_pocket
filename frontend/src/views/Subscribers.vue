<template>
  <section class="subscribers">
    <header class="page-header mb-4">
      <v-row align="center" class="ma-0">
        <v-col cols="12" md="9" class="px-0">
          <h1 class="text-h5 font-weight-semibold mb-0">
          {{ $t('globals.terms.subscribers') }}
          <span v-if="!isNaN(subscribers.total)">
            (<span data-cy="count">{{ subscribers.total }}</span>)
          </span>
          <span v-if="currentList">
            &raquo; {{ currentList.name }}
            <span v-if="queryParams.subStatus" class="text-medium-emphasis font-weight-regular text-capitalize">({{
              queryParams.subStatus }})</span>
          </span>
          </h1>
        </v-col>
        <v-col cols="12" md="3" class="px-0 d-flex justify-end justify-md-end">
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
        </v-col>
      </v-row>
    </header>

    <section class="subscribers-controls">
      <v-card class="mb-4 query-card" elevation="0">
        <v-card-text class="query-card-body">
          <form class="query-form" @submit.prevent="onSubmit">
            <div class="query-main-row">
              <v-text-field
                @update:model-value="onSimpleQueryInput"
                v-model="queryInput"
                class="query-input"
                name="query"
                :placeholder="$t('subscribers.queryPlaceholder')"
                prepend-inner-icon="mdi-magnify"
                variant="outlined"
                density="comfortable"
                hide-details
                ref="query"
                :disabled="isSearchAdvanced"
                data-cy="search"
              />
              <v-btn
                type="submit"
                class="query-submit"
                color="primary"
                icon="mdi-magnify"
                variant="flat"
                :disabled="isSearchAdvanced"
                data-cy="btn-search"
              />
            </div>

            <div v-if="isSearchAdvanced" class="advanced-query mt-2">
              <v-textarea
                v-model="queryParams.queryExp"
                @keydown.enter="onAdvancedQueryEnter"
                ref="queryExp"
                placeholder="subscribers.name LIKE '%user%' or subscribers.status='blocklisted'"
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
              <div class="buttons">
                <v-btn type="submit" color="primary" prepend-icon="mdi-magnify" data-cy="btn-query">
                  {{
                    $t('subscribers.query') }}
                </v-btn>
                <v-btn @click.prevent="toggleAdvancedSearch" prepend-icon="mdi-cancel" data-cy="btn-query-reset">
                  {{ $t('subscribers.reset') }}
                </v-btn>
              </div>
            </div>
          </form>
          <div v-if="!isSearchAdvanced" class="toggle-advanced">
            <a href="#" @click.prevent="toggleAdvancedSearch" data-cy="btn-advanced-search">
              <v-icon icon="mdi-cog-outline" size="16" />
              {{ $t('subscribers.advancedQuery') }}
            </a>
          </div>
        </v-card-text>
      </v-card>
    </section>
    <div class="actions mb-4">
      <a class="a" href="#" @click.prevent="exportSubscribers" data-cy="btn-export-subscribers">
        <v-icon icon="mdi-cloud-download-outline" size="16" />
        {{ $t('subscribers.export') }}
      </a>
      <template v-if="bulk.checked.length > 0">
        <a class="a" href="#" @click.prevent="showBulkListForm" data-cy="btn-manage-lists">
          <v-icon icon="mdi-format-list-bulleted-square" size="16" /> Manage lists
        </a>
        <a class="a" href="#" @click.prevent="deleteSubscribers" data-cy="btn-delete-subscribers">
          <v-icon icon="mdi-trash-can-outline" size="16" /> Delete
        </a>
        <a class="a" href="#" @click.prevent="blocklistSubscribers" data-cy="btn-manage-blocklist">
          <v-icon icon="mdi-account-off-outline" size="16" /> Blocklist
        </a>
        <span class="a">
          {{ $t('globals.messages.numSelected', { num: numSelectedSubscribers }) }}
          <span v-if="!bulk.all && subscribers.total > subscribers.perPage">
            &mdash;
            <a href="#" @click.prevent="selectAllSubscribers">
              {{ $t('globals.messages.selectAll', { num: subscribers.total }) }}
            </a>
          </span>
        </span>
      </template>
    </div>

    <div class="table-wrap">
      <v-data-table-server
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
        hide-default-footer
        @update:model-value="updateCheckedSubscribers"
        @update:options="onTableOptionsChange"
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
          <empty-placeholder v-if="!loading.subscribers" />
        </template>
      </v-data-table-server>
    </div>

    <div class="table-pagination" v-if="subscribers.total > 0">
      <v-pagination
        :length="subscriberPageCount"
        :model-value="queryParams.page"
        rounded="circle"
        total-visible="7"
        @update:model-value="onPageChange"
      />
    </div>

    <v-overlay
      :model-value="isBulkListFormVisible"
      class="admin-overlay align-center justify-center"
      scrim="rgba(15, 23, 42, 0.45)"
      @update:model-value="isBulkListFormVisible = $event"
    >
      <div class="admin-dialog-frame" style="max-width: 560px; width: calc(100vw - 32px);">
        <subscriber-bulk-list
          :num-subscribers="this.numSelectedSubscribers"
          @finished="bulkChangeLists"
          @close="isBulkListFormVisible = false"
        />
      </div>
    </v-overlay>

    <v-overlay
      :model-value="isFormVisible"
      class="admin-overlay align-center justify-center"
      scrim="rgba(15, 23, 42, 0.45)"
      @update:model-value="handleDialogModelUpdate"
    >
      <div class="admin-dialog-frame" style="max-width: 920px; width: calc(100vw - 32px);">
        <subscriber-form
          v-if="isFormVisible"
          :key="subscriberFormKey"
          :data="subscriberFormData"
          :is-editing="false"
          @finished="querySubscribers"
          @close="closeForm"
        />
      </div>
    </v-overlay>
  </section>
</template>

<script>
import { mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import { uris } from '../constants';
import SubscriberBulkList from './SubscriberBulkList.vue';
import SubscriberForm from './SubscriberForm.vue';
import CopyText from '../components/CopyText.vue';

export default {
  components: {
    SubscriberForm,
    SubscriberBulkList,
    CopyText,
    EmptyPlaceholder,
  },

  data() {
    return {
      // Current subscriber item for create overlay.
      curItem: null,
      isSearchAdvanced: false,
      isFormVisible: false,
      isBulkListFormVisible: false,

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
      this.isSearchAdvanced = !this.isSearchAdvanced;
      this.queryParams.search = '';

      // Toggling to simple search.
      if (!this.isSearchAdvanced) {
        this.queryInput = '';
        this.queryParams.queryExp = '';
        this.queryParams.page = 1;
        this.querySubscribers();
        if (this.$refs.query && typeof this.$refs.query.focus === 'function') {
          this.$refs.query.focus();
        }
        return;
      }

      // Toggling to advanced search.
      const q = this.queryInput.replace(/'/, "''").trim();
      if (q) {
        if (this.$utils.validateEmail(q)) {
          this.queryParams.queryExp = `email = '${q.toLowerCase()}'`;
        } else {
          this.queryParams.queryExp = `(name ~* '${q}' OR email ~* '${q.toLowerCase()}')`;
        }
      }

      // Toggling to advanced search.
      this.$nextTick(() => {
        if (this.$refs.queryExp && typeof this.$refs.queryExp.focus === 'function') {
          this.$refs.queryExp.focus();
        }
      });
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

    // Prepares an SQL expression for simple name search inputs and saves it
    // in this.queryExp.
    onSimpleQueryInput(v) {
      const q = v.replace(/'/, "''").trim();
      this.queryParams.queryExp = '';
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
    },

    // Search / query subscribers.
    querySubscribers(params) {
      this.queryParams = { ...this.queryParams, ...params };

      const qp = {
        list_record_id: this.queryParams.listRecordID,
        search: this.queryParams.search,
        query: this.queryParams.queryExp,
        page: this.queryParams.page,
        subscription_status: this.queryParams.subStatus,
        order_by: this.queryParams.orderBy,
        order: this.queryParams.order,
      };

      if (this.queryParams.queryExp) {
        delete qp.search;
      } else {
        delete qp.query;
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

        if (this.queryParams.search) {
          q.append('search', this.queryParams.search);
        } else if (this.queryParams.queryExp) {
          q.append('query', this.queryParams.queryExp);
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
            all: this.queryParams.queryExp.trim() === '' && this.queryParams.search.trim() === '',
            search: this.queryParams.search,
            query: this.queryParams.queryExp,
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
        query: this.fullQueryExp,
        search: this.queryParams.search,
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
  },

  created() {
    this.$events.$on('page.refresh', this.querySubscribers);
  },

  destroyed() {
    this.$events.$off('page.refresh', this.querySubscribers);
  },

  mounted() {
    if (this.$route.params.listID) {
      this.queryParams.listRecordID = this.$route.params.listID;
    }
    if (this.$route.query.subscription_status) {
      this.queryParams.subStatus = this.$route.query.subscription_status;
    }

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
