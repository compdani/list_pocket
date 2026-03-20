<template>
  <section class="bounces">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h4">
          {{ $t('globals.terms.bounces') }}
          <span v-if="bounces.total > 0">({{ bounces.total }})</span>
        </h1>
      </div>
    </header>

    <v-card v-if="bulk.checked.length > 0" class="mb-4 bulk-actions-card" elevation="0">
      <v-card-text class="bulk-actions-content">
        <v-btn
          variant="text"
          size="small"
          prepend-icon="mdi-trash-can-outline"
          @click="$utils.confirm(null, () => deleteBounces())"
          data-cy="btn-delete"
        >
          {{ $t('globals.buttons.delete') }}
        </v-btn>
        <v-btn
          variant="text"
          size="small"
          prepend-icon="mdi-account-off-outline"
          @click="$utils.confirm(null, () => blocklistSubscribers())"
          data-cy="btn-manage-blocklist"
        >
          {{ $t('import.blocklist') }}
        </v-btn>
        <span class="ml-2">
          {{ $t('globals.messages.numSelected', { num: numSelectedBounces }) }}
          <span v-if="!bulk.all && bounces.total > bounces.perPage">
            &mdash;
            <v-btn variant="text" size="small" @click="selectAllBounces">
              {{ $t('subscribers.selectAll', { num: bounces.total }) }}
            </v-btn>
          </span>
        </span>
      </v-card-text>
    </v-card>

    <v-data-table-server
      :headers="tableHeaders"
      :items="bounces.results || []"
      :items-length="bounces.total || 0"
      :loading="loading.bounces"
      :page="queryParams.page"
      :items-per-page="bounces.perPage || 20"
      :sort-by="tableSortBy"
      :model-value="bulk.checked"
      class="bounces-table"
      item-value="id"
      return-object
      show-select
      show-expand
      hide-default-footer
      @update:model-value="updateChecked"
      @update:options="onTableOptionsChange"
    >
      <template #[`item.email`]="{ item }">
        <router-link :to="{ name: 'subscriber', params: { id: item.subscriberId } }"
          :class="{ 'text-error': item.subscriberStatus === 'blocklisted' }">
          {{ item.email }}
        </router-link>
        <v-chip
          v-if="item.subscriberStatus !== 'enabled'"
          :class="item.subscriberStatus"
          size="x-small"
          class="ml-1"
          data-cy="blocklisted"
        >
          {{ $t(`subscribers.status.${item.subscriberStatus}`) }}
        </v-chip>
      </template>

      <template #[`item.campaign`]="{ item }">
        <router-link v-if="item.campaign" :to="{ name: 'bounces', query: { campaign_id: item.campaign.id } }">
          {{ item.campaign.name }}
        </router-link>
        <span v-else>-</span>
      </template>

      <template #[`item.source`]="{ item }">
        <router-link :to="{ name: 'bounces', query: { source: item.source } }">
          {{ item.source }}
        </router-link>
      </template>

      <template #[`item.type`]="{ item }">
        <router-link :to="{ name: 'bounces', query: { type: item.type } }">
          {{ $t(`bounces.${item.type}`) }}
        </router-link>
      </template>

      <template #[`item.created_at`]="{ item }">
        {{ $utils.niceDate(item.createdAt, true) }}
      </template>

      <template #[`item.actions`]="{ item }">
        <div class="actions">
          <v-tooltip :text="$t('globals.buttons.delete')" location="top">
            <template #activator="{ props }">
              <v-btn
                v-bind="props"
                icon="mdi-trash-can-outline"
                size="x-small"
                variant="text"
                @click="$utils.confirm(null, () => deleteBounce(item))"
                data-cy="btn-delete"
              />
            </template>
          </v-tooltip>
        </div>
      </template>

      <template #expanded-row="{ item }">
        <tr>
          <td :colspan="tableHeaders.length + 2">
            <pre class="text-caption pa-2">{{ item.meta }}</pre>
          </td>
        </tr>
      </template>

      <template #no-data>
        <empty-placeholder v-if="!loading.bounces" />
      </template>
    </v-data-table-server>

    <div class="table-pagination" v-if="bounces.total > 0">
      <v-pagination
        :length="bouncePageCount"
        :model-value="queryParams.page"
        rounded="circle"
        total-visible="7"
        @update:model-value="onPageChange"
      />
    </div>
  </section>
</template>

