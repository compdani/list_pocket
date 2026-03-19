<template>
  <div class="settings-section">
    <v-card
      v-for="(item, n) in data.smtp"
      :key="item.uuid || n"
      variant="outlined"
      class="server-card"
    >
      <div class="server-card__header">
        <div>
          <div class="server-card__title-row">
            <div class="text-subtitle-1 font-weight-medium">
              {{ item.name || item.host || `SMTP ${n + 1}` }}
            </div>
            <v-chip
              size="small"
              :color="item.enabled ? 'success' : 'default'"
              variant="tonal"
            >
              {{ item.enabled ? 'Enabled' : 'Disabled' }}
            </v-chip>
          </div>
          <div class="text-body-2 text-medium-emphasis mt-1">
            {{ item.host || 'No host configured' }}<span v-if="item.port">:{{ item.port }}</span>
          </div>
        </div>

        <div class="server-card__actions">
          <div class="toggle-field">
            <div class="text-caption">{{ $t('globals.buttons.enabled') }}</div>
            <v-switch
              v-model="item.enabled"
              color="primary"
              hide-details
              inset
              density="comfortable"
              name="enabled"
              data-cy="btn-enable-smtp"
            />
          </div>

          <v-btn
            variant="text"
            color="primary"
            prepend-icon="mdi-pencil-outline"
            @click="openEditor(n)"
          >
            Edit
          </v-btn>

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
      </div>

      <v-divider class="my-4" />

      <div class="server-card__meta">
        <div class="summary-item">
          <div class="summary-label">Auth</div>
          <div class="summary-value">{{ item.auth_protocol || 'none' }}</div>
        </div>
        <div class="summary-item">
          <div class="summary-label">TLS</div>
          <div class="summary-value">{{ item.tls_type || 'none' }}</div>
        </div>
        <div class="summary-item">
          <div class="summary-label">User</div>
          <div class="summary-value summary-truncate">{{ item.username || 'Not set' }}</div>
        </div>
        <div class="summary-item">
          <div class="summary-label">Default From</div>
          <div class="summary-value summary-truncate">{{ item.default_from_email || settings['app.from_email'] || 'Not set' }}</div>
        </div>
      </div>

      <div v-if="item.from_addresses?.length" class="server-card__chips">
        <v-chip
          v-for="address in item.from_addresses.slice(0, 3)"
          :key="address"
          size="small"
          variant="tonal"
        >
          {{ address }}
        </v-chip>
        <v-chip
          v-if="item.from_addresses.length > 3"
          size="small"
          variant="text"
        >
          +{{ item.from_addresses.length - 3 }} more
        </v-chip>
      </div>
    </v-card>

    <v-btn color="primary" prepend-icon="mdi-plus" @click="addSMTP">
      {{ $t('globals.buttons.addNew') }}
    </v-btn>

    <v-dialog
      v-model="isEditorOpen"
      max-width="1100"
      scrollable
    >
      <v-card v-if="activeItem" class="editor-card">
        <v-card-title class="editor-card__title">
          <span>{{ activeItem.name || activeItem.host || `SMTP ${activeIndex + 1}` }}</span>
          <v-btn icon="mdi-close" variant="text" @click="closeEditor" />
        </v-card-title>

        <v-divider />

        <v-card-text class="editor-card__body">
          <div :class="{ 'section-disabled': !activeItem.enabled }">
            <v-row>
              <v-col cols="12" md="8">
                <v-text-field
                  v-model="activeItem.host"
                  :hint="$t('settings.mailserver.hostHelp')"
                  :label="$t('settings.mailserver.host')"
                  maxlength="200"
                  name="host"
                  persistent-hint
                  placeholder="smtp.yourmailserver.net"
                />
              </v-col>
              <v-col cols="12" md="4">
                <v-text-field
                  v-model.number="activeItem.port"
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
              <v-col cols="12" md="4">
                <v-select
                  v-model="activeItem.auth_protocol"
                  :items="authProtocolOptions"
                  :label="$t('settings.mailserver.authProtocol')"
                  name="auth_protocol"
                />
              </v-col>
              <v-col cols="12" md="4">
                <v-text-field
                  ref="smtpUsernameInput"
                  v-model="activeItem.username"
                  :disabled="activeItem.auth_protocol === 'none'"
                  :label="$t('settings.mailserver.username')"
                  maxlength="200"
                  name="username"
                  placeholder="mysmtp"
                />
              </v-col>
              <v-col cols="12" md="4">
                <v-text-field
                  v-model="activeItem.password"
                  :disabled="activeItem.auth_protocol === 'none'"
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

            <div class="quick-links">
              <v-btn
                v-for="provider in providerTemplates"
                :key="provider.key"
                size="small"
                variant="text"
                @click="fillSettings(activeIndex, provider.key)"
              >
                {{ provider.label }}
              </v-btn>
            </div>

            <v-divider class="my-4" />

            <v-row>
              <v-col cols="12" md="6">
                <v-text-field
                  v-model="activeItem.name"
                  :hint="$t('settings.mailserver.nameHelp')"
                  :label="$t('globals.fields.name')"
                  maxlength="100"
                  name="name"
                  persistent-hint
                  placeholder="email-primary"
                />
              </v-col>
              <v-col cols="12" md="6">
                <v-text-field
                  v-model="activeItem.default_from_email"
                  :label="$t('settings.general.fromEmail')"
                  maxlength="200"
                  :placeholder="$t('campaigns.fromAddressPlaceholder')"
                  persistent-hint
                />
              </v-col>
            </v-row>

            <v-combobox
              v-model="activeItem.from_addresses"
              :items="activeItem.from_addresses"
              :label="$t('campaigns.fromAddress')"
              chips
              closable-chips
              multiple
              clearable
              variant="outlined"
              :placeholder="$t('campaigns.fromAddressPlaceholder')"
              class="mb-4"
            />

            <div class="text-body-2 text-medium-emphasis mb-4">
              {{ $t('settings.general.fromEmailHelp') }}
            </div>

            <v-row>
              <v-col cols="12" md="6">
                <v-text-field
                  v-model="activeItem.hello_hostname"
                  :hint="$t('settings.smtp.heloHostHelp')"
                  :label="$t('settings.smtp.heloHost')"
                  maxlength="200"
                  name="hello_hostname"
                  persistent-hint
                />
              </v-col>
              <v-col cols="12" md="3">
                <v-select
                  v-model="activeItem.tls_type"
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
                    v-model="activeItem.tls_skip_verify"
                    :disabled="activeItem.tls_type === 'none'"
                    color="primary"
                    hide-details
                    inset
                    name="item.tls_skip_verify"
                  />
                </div>
              </v-col>
            </v-row>

            <v-row>
              <v-col cols="12" md="3">
                <v-text-field
                  v-model.number="activeItem.max_conns"
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
                  v-model.number="activeItem.max_msg_retries"
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
                  v-model="activeItem.idle_timeout"
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
                  v-model="activeItem.wait_timeout"
                  :hint="$t('settings.mailserver.waitTimeoutHelp')"
                  :label="$t('settings.mailserver.waitTimeout')"
                  :maxlength="10"
                  name="wait_timeout"
                  persistent-hint
                  placeholder="5s"
                />
              </v-col>
            </v-row>

            <div class="mt-4">
              <v-btn
                v-if="activeItem.email_headers.length === 0 && !activeItem.showHeaders"
                variant="text"
                prepend-icon="mdi-plus"
                @click="showSMTPHeaders(activeIndex)"
              >
                {{ $t('settings.smtp.setCustomHeaders') }}
              </v-btn>

              <v-textarea
                v-if="activeItem.email_headers.length > 0 || activeItem.showHeaders"
                v-model="activeItem.strEmailHeaders"
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
              <template v-if="smtpTestItem === activeIndex">
                <div class="text-body-2">
                  <strong>{{ $t('settings.general.fromEmail') }}</strong><br>
                  {{ activeItem.default_from_email || settings['app.from_email'] }}
                </div>

                <v-text-field
                  ref="testEmailInput"
                  v-model="testEmail"
                  :label="$t('settings.smtp.toEmail')"
                  placeholder="email@site.com"
                  required
                  type="email"
                />

                <v-btn color="primary" @click.prevent="doSMTPTest(activeItem, activeIndex)">
                  {{ $t('settings.smtp.sendTest') }}
                </v-btn>
              </template>

              <v-btn
                v-else
                variant="text"
                color="primary"
                prepend-icon="mdi-rocket-launch-outline"
                @click.prevent="showTestForm(activeIndex)"
              >
                {{ $t('settings.smtp.testConnection') }}
              </v-btn>
            </div>

            <v-alert
              v-if="errMsg && smtpTestItem === activeIndex"
              type="error"
              variant="tonal"
              class="mt-4"
            >
              {{ errMsg }}
            </v-alert>
          </div>
        </v-card-text>

        <v-divider />

        <v-card-actions class="justify-space-between pa-4">
          <v-btn variant="text" @click="closeEditor">
            Close
          </v-btn>
          <v-btn color="primary" @click="closeEditor">
            Done
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
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
      isEditorOpen: false,
      activeIndex: null,
    };
  },

  computed: {
    ...mapState(['settings']),

    activeItem() {
      if (this.activeIndex === null || this.activeIndex < 0 || this.activeIndex >= this.data.smtp.length) {
        return null;
      }
      return this.data.smtp[this.activeIndex];
    },

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
        from_addresses: [],
        default_from_email: '',
        max_conns: 10,
        max_msg_retries: 2,
        idle_timeout: '15s',
        wait_timeout: '5s',
        tls_type: 'STARTTLS',
        tls_skip_verify: false,
      });

      this.$nextTick(() => {
        this.openEditor(this.data.smtp.length - 1);
      });
    },

    openEditor(index) {
      this.activeIndex = index;
      this.smtpTestItem = null;
      this.errMsg = '';
      this.testEmail = '';
      this.isEditorOpen = true;

      this.$nextTick(() => {
        this.$refs.smtpUsernameInput?.focus?.();
      });
    },

    closeEditor() {
      this.isEditorOpen = false;
      this.activeIndex = null;
      this.smtpTestItem = null;
      this.errMsg = '';
      this.testEmail = '';
    },

    removeSMTP(i) {
      if (this.activeIndex === i) {
        this.closeEditor();
      } else if (this.activeIndex !== null && this.activeIndex > i) {
        this.activeIndex -= 1;
      }
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
        this.$refs.testEmailInput?.focus?.();
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
        this.$refs.smtpUsernameInput?.focus?.();
      });
    },
  },
};
</script>

