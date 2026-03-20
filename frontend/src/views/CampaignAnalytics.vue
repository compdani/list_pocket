<template>
  <section class="analytics">
    <v-container>
      <v-row>
        <v-col cols="12">
          <h1 class="text-h4 mb-4">
            {{ $t('analytics.title') }}
          </h1>
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

      <v-form @submit.prevent="onSubmit" class="mb-6">
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
              icon="mdi-magnify"
            />
          </v-col>
        </v-row>
      </v-form>

      <v-row>
        <v-col cols="12">
          <div v-for="(v, k) in charts" :key="k" class="mb-6">
            <v-row>
              <v-col cols="12" md="9">
                <v-progress-circular
                  v-if="v.loading"
                  indeterminate
                  color="primary"
                  class="mx-auto d-block mb-4"
                />
                <div v-else>
                  <h3 v-if="v.chart !== null" class="text-h6 mb-3">
                    {{ v.name }}
                    <span class="text-caption text-medium-emphasis">({{ $utils.niceNumber(counts[k]) }})</span>
                  </h3>
                  <chart :type="v.type" :data="v.data" :on-click="v.onClick" />
                </div>
              </v-col>
              <v-col cols="12" md="3">
                <chart v-if="!v.loading" type="donut" :data="v.donutData" />
              </v-col>
            </v-row>
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
        views: 0,
        clicks: 0,
        bounces: 0,
        links: 0,
      },
      urls: [],
      charts: {
        views: {
          name: this.$t('campaigns.views'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignViewCounts,
          chartFn: this.makeCharts,
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
  },

  computed: {
    ...mapState(['serverConfig']),
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
          Object.keys(this.charts).forEach((k) => {
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