<script>
import { mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';

export default {
  components: {
    EmptyPlaceholder,
  },

  data() {
    return {
      bounces: {},

      // Table bulk row selection states.
      bulk: {
        checked: [],
        all: false,
      },

      // Query params to filter the getSubscribers() API call.
      queryParams: {
        page: 1,
        orderBy: 'created_at',
        order: 'desc',
        campaign_id: '',
        source: '',
      },
    };
  },

  methods: {
    onSort(field, direction) {
      this.queryParams.orderBy = field;
      this.queryParams.order = direction;
      this.getBounces();
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
      this.getBounces();
    },

    updateChecked(rows) {
      this.bulk.checked = rows;
      if (this.bulk.checked.length !== this.bounces.total) {
        this.bulk.all = false;
      }
    },

    onPageChange(p) {
      this.queryParams.page = p;
      this.getBounces();
    },
    // Mark all bounces in the query as selected.
    selectAllBounces() {
      this.bulk.all = true;
    },
    onTableCheck() {
      // Disable bulk.all selection if there are no rows checked in the table.
      if (this.bulk.checked.length !== this.bounces.total) {
        this.bulk.all = false;
      }
    },

    getBounces() {
      this.bulk.all = false;

      this.$api.getBounces({
        page: this.queryParams.page,
        order_by: this.queryParams.orderBy,
        order: this.queryParams.order,
        campaign_id: this.queryParams.campaign_id,
        source: this.queryParams.source,
      }).then((data) => {
        this.bounces = data;
      });
    },

    deleteBounce(b) {
      this.$api.deleteBounce(b.id).then(() => {
        this.getBounces();
        this.$utils.toast(this.$t('globals.messages.deleted', { name: b.email }));
      });
    },

    deleteBounces() {
      const params = {};
      if (!this.bulk.all && this.bulk.checked.length > 0) {
        params.id = this.bulk.checked.map((s) => s.id);
      } else if (this.bulk.all) {
        params.all = true;
      }

      this.$api.deleteBounces(params).then(() => {
        this.getBounces();
        this.$utils.toast(this.$t(
          'globals.messages.deletedCount',
          { name: this.$tc('globals.terms.bounces'), num: this.numSelectedBounces },
        ));
      });
    },

    blocklistSubscribers() {
      const cb = () => {
        this.getBounces();
        this.$utils.toast(this.$t('globals.messages.done'));
      };

      if (!this.bulk.all && this.bulk.checked.length > 0) {
        const subIds = this.bulk.checked.map((s) => s.subscriberId);
        this.$api.blocklistSubscribers({ subscriber_record_ids: subIds }).then(cb);
        return;
      }

      this.$api.blocklistBouncedSubscribers({ all: true }).then(cb);
    },
  },

  computed: {
    ...mapState(['loading']),

    tableHeaders() {
      return [
        { title: this.$t('subscribers.email'), key: 'email', sortable: true },
        { title: this.$tc('globals.terms.campaign'), key: 'campaign', sortable: false },
        { title: this.$t('bounces.source'), key: 'source', sortable: true },
        { title: this.$t('globals.fields.type'), key: 'type', sortable: true },
        { title: this.$t('globals.fields.createdAt'), key: 'created_at', sortable: true },
        { title: '', key: 'actions', sortable: false, align: 'end' },
      ];
    },

    tableSortBy() {
      return [{ key: this.queryParams.orderBy, order: this.queryParams.order }];
    },

    bouncePageCount() {
      if (!this.bounces.perPage || !this.bounces.total) {
        return 1;
      }
      return Math.max(1, Math.ceil(this.bounces.total / this.bounces.perPage));
    },

    numSelectedBounces() {
      return this.bulk.all ? this.bounces.total : this.bulk.checked.length;
    },
  },

  created() {
    this.$events.$on('page.refresh', this.getBounces);
  },

  destroyed() {
    this.$events.$off('page.refresh', this.getBounces);
  },

  mounted() {
    if (this.$route.query.campaign_id) {
      this.queryParams.campaign_id = this.$route.query.campaign_id;
    }

    if (this.$route.query.source) {
      this.queryParams.source = this.$route.query.source;
    }

    this.getBounces();
  },
};
</script>

<style scoped>
.bounces {
  --bounces-border: #dce5f2;
  --bounces-border-strong: #c7d5ea;
  --bounces-surface-soft: #f6f9ff;
}

.page-header {
  align-items: center;
  display: flex;
  gap: 16px;
  justify-content: space-between;
  margin-bottom: 16px;
}

.bulk-actions-card {
  background: #f0f7ff;
  border: 1px solid #d8e9ff;
  border-radius: 14px;
}

.bulk-actions-content {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.bounces-table {
  background: #fff;
  border: 1px solid var(--bounces-border);
  border-radius: 16px;
  overflow: hidden;
}

.bounces-table :deep(thead th) {
  background: var(--bounces-surface-soft);
  border-bottom: 1px solid var(--bounces-border-strong) !important;
  color: #334155;
  font-weight: 600;
}

.bounces-table :deep(tbody td) {
  padding-bottom: 14px !important;
  padding-top: 14px !important;
  vertical-align: top;
}

.bounces-table :deep(tbody tr:hover) {
  background: #f8fbff;
}

.bounces-table :deep(.v-data-table__tr--selected) {
  background: #edf5ff;
}

.table-pagination {
  align-items: center;
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
  margin-top: 14px;
}

.actions {
  display: flex;
  gap: 4px;
  justify-content: flex-end;
}
</style>
