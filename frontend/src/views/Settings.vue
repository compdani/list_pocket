<template>
  <v-form @submit.prevent="onSubmit">
    <section class="settings-page">
      <v-overlay
        :model-value="loading.settings || isLoading"
        class="align-center justify-center"
        contained
      >
        <v-progress-circular indeterminate color="primary" size="56" />
      </v-overlay>

      <div class="settings-header">
        <div>
          <h1 class="text-h4 font-weight-bold mb-1">
            {{ $t('settings.title') }}
          </h1>
          <p class="text-medium-emphasis">
            {{ serverConfig.version }}
          </p>
        </div>

        <v-btn
          v-if="$can('settings:manage')"
          type="submit"
          color="primary"
          prepend-icon="mdi-content-save-outline"
          :disabled="!hasFormChanged"
          class="isSaveEnabled"
          data-cy="btn-save"
        >
          {{ $t('globals.buttons.save') }}
        </v-btn>
      </div>

      <v-card v-if="form" variant="outlined" class="settings-shell">
        <v-tabs
          v-model="tab"
          color="primary"
          density="comfortable"
          class="settings-tabs"
          align-tabs="start"
        >
          <v-tab
            v-for="item in settingsTabs"
            :key="item.key"
            :value="item.key"
          >
            {{ item.label }}
          </v-tab>
        </v-tabs>

        <v-divider />

        <v-card-text class="pa-6">
          <v-window v-model="tab" :touch="false">
            <v-window-item value="general">
              <general-settings :form="form" :key="key" />
            </v-window-item>
            <v-window-item value="performance">
              <performance-settings :form="form" :key="key" />
            </v-window-item>
            <v-window-item value="privacy">
              <privacy-settings :form="form" :key="key" />
            </v-window-item>
            <v-window-item value="security">
              <security-settings :form="form" :key="key" />
            </v-window-item>
            <v-window-item value="smtp">
              <smtp-settings :form="form" :key="key" />
            </v-window-item>
            <v-window-item value="bounces">
              <bounce-settings :form="form" :key="key" />
            </v-window-item>
            <v-window-item value="messengers">
              <messenger-settings :form="form" :key="key" />
            </v-window-item>
            <v-window-item value="appearance">
              <appearance-settings :form="form" :key="key" />
            </v-window-item>
          </v-window>
        </v-card-text>
      </v-card>
    </section>
  </v-form>
</template>

<script>
import { mapState } from 'vuex';
import AppearanceSettings from './settings/appearance.vue';
import BounceSettings from './settings/bounces.vue';
import GeneralSettings from './settings/general.vue';
import MessengerSettings from './settings/messengers.vue';
import PerformanceSettings from './settings/performance.vue';
import PrivacySettings from './settings/privacy.vue';
import SecuritySettings from './settings/security.vue';
import SmtpSettings from './settings/smtp.vue';

const SETTINGS_TAB_KEYS = [
  'general',
  'performance',
  'privacy',
  'security',
  'smtp',
  'bounces',
  'messengers',
  'appearance',
];

