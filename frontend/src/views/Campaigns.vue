<template>
  <section class="campaigns">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h4">
          {{ $t('globals.terms.campaigns') }}
          <span v-if="!isNaN(campaigns.total)">({{ campaigns.total }})</span>
        </h1>
      </div>
      <div class="header-actions">
        <v-btn
          v-if="$can('campaigns:manage')"
          :to="{ name: 'campaign', params: { id: 'new' } }"
          color="primary"
          prepend-icon="mdi-plus"
          data-cy="btn-new"
        >
          {{ $t('globals.buttons.new') }}
        </v-btn>
      </div>
    </header>

    <v-card class="mb-4 query-card" elevation="0">
      <v-card-text class="query-card-body">
        <form class="query-form" @submit.prevent="onSearchSubmit">
          <v-text-field
            v-model="queryParams.query"
            class="query-input"
            name="query"
            :placeholder="$t('campaigns.queryPlaceholder')"
            prepend-inner-icon="mdi-magnify"
            variant="outlined"
            density="comfortable"
            hide-details
            ref="query"
            data-cy="search"
          />
          <v-btn
            type="submit"
            class="query-submit"
            color="primary"
            icon="mdi-magnify"
            :aria-label="$t('campaigns.queryPlaceholder')"
            data-cy="btn-search"
          />
        </form>
      </v-card-text>
    </v-card>

    <v-card v-if="bulk.checked.length > 0" class="mb-4 bulk-actions-card" elevation="0">
      <v-card-text class="actions bulk-actions-content">
        <v-btn
          variant="text"
          size="small"
          prepend-icon="mdi-trash-can-outline"
          @click="deleteCampaigns"
          data-cy="btn-delete-campaigns"
        >
          {{ $t('globals.buttons.delete') }}
        </v-btn>
        <span class="ml-4">
          {{ $tc('globals.messages.numSelected', numSelectedCampaigns, { num: numSelectedCampaigns }) }}
          <span v-if="!bulk.all && campaigns.total > campaigns.perPage">
            &mdash;
            <v-btn
              variant="text"
              size="small"
              @click="onSelectAll"
              data-cy="select-all-campaigns"
            >
              {{ $tc('globals.messages.selectAll', campaigns.total, { num: campaigns.total }) }}
            </v-btn>
          </span>
        </span>
      </v-card-text>
    </v-card>

    <v-data-table-server
      :headers="tableHeaders"
      :items="campaigns.results || []"
      :items-length="campaigns.total || 0"
      :loading="loading.campaigns"
      :page="queryParams.page"
      :items-per-page="campaigns.perPage || 20"
      :sort-by="tableSortBy"
      :model-value="bulk.checked"
      class="campaigns-table"
      item-value="id"
      return-object
      show-select
      hide-default-footer
      @update:model-value="updateCheckedCampaigns"
      @update:options="onTableOptionsChange"
    >
      <template #[`item.status`]="{ item }">
        <div>
          <div>
            <router-link :to="{ name: 'campaign', params: { id: item.id } }">
              <v-chip :class="item.status" size="small">
                {{ $t(`campaigns.status.${item.status}`) }}
              </v-chip>
              <v-progress-circular
                v-if="isRunning(item.id)"
                indeterminate
                size="20"
                width="2"
                class="ml-2"
              />
            </router-link>
          </div>
          <div v-if="isSheduled(item)" class="text-caption text-medium-emphasis mt-2">
            <v-icon size="small">mdi-alarm</v-icon>
            <span v-if="!isDone(item) && !isRunning(item)">
              {{ $utils.duration(new Date(), item.sendAt, true) }}
              <br />
            </span>
            {{ $utils.niceDate(item.sendAt, true) }}
          </div>
        </div>
      </template>

      <template #[`item.name`]="{ item }">
        <div>
          <div>
            <v-chip v-if="item.type === 'optin'" size="small" class="mr-2">
              {{ $t('lists.optin') }}
            </v-chip>
            <router-link :to="{ name: 'campaign', params: { id: item.id } }">
              {{ item.name }}
              <copy-text :text="item.name" hide-text />
            </router-link>
          </div>
          <div class="text-caption text-medium-emphasis mt-1">
            <copy-text :text="item.subject" />
          </div>
          <div class="mt-1">
            <v-chip
              v-for="t in item.tags"
              :key="t"
              size="small"
              class="mr-1"
            >
              {{ t }}
            </v-chip>
          </div>
        </div>
      </template>

      <template #[`item.lists`]="{ item }">
        <ul>
          <li v-for="l in item.lists" :key="l.id">
            <router-link :to="{ name: 'subscribers_list', params: { listID: l.id } }">
              {{ l.name }}
            </router-link>
          </li>
        </ul>
      </template>

      <template #[`item.created_at`]="{ item }">
        <div class="fields timestamps">
          <div>
            <div class="text-caption font-weight-bold">{{ $t('globals.fields.createdAt') }}</div>
            <div class="text-caption">{{ $utils.niceDate(item.createdAt, true) }}</div>
          </div>
          <div v-if="getCampaignStats(item).startedAt">
            <div class="text-caption font-weight-bold">{{ $t('campaigns.startedAt') }}</div>
            <div class="text-caption">{{ $utils.niceDate(getCampaignStats(item).startedAt, true) }}</div>
          </div>
          <div v-if="isDone(item)">
            <div class="text-caption font-weight-bold">{{ $t('campaigns.ended') }}</div>
            <div class="text-caption">{{ $utils.niceDate(getCampaignStats(item).updatedAt, true) }}</div>
          </div>
          <div v-if="getCampaignStats(item).startedAt && getCampaignStats(item).updatedAt" class="text-capitalize">
            <v-icon size="small">mdi-alarm</v-icon>
            <span class="text-caption">{{ $utils.duration(getCampaignStats(item).startedAt, getCampaignStats(item).updatedAt) }}</span>
          </div>
        </div>
      </template>

      <template #[`item.stats`]="{ item }">
        <div class="fields stats">
          <div>
            <div class="text-caption font-weight-bold">{{ $t('campaigns.views') }}</div>
            <div class="text-caption">{{ $utils.formatNumber(item.views) }}</div>
          </div>
          <div>
            <div class="text-caption font-weight-bold">{{ $t('campaigns.clicks') }}</div>
            <div class="text-caption">{{ $utils.formatNumber(item.clicks) }}</div>
          </div>
          <div>
            <div class="text-caption font-weight-bold">{{ $t('campaigns.sent') }}</div>
            <div class="text-caption">
              {{ $utils.formatNumber(getCampaignStats(item).sent) }} /
              {{ $utils.formatNumber(getCampaignStats(item).toSend) }}
            </div>
          </div>
          <div>
            <div class="text-caption font-weight-bold">{{ $t('globals.terms.bounces') }}</div>
            <div class="text-caption">
              <router-link :to="{ name: 'bounces', query: { campaign_id: item.id } }">
                {{ $utils.formatNumber(item.bounces) }}
              </router-link>
            </div>
          </div>
          <div v-if="getCampaignStats(item).rate">
            <div class="text-caption font-weight-bold">
              <v-icon size="small">mdi-speedometer</v-icon>
            </div>
            <div class="text-caption send-rate">
              <v-tooltip
                :text="`${getCampaignStats(item).netRate} / ${$t('campaigns.rateMinuteShort')} @ ${$utils.duration(getCampaignStats(item).startedAt, getCampaignStats(item).updatedAt)}`"
              >
                <template #activator="{ props }">
                  <span v-bind="props">
                    {{ getCampaignStats(item).rate.toFixed(0) }} / {{ $t('campaigns.rateMinuteShort') }}
                  </span>
                </template>
              </v-tooltip>
            </div>
          </div>
          <div v-if="isRunning(item.id)">
            <div class="text-caption font-weight-bold">
              {{ $t('campaigns.progress') }}
              <v-progress-circular indeterminate size="16" width="2" class="ml-1" />
            </div>
            <v-progress-linear
              :model-value="getCampaignStats(item).sent / getCampaignStats(item).toSend * 100"
              class="mt-1"
            />
          </div>
        </div>
      </template>

      <template #[`item.actions`]="{ item }">
        <div class="actions">
          <template v-if="$can('campaigns:manage')">
            <v-tooltip v-if="canStart(item)" text="$t('campaigns.start')" location="top">
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  icon="mdi-rocket-launch-outline"
                  size="x-small"
                  variant="text"
                  @click="$utils.confirm(null, () => changeCampaignStatus(item, 'running'))"
                  data-cy="btn-start"
                />
              </template>
            </v-tooltip>

            <v-tooltip v-if="canPause(item)" text="$t('campaigns.pause')" location="top">
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  icon="mdi-pause-circle-outline"
                  size="x-small"
                  variant="text"
                  @click="$utils.confirm(null, () => changeCampaignStatus(item, 'paused'))"
                  data-cy="btn-pause"
                />
              </template>
            </v-tooltip>

            <v-tooltip v-if="canResume(item)" text="$t('campaigns.send')" location="top">
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  icon="mdi-rocket-launch-outline"
                  size="x-small"
                  variant="text"
                  @click="$utils.confirm(null, () => changeCampaignStatus(item, 'running'))"
                  data-cy="btn-resume"
                />
              </template>
            </v-tooltip>

            <v-tooltip v-if="canSchedule(item)" text="$t('campaigns.schedule')" location="top">
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  icon="mdi-clock-start"
                  size="x-small"
                  variant="text"
                  @click="$utils.confirm($t('campaigns.confirmSchedule'), () => changeCampaignStatus(item, 'scheduled'))"
                  data-cy="btn-schedule"
                />
              </template>
            </v-tooltip>

            <v-btn
              v-if="!canCancel(item) && !canSchedule(item) && !canStart(item)"
              icon="mdi-rocket-launch-outline"
              size="x-small"
              variant="text"
              disabled
            />

            <v-tooltip v-if="canCancel(item)" text="$t('globals.buttons.cancel')" location="top">
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  icon="mdi-cancel"
                  size="x-small"
                  variant="text"
                  @click="$utils.confirm(null, () => changeCampaignStatus(item, 'cancelled'))"
                  data-cy="btn-cancel"
                />
              </template>
            </v-tooltip>
            <v-btn v-else icon="mdi-cancel" size="x-small" variant="text" disabled />
          </template>

          <v-tooltip text="$t('campaigns.preview')" location="top">
            <template #activator="{ props }">
              <v-btn
                v-bind="props"
                icon="mdi-file-find-outline"
                size="x-small"
                variant="text"
                @click="previewCampaign(item)"
                data-cy="btn-preview"
              />
            </template>
          </v-tooltip>

          <v-tooltip v-if="$can('campaigns:manage')" text="$t('globals.buttons.clone')" location="top">
            <template #activator="{ props }">
              <v-btn
                v-bind="props"
                icon="mdi-file-multiple-outline"
                size="x-small"
                variant="text"
                @click="$utils.prompt($t('globals.buttons.clone'), { placeholder: $t('globals.fields.name'), value: $t('campaigns.copyOf', { name: item.name }) }, (name) => cloneCampaign(name, item))"
                data-cy="btn-clone"
              />
            </template>
          </v-tooltip>

          <router-link v-if="$can('campaigns:get_analytics')" :to="{ name: 'campaignAnalytics', query: { id: item.id } }">
            <v-tooltip text="$t('globals.terms.analytics')" location="top">
              <template #activator="{ props }">
                <v-btn v-bind="props" icon="mdi-chart-bar" size="x-small" variant="text" />
              </template>
            </v-tooltip>
          </router-link>

          <v-tooltip v-if="$can('campaigns:manage')" text="$t('globals.buttons.delete')" location="top">
            <template #activator="{ props }">
              <v-btn
                v-bind="props"
                icon="mdi-trash-can-outline"
                size="x-small"
                variant="text"
                @click="$utils.confirm($t('campaigns.confirmDelete', { name: item.name }), () => deleteCampaign(item))"
                data-cy="btn-delete"
              />
            </template>
          </v-tooltip>
        </div>
      </template>

      <template #no-data>
        <empty-placeholder v-if="!loading.campaigns" />
      </template>
    </v-data-table-server>

    <div class="table-pagination" v-if="campaigns.total > 0">
      <v-pagination
        :length="campaignPageCount"
        :model-value="queryParams.page"
        rounded="circle"
        total-visible="7"
        @update:model-value="onPageChange"
      />
    </div>

    <campaign-preview
      v-if="previewItem"
      type="campaign"
      :id="previewItem.id"
      :title="previewItem.name"
      @close="closePreview"
    />
  </section>
