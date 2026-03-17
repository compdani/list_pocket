<template>
  <div class="items">
    <div class="columns">
      <div class="column is-6">
        <b-field :label="$t('settings.privacy.disableTracking')" :message="$t('settings.privacy.disableTrackingHelp')">
          <b-switch v-model="data['privacy.disable_tracking']" name="privacy.disable_tracking" />
        </b-field>
      </div>
      <div class="column is-6" :class="{ 'is-disabled': data['privacy.disable_tracking'] }">
        <b-field :label="$t('settings.privacy.individualSubTracking')"
          :message="$t('settings.privacy.individualSubTrackingHelp')">
          <b-switch v-model="data['privacy.individual_tracking']" :disabled="data['privacy.disable_tracking']"
            name="privacy.individual_tracking" />
        </b-field>
      </div>
    </div>

    <b-field :label="$t('settings.privacy.listUnsubHeader')" :message="$t('settings.privacy.listUnsubHeaderHelp')">
      <b-switch v-model="data['privacy.unsubscribe_header']" name="privacy.unsubscribe_header" />
    </b-field>

    <b-field :label="$t('settings.privacy.allowBlocklist')" :message="$t('settings.privacy.allowBlocklistHelp')">
      <b-switch v-model="data['privacy.allow_blocklist']" name="privacy.allow_blocklist" />
    </b-field>

    <b-field :label="$t('settings.privacy.allowPrefs')" :message="$t('settings.privacy.allowPrefsHelp')">
      <b-switch v-model="data['privacy.allow_preferences']" name="privacy.allow_blocklist" />
    </b-field>

    <b-field :label="$t('settings.privacy.allowExport')" :message="$t('settings.privacy.allowExportHelp')">
      <b-switch v-model="data['privacy.allow_export']" name="privacy.allow_export" />
    </b-field>

    <b-field :label="$t('settings.privacy.allowWipe')" :message="$t('settings.privacy.allowWipeHelp')">
      <b-switch v-model="data['privacy.allow_wipe']" name="privacy.allow_wipe" />
    </b-field>

    <b-field :label="$t('settings.privacy.recordOptinIP')" :message="$t('settings.privacy.recordOptinIPHelp')">
      <b-switch v-model="data['privacy.record_optin_ip']" name="privacy.record_optin_ip" />
    </b-field>

    <hr />

    <div class="settings-tabs">
      <button type="button" class="settings-tab" :class="{ 'is-active': tab === 0 }" @click="tab = 0">
        {{ `${$t('settings.privacy.domainBlocklist')} (${numBlocked})` }}
      </button>
      <button type="button" class="settings-tab" :class="{ 'is-active': tab === 1 }" @click="tab = 1">
        {{ `${$t('settings.privacy.domainAllowlist')} (${numAllowed})` }}
      </button>
    </div>

    <section v-show="tab === 0">
      <b-field :message="$t('settings.privacy.domainBlocklistHelp')">
        <b-input type="textarea" v-model="data['privacy.domain_blocklist']" name="privacy.domain_blocklist" />
      </b-field>
    </section>
    <section v-show="tab === 1">
      <b-field :message="$t('settings.privacy.domainAllowlistHelp')">
        <b-input type="textarea" v-model="data['privacy.domain_allowlist']" name="privacy.domain_allowlist" />
      </b-field>
    </section>
  </div>
</template>

<script>

export default {
  props: {
    form: {
      type: Object, default: () => { },
    },
  },

  data() {
    return {
      data: this.form,
      tab: 0,
    };
  },

  methods: {
    countItems(str) {
      return str.split('\n').filter((line) => line.trim()).length;
    },
  },

  mounted() {
    this.tab = this.$utils.getPref('settings.privacyDomainTab') || 0;
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
