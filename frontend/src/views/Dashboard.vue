<template>
  <section class="dashboard content">
    <header class="dashboard-header">
      <h1 class="title is-4 dashboard-date">
        {{ $utils.niceDate(new Date()) }}
      </h1>
    </header>

    <section class="counts wrap">
      <div class="overview-grid relative">
        <b-loading v-if="isCountsLoading" active :is-full-page="false" />

        <article class="overview-card" data-cy="lists">
          <div class="metric-head">
            <p class="metric-value">
              <span class="metric-icon"><b-icon icon="format-list-bulleted-square" /></span>
              {{ $utils.niceNumber(counts.lists.total) }}
            </p>
            <p class="metric-label">
              {{ $tc('globals.terms.list', counts.lists.total) }}
            </p>
          </div>
          <ul class="metric-breakdown">
            <li>
              <label for="#">{{ $utils.niceNumber(counts.lists.public) }}</label>
              {{ $t('lists.types.public') }}
            </li>
            <li>
              <label for="#">{{ $utils.niceNumber(counts.lists.private) }}</label>
              {{ $t('lists.types.private') }}
            </li>
            <li>
              <label for="#">{{ $utils.niceNumber(counts.lists.optinSingle) }}</label>
              {{ $t('lists.optins.single') }}
            </li>
            <li>
              <label for="#">{{ $utils.niceNumber(counts.lists.optinDouble) }}</label>
              {{ $t('lists.optins.double') }}
            </li>
          </ul>
        </article>

        <article class="overview-card" data-cy="campaigns">
          <div class="metric-head">
            <p class="metric-value">
              <span class="metric-icon"><b-icon icon="rocket-launch-outline" /></span>
              {{ $utils.niceNumber(counts.campaigns.total) }}
            </p>
            <p class="metric-label">
              {{ $tc('globals.terms.campaign', counts.campaigns.total) }}
            </p>
          </div>
          <ul class="metric-breakdown">
            <li v-for="(num, status) in counts.campaigns.byStatus || {}" :key="status">
              <label for="#" :data-cy="`campaigns-${status}`">{{ $utils.niceNumber(num) }}</label>
              {{ $t(`campaigns.status.${status}`) }}
              <span v-if="status === 'running'" class="spinner is-tiny">
                <b-loading :is-full-page="false" active />
              </span>
            </li>
          </ul>
        </article>

        <article class="overview-card" data-cy="subscribers">
          <div class="metric-head">
            <p class="metric-value">
              <span class="metric-icon"><b-icon icon="account-multiple" /></span>
              {{ $utils.niceNumber(counts.subscribers.total) }}
            </p>
            <p class="metric-label">
              {{ $tc('globals.terms.subscriber', counts.subscribers.total) }}
            </p>
          </div>

          <ul class="metric-breakdown">
            <li>
              <label for="#">{{ $utils.niceNumber(counts.subscribers.blocklisted) }}</label>
              {{ $t('subscribers.status.blocklisted') }}
            </li>
            <li>
              <label for="#">{{ $utils.niceNumber(counts.subscribers.orphans) }}</label>
              {{ $t('dashboard.orphanSubs') }}
            </li>
          </ul>

          <div class="metric-divider" />

          <div class="messages-block" data-cy="messages">
            <p class="metric-value compact">
              <span class="metric-icon"><b-icon icon="email-outline" /></span>
              {{ $utils.niceNumber(counts.messages) }}
            </p>
            <p class="metric-label">
              {{ $t('dashboard.messagesSent') }}
            </p>
          </div>
        </article>
      </div>

      <div class="charts-card relative">
        <b-loading v-if="isChartsLoading" active :is-full-page="false" />
        <article class="charts-panel">
          <div class="charts-grid">
            <div class="chart-block">
              <h3 class="title is-size-6 chart-title">
                {{ $t('dashboard.campaignViews') }}
              </h3>
              <chart type="line" v-if="campaignViews" :data="campaignViews" />
            </div>
            <div class="chart-block">
              <h3 class="title is-size-6 chart-title align-right">
                {{ $t('dashboard.linkClicks') }}
              </h3>
              <chart type="line" v-if="campaignClicks" :data="campaignClicks" />
            </div>
          </div>
        </article>
      </div>

      <p v-if="settings['app.cache_slow_queries']" class="has-text-grey">
        *{{ $t('globals.messages.slowQueriesCached') }}
        <a href="https://listmonk.app/docs/maintenance/performance/" target="_blank" rel="noopener noreferer"
          class="has-text-grey">
          <b-icon icon="link-variant" /> {{ $t('globals.buttons.learnMore') }}
        </a>
      </p>
    </section>
  </section>
