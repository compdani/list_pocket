<template>
  <div class="settings-section">
    <v-text-field
      v-model.number="data['app.concurrency']"
      :hint="$t('settings.performance.concurrencyHelp')"
      :label="$t('settings.performance.concurrency')"
      max="10000"
      min="1"
      name="app.concurrency"
      persistent-hint
      placeholder="5"
      type="number"
    />

    <v-text-field
      v-model.number="data['app.message_rate']"
      :hint="$t('settings.performance.messageRateHelp')"
      :label="$t('settings.performance.messageRate')"
      max="100000"
      min="1"
      name="app.message_rate"
      persistent-hint
      placeholder="5"
      type="number"
    />

    <v-text-field
      v-model.number="data['app.batch_size']"
      :hint="$t('settings.performance.batchSizeHelp')"
      :label="$t('settings.performance.batchSize')"
      max="100000"
      min="1"
      name="app.batch_size"
      persistent-hint
      placeholder="1000"
      type="number"
    />

    <v-text-field
      v-model.number="data['app.max_send_errors']"
      :hint="$t('settings.performance.maxErrThresholdHelp')"
      :label="$t('settings.performance.maxErrThreshold')"
      max="100000"
      min="0"
      name="app.max_send_errors"
      persistent-hint
      placeholder="1999"
      type="number"
    />

    <v-row>
      <v-col cols="12" md="6">
        <div class="toggle-field">
          <div>
            <div class="text-subtitle-2">{{ $t('settings.performance.slidingWindow') }}</div>
            <div class="text-body-2 text-medium-emphasis">{{ $t('settings.performance.slidingWindowHelp') }}</div>
          </div>
          <v-switch
            v-model="data['app.message_sliding_window']"
            color="primary"
            hide-details
            inset
            name="app.message_sliding_window"
          />
        </div>
      </v-col>

      <v-col cols="12" md="3">
        <v-text-field
          v-model.number="data['app.message_sliding_window_rate']"
          :disabled="!data['app.message_sliding_window']"
          :hint="$t('settings.performance.slidingWindowRateHelp')"
          :label="$t('settings.performance.slidingWindowRate')"
          max="10000000"
          min="1"
          name="sliding_window_rate"
          persistent-hint
          placeholder="25"
          type="number"
        />
      </v-col>

      <v-col cols="12" md="3">
        <v-text-field
          v-model="data['app.message_sliding_window_duration']"
          :disabled="!data['app.message_sliding_window']"
          :hint="$t('settings.performance.slidingWindowDurationHelp')"
          :label="$t('settings.performance.slidingWindowDuration')"
          :maxlength="10"
          name="sliding_window_duration"
          persistent-hint
          placeholder="1h"
        />
      </v-col>
    </v-row>

    <v-divider class="my-2" />

    <v-row>
      <v-col cols="12" md="4">
        <div class="toggle-field">
          <div>
            <div class="text-subtitle-2">{{ $t('settings.performance.cacheSlowQueries') }}</div>
            <div class="text-body-2 text-medium-emphasis">{{ $t('settings.performance.cacheSlowQueriesHelp') }}</div>
          </div>
          <v-switch
            v-model="data['app.cache_slow_queries']"
            color="primary"
            hide-details
            inset
            name="app.cache_slow_queries"
          />
        </div>
      </v-col>
      <v-col cols="12" md="4">
        <v-text-field
          v-model="data['app.cache_slow_queries_interval']"
          :disabled="!data['app.cache_slow_queries']"
          :label="$t('settings.maintenance.cron')"
          placeholder="0 3 * * *"
        />
      </v-col>
      <v-col cols="12" md="4" class="d-flex align-center">
        <a :href="$docsUrl('maintenance/performance/')" target="_blank" rel="noopener noreferrer">
          {{ $t('globals.buttons.learnMore') }}
        </a>
      </v-col>
    </v-row>
  </div>
</template>

<script>
export default {
  props: {
    form: {
      type: Object, default: () => {},
    },
  },

  data() {
    return {
      data: this.form,
    };
  },
};
</script>

<style scoped>
.settings-section {
  display: grid;
  gap: 20px;
}

.toggle-field {
  align-items: start;
  border: 1px solid rgba(15, 76, 129, 0.14);
  border-radius: 16px;
  display: flex;
  gap: 16px;
  justify-content: space-between;
  padding: 18px 20px;
}
</style>
