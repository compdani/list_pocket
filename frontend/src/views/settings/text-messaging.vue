<template>
  <div class="settings-section">
    <v-overlay
      :model-value="loading"
      class="align-center justify-center"
      contained
    >
      <v-progress-circular indeterminate color="primary" size="48" />
    </v-overlay>

    <v-alert
      v-if="!loading && !quo.enabled"
      type="info"
      variant="tonal"
      class="mb-4"
      density="comfortable"
    >
      {{ $t('settings.textMessaging.disabledHint') }}
    </v-alert>

    <template v-if="!loading">
      <div class="text-subtitle-1 font-weight-medium mb-4">
        {{ $t('settings.textMessaging.quoTitle') }}
      </div>

      <v-switch
        v-model="quo.enabled"
        :label="$t('settings.textMessaging.quoEnabled')"
        color="primary"
        hide-details
        class="mb-4"
        :disabled="!$can('settings:manage')"
        inset
      />

      <v-text-field
        v-model="quo.apiKey"
        :label="$t('settings.textMessaging.apiKey')"
        type="password"
        autocomplete="new-password"
        variant="outlined"
        density="comfortable"
        class="mb-4"
        :disabled="!$can('settings:manage')"
        :placeholder="quo.hasApiKey ? $t('settings.textMessaging.apiKeyPlaceholder') : ''"
      />

      <v-text-field
        v-model="quo.from"
        :label="$t('settings.textMessaging.fromNumber')"
        variant="outlined"
        density="comfortable"
        class="mb-4"
        :disabled="!$can('settings:manage')"
        :hint="$t('settings.textMessaging.fromHelp')"
        persistent-hint
      />

      <v-divider class="my-6" />

      <div class="text-subtitle-1 font-weight-medium mb-2">
        {{ $t('settings.textMessaging.rateLimits') }}
      </div>
      <v-text-field
        v-model.number="limits.maxMessagesPerSecond"
        type="number"
        min="1"
        max="500"
        :label="$t('settings.textMessaging.maxPerSecond')"
        variant="outlined"
        density="comfortable"
        class="mb-3"
        :disabled="!$can('settings:manage')"
      />
      <v-text-field
        v-model.number="limits.minDelayMs"
        type="number"
        min="0"
        :label="$t('settings.textMessaging.minDelayMs')"
        variant="outlined"
        density="comfortable"
        class="mb-3"
        :disabled="!$can('settings:manage')"
      />
      <v-switch
        v-model="limits.slidingWindowEnabled"
        :label="$t('settings.textMessaging.slidingEnabled')"
        hide-details
        class="mb-2"
        inset
        :disabled="!$can('settings:manage')"
      />
      <v-row v-if="limits.slidingWindowEnabled" dense>
        <v-col cols="12" md="6">
          <v-text-field
            v-model.number="limits.slidingWindowSeconds"
            type="number"
            min="1"
            :label="$t('settings.textMessaging.slidingSeconds')"
            variant="outlined"
            density="comfortable"
            :disabled="!$can('settings:manage')"
          />
        </v-col>
        <v-col cols="12" md="6">
          <v-text-field
            v-model.number="limits.slidingMaxMessages"
            type="number"
            min="1"
            :label="$t('settings.textMessaging.slidingMax')"
            variant="outlined"
            density="comfortable"
            :disabled="!$can('settings:manage')"
          />
        </v-col>
      </v-row>
      <v-text-field
        v-model.number="limits.maxSendErrors"
        type="number"
        min="1"
        :label="$t('settings.textMessaging.maxSendErrors')"
        variant="outlined"
        density="comfortable"
        class="mb-4 mt-2"
        :disabled="!$can('settings:manage')"
      />

      <v-divider class="my-6" />

      <div class="text-subtitle-1 font-weight-medium mb-2">
        {{ $t('settings.textMessaging.webhookTitle') }}
      </div>
      <p class="text-body-2 text-medium-emphasis mb-4">
        {{ $t('settings.textMessaging.webhookHelp') }}
      </p>
      <v-text-field
        v-model="webhookPath"
        :label="$t('settings.textMessaging.webhookPath')"
        variant="outlined"
        density="comfortable"
        class="mb-2"
        :disabled="!$can('settings:manage')"
        :hint="$t('settings.textMessaging.webhookUrlHelp')"
        persistent-hint
      />
      <v-text-field
        v-if="webhookUrlPreview"
        :model-value="webhookUrlPreview"
        :label="$t('settings.textMessaging.webhookUrl')"
        variant="outlined"
        density="comfortable"
        class="mb-4"
        readonly
      >
        <template #append-inner>
          <v-btn
            icon="mdi-content-copy"
            variant="text"
            size="small"
            :aria-label="$t('settings.textMessaging.copyWebhookUrl')"
            @click="copyWebhookUrl"
          />
        </template>
      </v-text-field>
      <v-text-field
        v-model="webhookSigningSecret"
        :label="$t('settings.textMessaging.webhookSecret')"
        type="password"
        autocomplete="new-password"
        variant="outlined"
        density="comfortable"
        class="mb-4"
        :disabled="!$can('settings:manage')"
        :placeholder="hasWebhookSigningSecret ? $t('settings.textMessaging.secretPlaceholder') : ''"
      />

      <div class="d-flex flex-wrap ga-2 align-center">
        <v-btn
          v-if="$can('settings:manage')"
          color="primary"
          :loading="saving"
          prepend-icon="mdi-content-save-outline"
          @click="save"
        >
          {{ $t('globals.buttons.save') }}
        </v-btn>
        <v-text-field
          v-model="testTo"
          :label="$t('settings.textMessaging.testTo')"
          density="comfortable"
          variant="outlined"
          hide-details
          class="test-to-field"
          :disabled="!$can('settings:manage')"
        />
        <v-btn
          v-if="$can('settings:manage')"
          variant="tonal"
          :loading="testing"
          @click="sendTest"
        >
          {{ $t('settings.textMessaging.sendTest') }}
        </v-btn>
      </div>
    </template>
  </div>
