<template>
  <section class="analytics">
    <v-container class="analytics-container">
      <v-row>
        <v-col cols="12">
          <h1 class="text-h4 mb-1">
            {{ $t('analytics.title') }}
          </h1>
          <p class="text-body-2 text-medium-emphasis mb-6 analytics-subtitle">
            {{ $t('analytics.title') }} {{ $t('globals.terms.dashboard').toLowerCase() }}
          </p>
        </v-col>
      </v-row>

      <v-row v-if="serverConfig.privacy.disable_tracking || !serverConfig.privacy.individual_tracking">
        <v-col cols="12">
          <v-alert type="info" variant="tonal">
            <template v-if="serverConfig.privacy.disable_tracking">
              {{ $t('analytics.trackingDisabled') }}
            </template>
            <template v-else-if="!serverConfig.privacy.individual_tracking">
              {{ $t('analytics.nonIndividualTracking') }}
            </template>
          </v-alert>
        </v-col>
      </v-row>

      <v-card variant="outlined" class="mb-6 analytics-filters-card">
        <v-card-text>
          <v-form @submit.prevent="onSubmit">
            <v-row>
              <v-col cols="12" md="6">
                <v-autocomplete
                  v-model="form.campaigns"
                  :items="queriedCampaigns"
                  :label="$t('globals.terms.campaigns')"
                  item-title="name"
                  item-value="id"
                  return-object
                  multiple
                  chips
                  closable-chips
                  :placeholder="$t('globals.terms.campaigns')"
                  :loading="isSearchLoading"
                  @focus="ensureCampaignOptions"
                  @update:search="queryCampaigns"
                />
              </v-col>

              <v-col cols="12" md="3">
                <v-text-field
                  :value="toDateTimeLocal(form.from)"
                  :label="$t('analytics.fromDate')"
                  type="datetime-local"
                  data-cy="from"
                  @input="onFromInput($event)"
                />
              </v-col>

              <v-col cols="12" md="3">
                <v-text-field
                  :value="toDateTimeLocal(form.to)"
                  :label="$t('analytics.toDate')"
                  type="datetime-local"
                  data-cy="to"
                  @input="onToInput($event)"
                />
              </v-col>

              <v-col cols="12" class="d-flex justify-end">
                <v-btn
                  type="submit"
                  color="primary"
                  :disabled="form.campaigns.length === 0"
                  data-cy="btn-search"
                  prepend-icon="mdi-magnify"
                >
                  {{ $t('globals.buttons.refresh') }}
                </v-btn>
              </v-col>
            </v-row>
          </v-form>
        </v-card-text>
      </v-card>

      <!-- Open tracking breakdown panel -->
      <v-row v-if="openTrackingVisible" class="mb-6">
        <v-col cols="12">
          <v-card variant="outlined" class="analytics-section-card">
            <v-card-title class="text-subtitle-1 font-weight-bold pa-4 pb-2">
              {{ $t('analytics.openTracking') }}
            </v-card-title>
            <v-card-subtitle class="px-4 pb-3">
              {{ $t('analytics.openTrackingDesc') }}
            </v-card-subtitle>
            <v-card-text>
              <!-- Summary stat chips -->
              <v-row class="mb-2">
                <v-col cols="12" md="4">
                  <div class="pa-4 rounded-lg text-center analytics-stat analytics-stat-confirmed">
                    <div class="text-caption text-medium-emphasis mb-1">{{ $t('analytics.confirmedCount') }}</div>
                    <div class="text-h5 font-weight-bold text-success">
                      {{ $utils.niceNumber(serverConfig.privacy.individual_tracking ? counts.viewsUnique : counts.views) }}
                    </div>
                  </div>
                </v-col>
                <v-col cols="12" md="4">
                  <div class="pa-4 rounded-lg text-center analytics-stat analytics-stat-raw">
                    <div class="text-caption text-medium-emphasis mb-1">{{ $t('analytics.rawCount') }}</div>
                    <div class="text-h5 font-weight-bold text-info">
                      {{ $utils.niceNumber(serverConfig.privacy.individual_tracking ? counts.viewsUniqueRaw : counts.viewsRaw) }}
                    </div>
                    <div v-if="serverConfig.privacy.individual_tracking" class="text-caption text-medium-emphasis">
                      {{ $utils.niceNumber(counts.viewsRaw) }} {{ $t('analytics.totalViews').toLowerCase() }}
                    </div>
                  </div>
                </v-col>
                <v-col cols="12" md="4">
                  <div class="pa-4 rounded-lg text-center analytics-stat analytics-stat-suspected">
                    <div class="text-caption text-medium-emphasis mb-1">{{ $t('analytics.suspectedCount') }}</div>
                    <div class="text-h5 font-weight-bold text-warning">
                      {{ $utils.niceNumber(serverConfig.privacy.individual_tracking ? counts.viewsUniqueSuspected : counts.viewsSuspected) }}
                    </div>
                  </div>
                </v-col>
              </v-row>

              <v-row>
                <v-col cols="12">
                  <v-progress-circular
                    v-if="openTrackingCombinedLoading"
                    indeterminate
                    color="primary"
                    class="mx-auto d-block mb-4"
                  />
                  <div v-else-if="openTrackingCombinedData">
                    <h3 class="text-h6 mb-3">
                      {{ $t('analytics.openTracking') }}
                    </h3>
                    <div class="d-flex align-center flex-wrap ga-2 mb-3">
                      <v-chip size="small" variant="tonal" color="info">
                        {{ openTrackingLineNames.rawUnique }}
                      </v-chip>
                      <v-chip
                        v-if="openTrackingLineNames.rawAll"
                        size="small"
                        variant="tonal"
                        color="grey"
                      >
                        {{ openTrackingLineNames.rawAll }}
                      </v-chip>
                      <v-chip size="small" variant="tonal" color="warning">
                        {{ openTrackingLineNames.suspected }}
                      </v-chip>
                    </div>
                    <div class="analytics-chart-wrap">
                      <chart type="line" :data="openTrackingCombinedData" />
                    </div>
                  </div>
                </v-col>
              </v-row>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12">
          <div v-for="k in analyticsChartKeys()" :key="k" class="mb-6">
            <template v-if="charts[k]">
              <v-card variant="outlined" class="analytics-section-card">
                <v-card-text>
                  <v-row>
                    <v-col cols="12" md="9">
                      <v-progress-circular
                        v-if="charts[k].loading"
                        indeterminate
                        color="primary"
                        class="mx-auto d-block mb-4"
                      />
                      <div v-else>
                        <h3 v-if="charts[k].chart !== null" class="text-h6 mb-3">
                          {{ charts[k].name }}
                          <span class="text-caption text-medium-emphasis">({{ $utils.niceNumber(counts[k]) }})</span>
                        </h3>
                        <div class="analytics-chart-wrap">
                          <chart :type="charts[k].type" :data="charts[k].data" :on-click="charts[k].onClick" />
                        </div>
                      </div>
                    </v-col>
                    <v-col cols="12" md="3">
                      <chart v-if="!charts[k].loading" type="donut" :data="charts[k].donutData" />
                    </v-col>
                  </v-row>
                </v-card-text>
              </v-card>
            </template>
          </div>
        </v-col>
      </v-row>
    </v-container>
  </section>
