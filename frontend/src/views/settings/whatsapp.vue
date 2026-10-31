<template>
  <div class="settings-section" data-cy="whatsapp-settings">
    <v-progress-linear v-if="loading" indeterminate color="primary" class="mb-4" />

    <template v-if="!loading">
      <v-switch
        v-model="form.enabled"
        :label="$t('settings.whatsapp.enabled')"
        :disabled="!canManage"
        color="primary"
        hide-details
        inset
        class="mb-4"
        data-cy="whatsapp-enabled"
      />

      <v-select
        v-model="form.provider"
        :items="providerItems"
        :label="$t('settings.whatsapp.provider')"
        :disabled="!canManage"
        variant="outlined"
        density="comfortable"
        class="mb-4"
        data-cy="whatsapp-provider"
      />

      <template v-if="form.provider === 'cloud'">
        <p class="form-help mb-4">{{ $t('settings.whatsapp.cloudHelp') }}</p>
        <v-text-field
          v-for="field in cloudSecrets"
          :key="field.key"
          v-model="form[field.key]"
          :label="$t(field.label)"
          :placeholder="saved[field.has] ? $t('settings.whatsapp.secretSaved') : ''"
          :disabled="!canManage"
          :data-cy="`whatsapp-${field.key}`"
          type="password"
          autocomplete="new-password"
          variant="outlined"
          density="comfortable"
          class="mb-4"
        />
        <v-text-field
          v-model="form.phone_number_id"
          :label="$t('settings.whatsapp.phoneNumberId')"
          :disabled="!canManage"
          variant="outlined"
          density="comfortable"
          class="mb-4"
          data-cy="whatsapp-phone_number_id"
        />
        <v-text-field
          v-model="form.waba_id"
          :label="$t('settings.whatsapp.wabaId')"
          :disabled="!canManage"
          variant="outlined"
          density="comfortable"
          class="mb-4"
          data-cy="whatsapp-waba_id"
        />
        <v-text-field
          v-model="form.api_version"
          :label="$t('settings.whatsapp.apiVersion')"
          :disabled="!canManage"
          variant="outlined"
          density="comfortable"
          class="mb-4"
        />
        <p v-if="saved.webhook_url" class="form-help mb-4">
          {{ $t('settings.whatsapp.webhookUrl') }}: <code data-cy="whatsapp-webhook-url">{{ absoluteUrl(saved.webhook_url) }}</code>
        </p>
      </template>

      <template v-else>
        <v-alert type="warning" variant="tonal" class="mb-4" data-cy="whatsapp-whapi-risk">
          <p class="mb-2">{{ $t('settings.whatsapp.whapiRisk') }}</p>
          <v-checkbox
            v-model="form.whapi_risk_accepted"
            :label="$t('settings.whatsapp.whapiRiskAccept')"
            :disabled="!canManage"
            hide-details
            density="compact"
            data-cy="whatsapp-whapi-accept"
          />
        </v-alert>
        <v-alert
          v-if="saved.whapi_connected === false"
          type="error"
          variant="tonal"
          density="comfortable"
          class="mb-4"
          :text="saved.whapi_connection_note"
          data-cy="whatsapp-disconnected"
        />
        <v-text-field
          v-model="form.whapi_token"
          :label="$t('settings.whatsapp.whapiToken')"
          :placeholder="saved.has_whapi_token ? $t('settings.whatsapp.secretSaved') : ''"
          :disabled="!canManage"
          type="password"
          autocomplete="new-password"
          variant="outlined"
          density="comfortable"
          class="mb-4"
          data-cy="whatsapp-whapi_token"
        />
        <p v-if="saved.whapi_webhook_url" class="form-help mb-4">
          {{ $t('settings.whatsapp.whapiWebhookUrl') }}: <code data-cy="whatsapp-whapi-webhook-url">{{ absoluteUrl(saved.whapi_webhook_url) }}</code>
        </p>
      </template>

      <v-text-field
        v-model.number="form.max_per_second"
        :label="$t('settings.whatsapp.maxPerSecond')"
        :hint="$t('settings.whatsapp.maxPerSecondHelp', { rate: saved.send_rate })"
        :disabled="!canManage"
        persistent-hint
        type="number"
        min="0"
        variant="outlined"
        density="comfortable"
        class="mb-4"
      />

      <div v-if="canManage" class="d-flex flex-wrap ga-2 mb-2">
        <v-btn color="primary" :loading="saving" data-cy="whatsapp-save" @click="save">
          {{ $t('globals.buttons.save') }}
        </v-btn>
        <v-btn variant="tonal" :loading="testing" data-cy="whatsapp-test" @click="test">
          {{ $t('settings.whatsapp.test') }}
        </v-btn>
      </div>

      <template v-if="canManage">
        <v-divider class="my-6" />
        <div class="text-subtitle-1 font-weight-medium mb-2">{{ $t('settings.whatsapp.consentTitle') }}</div>
        <p class="form-help mb-3">{{ $t('settings.whatsapp.consentHelp') }}</p>
        <v-alert type="warning" variant="tonal" density="comfortable" class="mb-3">
          {{ $t('settings.whatsapp.consentWarning') }}
        </v-alert>
        <v-btn variant="tonal" :loading="counting" class="mb-3" data-cy="whatsapp-consent-count" @click="countConsent">
          {{ $t('settings.whatsapp.consentCount') }}
        </v-btn>
        <template v-if="consent">
          <p class="mb-1" data-cy="whatsapp-consent-explicit">{{ $t('settings.whatsapp.consentExplicit', { n: consent.explicit }) }}</p>
          <p class="mb-2" data-cy="whatsapp-consent-inherited">{{ $t('settings.whatsapp.consentInherited', { n: consent.inherited }) }}</p>
          <v-checkbox
            v-model="includeInherited"
            :label="$t('settings.whatsapp.consentIncludeInherited')"
            hide-details
            density="compact"
          />
          <v-checkbox
            v-model="attested"
            :label="$t('settings.whatsapp.consentAttest')"
            hide-details
            density="compact"
            class="mb-3"
            data-cy="whatsapp-consent-attest"
          />
          <v-btn
            color="primary"
            :disabled="!attested || applyCount === 0"
            :loading="applying"
            data-cy="whatsapp-consent-apply"
            @click="applyConsent"
          >
            {{ $t('settings.whatsapp.consentApply', { n: applyCount }) }}
          </v-btn>
        </template>
      </template>
    </template>
  </div>