</template>

<script>
import dayjs from 'dayjs';
import { mapState } from 'vuex';
import { colors } from '../constants';
import Chart from '../components/Chart.vue';

export default {
  components: {
    Chart,
  },

  data() {
    return {
      isChartsLoading: true,
      isCountsLoading: true,
      campaignViews: null,
      campaignClicks: null,
      counts: {
        lists: {},
        subscribers: {},
        campaigns: {},
        messages: 0,
      },
    };
  },

  methods: {
    fetchData() {
      this.isCountsLoading = true;
      this.isChartsLoading = true;

      this.$api.getDashboardCounts().then((data) => {
        this.counts = data;
        this.isCountsLoading = false;
      });

      this.$api.getDashboardCharts().then((data) => {
        this.isChartsLoading = false;
        this.campaignViews = this.makeChart(data.campaignViews);
        this.campaignClicks = this.makeChart(data.linkClicks);
      });
    },

    makeChart(data) {
      if (data.length === 0) {
        return {};
      }
      return {
        labels: data.map((d) => dayjs(d.date).format('DD MMM')),
        datasets: [
          {
            data: [...data.map((d) => d.count)],
            borderColor: colors.primary,
            borderWidth: 2,
            pointHoverBorderWidth: 5,
            pointBorderWidth: 0.5,
          },
        ],
      };
    },
  },

  computed: {
    ...mapState(['settings']),
    dayjs() {
      return dayjs;
    },
  },

  created() {
    this.$events.$on('page.refresh', this.fetchData);
  },

  destroyed() {
    this.$events.$off('page.refresh', this.fetchData);
  },

  mounted() {
    this.fetchData();
  },
};
</script>

<style scoped>
.dashboard {
  --dashboard-border: #dce5f2;
  --dashboard-border-strong: #c7d5ea;
  --dashboard-surface-soft: #f6f9ff;
}

.dashboard-header {
  margin-bottom: 14px;
}

.dashboard-date {
  line-height: 1.25;
  margin: 0;
}

.overview-grid {
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  margin-bottom: 16px;
}

.overview-card {
  background: linear-gradient(180deg, #ffffff 0%, #f6f9ff 100%);
  border: 1px solid var(--dashboard-border);
  border-radius: 16px;
  min-height: 220px;
  padding: 16px;
}

.metric-head {
  margin-bottom: 12px;
}

.metric-value {
  align-items: center;
  color: #0f172a;
  display: flex;
  font-size: 2rem;
  font-weight: 700;
  gap: 8px;
  line-height: 1.15;
  margin: 0;
}

.metric-value.compact {
  font-size: 1.5rem;
}

.metric-icon {
  color: #0f5bd8;
  display: inline-flex;
  font-size: 1.1rem;
}

.metric-label {
  color: #64748b;
  margin: 6px 0 0;
}

.metric-breakdown {
  color: #475569;
  list-style: none;
  margin: 0;
  padding: 0;
}

.metric-breakdown li {
  align-items: center;
  display: flex;
  font-size: 0.95rem;
  gap: 8px;
  justify-content: space-between;
  padding: 4px 0;
}

.metric-breakdown label {
  color: #0f172a;
  font-weight: 700;
  margin-right: auto;
}

.metric-divider {
  border-top: 1px solid var(--dashboard-border-strong);
  margin: 14px 0;
}

.charts-card {
  margin-bottom: 16px;
}

.charts-panel {
  background: #fff;
  border: 1px solid var(--dashboard-border);
  border-radius: 16px;
  padding: 16px;
}

.charts-grid {
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.chart-block {
  border: 1px solid #e6edf8;
  border-radius: 12px;
  padding: 14px;
}

.chart-title {
  margin-bottom: 8px !important;
}

.align-right {
  text-align: right;
}

@media (max-width: 960px) {
  .charts-grid {
    grid-template-columns: 1fr;
  }

  .align-right {
    text-align: left;
  }
}
</style>