</template>

<script>
import dayjs from 'dayjs';
import { mapState } from 'vuex';
import { colors } from '../constants';
import Chart from '../components/Chart.vue';

const chartColorRed = '#ee7d5b';
const chartColors = [
  colors.primary,
  '#FFB50D',
  '#41AC9C',
  chartColorRed,
  '#7FC7BC',
  '#3a82d6',
  '#688ED9',
  '#FFC43D',
];

export default {
  components: {
    Chart,
  },

  data() {
    return {
      isSearchLoading: false,
      queriedCampaigns: [],

      // Data for each view.
      counts: {
        viewsUnique: 0,
        views: 0,
        viewsUniqueRaw: 0,
        viewsRaw: 0,
        viewsUniqueSuspected: 0,
        viewsSuspected: 0,
        clicks: 0,
        bounces: 0,
        unsubscribes: 0,
        links: 0,
      },
      urls: [],
      charts: {
        viewsUnique: {
          name: this.$t('analytics.uniqueViews'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignUniqueViewCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        views: {
          name: this.$t('analytics.totalViews'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignViewCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        viewsRaw: {
          name: this.$t('analytics.rawViews'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignRawViewCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        viewsUniqueRaw: {
          name: this.$t('analytics.uniqueRawViews'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignUniqueRawViewCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        viewsSuspected: {
          name: this.$t('analytics.suspectedViews'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignSuspectedViewCounts,
          chartFn: this.makeCharts,
          donutColor: '#FFB50D',
          loading: false,
        },

        viewsUniqueSuspected: {
          name: this.$t('analytics.uniqueSuspectedViews'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignUniqueSuspectedViewCounts,
          chartFn: this.makeCharts,
          donutColor: '#FFB50D',
          loading: false,
        },

        clicks: {
          name: this.$t('campaigns.clicks'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignClickCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        bounces: {
          name: this.$t('globals.terms.bounces'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignBounceCounts,
          chartFn: this.makeCharts,
          donutColor: chartColorRed,
          loading: false,
        },

        unsubscribes: {
          name: this.$t('analytics.unsubscribes'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignUnsubscribeCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        links: {
          name: this.$t('analytics.links'),
          type: 'bar',
          data: null,
          chart: null,
          loading: false,
          fn: this.$api.getCampaignLinkCounts,
          chartFn: this.makeLinksChart,
          onClick: this.onLinkClick,
        },
      },

      form: {
        campaigns: [],
        from: null,
        to: null,
      },
    };
  },

  methods: {
    formatCampaignOption(campaign) {
      return {
        ...campaign,
        name: `#${campaign.id}: ${campaign.name}`,
      };
    },

    mergeCampaignOptions(campaigns = []) {
      const merged = [...this.queriedCampaigns];
      const seen = new Set(merged.map((item) => item.id));

      campaigns.forEach((campaign) => {
        if (!campaign || !campaign.id || seen.has(campaign.id)) {
          return;
        }

        merged.push(campaign);
        seen.add(campaign.id);
      });

      this.queriedCampaigns = merged;
    },

    parseDateTimeLocal(value) {
      const rawValue = typeof value === 'string'
        ? value
        : value && typeof value === 'object' && 'target' in value
          ? value.target?.value
          : '';

      if (!rawValue) {
        return null;
      }

      const parsed = dayjs(rawValue);
      return parsed.isValid() ? parsed.toDate() : null;
    },

    toDateTimeLocal(value) {
      if (!value) {
        return '';
      }

      const parsed = dayjs(value);
      return parsed.isValid() ? parsed.format('YYYY-MM-DDTHH:mm') : '';
    },

    onFromInput(value) {
      this.form.from = this.parseDateTimeLocal(value);
      this.onFromDateChange();
    },

    onToInput(value) {
      this.form.to = this.parseDateTimeLocal(value);
      this.onToDateChange();
    },

    onFromDateChange() {
      if (this.form.from && this.form.to && this.form.from > this.form.to) {
        this.form.to = dayjs(this.form.from).add(7, 'day').toDate();
      }
    },

    onToDateChange() {
      if (this.form.from && this.form.to && this.form.from > this.form.to) {
        this.form.from = dayjs(this.form.to).add(-7, 'day').toDate();
      }
    },

    formatDateTime(point) {
      const parsed = dayjs(point?.timestamp || point);
      if (!parsed.isValid()) {
        return '';
      }

      return point?.bucket === 'hour'
        ? parsed.format('MMM D, h:mm A')
        : parsed.format('MMM D, YYYY');
    },

    makeLinksChart(typ, camps, data) {
      this.urls = [];
      const labels = data.map((l) => {
        try {
          this.urls.push(l.url);
          const u = new URL(l.url);
          if (l.url.length > 80) {
            return `${u.hostname}${u.pathname.substr(0, 50)}..`;
          }
          return u.hostname + u.pathname;
        } catch {
          return l.url;
        }
      });

      const out = {
        labels,
        datasets: [
          {
            data: data.map((l) => l.count),
            backgroundColor: chartColors,
          }],
      };

      return { points: out, donut: null };
    },

    makeCharts(typ, campaigns, data) {
      // Make a campaign id => camp lookup map to group incoming
      // data by campaigns.
      const camps = campaigns.reduce((obj, c) => {
        const out = { ...obj };
        out[c.id] = c;
        return out;
      }, {});
      const campIDs = Object.keys(camps);
      const labels = [...new Set(data.map((item) => this.formatDateTime(item)))];
      const firstHourlyPoint = data.find((item) => item.bucket === 'hour');
      const transitionLabel = firstHourlyPoint ? this.formatDateTime(firstHourlyPoint) : null;

      // datasets[] array for line chart.
      const lines = campIDs.map((id, n) => {
        const points = data.filter((item) => item.campaignId === id);

        return {
          label: camps[id].name,
          data: points.map((item) => ({ x: this.formatDateTime(item), y: item.count })),
          borderColor: chartColors[n % campIDs.length],
          borderWidth: 2,
          pointHoverBorderWidth: 5,
          pointBorderWidth: 0.5,
        };
      });

      // Donut.
      const donutLabels = [];
      const points = campIDs.map((id) => {
        donutLabels.push(camps[id].name);
        const sum = data.reduce((a, item) => (item.campaignId === id ? a + item.count : a), 0);
        return sum;
      });

      const donut = {
        labels: donutLabels,
        datasets: [{
          data: points, backgroundColor: chartColors, borderWidth: 6,
        }],
      };
      return {
        points: {
          labels,
          datasets: lines,
          transitionLabel,
        },
        donut,
      };
    },

    aggregateLineDatasets(lineData, labels = []) {
      if (!lineData || !Array.isArray(lineData.labels) || !Array.isArray(lineData.datasets)) {
        return null;
      }

      const allLabels = labels.length > 0 ? labels : lineData.labels;
      const totals = new Map(allLabels.map((label) => [label, 0]));
      lineData.datasets.forEach((dataset) => {
        (dataset.data || []).forEach((point) => {
          if (!point || typeof point.x === 'undefined') {
            return;
          }
          const current = totals.get(point.x) || 0;
          totals.set(point.x, current + (point.y || 0));
        });
      });

      return allLabels.map((label) => ({ x: label, y: totals.get(label) || 0 }));
    },

    onSubmit() {
      this.$router.push({ query: { id: this.form.campaigns.map((c) => c.id), from: dayjs(this.form.from).unix(), to: dayjs(this.form.to).unix() } });
    },

    queryCampaigns(q = '') {
      this.isSearchLoading = true;
      this.$api.getCampaigns({
        query: (q || '').trim(),
        order_by: 'created_at',
        order: 'DESC',
        per_page: 20,
      }).then((data) => {
        const options = (data.results || []).map((c) => this.formatCampaignOption(c));
        this.queriedCampaigns = [];
        this.mergeCampaignOptions([...this.form.campaigns, ...options]);
      }).finally(() => {
        this.isSearchLoading = false;
      });
    },

    ensureCampaignOptions() {
      if (this.queriedCampaigns.length > 0) {
        return;
      }

      this.queryCampaigns('');
    },

    getData(typ, camps) {
      this.charts[typ].loading = true;
      // Call the HTTP API.
      this.charts[typ].fn({
        id: camps.map((c) => c.id),
        from: this.form.from,
        to: this.form.to,
      }).then((data) => {
        // Set the total count.
        this.counts[typ] = data.reduce((sum, d) => sum + d.count, 0);

        const { points, donut } = this.charts[typ].chartFn(typ, camps, data);
        this.charts[typ].data = points;
        this.charts[typ].donutData = donut;
        this.charts[typ].loading = false;
      });
    },

    onLinkClick(e) {
      const bars = e.chart.getElementsAtEventForMode(e, 'nearest', { intersect: true }, true);
      if (bars.length > 0) {
        window.open(this.urls[bars[0].index], '_blank', 'noopener noreferrer');
      }
    },

    analyticsChartKeys() {
      if (this.serverConfig.privacy.disable_tracking) {
        return ['clicks', 'bounces', 'unsubscribes', 'links'];
      }

      if (!this.serverConfig.privacy.individual_tracking) {
        return ['clicks', 'bounces', 'unsubscribes', 'links'];
      }

      return ['clicks', 'bounces', 'unsubscribes', 'links'];
    },

    openTrackingChartKeys() {
      if (this.serverConfig.privacy.individual_tracking) {
        return ['viewsUniqueRaw', 'viewsUniqueSuspected'];
      }
      return ['viewsRaw', 'viewsSuspected'];
    },

    openTrackingCountKeys() {
      if (this.serverConfig.privacy.individual_tracking) {
        return ['viewsUnique', 'viewsRaw', 'viewsUniqueRaw', 'viewsUniqueSuspected'];
      }
      return ['views', 'viewsRaw', 'viewsSuspected'];
    },
  },

  computed: {
    ...mapState(['serverConfig']),

    openTrackingVisible() {
      return !this.serverConfig.privacy.disable_tracking
        && (this.form.campaigns.length > 0)
        && (this.counts.viewsRaw > 0 || this.counts.viewsUniqueRaw > 0
            || this.counts.viewsSuspected > 0 || this.counts.viewsUniqueSuspected > 0
            || this.counts.views > 0 || this.counts.viewsUnique > 0);
    },

    openTrackingCombinedLoading() {
      const keys = this.serverConfig.privacy.individual_tracking
        ? [...this.openTrackingChartKeys(), 'viewsRaw']
        : this.openTrackingChartKeys();
      return keys.some((key) => this.charts[key]?.loading);
    },

    openTrackingCombinedData() {
      const [rawKey, suspectedKey] = this.openTrackingChartKeys();
      const rawChart = this.charts[rawKey]?.data;
      const suspectedChart = this.charts[suspectedKey]?.data;
      if (!rawChart || !suspectedChart) {
        return null;
      }

      const labels = [...new Set([...(rawChart.labels || []), ...(suspectedChart.labels || [])])];
      const rawSeries = this.aggregateLineDatasets(rawChart, labels);
      const suspectedSeries = this.aggregateLineDatasets(suspectedChart, labels);
      const rawAllSeries = this.serverConfig.privacy.individual_tracking
        ? this.aggregateLineDatasets(this.charts.viewsRaw?.data, labels)
        : null;
      if (!rawSeries || !suspectedSeries || labels.length === 0) {
        return null;
      }

      const datasets = [
        {
          label: this.charts[rawKey].name,
          data: rawSeries,
          borderColor: '#3a82d6',
          borderWidth: 2,
          pointHoverBorderWidth: 5,
          pointBorderWidth: 0.5,
        },
      ];

      if (rawAllSeries) {
        datasets.push({
          label: this.charts.viewsRaw.name,
          data: rawAllSeries,
          borderColor: '#9E9E9E',
          borderWidth: 2,
          pointHoverBorderWidth: 5,
          pointBorderWidth: 0.5,
        });
      }

      datasets.push({
        label: this.charts[suspectedKey].name,
        data: suspectedSeries,
        borderColor: '#FFB50D',
        borderWidth: 2,
        pointHoverBorderWidth: 5,
        pointBorderWidth: 0.5,
      });

      return {
        labels,
        transitionLabel: rawChart.transitionLabel || suspectedChart.transitionLabel || null,
        datasets,
      };
    },

    openTrackingLineNames() {
      const [rawKey, suspectedKey] = this.openTrackingChartKeys();
      return {
        rawUnique: this.charts[rawKey]?.name || '',
        rawAll: this.serverConfig.privacy.individual_tracking ? this.charts.viewsRaw?.name || '' : '',
        suspected: this.charts[suspectedKey]?.name || '',
      };
    },
  },

  created() {
    const now = dayjs().set('hour', 23).set('minute', 59).set('seconds', 0);
    const weekAgo = now.subtract(7, 'day').set('hour', 0).set('minute', 0);
    const fromUnix = Number(this.$route.query.from);
    const toUnix = Number(this.$route.query.to);
    const from = Number.isFinite(fromUnix) && fromUnix > 0 ? dayjs.unix(fromUnix) : weekAgo;
    const to = Number.isFinite(toUnix) && toUnix > 0 ? dayjs.unix(toUnix) : now;
    this.form.from = from.toDate();
    this.form.to = to.toDate();
  },

  mounted() {
    // Fetch one or more campaigns if there are ?id params, wait for the fetches
    // to finish, add them to the campaign selector and submit the form.
    const ids = this.$utils.parseQueryIDs(this.$route.query.id);
    if (ids.length > 0) {
      this.isSearchLoading = true;
      Promise.allSettled(ids.map((id) => this.$api.getCampaign(id))).then((data) => {
        data.forEach((d) => {
          if (d.status !== 'fulfilled') {
            return;
          }

          const camp = {
            ...this.formatCampaignOption(d.value),
          };
          this.form.campaigns.push(camp);
        });

        this.$nextTick(() => {
          this.isSearchLoading = false;
          this.mergeCampaignOptions(this.form.campaigns);

          // Fetch count for each analytics type (views, counts, bounces);
          const allKeys = [...new Set([
            ...this.analyticsChartKeys(),
            ...this.openTrackingChartKeys(),
            ...this.openTrackingCountKeys(),
          ])];
          allKeys.forEach((k) => {
            this.charts[k].data = null;
            this.charts[k].donutData = null;

            // Fetch views, clicks, bounces for every campaign.
            this.getData(k, this.form.campaigns);
          });
        });
      });
    } else {
      this.ensureCampaignOptions();
    }
  },
};
</script>

<style scoped>
.analytics-container {
  max-width: 1280px;
}

.analytics-subtitle {
  letter-spacing: 0.1px;
}

.analytics-filters-card {
  background: rgba(var(--v-theme-surface), 0.9);
  border-color: rgba(var(--v-theme-on-surface), 0.08);
}

.analytics-section-card {
  background: rgba(var(--v-theme-surface), 0.98);
  border-color: rgba(var(--v-theme-on-surface), 0.08);
}

.analytics-stat {
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}

.analytics-stat-confirmed {
  background: rgba(76, 175, 80, 0.08);
}

.analytics-stat-raw {
  background: rgba(25, 118, 210, 0.08);
}

.analytics-stat-suspected {
  background: rgba(251, 140, 0, 0.12);
}

.analytics-chart-wrap {
  min-height: 320px;
}

@media (max-width: 960px) {
  .analytics-subtitle {
    margin-bottom: 1.25rem !important;
  }

  .analytics-chart-wrap {
    min-height: 280px;
  }
}

@media (max-width: 600px) {
  .analytics-container {
    padding-left: 0.75rem;
    padding-right: 0.75rem;
  }

  .analytics-filters-card :deep(.v-card-text),
  .analytics-section-card :deep(.v-card-text) {
    padding: 0.9rem;
  }

  .analytics-section-card :deep(.v-card-title) {
    padding: 0.9rem 0.9rem 0.35rem;
    font-size: 0.95rem;
  }

  .analytics-section-card :deep(.v-card-subtitle) {
    padding: 0 0.9rem 0.7rem;
    white-space: normal;
    line-height: 1.35;
  }

  .analytics-stat {
    padding: 0.8rem !important;
  }

  .analytics-chart-wrap {
    min-height: 230px;
  }
}
</style>
