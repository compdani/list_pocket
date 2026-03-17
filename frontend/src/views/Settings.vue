<template>
  <form novalidate @submit.prevent="onSubmit">
    <section class="settings">
      <b-loading :is-full-page="true" v-if="loading.settings || isLoading" active />
      <header class="columns page-header">
        <div class="column is-half">
          <h1 class="title is-4">
            {{ $t('settings.title') }}
            <span class="has-text-grey-light">({{ serverConfig.version }})</span>
          </h1>
        </div>
        <div class="column has-text-right">
          <b-field v-if="$can('settings:manage')" expanded>
            <b-button expanded :disabled="!hasFormChanged" type="is-primary" icon-left="content-save-outline"
              native-type="submit" class="isSaveEnabled" data-cy="btn-save">
              {{ $t('globals.buttons.save') }}
            </b-button>
          </b-field>
        </div>
      </header>
      <hr />

      <section class="wrap" v-if="form">
        <div class="settings-tabs">
          <button
            v-for="(item, index) in settingsTabs"
            :key="item.key"
            type="button"
            class="settings-tab"
            :class="{ 'is-active': tab === index }"
            @click="tab = index"
          >
            {{ item.label }}
          </button>
        </div>

        <section v-show="tab === 0"><general-settings :form="form" :key="key" /></section>
        <section v-show="tab === 1"><performance-settings :form="form" :key="key" /></section>
        <section v-show="tab === 2"><privacy-settings :form="form" :key="key" /></section>
        <section v-show="tab === 3"><security-settings :form="form" :key="key" /></section>
        <section v-show="tab === 4"><media-settings :form="form" :key="key" /></section>
        <section v-show="tab === 5"><smtp-settings :form="form" :key="key" /></section>
        <section v-show="tab === 6"><bounce-settings :form="form" :key="key" /></section>
        <section v-show="tab === 7"><messenger-settings :form="form" :key="key" /></section>
        <section v-show="tab === 8"><appearance-settings :form="form" :key="key" /></section>
      </section>
    </section>
  </form>
</template>

<script>
import { mapState } from 'vuex';
import AppearanceSettings from './settings/appearance.vue';
import BounceSettings from './settings/bounces.vue';
import GeneralSettings from './settings/general.vue';
import MediaSettings from './settings/media.vue';
import MessengerSettings from './settings/messengers.vue';
import PerformanceSettings from './settings/performance.vue';
import PrivacySettings from './settings/privacy.vue';
import SecuritySettings from './settings/security.vue';
import SmtpSettings from './settings/smtp.vue';

export default {
  components: {
    GeneralSettings,
    PerformanceSettings,
    PrivacySettings,
    SecuritySettings,
    MediaSettings,
    SmtpSettings,
    BounceSettings,
    MessengerSettings,
    AppearanceSettings,
  },

  data() {
    return {
      // :key="key" is a ack to re-render child components every time settings
      // is pulled. Otherwise, props don't react.
      key: 0,

      isLoading: false,

      // formCopy is a stringified copy of the original settings against which
      // form is compared to detect changes.
      formCopy: '',
      form: null,
      tab: 0,
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
      d['security.captcha'].hcaptcha = d['security.captcha'].hcaptcha || { secret: '' };

      d['security.oidc'] = d['security.oidc'] || { client_secret: '' };

      return d;
    },

    async onSubmit() {
      const form = this.normalizeSettings(JSON.parse(JSON.stringify(this.form)));

      // SMTP boxes.
      let hasDummy = '';
      for (let i = 0; i < form.smtp.length; i += 1) {
        // trim the host before saving
        form.smtp[i].host = form.smtp[i].host?.trim();

        // If it's the dummy UI password placeholder, ignore it.
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

      // Bounces boxes.
      for (let i = 0; i < form['bounce.mailboxes'].length; i += 1) {
        // trim the host before saving
        form['bounce.mailboxes'][i].host = form['bounce.mailboxes'][i].host?.trim();

        // If it's the dummy UI password placeholder, ignore it.
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
        // If it's the dummy UI password placeholder, ignore it.
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

      // Domain blocklist array from multi-line strings.
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
          // Create a deep-copy of the settings hierarchy.
          d = JSON.parse(JSON.stringify(data));
        } catch (err) {
          return;
        }

        d = this.normalizeSettings(d);

        // Serialize the `email_headers` array map to display on the form.
        for (let i = 0; i < d.smtp.length; i += 1) {
          d.smtp[i].email_headers = Array.isArray(d.smtp[i].email_headers) ? d.smtp[i].email_headers : [];
          d.smtp[i].strEmailHeaders = JSON.stringify(d.smtp[i].email_headers, null, 4);
        }

        // Domain blocklist array to multi-line string.
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
        { key: 'media', label: this.$t('settings.media.title') },
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

  beforeRouteLeave(to, from, next) {
    if (this.hasFormChanged) {
      this.$utils.confirm(this.$t('globals.messages.confirmDiscard'), () => next(true));
      return;
    }
    next(true);
  },

  mounted() {
    this.tab = this.$utils.getPref('settings.tab') || 0;
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
.settings-tabs {
  border-bottom: 1px solid #d8dfec;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 20px;
}

.settings-tab {
  background: #fff;
  border: 1px solid #d8dfec;
  border-bottom: 0;
  border-radius: 12px 12px 0 0;
  color: #667085;
  cursor: pointer;
  font-size: 0.95rem;
  padding: 10px 16px;
}

.settings-tab.is-active {
  background: #f8fbff;
  color: #0f5bd8;
  font-weight: 600;
}
</style>
