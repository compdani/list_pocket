<template>
  <section class="maintenance">
    <header class="page-header">
      <h1 class="page-title">
        {{ $t('maintenance.title') }}
      </h1>
      <p class="page-help">
        {{ $t('maintenance.help') }}
      </p>
    </header>

    <v-card variant="outlined" class="section-card">
      <h2 class="section-title">
        {{ $t('globals.terms.subscribers') }}
      </h2>
      <v-row align="end">
        <v-col cols="12" md="5">
          <v-select
            v-model="subscriberType"
            :items="subscriberTypeOptions"
            item-title="title"
            item-value="value"
            label="Data"
            :hint="$t('maintenance.orphanHelp')"
            persistent-hint
            variant="outlined"
            density="comfortable"
          />
        </v-col>
        <v-col cols="12" md="3" offset-md="4">
          <v-btn
            color="primary"
            variant="flat"
            block
            :loading="loading.maintenance"
            @click="deleteSubscribers"
          >
            {{ $t('globals.buttons.delete') }}
          </v-btn>
        </v-col>
      </v-row>
    </v-card>

    <v-card variant="outlined" class="section-card">
      <h2 class="section-title">
        {{ $tc('globals.terms.subscriptions', 2) }}
      </h2>
      <v-row align="end">
        <v-col cols="12" md="4">
          <v-select
            v-model="subscriptionType"
            :items="subscriptionTypeOptions"
            item-title="title"
            item-value="value"
            label="Data"
            variant="outlined"
            density="comfortable"
          />
        </v-col>
        <v-col cols="12" md="4">
          <v-text-field
            v-model="subscriptionDate"
            :label="$t('maintenance.olderThan')"
            type="date"
            required
            variant="outlined"
            density="comfortable"
            prepend-inner-icon="mdi-calendar-clock"
          />
        </v-col>
        <v-col cols="12" md="3" offset-md="1">
          <v-btn
            color="primary"
            variant="flat"
            block
            :loading="loading.maintenance"
            @click="deleteSubscriptions"
          >
            {{ $t('globals.buttons.delete') }}
          </v-btn>
        </v-col>
      </v-row>
    </v-card>

    <v-card variant="outlined" class="section-card">
      <h2 class="section-title">
        {{ $t('globals.terms.analytics') }}
      </h2>
      <v-row align="end">
        <v-col cols="12" md="4">
          <v-select
            v-model="analyticsType"
            :items="analyticsTypeOptions"
            item-title="title"
            item-value="value"
            label="Data"
            variant="outlined"
            density="comfortable"
          />
        </v-col>
        <v-col cols="12" md="4">
          <v-text-field
            v-model="analyticsDate"
            :label="$t('maintenance.olderThan')"
            type="date"
            required
            variant="outlined"
            density="comfortable"
            prepend-inner-icon="mdi-calendar-clock"
          />
        </v-col>
        <v-col cols="12" md="3" offset-md="1">
          <v-btn
            color="primary"
            variant="flat"
            block
            :loading="loading.maintenance"
            @click="deleteAnalytics"
          >
            {{ $t('globals.buttons.delete') }}
          </v-btn>
        </v-col>
      </v-row>
    </v-card>

    <v-card variant="outlined" class="section-card">
      <h2 class="section-title">
        {{ $t('maintenance.database.title') }}
      </h2>
      <h3 class="subsection-title">Vacuum</h3>
      <p class="section-help">
        {{ $t('maintenance.database.vacuumHelp') }}
      </p>
      <v-form @submit.prevent="onUpdateDBSettings">
        <v-row align="end">
          <v-col cols="12" md="2">
            <v-switch
              v-model="dbSettings.vacuum"
              :label="$t('globals.buttons.enabled')"
              color="primary"
              hide-details
            />
          </v-col>
          <v-col cols="12" md="5">
            <v-text-field
              v-model="dbSettings.vacuum_cron_interval"
              :label="$t('settings.maintenance.cron')"
              placeholder="0 2 * * *"
              :disabled="!dbSettings.vacuum"
              pattern="((\*|[0-9,\-\/]+)\s+){4}(\*|[0-9,\-\/]+)"
              variant="outlined"
              density="comfortable"
            />
          </v-col>
          <v-col cols="12" md="3" offset-md="2">
            <v-btn
              color="primary"
              variant="flat"
              block
              type="submit"
              :loading="loading.settings"
            >
              {{ $t('globals.buttons.save') }}
            </v-btn>
          </v-col>
        </v-row>
      </v-form>
    </v-card>

    <v-overlay
      :model-value="isLoading"
      class="align-center justify-center"
      persistent
    >
      <v-progress-circular indeterminate color="primary" size="56" />
    </v-overlay>
  </section>
