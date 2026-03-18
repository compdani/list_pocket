<template>
  <section class="lists">
    <header class="page-header">
      <v-row align="center" class="ma-0">
        <v-col cols="12" md="9" class="px-0">
          <h1 class="text-h5 font-weight-semibold mb-2">
          {{ $t('globals.terms.lists') }}
          <span v-if="queryParams.status === 'archived'" class="text-medium-emphasis">/ {{ queryParams.status }} </span>
          <span v-if="!isNaN(lists.total)">({{ lists.total }})</span>
          </h1>

          <div class="text-caption">
            <router-link v-if="queryParams.status !== 'archived'" :to="{ name: 'lists', query: { status: 'archived' } }">
              {{ $t('globals.buttons.view') }} {{ $t('lists.archived').toLowerCase() }} &rarr;
            </router-link>
            <router-link v-else :to="{ name: 'lists' }">
              {{ $t('globals.buttons.view') }} {{ $t('menu.allLists').toLowerCase() }} &rarr;
            </router-link>
          </div>
        </v-col>
        <v-col cols="12" md="3" class="px-0 d-flex justify-end justify-md-end">
          <v-btn
            v-if="$can('lists:manage_all')"
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

    <v-card class="mb-4 query-card" elevation="0">
      <v-card-text class="query-card-body">
        <form class="query-form" @submit.prevent="onSearchSubmit">
          <v-text-field
            v-model="queryParams.query"
            class="query-input"
            name="query"
            :placeholder="$t('globals.fields.name')"
            prepend-inner-icon="mdi-magnify"
            variant="outlined"
            density="comfortable"
            hide-details
            ref="query"
            data-cy="query"
          />
          <v-btn
            type="submit"
            class="query-submit"
            color="primary"
            icon="mdi-magnify"
            data-cy="btn-query"
          />
        </form>
      </v-card-text>
    </v-card>

    <div class="actions mb-4" v-if="bulk.checked.length > 0">
      <a class="a" href="#" @click.prevent="deleteLists" data-cy="btn-delete-lists">
        <b-icon icon="trash-can-outline" size="is-small" /> {{ $t('globals.buttons.delete') }}
      </a>
      <span class="a">
        {{ $tc('globals.messages.numSelected', numSelectedLists, { num: numSelectedLists }) }}
        <span v-if="!bulk.all && lists.total > lists.perPage">
          &mdash;
          <a href="#" @click.prevent="onSelectAll" data-cy="select-all-lists">
            {{ $tc('globals.messages.selectAll', lists.total, { num: lists.total }) }}
          </a>
        </span>
      </span>
    </div>

    <div class="table-wrap">
      <v-data-table-server
        :headers="tableHeaders"
        :items="listRows"
        :items-length="lists.total || 0"
        :loading="loading.listsFull"
        :page="queryParams.page"
        :items-per-page="lists.perPage || 20"
        :sort-by="tableSortBy"
        :model-value="bulk.checked"
        class="admin-data-table lists-table"
        item-value="id"
        return-object
        show-select
        select-strategy="page"
        hide-default-footer
        @update:model-value="updateCheckedLists"
        @update:options="onTableOptionsChange"
      >
        <template #[`item.name`]="{ item }">
          <div>
            <button type="button" class="link-button" @click.stop.prevent="showEditForm(item)">{{ item.name }}</button>
            <div class="tag-list">
              <b-tag class="is-small" v-for="t in item.tags" :key="t">{{ t }}</b-tag>
            </div>
          </div>
        </template>

        <template #[`item.type`]="{ item }">
          <div class="tags">
            <b-tag :class="item.type" :data-cy="`type-${item.type}`">{{ $t(`lists.types.${item.type}`) }}</b-tag>
            <b-tag :class="item.optin" :data-cy="`optin-${item.optin}`">
              <b-icon :icon="item.optin === 'double' ? 'account-check-outline' : 'account-off-outline'" size="is-small" />
              {{ $t(`lists.optins.${item.optin}`) }}
            </b-tag>
            <a v-if="item.optin === 'double'" class="text-caption send-optin" href="#" @click.prevent="$utils.confirm(null, () => createOptinCampaign(item))" data-cy="btn-send-optin-campaign">
              <b-icon icon="rocket-launch-outline" size="is-small" />
              {{ $t('lists.sendOptinCampaign') }}
            </a>
          </div>
        </template>

        <template #[`item.subscriber_count`]="{ item }">
          <template v-if="$can('subscribers:get_all', 'subscribers:get')">
            <router-link :to="`/subscribers/lists/${item.id}`">
              {{ $utils.formatNumber(item.subscriberCount) }}
              <span class="text-caption view">{{ $t('globals.buttons.view') }}</span>
            </router-link>
          </template>
          <template v-else>
            {{ $utils.formatNumber(item.subscriberCount) }}
          </template>
        </template>

        <template #[`item.subscriber_statuses`]="{ item }">
          <div class="fields stats">
            <p v-for="(count, status) in filterStatuses(item)" :key="status">
              <label for="#">{{ $tc(`subscribers.status.${status}`, count) }}</label>
              <router-link :to="`/subscribers/lists/${item.id}?subscription_status=${status}`" :class="status">
                {{ $utils.formatNumber(count) }}
              </router-link>
            </p>
          </div>
        </template>

        <template #[`item.created_at`]="{ item }">
          {{ $utils.niceDate(item.createdAt) }}
        </template>

        <template #[`item.updated_at`]="{ item }">
          {{ $utils.niceDate(item.updatedAt) }}
        </template>

        <template #[`item.actions`]="{ item }">
          <div class="action-group">
            <router-link
              v-if="$can('campaigns:manage')"
              :to="`/campaigns/new?list_id=${item.id}`"
              class="action-button"
              data-cy="btn-campaign"
              :aria-label="$t('campaigns.new')"
            >
              <b-icon icon="rocket-launch-outline" size="is-small" />
            </router-link>
            <button
              v-if="$can('lists:manage') || $canList(item.id, 'list:manage')"
              type="button"
              class="action-button"
              @click.stop.prevent="showEditForm(item)"
              data-cy="btn-edit"
              :aria-label="$t('globals.buttons.edit')"
            >
              <b-icon icon="pencil-outline" size="is-small" />
            </button>
            <router-link
              v-if="$can('subscribers:import')"
              :to="{ name: 'import', query: { list_id: item.id } }"
              class="action-button"
              data-cy="btn-import"
              :aria-label="$t('globals.buttons.import')"
            >
              <b-icon icon="file-upload-outline" size="is-small" />
            </router-link>
            <a
              v-if="$can('lists:manage') || $canList(item.id, 'list:manage')"
              href="#"
              class="action-button action-button-danger"
              @click.prevent="deleteList(item)"
              data-cy="btn-delete"
              :aria-label="$t('globals.buttons.delete')"
            >
              <b-icon icon="trash-can-outline" size="is-small" />
            </a>
          </div>
        </template>

        <template #no-data>
          <empty-placeholder v-if="!loading.listsFull" />
        </template>
      </v-data-table-server>
    </div>

    <div class="table-pagination" v-if="lists.total > 0">
      <v-pagination
        :length="listPageCount"
        :model-value="queryParams.page"
        rounded="circle"
        total-visible="7"
        @update:model-value="onPageChange"
      />
    </div>

    <v-overlay
      :model-value="isFormVisible"
      class="admin-overlay align-center justify-center"
      scrim="rgba(15, 23, 42, 0.45)"
      @update:model-value="handleDialogModelUpdate"
    >
      <div class="admin-dialog-frame" style="max-width: 680px; width: calc(100vw - 32px);">
        <list-form
          v-if="isFormVisible"
          :data="listFormData"
          :is-editing="isEditing"
          @finished="formFinished"
          @close="closeForm"
        />
      </div>
    </v-overlay>

    <p v-if="settings['app.cache_slow_queries']" class="text-medium-emphasis">
      *{{ $t('globals.messages.slowQueriesCached') }}
      <a href="https://listmonk.app/docs/maintenance/performance/" target="_blank" rel="noopener noreferer"
        class="text-medium-emphasis">
        <b-icon icon="link-variant" /> {{ $t('globals.buttons.learnMore') }}
      </a>
    </p>
  </section>