<style scoped>
.settings-section {
  display: grid;
  gap: 16px;
}

.server-card {
  padding: 20px;
}

.server-card__header {
  align-items: flex-start;
  display: flex;
  gap: 16px;
  justify-content: space-between;
}

.server-card__title-row {
  align-items: center;
  display: flex;
  gap: 10px;
}

.server-card__actions {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}

.server-card__meta {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.server-card__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 16px;
}

.summary-item {
  background: rgba(var(--v-theme-on-surface), 0.03);
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 14px;
  min-width: 0;
  padding: 12px 14px;
}

.summary-label {
  color: rgba(var(--v-theme-on-surface), 0.62);
  font-size: 0.75rem;
  letter-spacing: 0.04em;
  margin-bottom: 4px;
  text-transform: uppercase;
}

.summary-value {
  font-size: 0.95rem;
}

.summary-truncate {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.editor-card {
  max-height: 85vh;
}

.editor-card__title {
  align-items: center;
  display: flex;
  justify-content: space-between;
}

.editor-card__body {
  padding-top: 20px;
}

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

@media (max-width: 959px) {
  .server-card__header,
  .server-card__actions,
  .test-panel {
    align-items: stretch;
    flex-direction: column;
  }

  .server-card__meta {
    grid-template-columns: 1fr;
  }
}
</style>