</template>

<script>
import dayjs from 'dayjs';
import { mapState } from 'vuex';

export default {
  data() {
    return {
      isLoading: false,
      subscriberType: 'orphan',
      analyticsType: 'all',
      subscriptionType: 'optin',
      analyticsDate: dayjs().subtract(7, 'day').format('YYYY-MM-DD'),
      subscriptionDate: dayjs().subtract(7, 'day').format('YYYY-MM-DD'),
      dbSettings: {
        vacuum: false,
        vacuum_cron_interval: '0 2 * * *',
      },
    };
  },

  mounted() {
    this.loadDBSettings();
  },

  methods: {
    deleteSubscribers() {
      this.$utils.confirm(
        null,
        () => {
          this.$api.deleteGCSubscribers(this.subscriberType).then((data) => {
            this.$utils.toast(this.$t(
              'globals.messages.deletedCount',
              { name: this.$tc('globals.terms.subscribers', 2), num: data.count },
            ));
          });
        },
      );
    },

    deleteSubscriptions() {
      this.$utils.confirm(
        null,
        () => {
          this.$api.deleteGCSubscriptions(this.subscriptionDate).then((data) => {
            this.$utils.toast(this.$t(
              'globals.messages.deletedCount',
              { name: this.$tc('globals.terms.subscriptions', 2), num: data.count },
            ));
          });
        },
      );
    },

    deleteAnalytics() {
      this.$utils.confirm(
        null,
        () => {
          this.$api.deleteGCCampaignAnalytics(this.analyticsType, this.analyticsDate)
            .then(() => {
              this.$utils.toast(this.$t('globals.messages.done'));
            });
        },
      );
    },

    loadDBSettings() {
      this.$api.getSettings().then((data) => {
        if (data['maintenance.db'] !== undefined) {
          this.dbSettings = { ...data['maintenance.db'] };
        }
      });
    },

    async onUpdateDBSettings() {
      this.isLoading = true;
      const data = await this.$api.updateSettingsByKey('maintenance.db', this.dbSettings);
      await this.$root.awaitRestart(data);
      this.isLoading = false;
    },
  },

  computed: {
    ...mapState(['loading']),

    subscriberTypeOptions() {
      return [
        { title: this.$t('dashboard.orphanSubs'), value: 'orphan' },
        { title: this.$t('subscribers.status.blocklisted'), value: 'blocklisted' },
      ];
    },

    subscriptionTypeOptions() {
      return [
        { title: this.$t('maintenance.maintenance.unconfirmedOptins'), value: 'optin' },
      ];
    },

    analyticsTypeOptions() {
      return [
        { title: this.$t('globals.terms.all'), value: 'all' },
        { title: this.$t('dashboard.campaignViews'), value: 'views' },
        { title: this.$t('dashboard.linkClicks'), value: 'clicks' },
      ];
    },
  },
};
</script>

<style scoped>
.maintenance {
  max-width: 960px;
}

.page-header {
  margin-bottom: 24px;
}

.page-title {
  font-size: 1.5rem;
  font-weight: 700;
  line-height: 1.25;
  margin: 0 0 8px;
}

.page-help {
  color: #64748b;
  margin: 0;
}

.section-card {
  margin-bottom: 20px;
  padding: 20px;
}

.section-title {
  font-size: 1.25rem;
  font-weight: 600;
  margin: 0 0 16px;
}

.subsection-title {
  font-size: 1.05rem;
  font-weight: 600;
  margin: 0 0 4px;
}

.section-help {
  color: #64748b;
  font-size: 0.85rem;
  margin: 0 0 16px;
}
</style>