</template>

<script>
import { mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import ListForm from './ListForm.vue';

export default {
  components: {
    ListForm,
    EmptyPlaceholder,
  },

  data() {
    return {
      // Current list item being edited.
      curItem: null,
      isEditing: false,
      isFormVisible: false,
      lists: [],
      queryParams: {
        page: 1,
        query: '',
        orderBy: 'id',
        order: 'asc',
        status: this.$route.query.status || 'active',
      },

      // Table bulk row selection states.
      bulk: {
        checked: [],
        all: false,
      },
    };
  },

  methods: {
    onPageChange(p) {
      this.queryParams.page = p;
      this.getLists();
    },

    onSearchSubmit() {
      this.queryParams.page = 1;
      this.getLists();
    },

    onSort(field, direction) {
      this.queryParams.orderBy = field;
      this.queryParams.order = direction;
      this.getLists();
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

      this.queryParams.page = nextPage;
      this.queryParams.orderBy = nextOrderBy;
      this.queryParams.order = nextOrder;
      this.getLists();
    },

    updateCheckedLists(rows) {
      this.bulk.checked = rows;
      this.onTableCheck();
    },

    // Show the edit list form.
    showEditForm(list) {
      this.curItem = list;
      this.isFormVisible = true;
      this.isEditing = true;
    },

    // Show the new list form.
    showNewForm() {
      this.curItem = {};
      this.isFormVisible = true;
      this.isEditing = false;
    },

    formFinished() {
      this.getLists();
    },

    onFormClose() {
      if (this.$route.params.id) {
        this.$router.push({ name: 'lists' });
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

    filterStatuses(list) {
      const out = { ...list.subscriberStatuses };
      if (list.optin === 'single') {
        delete out.unconfirmed;
        delete out.confirmed;
      }
      return out;
    },

    getLists() {
      this.$api.queryLists({
        page: this.queryParams.page,
        query: this.queryParams.query.replace(/[^\p{L}\p{N}\s]/gu, ' '),
        order_by: this.queryParams.orderBy,
        order: this.queryParams.order,
        status: this.queryParams.status,
      }).then((resp) => {
        this.lists = resp;
      });

      // Also fetch the minimal lists for the global store that appears
      // in dropdown menus on other pages like import and campaigns.
      this.$api.getLists({ minimal: true, per_page: 'all', status: 'active' });
    },

    deleteList(list) {
      this.$utils.confirm(
        this.$t('lists.confirmDelete'),
        () => {
          this.$api.deleteList(list.id).then(() => {
            this.getLists();

            this.$utils.toast(this.$t('globals.messages.deleted', { name: list.name }));
          });
        },
      );
    },

    // Mark all lists in the query as selected.
    onSelectAll() {
      this.bulk.all = true;
    },

    onTableCheck() {
      // Disable bulk.all selection if there are no rows checked in the table.
      if (this.bulk.checked.length !== this.lists.total) {
        this.bulk.all = false;
      }
    },

    isListChecked(id) {
      return this.bulk.checked.some((item) => item.id === id);
    },

    toggleListSelection(list, checked) {
      if (checked) {
        if (!this.isListChecked(list.id)) {
          this.bulk.checked = [...this.bulk.checked, list];
        }
      } else {
        this.bulk.checked = this.bulk.checked.filter((item) => item.id !== list.id);
      }
      this.onTableCheck();
    },

    toggleCurrentPageLists(checked) {
      if (checked) {
        const checkedMap = new Map(this.bulk.checked.map((item) => [item.id, item]));
        this.listRows.forEach((row) => {
          checkedMap.set(row.id, row);
        });
        this.bulk.checked = [...checkedMap.values()];
      } else {
        const currentIDs = new Set(this.listRows.map((row) => row.id));
        this.bulk.checked = this.bulk.checked.filter((item) => !currentIDs.has(item.id));
      }
      this.onTableCheck();
    },

    deleteLists() {
      const name = this.$tc('globals.terms.list', this.numSelectedCampaigns);

      const fn = () => {
        const params = {};
        if (!this.bulk.all && this.bulk.checked.length > 0) {
          // If 'all' is not selected, delete lists by IDs.
          params.id = this.bulk.checked.map((l) => l.id);
        } else {
          // 'All' is selected, delete by query.
          params.query = this.queryParams.query.replace(/[^\p{L}\p{N}\s]/gu, ' ');
          params.all = this.bulk.all;
        }

        this.$api.deleteLists(params)
          .then(() => {
            this.getLists();
            this.$utils.toast(this.$tc(
              'globals.messages.deletedCount',
              this.numSelectedLists,
              { num: this.numSelectedLists, name },
            ));
          });
      };

      this.$utils.confirm(this.$tc(
        'globals.messages.confirmDelete',
        this.numSelectedLists,
        { num: this.numSelectedLists, name: name.toLowerCase() },
      ), fn);
    },

    createOptinCampaign(list) {
      const data = {
        name: this.$t('lists.optinTo', { name: list.name }),
        subject: this.$t('lists.confirmSub', { name: list.name }),
        lists: [list.id],
        from_email: this.settings['app.from_email'],
        content_type: 'richtext',
        messenger: 'email',
        type: 'optin',
      };

      this.$api.createCampaign(data).then((d) => {
        this.$router.push({ name: 'campaign', hash: '#content', params: { id: d.id } });
      });
      return false;
    },
  },

  computed: {
    ...mapState(['loading', 'settings']),

    listRows() {
      return this.lists.results ?? [];
    },

    tableHeaders() {
      return [
        { title: this.$t('globals.fields.name'), key: 'name', sortable: true },
        { title: this.$t('globals.fields.type'), key: 'type', sortable: true },
        { title: this.$t('globals.terms.subscribers'), key: 'subscriber_count', sortable: true },
        { title: `${this.$t('globals.terms.subscribers')} ${this.$t('globals.terms.all').toLowerCase()}`, key: 'subscriber_statuses', sortable: false },
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

    listPageCount() {
      if (!this.lists.perPage || !this.lists.total) {
        return 1;
      }
      return Math.max(1, Math.ceil(this.lists.total / this.lists.perPage));
    },

    numSelectedLists() {
      return this.bulk.all ? this.lists.total : this.bulk.checked.length;
    },

    listFormData() {
      const item = this.curItem && typeof this.curItem === 'object' ? this.curItem : {};
      const normalized = { ...item };
      normalized.tags = Array.isArray(item.tags) ? item.tags : [];
      return normalized;
    },
  },

  created() {
    this.$events.$on('page.refresh', this.getLists);
  },

  destroyed() {
    this.$events.$off('page.refresh', this.getLists);
  },

  mounted() {
    if (this.$route.params.id) {
      this.$api.getList(parseInt(this.$route.params.id, 10)).then((data) => {
        this.showEditForm(data);
      });
    } else {
      this.getLists();
    }
  },
};
</script>

<style scoped>
.lists {
  --lists-border: #dce5f2;
  --lists-border-strong: #c7d5ea;
  --lists-surface-soft: #f6f9ff;
}

.page-header {
  margin-bottom: 12px;
}

.query-card {
  background: linear-gradient(180deg, #ffffff 0%, #f6f9ff 100%);
  border: 1px solid var(--lists-border);
  border-radius: 16px;
}

.query-card-body {
  padding: 16px;
}

.query-form {
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

.lists-table {
  background: #fff;
  border: 1px solid var(--lists-border);
  border-radius: 16px;
  overflow: hidden;
}

.lists-table :deep(thead th) {
  background: var(--lists-surface-soft);
  border-bottom: 1px solid var(--lists-border-strong) !important;
  color: #334155;
  font-weight: 600;
}

.lists-table :deep(tbody td) {
  padding-bottom: 14px !important;
  padding-top: 14px !important;
  vertical-align: top;
}

.lists-table :deep(tbody tr:hover) {
  background: #f8fbff;
}

.lists-table :deep(.v-data-table__tr--selected) {
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

@media (max-width: 960px) {
  .query-form {
    align-items: stretch;
    flex-direction: column;
  }

  .query-submit {
    width: 100%;
  }
}
</style>