export default {
  components: {
    GeneralSettings,
    PerformanceSettings,
    PrivacySettings,
    SecuritySettings,
    SmtpSettings,
    BounceSettings,
    MessengerSettings,
    AppearanceSettings,
  },

  data() {
    return {
      key: 0,
      isLoading: false,
      formCopy: '',
      form: null,
      tab: 'general',
    };
  },

  methods: {
    normalizeSettings(data) {
      const d = data || {};

      d.smtp = Array.isArray(d.smtp) ? d.smtp : [];
      if (d.smtp.length === 0) {
        d.smtp = [{
          enabled: true,
          host: '',
          hello_hostname: '',
          port: 25,
          auth_protocol: 'login',
          username: '',
          password: '',
          email_headers: [],
          max_conns: 10,
          max_msg_retries: 2,
          idle_timeout: '15s',
          wait_timeout: '5s',
          tls_type: 'none',
          tls_skip_verify: false,
          name: '',
        }];
      }
      d.messengers = Array.isArray(d.messengers) ? d.messengers : [];
      d['bounce.mailboxes'] = Array.isArray(d['bounce.mailboxes']) ? d['bounce.mailboxes'] : [{
        enabled: false,
        type: 'pop',
        host: '',
        port: 110,
        auth_protocol: 'userpass',
        username: '',
        password: '',
        tls_enabled: false,
        tls_skip_verify: false,
        scan_interval: '15m',
      }];

      d['privacy.domain_blocklist'] = Array.isArray(d['privacy.domain_blocklist'])
        ? d['privacy.domain_blocklist']
        : [];
      d['privacy.domain_allowlist'] = Array.isArray(d['privacy.domain_allowlist'])
        ? d['privacy.domain_allowlist']
        : [];

      d['bounce.actions'] = d['bounce.actions'] || {};
      ['soft', 'hard', 'complaint'].forEach((typ) => {
        d['bounce.actions'][typ] = d['bounce.actions'][typ] || { count: 1, action: 'none' };
      });

      d['bounce.postmark'] = d['bounce.postmark'] || {
        enabled: false,
        username: '',
        password: '',
      };

      d['bounce.forwardemail'] = d['bounce.forwardemail'] || {
        enabled: false,
        key: '',
      };

      d['security.captcha'] = d['security.captcha'] || {};
      d['security.captcha'].altcha = d['security.captcha'].altcha || {
        enabled: true,
        complexity: 50000,
      };
      d['security.captcha'].hcaptcha = d['security.captcha'].hcaptcha || { enabled: false, key: '', secret: '' };

      d['security.oidc'] = d['security.oidc'] || { client_secret: '' };

      return d;
    },

    async onSubmit() {
      const form = this.normalizeSettings(JSON.parse(JSON.stringify(this.form)));

      let hasDummy = '';
      for (let i = 0; i < form.smtp.length; i += 1) {
        form.smtp[i].host = form.smtp[i].host?.trim();

        if (this.isDummy(form.smtp[i].password)) {
          form.smtp[i].password = '';
        } else if (this.hasDummy(form.smtp[i].password)) {
          hasDummy = `smtp #${i + 1}`;
        }

        if (form.smtp[i].strEmailHeaders && form.smtp[i].strEmailHeaders !== '[]') {
          form.smtp[i].email_headers = JSON.parse(form.smtp[i].strEmailHeaders);
        } else {
          form.smtp[i].email_headers = [];
        }
      }

      for (let i = 0; i < form['bounce.mailboxes'].length; i += 1) {
        form['bounce.mailboxes'][i].host = form['bounce.mailboxes'][i].host?.trim();

        if (this.isDummy(form['bounce.mailboxes'][i].password)) {
          form['bounce.mailboxes'][i].password = '';
        } else if (this.hasDummy(form['bounce.mailboxes'][i].password)) {
          hasDummy = `bounce #${i + 1}`;
        }
      }

      if (this.isDummy(form['upload.s3.aws_secret_access_key'])) {
        form['upload.s3.aws_secret_access_key'] = '';
      } else if (this.hasDummy(form['upload.s3.aws_secret_access_key'])) {
        hasDummy = 's3';
      }

      if (this.isDummy(form['bounce.sendgrid_key'])) {
        form['bounce.sendgrid_key'] = '';
      } else if (this.hasDummy(form['bounce.sendgrid_key'])) {
        hasDummy = 'sendgrid';
      }

      if (this.isDummy(form['security.captcha'].hcaptcha.secret)) {
        form['security.captcha'].hcaptcha.secret = '';
      } else if (this.hasDummy(form['security.captcha'].hcaptcha.secret)) {
        hasDummy = 'captcha';
      }

      if (this.isDummy(form['security.oidc'].client_secret)) {
        form['security.oidc'].client_secret = '';
      } else if (this.hasDummy(form['security.oidc'].client_secret)) {
        hasDummy = 'oidc';
      }

      if (this.isDummy(form['bounce.postmark'].password)) {
        form['bounce.postmark'].password = '';
      } else if (this.hasDummy(form['bounce.postmark'].password)) {
        hasDummy = 'postmark';
      }

      if (this.isDummy(form['bounce.forwardemail'].key)) {
        form['bounce.forwardemail'].key = '';
      } else if (this.hasDummy(form['bounce.forwardemail'].key)) {
        hasDummy = 'forwardemail';
      }

      for (let i = 0; i < form.messengers.length; i += 1) {
        if (this.isDummy(form.messengers[i].password)) {
          form.messengers[i].password = '';
        } else if (this.hasDummy(form.messengers[i].password)) {
          hasDummy = `messenger #${i + 1}`;
        }
      }

      if (hasDummy) {
        this.$utils.toast(this.$t('globals.messages.passwordChangeFull', { name: hasDummy }), 'is-danger');
        return false;
      }

      form['privacy.domain_blocklist'] = (Array.isArray(form['privacy.domain_blocklist'])
        ? form['privacy.domain_blocklist']
        : String(form['privacy.domain_blocklist'] || '').split('\n'))
        .map((v) => String(v).trim().toLowerCase())
        .filter((v) => v !== '');
      form['privacy.domain_allowlist'] = (Array.isArray(form['privacy.domain_allowlist'])
        ? form['privacy.domain_allowlist']
        : String(form['privacy.domain_allowlist'] || '').split('\n'))
        .map((v) => String(v).trim().toLowerCase())
        .filter((v) => v !== '');

      this.isLoading = true;
      try {
        const data = await this.$api.updateSettings(form);
        await this.$root.awaitRestart(data);
        this.getSettings();
      } finally {
        this.isLoading = false;
      }

      return false;
    },

    getSettings() {
      this.isLoading = true;
      this.$api.getSettings().then((data) => {
        let d = {};
        try {
          d = JSON.parse(JSON.stringify(data));
        } catch (err) {
          return;
        }

        d = this.normalizeSettings(d);

        for (let i = 0; i < d.smtp.length; i += 1) {
          d.smtp[i].email_headers = Array.isArray(d.smtp[i].email_headers) ? d.smtp[i].email_headers : [];
          d.smtp[i].strEmailHeaders = JSON.stringify(d.smtp[i].email_headers, null, 4);
        }

        d['privacy.domain_blocklist'] = d['privacy.domain_blocklist'].join('\n');
        d['privacy.domain_allowlist'] = d['privacy.domain_allowlist'].join('\n');

        this.key += 1;
        this.form = d;
        this.formCopy = JSON.stringify(d);

        this.$nextTick(() => {
          this.isLoading = false;
        });
      });
    },

    isDummy(pwd) {
      return !pwd || (pwd.match(/•/g) || []).length === pwd.length;
    },

    hasDummy(pwd) {
      return pwd.includes('•');
    },
  },

  computed: {
    ...mapState(['serverConfig', 'loading']),

    settingsTabs() {
      return [
        { key: 'general', label: this.$t('settings.general.name') },
        { key: 'performance', label: this.$t('settings.performance.name') },
        { key: 'privacy', label: this.$t('settings.privacy.name') },
        { key: 'security', label: this.$t('settings.security.name') },
        { key: 'smtp', label: this.$t('settings.smtp.name') },
        { key: 'bounces', label: this.$t('settings.bounces.name') },
        { key: 'messengers', label: this.$t('settings.messengers.name') },
        { key: 'appearance', label: this.$t('settings.appearance.name') },
      ];
    },

    hasFormChanged() {
      if (!this.formCopy) {
        return false;
      }
      return JSON.stringify(this.form) !== this.formCopy;
    },
  },

  beforeRouteLeave() {
    if (this.hasFormChanged()) {
      return new Promise((resolve) => {
        this.$utils.confirm(
          this.$t('globals.messages.confirmDiscard'),
          () => resolve(true),
          () => resolve(false),
        );
      });
    }
    return true;
  },

  mounted() {
    const storedTab = this.$utils.getPref('settings.tab');
    this.tab = SETTINGS_TAB_KEYS.includes(storedTab)
      ? storedTab
      : SETTINGS_TAB_KEYS[Number.isInteger(storedTab) ? storedTab : 0] || 'general';
    this.getSettings();
  },

  watch: {
    tab(t) {
      this.$utils.setPref('settings.tab', t);
    },
  },
};
</script>

<style scoped>
.settings-page {
  position: relative;
}

.settings-header {
  align-items: flex-start;
  display: flex;
  gap: 16px;
  justify-content: space-between;
  margin-bottom: 24px;
}

.settings-shell {
  overflow: visible;
}

.settings-tabs {
  padding: 0 12px;
}

:deep(.v-window-item) {
  padding-top: 12px;
}

@media (max-width: 768px) {
  .settings-header {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
