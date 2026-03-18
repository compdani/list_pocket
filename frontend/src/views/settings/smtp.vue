<template>
  <div class="settings-section">
    <v-card
      v-for="(item, n) in data.smtp"
      :key="n"
      variant="outlined"
      class="server-card"
    >
      <div class="card-head">
        <div class="toggle-field">
          <div class="text-subtitle-2">{{ $t('globals.buttons.enabled') }}</div>
          <v-switch
            v-model="item.enabled"
            color="primary"
            hide-details
            inset
            name="enabled"
            data-cy="btn-enable-smtp"
          />
        </div>

        <v-btn
          v-if="data.smtp.length > 1"
          variant="text"
          color="error"
          prepend-icon="mdi-trash-can-outline"
          data-cy="btn-delete-smtp"
          @click.prevent="$utils.confirm(null, () => removeSMTP(n))"
        >
          {{ $t('globals.buttons.delete') }}
        </v-btn>
      </div>

      <div :class="{ 'section-disabled': !item.enabled }">
        <v-row>
          <v-col cols="12" md="9">
            <v-text-field
              v-model="item.host"
              :hint="$t('settings.mailserver.hostHelp')"
              :label="$t('settings.mailserver.host')"
              maxlength="200"
              name="host"
              persistent-hint
              placeholder="smtp.yourmailserver.net"
            />
          </v-col>
          <v-col cols="12" md="3">
            <v-text-field
              v-model.number="item.port"
              :hint="$t('settings.mailserver.portHelp')"
              :label="$t('settings.mailserver.port')"
              max="65535"
              min="1"
              name="port"
              persistent-hint
              placeholder="25"
              type="number"
            />
          </v-col>
        </v-row>

        <v-row>
          <v-col cols="12" md="3">
            <v-select
              v-model="item.auth_protocol"
              :items="authProtocolOptions"
              :label="$t('settings.mailserver.authProtocol')"
              name="auth_protocol"
            />
          </v-col>
          <v-col cols="12" md="9">
            <v-row>
              <v-col cols="12" md="6">
                <v-text-field
                  :ref="`smtpUsername${n}`"
                  v-model="item.username"
                  :disabled="item.auth_protocol === 'none'"
                  :label="$t('settings.mailserver.username')"
                  maxlength="200"
                  name="username"
                  placeholder="mysmtp"
                />
              </v-col>
              <v-col cols="12" md="6">
                <v-text-field
                  v-model="item.password"
                  :disabled="item.auth_protocol === 'none'"
                  :hint="$t('settings.mailserver.passwordHelp')"
                  :label="$t('settings.mailserver.password')"
                  maxlength="200"
                  name="password"
                  persistent-hint
                  :placeholder="$t('settings.mailserver.passwordHelp')"
                  type="password"
                />
              </v-col>
            </v-row>
          </v-col>
        </v-row>

        <div class="quick-links">
          <v-btn v-for="provider in providerTemplates" :key="provider.key" size="small" variant="text" @click="fillSettings(n, provider.key)">
            {{ provider.label }}
          </v-btn>
        </div>

        <v-divider class="my-4" />

        <v-row>
          <v-col cols="12" md="6">
            <v-text-field
              v-model="item.hello_hostname"
              :hint="$t('settings.smtp.heloHostHelp')"
              :label="$t('settings.smtp.heloHost')"
              maxlength="200"
              name="hello_hostname"
              persistent-hint
            />
          </v-col>
          <v-col cols="12" md="3">
            <v-select
              v-model="item.tls_type"
              :items="tlsOptions"
              :hint="$t('settings.mailserver.tlsHelp')"
              :label="$t('settings.mailserver.tls')"
              item-title="label"
              item-value="value"
              name="items.tls_type"
              persistent-hint
            />
          </v-col>
          <v-col cols="12" md="3">
            <div class="toggle-field outlined">
              <div>
                <div class="text-subtitle-2">{{ $t('settings.mailserver.skipTLS') }}</div>
                <div class="text-body-2 text-medium-emphasis">{{ $t('settings.mailserver.skipTLSHelp') }}</div>
              </div>
              <v-switch
                v-model="item.tls_skip_verify"
                :disabled="item.tls_type === 'none'"
                color="primary"
                hide-details
                inset
                name="item.tls_skip_verify"
              />
            </div>
          </v-col>
        </v-row>

        <v-divider class="my-4" />

        <v-row>
          <v-col cols="12" md="3">
            <v-text-field
              v-model.number="item.max_conns"
              :hint="$t('settings.mailserver.maxConnsHelp')"
              :label="$t('settings.mailserver.maxConns')"
              max="65535"
              min="1"
              name="max_conns"
              persistent-hint
              placeholder="25"
              type="number"
            />
          </v-col>
          <v-col cols="12" md="3">
            <v-text-field
              v-model.number="item.max_msg_retries"
              :hint="$t('settings.smtp.retriesHelp')"
              :label="$t('settings.smtp.retries')"
              max="1000"
              min="1"
              name="max_msg_retries"
              persistent-hint
              placeholder="2"
              type="number"
            />
          </v-col>
          <v-col cols="12" md="3">
            <v-text-field
              v-model="item.idle_timeout"
              :hint="$t('settings.mailserver.idleTimeoutHelp')"
              :label="$t('settings.mailserver.idleTimeout')"
              :maxlength="10"
              name="idle_timeout"
              persistent-hint
              placeholder="15s"
            />
          </v-col>
          <v-col cols="12" md="3">
            <v-text-field
              v-model="item.wait_timeout"
              :hint="$t('settings.mailserver.waitTimeoutHelp')"
              :label="$t('settings.mailserver.waitTimeout')"
              :maxlength="10"
              name="wait_timeout"
              persistent-hint
              placeholder="5s"
            />
          </v-col>
        </v-row>

        <v-row>
          <v-col cols="12" md="6">
            <v-text-field
              v-model="item.name"
              :hint="$t('settings.mailserver.nameHelp')"
              :label="$t('globals.fields.name')"
              maxlength="100"
              name="name"
              persistent-hint
              placeholder="email-primary"
            />
          </v-col>
        </v-row>

        <div>
          <v-btn
            v-if="item.email_headers.length === 0 && !item.showHeaders"
            variant="text"
            prepend-icon="mdi-plus"
            @click="showSMTPHeaders(n)"
          >
            {{ $t('settings.smtp.setCustomHeaders') }}
          </v-btn>

          <v-textarea
            v-if="item.email_headers.length > 0 || item.showHeaders"
            v-model="item.strEmailHeaders"
            :hint="$t('settings.smtp.customHeadersHelp')"
            label="Headers"
            name="email_headers"
            persistent-hint
            placeholder="[{&quot;X-Custom&quot;: &quot;value&quot;}, {&quot;X-Custom2&quot;: &quot;value&quot;}]"
            rows="4"
          />
        </div>

        <v-divider class="my-4" />

        <div class="test-panel">
          <template v-if="smtpTestItem === n">
            <div class="text-body-2">
              <strong>{{ $t('settings.general.fromEmail') }}</strong><br>
              {{ settings['app.from_email'] }}
            </div>

            <v-text-field
              :ref="`testEmailTo${n}`"
              v-model="testEmail"
              :label="$t('settings.smtp.toEmail')"
              placeholder="email@site.com"
              required
              type="email"
            />

            <v-btn color="primary" @click.prevent="doSMTPTest(item, n)">
              {{ $t('settings.smtp.sendTest') }}
            </v-btn>
          </template>

          <v-btn
            v-else
            variant="text"
            color="primary"
            prepend-icon="mdi-rocket-launch-outline"
            @click.prevent="showTestForm(n)"
          >
            {{ $t('settings.smtp.testConnection') }}
          </v-btn>
        </div>

        <v-alert
          v-if="errMsg && smtpTestItem === n"
          type="error"
          variant="tonal"
          class="mt-4"
        >
          {{ errMsg }}
        </v-alert>
      </div>
    </v-card>

    <v-btn color="primary" prepend-icon="mdi-plus" @click="addSMTP">
      {{ $t('globals.buttons.addNew') }}
    </v-btn>
  </div>
