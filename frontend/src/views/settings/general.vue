<template>
  <div class="settings-section">
    <v-text-field
      v-model="data['app.site_name']"
      :label="$t('settings.general.siteName')"
      maxlength="300"
      name="app.site_name"
      required
    />

    <v-text-field
      v-model="data['app.root_url']"
      :label="$t('settings.general.rootURL')"
      :hint="$t('settings.general.rootURLHelp')"
      maxlength="300"
      name="app.root_url"
      persistent-hint
      placeholder="https://listmonk.yoursite.com"
      required
      type="url"
    />

    <v-row>
      <v-col cols="12" md="6">
        <v-text-field
          v-model="data['app.logo_url']"
          :label="$t('settings.general.logoURL')"
          :hint="$t('settings.general.logoURLHelp')"
          maxlength="300"
          name="app.logo_url"
          persistent-hint
          placeholder="https://listmonk.yoursite.com/logo.png"
          type="url"
        />
      </v-col>
      <v-col cols="12" md="6">
        <v-text-field
          v-model="data['app.favicon_url']"
          :label="$t('settings.general.faviconURL')"
          :hint="$t('settings.general.faviconURLHelp')"
          maxlength="300"
          name="app.favicon_url"
          persistent-hint
          placeholder="https://listmonk.yoursite.com/favicon.png"
          type="url"
        />
      </v-col>
    </v-row>

    <v-divider class="my-6" />

    <v-text-field
      v-model="data['app.from_email']"
      :label="$t('settings.general.fromEmail')"
      :hint="$t('settings.general.fromEmailHelp')"
      maxlength="300"
      name="app.from_email"
      persistent-hint
      placeholder="Listmonk <noreply@listmonk.yoursite.com>"
    />

    <v-textarea
      v-model="notifyEmailsInput"
      :label="$t('settings.general.adminNotifEmails')"
      :hint="$t('settings.general.adminNotifEmailsHelp')"
      auto-grow
      name="app.notify_emails"
      persistent-hint
      placeholder="you@yoursite.com"
      rows="3"
    />

    <v-divider class="my-6" />

    <section class="settings-group">
      <h2 class="text-h6 mb-4">
        {{ $tc('globals.terms.subscriptions', 2) }}
      </h2>
      <v-row>
        <v-col cols="12" md="6">
          <div class="toggle-field">
            <div>
              <div class="text-subtitle-2">{{ $t('settings.general.enablePublicSubPage') }}</div>
              <div class="text-body-2 text-medium-emphasis">{{ $t('settings.general.enablePublicSubPageHelp') }}</div>
            </div>
            <v-switch
              v-model="data['app.enable_public_subscription_page']"
              color="primary"
              hide-details
              inset
              name="app.enable_public_subscription_page"
            />
          </div>
        </v-col>
        <v-col cols="12" md="6">
          <div class="toggle-field">
            <div>
              <div class="text-subtitle-2">{{ $t('settings.general.sendOptinConfirm') }}</div>
              <div class="text-body-2 text-medium-emphasis">{{ $t('settings.general.sendOptinConfirmHelp') }}</div>
            </div>
            <v-switch
              v-model="data['app.send_optin_confirmation']"
              color="primary"
              hide-details
              inset
              name="app.send_optin_confirmation"
            />
          </div>
        </v-col>
      </v-row>
    </section>

    <v-divider class="my-6" />

    <section class="settings-group">
      <h2 class="text-h6 mb-4">
        {{ $t('campaigns.archive') }}
      </h2>
      <v-row>
        <v-col cols="12" md="6">
          <div class="toggle-field">
            <div>
              <div class="text-subtitle-2">{{ $t('settings.general.enablePublicArchive') }}</div>
              <div class="text-body-2 text-medium-emphasis">{{ $t('settings.general.enablePublicArchiveHelp') }}</div>
            </div>
            <v-switch
              v-model="data['app.enable_public_archive']"
              color="primary"
              hide-details
              inset
              name="app.enable_public_archive"
            />
          </div>
        </v-col>
        <v-col cols="12" md="6">
          <div class="toggle-field">
            <div>
              <div class="text-subtitle-2">{{ $t('settings.general.enablePublicArchiveRSSContent') }}</div>
              <div class="text-body-2 text-medium-emphasis">{{ $t('settings.general.enablePublicArchiveRSSContentHelp') }}</div>
            </div>
            <v-switch
              v-model="data['app.enable_public_archive_rss_content']"
              color="primary"
              hide-details
              inset
              name="app.enable_public_archive_rss_content"
            />
          </div>
        </v-col>
      </v-row>
    </section>

    <v-divider class="my-6" />

    <div class="toggle-field">
      <div>
        <div class="text-subtitle-2">{{ $t('settings.general.checkUpdates') }}</div>
        <div class="text-body-2 text-medium-emphasis">{{ $t('settings.general.checkUpdatesHelp') }}</div>
      </div>
      <v-switch
        v-model="data['app.check_updates']"
        color="primary"
        hide-details
        inset
        name="app.check_updates"
      />
    </div>

    <v-divider class="my-6" />

    <v-select
      v-model="data['app.lang']"
      :items="serverConfig.langs"
      :label="$t('settings.general.language')"
      item-title="name"
      item-value="code"
      name="app.lang"
    />

    <a :href="$docsUrl('i18n/')" target="_blank" rel="noopener noreferrer">
      {{ $t('globals.buttons.more') }} →
    </a>
  </div>
</template>

<script>
import { mapState } from 'vuex';

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

  computed: {
    ...mapState(['serverConfig']),

    notifyEmailsInput: {
      get() {
        return Array.isArray(this.data['app.notify_emails']) ? this.data['app.notify_emails'].join(', ') : '';
      },
      set(value) {
        this.data['app.notify_emails'] = value
          .split(/[\n,]/)
          .map((email) => email.trim())
          .filter(Boolean)
          .filter((email, index, all) => all.indexOf(email) === index);
      },
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
</style>
