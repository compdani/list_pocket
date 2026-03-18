<template>
  <div class="settings-section">
    <v-row>
      <v-col cols="12" md="6">
        <div class="toggle-field">
          <div>
            <div class="text-subtitle-2">{{ $t('settings.privacy.disableTracking') }}</div>
            <div class="text-body-2 text-medium-emphasis">{{ $t('settings.privacy.disableTrackingHelp') }}</div>
          </div>
          <v-switch
            v-model="data['privacy.disable_tracking']"
            color="primary"
            hide-details
            inset
            name="privacy.disable_tracking"
          />
        </div>
      </v-col>
      <v-col cols="12" md="6">
        <div class="toggle-field" :class="{ 'is-disabled': data['privacy.disable_tracking'] }">
          <div>
            <div class="text-subtitle-2">{{ $t('settings.privacy.individualSubTracking') }}</div>
            <div class="text-body-2 text-medium-emphasis">{{ $t('settings.privacy.individualSubTrackingHelp') }}</div>
          </div>
          <v-switch
            v-model="data['privacy.individual_tracking']"
            color="primary"
            :disabled="data['privacy.disable_tracking']"
            hide-details
            inset
            name="privacy.individual_tracking"
          />
        </div>
      </v-col>
    </v-row>

    <div v-for="item in privacyToggles" :key="item.key" class="toggle-field">
      <div>
        <div class="text-subtitle-2">{{ $t(item.label) }}</div>
        <div class="text-body-2 text-medium-emphasis">{{ $t(item.help) }}</div>
      </div>
      <v-switch
        v-model="data[item.key]"
        color="primary"
        hide-details
        inset
        :name="item.key"
      />
    </div>

    <v-divider class="my-2" />

    <v-tabs
      v-model="tab"
      color="primary"
      density="comfortable"
      align-tabs="start"
    >
      <v-tab value="blocklist">
        {{ `${$t('settings.privacy.domainBlocklist')} (${numBlocked})` }}
      </v-tab>
      <v-tab value="allowlist">
        {{ `${$t('settings.privacy.domainAllowlist')} (${numAllowed})` }}
      </v-tab>
    </v-tabs>

    <v-window v-model="tab" :touch="false">
      <v-window-item value="blocklist">
        <v-textarea
          v-model="data['privacy.domain_blocklist']"
          :hint="$t('settings.privacy.domainBlocklistHelp')"
          auto-grow
          name="privacy.domain_blocklist"
          persistent-hint
          rows="8"
        />
      </v-window-item>
      <v-window-item value="allowlist">
        <v-textarea
          v-model="data['privacy.domain_allowlist']"
          :hint="$t('settings.privacy.domainAllowlistHelp')"
          auto-grow
          name="privacy.domain_allowlist"
          persistent-hint
          rows="8"
        />
      </v-window-item>
    </v-window>
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
      tab: 'blocklist',
      privacyToggles: [
        {
          key: 'privacy.unsubscribe_header',
          label: 'settings.privacy.listUnsubHeader',
          help: 'settings.privacy.listUnsubHeaderHelp',
        },
        {
          key: 'privacy.allow_blocklist',
          label: 'settings.privacy.allowBlocklist',
          help: 'settings.privacy.allowBlocklistHelp',
        },
        {
          key: 'privacy.allow_preferences',
          label: 'settings.privacy.allowPrefs',
          help: 'settings.privacy.allowPrefsHelp',
        },
        {
          key: 'privacy.allow_export',
          label: 'settings.privacy.allowExport',
          help: 'settings.privacy.allowExportHelp',
        },
        {
          key: 'privacy.allow_wipe',
          label: 'settings.privacy.allowWipe',
          help: 'settings.privacy.allowWipeHelp',
        },
        {
          key: 'privacy.record_optin_ip',
          label: 'settings.privacy.recordOptinIP',
          help: 'settings.privacy.recordOptinIPHelp',
        },
      ],
    };
  },

  methods: {
    countItems(str = '') {
      return String(str).split('\n').filter((line) => line.trim()).length;
    },
  },

  mounted() {
    this.tab = this.$utils.getPref('settings.privacyDomainTab') || 'blocklist';
  },

  computed: {
    numBlocked() {
      return this.countItems(this.form['privacy.domain_blocklist']);
    },
    numAllowed() {
      return this.countItems(this.form['privacy.domain_allowlist']);
    },
  },

  watch: {
    tab(t) {
      this.$utils.setPref('settings.privacyDomainTab', t);
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

.toggle-field.is-disabled {
  opacity: 0.65;
}
</style>