</template>

<script>
import { mapState } from 'vuex';

const smtpTemplates = {
  gmail: {
    host: 'smtp.gmail.com', port: 465, auth_protocol: 'login', tls_type: 'TLS',
  },
  ses: {
    host: 'email-smtp.YOUR-REGION.amazonaws.com', port: 465, auth_protocol: 'login', tls_type: 'TLS',
  },
  mailjet: {
    host: 'in-v3.mailjet.com', port: 465, auth_protocol: 'cram', tls_type: 'TLS',
  },
  mailgun: {
    host: 'smtp.mailgun.org', port: 465, auth_protocol: 'login', tls_type: 'TLS',
  },
  sendgrid: {
    host: 'smtp.sendgrid.net', port: 465, auth_protocol: 'login', tls_type: 'TLS',
  },
  forwardemail: {
    host: 'smtp.forwardemail.net', port: 465, auth_protocol: 'login', tls_type: 'TLS',
  },
  postmark: {
    host: 'smtp.postmarkapp.com', port: 587, auth_protocol: 'cram', tls_type: 'STARTTLS',
  },
};

export default {
  props: {
    form: {
      type: Object, default: () => {},
    },
  },

  data() {
    return {
      data: this.form,
      smtpTestItem: null,
      testEmail: '',
      errMsg: '',
    };
  },

  computed: {
    ...mapState(['settings']),

    authProtocolOptions() {
      return ['login', 'cram', 'plain', 'none'];
    },

    tlsOptions() {
      return [
        { label: this.$t('globals.states.off'), value: 'none' },
        { label: 'STARTTLS', value: 'STARTTLS' },
        { label: 'SSL/TLS', value: 'TLS' },
      ];
    },

    providerTemplates() {
      return [
        { key: 'gmail', label: 'Gmail' },
        { key: 'ses', label: 'Amazon SES' },
        { key: 'mailgun', label: 'Mailgun' },
        { key: 'mailjet', label: 'Mailjet' },
        { key: 'sendgrid', label: 'Sendgrid' },
        { key: 'postmark', label: 'Postmark' },
        { key: 'forwardemail', label: 'Forward Email' },
      ];
    },
  },

  methods: {
    addSMTP() {
      this.data.smtp.push({
        name: '',
        enabled: true,
        host: '',
        hello_hostname: '',
        port: 587,
        auth_protocol: 'none',
        username: '',
        password: '',
        email_headers: [],
        max_conns: 10,
        max_msg_retries: 2,
        idle_timeout: '15s',
        wait_timeout: '5s',
        tls_type: 'STARTTLS',
        tls_skip_verify: false,
      });

      this.$nextTick(() => {
        const latest = this.$refs[`smtpUsername${this.data.smtp.length - 1}`];
        latest?.focus?.();
      });
    },

    removeSMTP(i) {
      this.data.smtp.splice(i, 1);
    },

    showSMTPHeaders(i) {
      const s = this.data.smtp[i];
      s.showHeaders = true;
      this.data.smtp.splice(i, 1, s);
    },

    doSMTPTest(item, n) {
      if (!this.isTestEnabled(item)) {
        this.$utils.toast(this.$t('settings.smtp.testEnterEmail'), 'is-danger');
        this.$nextTick(() => {
          this.data.smtp[n].password = '';
        });
        return;
      }

      this.errMsg = '';
      this.$api.testSMTP({ ...item, email: this.testEmail }).then(() => {
        this.$utils.toast(this.$t('campaigns.testSent'));
      }).catch((err) => {
        if (err.response?.data?.message) {
          this.errMsg = err.response.data.message;
        }
      });
    },

    showTestForm(n) {
      this.smtpTestItem = n;
      this.errMsg = '';

      this.$nextTick(() => {
        this.$refs[`testEmailTo${n}`]?.focus?.();
      });
    },

    isTestEnabled(item) {
      if (!item.host || !item.port) {
        return false;
      }
      if (item.auth_protocol !== 'none' && item.password.includes('•')) {
        return false;
      }
      return true;
    },

    fillSettings(n, key) {
      this.data.smtp.splice(n, 1, {
        ...this.data.smtp[n],
        ...smtpTemplates[key],
        username: '',
        password: '',
        hello_hostname: '',
        tls_skip_verify: false,
      });

      this.$nextTick(() => {
        this.$refs[`smtpUsername${n}`]?.focus?.();
      });
    },
  },
};
</script>

<style scoped>
.settings-section {
  display: grid;
  gap: 20px;
}

.server-card {
  padding: 20px;
}

.card-head,
.toggle-field,
.test-panel {
  align-items: start;
  display: flex;
  gap: 16px;
  justify-content: space-between;
}

.toggle-field.outlined {
  border: 1px solid rgba(15, 76, 129, 0.14);
  border-radius: 16px;
  padding: 18px 20px;
}

.section-disabled {
  opacity: 0.65;
}

.quick-links {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.test-panel {
  align-items: center;
}

@media (max-width: 768px) {
  .card-head,
  .test-panel {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