</template>

<script>
import dayjs from 'dayjs';
import { mapState } from 'vuex';
import CampaignPreview from '../components/CampaignPreview.vue';
import CopyText from '../components/CopyText.vue';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';

export default {
  components: {
    CampaignPreview,
    EmptyPlaceholder,
    CopyText,
  },

  data() {
    return {
      previewItem: null,
      queryParams: {
        page: 1,
        query: '',
        orderBy: 'created_at',
        order: 'desc',
      },
      pollID: null,
      campaignStatsData: {},

      // Table bulk row selection states.
      bulk: {
        checked: [],
        all: false,
      },
    };
  },

  methods: {
    // Campaign statuses.
    canStart(c) {
      return c.status === 'draft' && !c.sendAt;
    },
    canSchedule(c) {
      return c.status === 'draft' && c.sendAt;
    },
    canPause(c) {
      return c.status === 'running';
    },
    canCancel(c) {
      return c.status === 'running' || c.status === 'paused';
    },
    canResume(c) {
      return c.status === 'paused';
    },
    isSheduled(c) {
      return c.status === 'scheduled' || c.sendAt !== null;
    },
    isDone(c) {
      return c.status === 'finished' || c.status === 'cancelled';
    },

    isRunning(id) {
      if (id in this.campaignStatsData) {
        return true;
      }
      return false;
    },

    highlightedRow(data) {
      if (data.status === 'running') {
        return ['running'];
      }
      return '';
    },

    onPageChange(p) {
      this.queryParams.page = p;
      this.getCampaigns();
    },

    onSearchSubmit() {
      this.queryParams.page = 1;
      this.getCampaigns();
    },

    onSort(field, direction) {
      this.queryParams.orderBy = field;
      this.queryParams.order = direction;
      this.getCampaigns();
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
      this.getCampaigns();
    },

    updateCheckedCampaigns(rows) {
      this.bulk.checked = rows;
      this.onTableCheck();
    },

    // Campaign actions.
    previewCampaign(c) {
      this.previewItem = c;
    },

    closePreview() {
      this.previewItem = null;
    },

    getCampaigns() {
      this.$api.getCampaigns({
        page: this.queryParams.page,
        query: this.queryParams.query.replace(/[^\p{L}\p{N}\s]/gu, ' '),
        order_by: this.queryParams.orderBy,
        order: this.queryParams.order,
        no_body: true,
      });
    },

    // Stats returns the campaign object with stats (sent, toSend etc.)
    // if there's live stats available for running campaigns. Otherwise,
    // it returns the incoming campaign object that has the static stats
    // values.
    getCampaignStats(c) {
      if (c.id in this.campaignStatsData) {
        return this.campaignStatsData[c.id];
      }
      return c;
    },

    pollStats() {
      // Clear any running status polls.
      clearInterval(this.pollID);

      // Poll for the status as long as the import is running.
      this.pollID = setInterval(() => {
        this.$api.getCampaignStats().then((data) => {
          // Stop polling. No running campaigns.
          if (data.length === 0) {
            clearInterval(this.pollID);

            // There were running campaigns and stats earlier. Clear them
            // and refetch the campaigns list with up-to-date fields.
            if (Object.keys(this.campaignStatsData).length > 0) {
              this.getCampaigns();
              this.campaignStatsData = {};
            }
          } else {
            // Turn the list of campaigns [{id: 1, ...}, {id: 2, ...}] into
            // a map indexed by the id: {1: {}, 2: {}}.
            this.campaignStatsData = data.reduce((obj, cur) => ({ ...obj, [cur.id]: cur }), {});
          }
        }, () => {
          clearInterval(this.pollID);
        });
      }, 1000);
    },

    changeCampaignStatus(c, status) {
      this.$api.changeCampaignStatus(c.id, status).then(() => {
        this.$utils.toast(this.$t('campaigns.statusChanged', { name: c.name, status }));
        this.getCampaigns();
        this.pollStats();
      });
    },

    async cloneCampaign(name, c) {
      // Fetch the template body from the server.
      let body = '';
      let bodySource = null;
      await this.$api.getCampaign(c.id).then((data) => {
        body = data.body;
        bodySource = data.bodySource;
      });

      const now = this.$utils.getDate();
      const sendLater = !!c.sendAt;
      let sendAt = null;
      if (sendLater) {
        sendAt = dayjs(c.sendAt).isAfter(now) ? c.sendAt : now.add(7, 'day');
      }

      const data = {
        name,
        subject: c.subject,
        lists: c.lists.map((l) => l.id),
        type: c.type,
        from_email: c.fromEmail,
        content_type: c.contentType,
        messenger: c.messenger,
        tags: c.tags,
        template_id: c.templateId,
        body,
        body_source: bodySource,
        altbody: c.altbody,
        headers: c.headers,
        send_later: sendLater,
        send_at: sendAt,
        archive: c.archive,
        archive_template_id: c.archiveTemplateId,
        archive_meta: c.archiveMeta,
        media: c.media.map((m) => m.id),
      };

      if (c.archive) {
        data.archive_slug = `${name.toLowerCase().replace(/[^a-z0-9]/g, '-')}-${Date.now().toString().slice(-4)}`;
      }

      this.$api.createCampaign(data).then((d) => {
        this.$router.push({ name: 'campaign', params: { id: d.id } });
      });
    },

    deleteCampaign(c) {
      this.$api.deleteCampaign(c.id).then(() => {
        this.getCampaigns();
        this.$utils.toast(this.$t('globals.messages.deleted', { name: c.name }));
      });
    },

    // Mark all campaigns in the query as selected.
    onSelectAll() {
      this.bulk.all = true;
    },

    onTableCheck() {
      // Disable bulk.all selection if there are no rows checked in the table.
      if (this.bulk.checked.length !== this.campaigns.total) {
        this.bulk.all = false;
      }
    },

    deleteCampaigns() {
      const name = this.$tc('globals.terms.campaign', this.numSelectedCampaigns);

      const fn = () => {
        const params = {};
        if (!this.bulk.all && this.bulk.checked.length > 0) {
          // If 'all' is not selected, delete campaigns by IDs.
          params.id = this.bulk.checked.map((c) => c.id);
        } else {
          // 'All' is selected, delete by query.
          params.query = this.queryParams.query.replace(/[^\p{L}\p{N}\s]/gu, ' ');
          params.all = this.bulk.all;
        }

        this.$api.deleteCampaigns(params)
          .then(() => {
            this.getCampaigns();
            this.$utils.toast(this.$tc(
              'globals.messages.deletedCount',
              this.numSelectedCampaigns,
              { num: this.numSelectedCampaigns, name },
            ));
          });
      };

      this.$utils.confirm(this.$tc(
        'globals.messages.confirmDelete',
        this.numSelectedCampaigns,
        { num: this.numSelectedCampaigns, name: name.toLowerCase() },
      ), fn);
    },
  },

  computed: {
    ...mapState(['campaigns', 'loading']),

    tableHeaders() {
      return [
        { title: this.$t('globals.fields.status'), key: 'status', sortable: true },
        { title: this.$t('globals.fields.name'), key: 'name', sortable: true },
        { title: this.$t('globals.terms.lists'), key: 'lists', sortable: false },
        { title: this.$t('campaigns.timestamps'), key: 'created_at', sortable: true },
        { title: this.$t('campaigns.stats'), key: 'stats', sortable: false },
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

    campaignPageCount() {
      if (!this.campaigns.perPage || !this.campaigns.total) {
        return 1;
      }
      return Math.max(1, Math.ceil(this.campaigns.total / this.campaigns.perPage));
    },

    numSelectedCampaigns() {
      return this.bulk.all ? this.campaigns.total : this.bulk.checked.length;
    },
  },

  created() {
    this.$events.$on('page.refresh', this.getCampaigns);
  },

  mounted() {
    this.getCampaigns();
    this.pollStats();
  },

  destroyed() {
    this.$events.$off('page.refresh', this.getCampaigns);
    clearInterval(this.pollID);
  },
};
</script>

<style scoped>
.campaigns {
  --campaigns-border: #dce5f2;
  --campaigns-border-strong: #c7d5ea;
  --campaigns-surface-soft: #f6f9ff;
}

.page-header {
  align-items: center;
  display: flex;
  gap: 16px;
  justify-content: space-between;
  margin-bottom: 16px;
}

.header-content h1 {
  line-height: 1.25;
  margin: 0;
}

.header-actions {
  align-items: center;
  display: flex;
  justify-content: flex-end;
}

.query-card {
  background: linear-gradient(180deg, #ffffff 0%, #f6f9ff 100%);
  border: 1px solid var(--campaigns-border);
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

.campaigns-table {
  background: #fff;
  border: 1px solid var(--campaigns-border);
  border-radius: 16px;
  overflow: hidden;
}

.campaigns-table :deep(thead th) {
  background: var(--campaigns-surface-soft);
  border-bottom: 1px solid var(--campaigns-border-strong) !important;
  color: #334155;
  font-weight: 600;
}

.campaigns-table :deep(tbody td) {
  padding-bottom: 14px !important;
  padding-top: 14px !important;
  vertical-align: top;
}

.campaigns-table :deep(tbody tr:hover) {
  background: #f8fbff;
}

.campaigns-table :deep(.v-data-table__tr--selected) {
  background: #edf5ff;
}

.campaigns-table :deep(td ul) {
  margin: 0;
  padding-left: 1rem;
}

.table-pagination {
  align-items: center;
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
  margin-top: 14px;
}

@media (max-width: 960px) {
  .page-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .header-actions {
    justify-content: flex-start;
  }

  .query-form {
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
