<template>
  <section class="lists">
    <header class="columns page-header">
      <div class="column is-10">
        <h1 class="title is-4 mb-2">
          {{ $t('globals.terms.lists') }}
          <span v-if="queryParams.status === 'archived'" class="has-text-grey-light">/ {{ queryParams.status }} </span>
          <span v-if="!isNaN(lists.total)">({{ lists.total }})</span>
        </h1>

        <div class="is-size-7">
          <router-link v-if="queryParams.status !== 'archived'" :to="{ name: 'lists', query: { status: 'archived' } }">
            {{ $t('globals.buttons.view') }} {{ $t('lists.archived').toLowerCase() }} &rarr;
          </router-link>
          <router-link v-else :to="{ name: 'lists' }">
            {{ $t('globals.buttons.view') }} {{ $t('menu.allLists').toLowerCase() }} &rarr;
          </router-link>
        </div>
      </div>
      <div class="column has-text-right">
        <div v-if="$can('lists:manage_all')">
          <button type="button" class="btn-new admin-open-button" @click.stop.prevent="showNewForm" data-cy="btn-new">
            {{ $t('globals.buttons.new') }}
          </button>
        </div>
      </div>
    </header>

    <div class="columns">
      <div class="column is-6">
        <form @submit.prevent="getLists">
          <b-field>
            <b-input v-model="queryParams.query" name="query" expanded icon="magnify" ref="query" data-cy="query" />
            <p class="controls">
              <b-button native-type="submit" type="is-primary" icon-left="magnify" data-cy="btn-query" />
            </p>
          </b-field>
        </form>
      </div>
    </div>

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

    <div class="table-pagination" v-if="lists.total > 0">
      <span class="page-indicator">{{ queryParams.page }}</span>
      <button class="button" type="button" :disabled="queryParams.page <= 1" @click="onPageChange(queryParams.page - 1)">
        <b-icon icon="chevron-left" size="is-small" />
      </button>
      <button class="button" type="button" :disabled="queryParams.page >= listPageCount" @click="onPageChange(queryParams.page + 1)">
        <b-icon icon="chevron-right" size="is-small" />
      </button>
    </div>

    <div class="table-wrap">
      <table class="table is-fullwidth is-hoverable admin-table">
        <thead>
          <tr>
            <th class="checkbox-col">
              <input type="checkbox" :checked="isAllCurrentPageListsChecked" aria-label="Select current page lists" @change="toggleCurrentPageLists($event.target.checked)" />
            </th>
            <th><button type="button" class="sort-button" @click="onSort('name', nextSortDirection('name'))">{{ $t('globals.fields.name') }}</button></th>
            <th><button type="button" class="sort-button" @click="onSort('type', nextSortDirection('type'))">{{ $t('globals.fields.type') }}</button></th>
            <th><button type="button" class="sort-button" @click="onSort('subscriber_count', nextSortDirection('subscriber_count'))">{{ $t('globals.terms.subscribers') }}</button></th>
            <th>{{ $t('globals.terms.subscribers') }} {{ $t('globals.terms.all').toLowerCase() }}</th>
            <th><button type="button" class="sort-button" @click="onSort('created_at', nextSortDirection('created_at'))">{{ $t('globals.fields.createdAt') }}</button></th>
            <th><button type="button" class="sort-button" @click="onSort('updated_at', nextSortDirection('updated_at'))">{{ $t('globals.fields.updatedAt') }}</button></th>
            <th class="actions-col" />
          </tr>
        </thead>
        <tbody v-if="listRows.length > 0">
          <tr v-for="row in listRows" :key="row.id">
            <td class="checkbox-col">
              <input type="checkbox" :checked="isListChecked(row.id)" :aria-label="`Select list ${row.name}`" @change="toggleListSelection(row, $event.target.checked)" />
            </td>
            <td>
              <button type="button" class="link-button" @click.stop.prevent="showEditForm(row)">{{ row.name }}</button>
              <div class="tag-list">
                <b-tag class="is-small" v-for="t in row.tags" :key="t">{{ t }}</b-tag>
              </div>
            </td>
            <td>
              <div class="tags">
                <b-tag :class="row.type" :data-cy="`type-${row.type}`">{{ $t(`lists.types.${row.type}`) }}</b-tag>
                <b-tag :class="row.optin" :data-cy="`optin-${row.optin}`">
                  <b-icon :icon="row.optin === 'double' ? 'account-check-outline' : 'account-off-outline'" size="is-small" />
                  {{ $t(`lists.optins.${row.optin}`) }}
                </b-tag>
                <a v-if="row.optin === 'double'" class="is-size-7 send-optin" href="#" @click.prevent="$utils.confirm(null, () => createOptinCampaign(row))" data-cy="btn-send-optin-campaign">
                  <b-icon icon="rocket-launch-outline" size="is-small" />
                  {{ $t('lists.sendOptinCampaign') }}
                </a>
              </div>
            </td>
            <td>
              <template v-if="$can('subscribers:get_all', 'subscribers:get')">
                <router-link :to="`/subscribers/lists/${row.id}`">
                  {{ $utils.formatNumber(row.subscriberCount) }}
                  <span class="is-size-7 view">{{ $t('globals.buttons.view') }}</span>
                </router-link>
              </template>
              <template v-else>
                {{ $utils.formatNumber(row.subscriberCount) }}
              </template>
            </td>
            <td>
              <div class="fields stats">
                <p v-for="(count, status) in filterStatuses(row)" :key="status">
                  <label for="#">{{ $tc(`subscribers.status.${status}`, count) }}</label>
                  <router-link :to="`/subscribers/lists/${row.id}?subscription_status=${status}`" :class="status">
                    {{ $utils.formatNumber(count) }}
                  </router-link>
                </p>
              </div>
            </td>
            <td>{{ $utils.niceDate(row.createdAt) }}</td>
            <td>{{ $utils.niceDate(row.updatedAt) }}</td>
            <td class="actions">
              <div class="action-group">
              <router-link
                v-if="$can('campaigns:manage')"
                :to="`/campaigns/new?list_id=${row.id}`"
                class="action-button"
                data-cy="btn-campaign"
                :aria-label="$t('campaigns.new')"
              >
                <b-icon icon="rocket-launch-outline" size="is-small" />
              </router-link>
              <button
                v-if="$can('lists:manage') || $canList(row.id, 'list:manage')"
                type="button"
                class="action-button"
                @click.stop.prevent="showEditForm(row)"
                data-cy="btn-edit"
                :aria-label="$t('globals.buttons.edit')"
              >
                <b-icon icon="pencil-outline" size="is-small" />
              </button>
              <router-link
                v-if="$can('subscribers:import')"
                :to="{ name: 'import', query: { list_id: row.id } }"
                class="action-button"
                data-cy="btn-import"
                :aria-label="$t('globals.buttons.import')"
              >
                <b-icon icon="file-upload-outline" size="is-small" />
              </router-link>
              <a
                v-if="$can('lists:manage') || $canList(row.id, 'list:manage')"
                href="#"
                class="action-button action-button-danger"
                @click.prevent="deleteList(row)"
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

      <empty-placeholder v-if="listRows.length === 0 && !loading.listsFull" />
    </div>

    <div class="table-pagination" v-if="lists.total > 0">
      <span class="page-indicator">{{ queryParams.page }}</span>
      <button class="button" type="button" :disabled="queryParams.page <= 1" @click="onPageChange(queryParams.page - 1)">
        <b-icon icon="chevron-left" size="is-small" />
      </button>
      <button class="button" type="button" :disabled="queryParams.page >= listPageCount" @click="onPageChange(queryParams.page + 1)">
        <b-icon icon="chevron-right" size="is-small" />
      </button>
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

    <p v-if="settings['app.cache_slow_queries']" class="has-text-grey">
      *{{ $t('globals.messages.slowQueriesCached') }}
      <a href="https://listmonk.app/docs/maintenance/performance/" target="_blank" rel="noopener noreferer"
        class="has-text-grey">
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

    onSort(field, direction) {
      this.queryParams.orderBy = field;
      this.queryParams.order = direction;
      this.getLists();
    },

    nextSortDirection(field) {
      return this.queryParams.orderBy === field && this.queryParams.order === 'asc' ? 'desc' : 'asc';
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

    listPageCount() {
      if (!this.lists.perPage || !this.lists.total) {
        return 1;
      }
      return Math.max(1, Math.ceil(this.lists.total / this.lists.perPage));
    },

    isAllCurrentPageListsChecked() {
      return this.listRows.length > 0
        && this.listRows.every((row) => this.isListChecked(row.id));
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
</style>
