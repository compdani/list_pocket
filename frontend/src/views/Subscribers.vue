<template>
  <section class="subscribers">
    <header class="columns page-header">
      <div class="column is-10">
        <h1 class="title is-4">
          {{ $t('globals.terms.subscribers') }}
          <span v-if="!isNaN(subscribers.total)">
            (<span data-cy="count">{{ subscribers.total }}</span>)
          </span>
          <span v-if="currentList">
            &raquo; {{ currentList.name }}
            <span v-if="queryParams.subStatus" class="has-text-grey has-text-weight-normal is-capitalized">({{
              queryParams.subStatus }})</span>
          </span>
        </h1>
      </div>
      <div class="column has-text-right">
        <div v-if="$can('subscribers:manage')">
          <button type="button" @click.stop.prevent="showNewForm" data-cy="btn-new" class="btn-new admin-open-button">
            {{ $t('globals.buttons.new') }}
          </button>
        </div>
      </div>
    </header>

    <section class="subscribers-controls">
      <div class="columns">
        <div class="column is-8">
          <form @submit.prevent="onSubmit">
            <div>
              <b-field addons>
                <b-input @input="onSimpleQueryInput" v-model="queryInput" expanded
                  :placeholder="$t('subscribers.queryPlaceholder')" icon="magnify" ref="query"
                  :disabled="isSearchAdvanced" data-cy="search" />
                <p class="controls">
                  <b-button native-type="submit" type="is-primary" icon-left="magnify" :disabled="isSearchAdvanced"
                    data-cy="btn-search" />
                </p>
              </b-field>

              <div v-if="isSearchAdvanced">
                <b-input v-model="queryParams.queryExp" @keydown.enter="onAdvancedQueryEnter" type="textarea"
                  ref="queryExp" placeholder="subscribers.name LIKE '%user%' or subscribers.status='blocklisted'"
                  data-cy="query" />
                <span class="is-size-6 has-text-grey">
                  {{ $t('subscribers.advancedQueryHelp') }}.{{ ' ' }}
                  <a href="https://listmonk.app/docs/querying-and-segmentation" target="_blank"
                    rel="noopener noreferrer">
                    {{ $t('globals.buttons.learnMore') }}.
                  </a>
                </span>
                <div class="buttons">
                  <b-button native-type="submit" type="is-primary" icon-left="magnify" data-cy="btn-query">
                    {{
                      $t('subscribers.query') }}
                  </b-button>
                  <b-button @click.prevent="toggleAdvancedSearch" icon-left="cancel" data-cy="btn-query-reset">
                    {{ $t('subscribers.reset') }}
                  </b-button>
                </div>
              </div><!-- advanced query -->
            </div>
          </form>
          <div v-if="!isSearchAdvanced" class="toggle-advanced">
            <a href="#" @click.prevent="toggleAdvancedSearch" data-cy="btn-advanced-search">
              <b-icon icon="cog-outline" size="is-small" />
              {{ $t('subscribers.advancedQuery') }}
            </a>
          </div>
        </div><!-- search -->
      </div>
    </section><!-- control -->

    <br />
    <div class="actions mb-4">
      <a class="a" href="#" @click.prevent="exportSubscribers" data-cy="btn-export-subscribers">
        <b-icon icon="cloud-download-outline" size="is-small" />
        {{ $t('subscribers.export') }}
      </a>
      <template v-if="bulk.checked.length > 0">
        <a class="a" href="#" @click.prevent="showBulkListForm" data-cy="btn-manage-lists">
          <b-icon icon="format-list-bulleted-square" size="is-small" /> Manage lists
        </a>
        <a class="a" href="#" @click.prevent="deleteSubscribers" data-cy="btn-delete-subscribers">
          <b-icon icon="trash-can-outline" size="is-small" /> Delete
        </a>
        <a class="a" href="#" @click.prevent="blocklistSubscribers" data-cy="btn-manage-blocklist">
          <b-icon icon="account-off-outline" size="is-small" /> Blocklist
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

    <div class="table-pagination" v-if="subscribers.total > 0">
      <span class="page-indicator">{{ queryParams.page }}</span>
      <button class="button" type="button" :disabled="queryParams.page <= 1" @click="onPageChange(queryParams.page - 1)">
        <b-icon icon="chevron-left" size="is-small" />
      </button>
      <button class="button" type="button" :disabled="queryParams.page >= subscriberPageCount" @click="onPageChange(queryParams.page + 1)">
        <b-icon icon="chevron-right" size="is-small" />
      </button>
    </div>

    <div class="table-wrap">
      <table class="table is-fullwidth is-hoverable admin-table">
        <thead>
          <tr>
            <th class="checkbox-col">
              <input
                type="checkbox"
                :checked="isAllCurrentPageSubscribersChecked"
                aria-label="Select current page subscribers"
                @change="toggleCurrentPageSubscribers($event.target.checked)"
              />
            </th>
            <th>
              <button type="button" class="sort-button" @click="onSort('email', nextSortDirection('email'))">
                {{ $t('subscribers.email') }}
              </button>
            </th>
            <th>
              <button type="button" class="sort-button" @click="onSort('name', nextSortDirection('name'))">
                {{ $t('globals.fields.name') }}
              </button>
            </th>
            <th>{{ $t('globals.terms.lists') }}</th>
            <th>
              <button type="button" class="sort-button" @click="onSort('created_at', nextSortDirection('created_at'))">
                {{ $t('globals.fields.createdAt') }}
              </button>
            </th>
            <th>
              <button type="button" class="sort-button" @click="onSort('updated_at', nextSortDirection('updated_at'))">
                {{ $t('globals.fields.updatedAt') }}
              </button>
            </th>
            <th class="actions-col" />
          </tr>
        </thead>
        <tbody v-if="subscriberRows.length > 0">
          <tr v-for="row in subscriberRows" :key="row.id">
            <td class="checkbox-col">
              <input type="checkbox" :checked="isSubscriberChecked(row.id)" :aria-label="`Select subscriber ${row.email}`" @change="toggleSubscriberSelection(row, $event.target.checked)" />
            </td>
            <td>
              <button type="button" class="link-button" @click.stop.prevent="showEditForm(row)" :class="{ blocklisted: row.status === 'blocklisted' }">
                {{ row.email }}
              </button>
              <copy-text :text="`${row.email}`" hide-text />
              <b-tag v-if="row.status !== 'enabled'" :class="row.status" data-cy="blocklisted">
                {{ $t(`subscribers.status.${row.status}`) }}
              </b-tag>
              <div class="tag-list">
                <router-link v-for="l in row.lists" :key="l.id" :to="`/subscribers/lists/${l.id}`">
                  <b-tag :class="l.subscriptionStatus" size="is-small">
                    {{ l.name }}
                    <sup v-if="l.optin === 'double' || l.subscriptionStatus === 'unsubscribed'">
                      {{ $t(`subscribers.status.${l.subscriptionStatus}`) }}
                    </sup>
                  </b-tag>
                </router-link>
              </div>
            </td>
            <td>
              <button type="button" class="link-button" @click.stop.prevent="showEditForm(row)" :class="{ blocklisted: row.status === 'blocklisted' }">
                {{ row.name }}
              </button>
              <copy-text :text="`${row.name}`" hide-text />
            </td>
            <td>{{ listCount(row.lists) }}</td>
            <td>{{ $utils.niceDate(row.createdAt) }}</td>
            <td>{{ $utils.niceDate(row.updatedAt) }}</td>
            <td class="actions">
              <div class="action-group">
              <a
                :href="`/mailapi/subscribers/${row.id}/export`"
                class="action-button"
                data-cy="btn-download"
                :aria-label="$t('subscribers.downloadData')"
              >
                <b-icon icon="cloud-download-outline" size="is-small" />
              </a>
              <button
                v-if="$can('subscribers:manage')"
                type="button"
                class="action-button"
                @click.stop.prevent="showEditForm(row)"
                data-cy="btn-edit"
                :aria-label="$t('globals.buttons.edit')"
              >
                <b-icon icon="pencil-outline" size="is-small" />
              </button>
              <a
                v-if="$can('subscribers:manage')"
                href="#"
                class="action-button action-button-danger"
                @click.prevent="deleteSubscriber(row)"
                data-cy="btn-delete"
                :aria-label="$t('globals.buttons.delete')"
              >
                <b-icon icon="trash-can-outline" size="is-small" />
              </a>
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <empty-placeholder v-if="subscriberRows.length === 0 && !loading.subscribers" />
    </div>

    <div class="table-pagination" v-if="subscribers.total > 0">
      <span class="page-indicator">{{ queryParams.page }}</span>
      <button class="button" type="button" :disabled="queryParams.page <= 1" @click="onPageChange(queryParams.page - 1)">
        <b-icon icon="chevron-left" size="is-small" />
      </button>
      <button class="button" type="button" :disabled="queryParams.page >= subscriberPageCount" @click="onPageChange(queryParams.page + 1)">
        <b-icon icon="chevron-right" size="is-small" />
      </button>
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
          :is-editing="isEditing"
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
      // Current subscriber item being edited.
      curItem: null,
      isSearchAdvanced: false,
      isEditing: false,
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
        listID: null,
        page: 1,
        orderBy: 'id',
        order: 'desc',
        subStatus: null,
      },
    };
  },

  methods: {
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
      return this.bulk.checked.some((item) => item.id === id);
    },

    toggleSubscriberSelection(subscriber, checked) {
      if (checked) {
        if (!this.isSubscriberChecked(subscriber.id)) {
          this.bulk.checked = [...this.bulk.checked, subscriber];
        }
      } else {
        this.bulk.checked = this.bulk.checked.filter((item) => item.id !== subscriber.id);
      }
      this.onTableCheck();
    },

    toggleCurrentPageSubscribers(checked) {
      if (checked) {
        const checkedMap = new Map(this.bulk.checked.map((item) => [item.id, item]));
        this.subscriberRows.forEach((row) => {
          checkedMap.set(row.id, row);
        });
        this.bulk.checked = [...checkedMap.values()];
      } else {
        const currentIDs = new Set(this.subscriberRows.map((row) => row.id));
        this.bulk.checked = this.bulk.checked.filter((item) => !currentIDs.has(item.id));
      }
      this.onTableCheck();
    },

    // Show the edit list form.
    showEditForm(sub) {
      this.curItem = sub;
      this.isFormVisible = true;
      this.isEditing = true;
    },

    // Show the new list form.
    showNewForm() {
      this.curItem = {};
      this.isFormVisible = true;
      this.isEditing = false;
    },

    showBulkListForm() {
      this.isBulkListFormVisible = true;
    },

    onFormClose() {
      if (this.$route.params.id) {
        this.$router.push({ name: 'subscribers' });
      }
    },

    closeForm() {
      this.isFormVisible = false;
      this.onFormClose();
    },

    handleDialogModelUpdate(value) {
      this.isFormVisible = value;
      if (!value) {
        this.onFormClose();
      }
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
        list_id: this.queryParams.listID,
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
        delete qp.queryExp;
      }

      this.$nextTick(() => {
        this.$api.getSubscribers(qp).then(() => {
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
          const ids = this.bulk.checked.map((s) => s.id);
          this.$api.blocklistSubscribers({ ids })
            .then(() => this.querySubscribers());
        };
      } else {
        // 'All' is selected, blocklist by query.
        fn = () => {
          this.$api.blocklistSubscribersByQuery({
            search: this.queryParams.search,
            query: this.queryParams.queryExp,
            list_ids: this.queryParams.listID ? [this.queryParams.listID] : null,
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

        if (this.queryParams.listID) {
          q.append('list_id', this.queryParams.listID);
        }

        if (this.queryParams.subStatus) {
          q.append('subscription_status', this.queryParams.subStatus);
        }

        // Export selected subscribers.
        if (!this.bulk.all && this.bulk.checked.length > 0) {
          this.bulk.checked.map((s) => q.append('id', s.id));
        }

        document.location.href = `${uris.exportSubscribers}?${q.toString()}`;
      });
    },

    deleteSubscribers() {
      let fn = null;
      if (!this.bulk.all && this.bulk.checked.length > 0) {
        // If 'all' is not selected, delete subscribers by IDs.
        fn = () => {
          const ids = this.bulk.checked.map((s) => s.id);
          this.$api.deleteSubscribers({ id: ids })
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
            list_ids: this.queryParams.listID ? [this.queryParams.listID] : null,
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
        list_ids: this.queryParams.listID ? [this.queryParams.listID] : null,
        target_list_ids: lists.map((l) => l.id),
      };

      if (preconfirm) {
        data.status = 'confirmed';
      }

      let fn = null;
      if (!this.bulk.all && this.bulk.checked.length > 0) {
        // If 'all' is not selected, perform by IDs.
        fn = this.$api.addSubscribersToLists;
        data.ids = this.bulk.checked.map((s) => s.id);
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

    subscriberPageCount() {
      if (!this.subscribers.perPage || !this.subscribers.total) {
        return 1;
      }
      return Math.max(1, Math.ceil(this.subscribers.total / this.subscribers.perPage));
    },

    isAllCurrentPageSubscribersChecked() {
      return this.subscriberRows.length > 0
        && this.subscriberRows.every((row) => this.isSubscriberChecked(row.id));
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

      return `${this.isEditing ? 'edit' : 'new'}-${this.curItem?.id ?? 'new'}`;
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
      if (!this.queryParams.listID || !this.lists.results) {
        return null;
      }

      return this.lists.results.find((l) => l.id === this.queryParams.listID);
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
      this.queryParams.listID = parseInt(this.$route.params.listID, 10);
    }
    if (this.$route.query.subscription_status) {
      this.queryParams.subStatus = this.$route.query.subscription_status;
    }

    if (this.$route.params.id) {
      this.$api.getSubscriber(parseInt(this.$route.params.id, 10)).then((data) => {
        this.showEditForm(data);
      });
    } else {
      // Get subscribers on load.
      this.querySubscribers();
    }
  },
};
</script>

<style scoped>
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
  gap: 8px;
  justify-content: flex-end;
}

.action-button {
  align-items: center;
  background: #f5f7fb;
  border: 1px solid #dbe2ef;
  border-radius: 10px;
  color: #0f5bd8;
  display: inline-flex;
  height: 34px;
  justify-content: center;
  text-decoration: none;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
  width: 34px;
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

.admin-open-button {
  background: #0f5bd8;
  border: 1px solid #0f5bd8;
  border-radius: 10px;
  color: #fff;
  cursor: pointer;
  font-weight: 600;
  min-height: 44px;
  min-width: 96px;
  padding: 0 18px;
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
</style>
