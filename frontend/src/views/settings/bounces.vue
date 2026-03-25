<template>
  <div class="settings-section">
    <v-card variant="outlined" class="pa-5">
      <div class="toggle-field">
        <div>
          <div class="text-subtitle-2">{{ $t('settings.bounces.enable') }}</div>
        </div>
        <v-switch
          v-model="data['bounce.enabled']"
          color="primary"
          hide-details
          inset
          name="bounce.enabled"
          data-cy="btn-enable-bounce"
        />
      </div>

      <v-row class="mt-4" v-for="typ in bounceTypes" :key="typ">
        <v-col cols="12" md="2" class="d-flex align-center text-subtitle-2" :class="{ 'section-disabled': !data['bounce.enabled'] }">
          {{ $t(`bounces.${typ}`) }}
        </v-col>
        <v-col cols="12" md="5">
          <v-text-field
            v-model.number="data['bounce.actions'][typ].count"
            :disabled="!data['bounce.enabled']"
            :hint="$t('settings.bounces.countHelp')"
            :label="$t('settings.bounces.count')"
            max="1000"
            min="1"
            name="bounce.count"
            persistent-hint
            placeholder="3"
            type="number"
            data-cy="btn-bounce-count"
          />
        </v-col>
        <v-col cols="12" md="5">
          <v-select
            v-model="data['bounce.actions'][typ].action"
            :disabled="!data['bounce.enabled']"
            :items="bounceActionOptions"
            :label="$t('settings.bounces.action')"
            item-title="label"
            item-value="value"
            name="bounce.action"
          />
        </v-col>
      </v-row>
    </v-card>

    <v-card variant="outlined" class="pa-5">
      <div class="toggle-field">
        <div>
          <div class="text-subtitle-2">{{ $t('settings.bounces.enableWebhooks') }}</div>
          <a :href="$docsUrl('bounces/')" target="_blank" rel="noopener noreferrer">
            {{ $t('globals.buttons.learnMore') }}
          </a>
        </div>
        <v-switch
          v-model="data['bounce.webhooks_enabled']"
          :disabled="!data['bounce.enabled']"
          color="primary"
          hide-details
          inset
          name="webhooks_enabled"
          data-cy="btn-enable-bounce-webhook"
        />
      </div>

      <div v-if="data['bounce.webhooks_enabled']" class="settings-section compact mt-4">
        <div class="toggle-field compact">
          <div class="text-subtitle-2">{{ $t('settings.bounces.enableSES') }}</div>
          <v-switch
            v-model="data['bounce.ses_enabled']"
            color="primary"
            hide-details
            inset
            name="ses_enabled"
            data-cy="btn-enable-bounce-ses"
          />
        </div>

        <v-row>
          <v-col cols="12" md="3">
            <div class="toggle-field compact">
              <div class="text-subtitle-2">{{ $t('settings.bounces.enableSendgrid') }}</div>
              <v-switch
                v-model="data['bounce.sendgrid_enabled']"
                color="primary"
                hide-details
                inset
                name="sendgrid_enabled"
                data-cy="btn-enable-bounce-sendgrid"
              />
            </div>
          </v-col>
          <v-col cols="12" md="9">
            <v-text-field
              v-model="data['bounce.sendgrid_key']"
              :disabled="!data['bounce.sendgrid_enabled']"
              :hint="$t('globals.messages.passwordChange')"
              :label="$t('settings.bounces.sendgridKey')"
              name="sendgrid_enabled"
              persistent-hint
              type="password"
            />
          </v-col>
        </v-row>

        <v-row>
          <v-col cols="12" md="3">
            <div class="toggle-field compact">
              <div class="text-subtitle-2">{{ $t('settings.bounces.enablePostmark') }}</div>
              <v-switch
                v-model="data['bounce.postmark'].enabled"
                color="primary"
                hide-details
                inset
                name="postmark_enabled"
                data-cy="btn-enable-bounce-postmark"
              />
            </div>
          </v-col>
          <v-col cols="12" md="4">
            <v-text-field
              v-model="data['bounce.postmark'].username"
              :disabled="!data['bounce.postmark'].enabled"
              :hint="$t('settings.bounces.postmarkUsernameHelp')"
              :label="$t('settings.bounces.postmarkUsername')"
              name="postmark_username"
              persistent-hint
            />
          </v-col>
          <v-col cols="12" md="5">
            <v-text-field
              v-model="data['bounce.postmark'].password"
              :disabled="!data['bounce.postmark'].enabled"
              :hint="$t('globals.messages.passwordChange')"
              :label="$t('settings.bounces.postmarkPassword')"
              name="postmark_password"
              persistent-hint
              type="password"
            />
          </v-col>
        </v-row>

        <v-row>
          <v-col cols="12" md="3">
            <div class="toggle-field compact">
              <div class="text-subtitle-2">{{ $t('settings.bounces.enableForwardemail') }}</div>
              <v-switch
                v-model="data['bounce.forwardemail'].enabled"
                color="primary"
                hide-details
                inset
                name="forwardemail_enabled"
                data-cy="btn-enable-bounce-forwardemail"
              />
            </div>
          </v-col>
          <v-col cols="12" md="9">
            <v-text-field
              v-model="data['bounce.forwardemail'].key"
              :disabled="!data['bounce.forwardemail'].enabled"
              :hint="$t('globals.messages.passwordChange')"
              :label="$t('settings.bounces.forwardemailKey')"
              name="forwardemail_enabled"
              persistent-hint
              type="password"
            />
          </v-col>
        </v-row>

        <v-row>
          <v-col cols="12" md="3">
            <div class="toggle-field compact">
              <div class="text-subtitle-2">{{ $t('settings.bounces.enableBrevo') }}</div>
              <v-switch
                v-model="data['bounce.brevo'].enabled"
                color="primary"
                hide-details
                inset
                name="brevo_enabled"
                data-cy="btn-enable-bounce-brevo"
              />
            </div>
          </v-col>
          <v-col cols="12" md="6">
            <v-text-field
              v-model="data['bounce.brevo'].token"
              :disabled="!data['bounce.brevo'].enabled"
              :hint="$t('settings.bounces.brevoTokenHelp')"
              :label="$t('settings.bounces.brevoToken')"
              name="brevo_token"
              persistent-hint
              type="password"
            />
          </v-col>
          <v-col cols="12" md="3" class="d-flex align-center">
            <v-btn
              variant="tonal"
              color="primary"
              :disabled="!data['bounce.brevo'].enabled"
              @click="generateBrevoToken"
            >
              {{ $t('settings.bounces.generateWebhookToken') }}
            </v-btn>
          </v-col>
        </v-row>
      </div>
    </v-card>

    <v-card variant="outlined" class="pa-5">
      <div class="toggle-field">
        <div class="text-subtitle-2">{{ $t('settings.bounces.enableMailbox') }}</div>
        <v-switch
          v-if="data['bounce.mailboxes']"
          v-model="data['bounce.mailboxes'][0].enabled"
          :disabled="!data['bounce.enabled']"
          color="primary"
          hide-details
          inset
          name="enabled"
          data-cy="btn-enable-bounce-mailbox"
        />
      </div>

      <template v-if="data['bounce.enabled'] && data['bounce.mailboxes'][0].enabled">
        <v-card
          v-for="(item, n) in data['bounce.mailboxes']"
          :key="n"
          variant="tonal"
          class="mailbox-card mt-4"
        >
          <div :class="{ 'section-disabled': !item.enabled }">
            <v-row>
              <v-col cols="12" md="3">
                <v-select
                  v-model="item.type"
                  :items="mailboxTypeOptions"
                  :label="$t('settings.bounces.type')"
                  name="type"
                />
              </v-col>
              <v-col cols="12" md="6">
                <v-text-field
                  v-model="item.host"
                  :hint="$t('settings.mailserver.hostHelp')"
                  :label="$t('settings.mailserver.host')"
                  maxlength="200"
                  name="host"
                  persistent-hint
                  placeholder="bounce.yourmailserver.net"
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
                  :items="getMailboxAuthOptions(item.type)"
                  :label="$t('settings.mailserver.authProtocol')"
                  name="auth_protocol"
                />
              </v-col>
              <v-col cols="12" md="4">
                <v-text-field
                  v-model="item.username"
                  :disabled="item.auth_protocol === 'none'"
                  :label="$t('settings.mailserver.username')"
                  maxlength="200"
                  name="username"
                  placeholder="mysmtp"
                />
              </v-col>
              <v-col cols="12" md="5">
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

            <v-row>
              <v-col cols="12" md="4">
                <div class="toggle-field compact">
                  <div>
                    <div class="text-subtitle-2">{{ $t('settings.mailserver.tls') }}</div>
                    <div class="text-body-2 text-medium-emphasis">{{ $t('settings.mailserver.tlsHelp') }}</div>
                  </div>
                  <v-switch v-model="item.tls_enabled" color="primary" hide-details inset name="item.tls_enabled" />
                </div>
              </v-col>
              <v-col cols="12" md="4">
                <div class="toggle-field compact">
                  <div>
                    <div class="text-subtitle-2">{{ $t('settings.mailserver.skipTLS') }}</div>
                    <div class="text-body-2 text-medium-emphasis">{{ $t('settings.mailserver.skipTLSHelp') }}</div>
                  </div>
                  <v-switch
                    v-model="item.tls_skip_verify"
                    :disabled="!item.tls_enabled"
                    color="primary"
                    hide-details
                    inset
                    name="item.tls_skip_verify"
                  />
                </div>
              </v-col>
              <v-col cols="12" md="4">
                <v-text-field
                  v-model="item.scan_interval"
                  :hint="$t('settings.bounces.scanIntervalHelp')"
                  :label="$t('settings.bounces.scanInterval')"
                  :maxlength="10"
                  name="scan_interval"
                  persistent-hint
                  placeholder="15m"
                />
              </v-col>
            </v-row>
          </div>
        </v-card>
      </template>
    </v-card>
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
      bounceTypes: ['soft', 'hard', 'complaint'],
      data: this.form,
    };
  },

  computed: {
    bounceActionOptions() {
      return [
        { label: this.$t('globals.terms.none'), value: 'none' },
        { label: this.$t('email.unsub'), value: 'unsubscribe' },
        { label: this.$t('settings.bounces.blocklist'), value: 'blocklist' },
        { label: this.$t('globals.buttons.delete'), value: 'delete' },
      ];
    },

    mailboxTypeOptions() {
      return ['pop'];
    },
  },

  methods: {
    getMailboxAuthOptions(type) {
      if (type === 'pop') {
        return ['none', 'userpass'];
      }
      return ['none', 'cram', 'plain', 'login'];
    },

    generateBrevoToken() {
      const arr = new Uint8Array(32);
      crypto.getRandomValues(arr);
      this.data['bounce.brevo'].token = Array.from(arr, (b) => b.toString(16).padStart(2, '0')).join('');
    },
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

.toggle-field.compact {
  padding: 14px 16px;
}

.settings-section.compact {
  gap: 16px;
}

.mailbox-card {
  padding: 16px;
}

.section-disabled {
  opacity: 0.65;
}
</style>