</template>

<script>
const SECRET_KEYS = ['access_token', 'app_secret', 'verify_token', 'whapi_token'];

export default {
  data() {
    return {
      loading: true,
      saving: false,
      testing: false,
      counting: false,
      applying: false,
      saved: {},
      form: {},
      consent: null,
      includeInherited: false,
      attested: false,
    };
  },

  computed: {
    canManage() {
      return this.$can('settings:manage');
    },

    providerItems() {
      return [
        { title: this.$t('settings.whatsapp.providerCloud'), value: 'cloud' },
        { title: this.$t('settings.whatsapp.providerWhapi'), value: 'whapi' },
      ];
    },

    cloudSecrets() {
      return [
        { key: 'access_token', has: 'has_access_token', label: 'settings.whatsapp.accessToken' },
        { key: 'app_secret', has: 'has_app_secret', label: 'settings.whatsapp.appSecret' },
        { key: 'verify_token', has: 'has_verify_token', label: 'settings.whatsapp.verifyToken' },
      ];
    },

    applyCount() {
      if (!this.consent) {
        return 0;
      }
      return this.consent.explicit + (this.includeInherited ? this.consent.inherited : 0);
    },
  },

  mounted() {
    this.load();
  },

  methods: {
    absoluteUrl(url) {
      return url.startsWith('/') ? `${window.location.origin}${url}` : url;
    },

    errorText(err) {
      return (err.response && err.response.message) || err.message;
    },

    async load() {
      this.loading = true;
      try {
        const data = await this.$api.getWhatsAppSettings();
        this.saved = data;
        this.form = {
          enabled: data.enabled,
          provider: data.provider,
          access_token: '',
          app_secret: '',
          verify_token: '',
          phone_number_id: data.phone_number_id,
          waba_id: data.waba_id,
          api_base: data.api_base,
          api_version: data.api_version,
          whapi_token: '',
          whapi_api_base: data.whapi_api_base,
          whapi_risk_accepted: data.whapi_risk_accepted,
          max_per_second: data.max_per_second,
        };
      } catch (err) {
        this.$utils.toast(this.errorText(err), 'error');
      } finally {
        this.loading = false;
      }
    },

    async save() {
      this.saving = true;
      try {
        const out = await this.$api.updateWhatsAppSettings({
          ...this.form,
          max_per_second: Number(this.form.max_per_second) || 0,
        });
        this.$utils.toast(this.$t('globals.messages.saved', { name: this.$t('settings.whatsapp.name') }));
        SECRET_KEYS.forEach((key) => { this.form[key] = ''; });
        if (this.$root.awaitRestart && out !== true) {
          await this.$root.awaitRestart({ needsRestart: Boolean(out && out.needs_restart) });
        }
        await this.$api.getServerConfig();
        await this.load();
      } catch (err) {
        this.$utils.toast(this.errorText(err), 'error');
      } finally {
        this.saving = false;
      }
    },

    async test() {
      this.testing = true;
      try {
        await this.$api.testWhatsAppSettings();
        this.$utils.toast(this.$t('settings.whatsapp.testOk'));
      } catch (err) {
        this.$utils.toast(this.errorText(err), 'error');
      } finally {
        this.testing = false;
      }
    },

    async countConsent() {
      this.counting = true;
      try {
        this.consent = await this.$api.copyWhatsAppConsent({ dry_run: true });
      } catch (err) {
        this.$utils.toast(this.errorText(err), 'error');
      } finally {
        this.counting = false;
      }
    },

    async applyConsent() {
      this.applying = true;
      try {
        const out = await this.$api.copyWhatsAppConsent({
          dry_run: false,
          include_inherited: this.includeInherited,
          attested: this.attested,
        });
        this.$utils.toast(this.$t('settings.whatsapp.consentApplied', { n: out.applied }));
        this.attested = false;
        await this.countConsent();
      } catch (err) {
        this.$utils.toast(this.errorText(err), 'error');
      } finally {
        this.applying = false;
      }
    },
  },
};
</script>