</template>

<script>
import { mapState } from 'vuex';

export default {
  data() {
    return {
      loading: true,
      saving: false,
      testing: false,
      quo: {
        enabled: false,
        apiKey: '',
        from: '',
        hasApiKey: false,
      },
      limits: {
        maxMessagesPerSecond: 5,
        slidingWindowEnabled: false,
        slidingWindowSeconds: 60,
        slidingMaxMessages: 60,
        minDelayMs: 0,
        maxSendErrors: 5,
      },
      webhookPath: '',
      webhookSigningSecret: '',
      hasWebhookSigningSecret: false,
      testTo: '',
    };
  },

  computed: {
    ...mapState(['serverConfig']),

    webhookUrlPreview() {
      const path = String(this.webhookPath || '').trim().replace(/^\/+/g, '').replace(/\/+$/g, '');
      if (!path) {
        return '';
      }
      let root = '';
      if (this.serverConfig && this.serverConfig.root_url) {
        root = String(this.serverConfig.root_url).trim();
      }
      if (!root && typeof window !== 'undefined') {
        root = window.location.origin;
      }
      root = root.replace(/\/+$/, '');
      if (!root) {
        return '';
      }
      return `${root}/webhooks/quo/${path}`;
    },
  },

  mounted() {
    this.load();
  },

  methods: {
    isDummy(s) {
      return !s || (String(s).match(/•/g) || []).length === String(s).length;
    },

    applyFromApi(data) {
      const q = (data.providers || []).find((p) => p.id === 'quo') || {};
      this.quo = {
        enabled: !!q.enabled,
        apiKey: q.hasApiKey ? '••••••••' : (q.apiKey || ''),
        from: q.from || '',
        hasApiKey: !!q.hasApiKey,
      };
      const sl = data.sendLimits || {};
      this.limits = {
        maxMessagesPerSecond: sl.maxMessagesPerSecond ?? 5,
        slidingWindowEnabled: !!sl.slidingWindowEnabled,
        slidingWindowSeconds: sl.slidingWindowSeconds ?? 60,
        slidingMaxMessages: sl.slidingMaxMessages ?? 60,
        minDelayMs: sl.minDelayMs ?? 0,
        maxSendErrors: sl.maxSendErrors ?? 5,
      };
      this.webhookPath = data.webhookPath || '';
      this.webhookSigningSecret = '';
      this.hasWebhookSigningSecret = !!data.hasWebhookSigningSecret;
    },

    async load() {
      this.loading = true;
      try {
        const raw = await this.$api.getTextMessagingSettings();
        this.applyFromApi(raw);
      } finally {
        this.loading = false;
      }
    },

    copyWebhookUrl() {
      const url = this.webhookUrlPreview;
      if (!url) {
        return;
      }
      const done = () => this.$utils.toast(this.$t('globals.messages.copied'));
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(url).then(done).catch(() => this.copyWebhookUrlFallback(url, done));
      } else {
        this.copyWebhookUrlFallback(url, done);
      }
    },

    copyWebhookUrlFallback(url, done) {
      const input = document.createElement('input');
      input.setAttribute('type', 'text');
      input.style.opacity = '0';
      input.value = url;
      document.body.appendChild(input);
      input.select();
      try {
        document.execCommand('copy');
        done();
      } finally {
        document.body.removeChild(input);
      }
    },

    buildPayload() {
      const apiKey = this.isDummy(this.quo.apiKey) ? '' : this.quo.apiKey;
      const secret = this.isDummy(this.webhookSigningSecret) ? '' : this.webhookSigningSecret;
      return {
        providers: [{
          id: 'quo',
          enabled: this.quo.enabled,
          api_key: apiKey,
          from: this.quo.from,
        }],
        send_limits: {
          max_messages_per_second: this.limits.maxMessagesPerSecond,
          sliding_window_enabled: this.limits.slidingWindowEnabled,
          sliding_window_seconds: this.limits.slidingWindowSeconds,
          sliding_max_messages: this.limits.slidingMaxMessages,
          min_delay_ms: this.limits.minDelayMs,
          max_send_errors: this.limits.maxSendErrors,
        },
        webhook_path: this.webhookPath,
        webhook_signing_secret: secret,
      };
    },

    async save() {
      this.saving = true;
      try {
        const out = await this.$api.updateTextMessagingSettings(this.buildPayload());
        this.$utils.toast(this.$t('globals.messages.saved'));
        this.applyFromApi(out);
        await this.$api.getServerConfig();
        if (this.$root.awaitRestart) {
          await this.$root.awaitRestart(out);
        }
      } finally {
        this.saving = false;
      }
    },

    async sendTest() {
      const to = String(this.testTo || '').trim();
      if (!to) {
        this.$utils.toast(this.$t('settings.textMessaging.testToRequired'), 'is-danger');
        return;
      }
      this.testing = true;
      try {
        await this.$api.testTextMessagingSettings({ to });
        this.$utils.toast(this.$t('settings.textMessaging.testOk'));
      } finally {
        this.testing = false;
      }
    },
  },
};
</script>

<style scoped>
.settings-section {
  position: relative;
  min-height: 120px;
}

.test-to-field {
  max-width: 280px;
}
</style>
