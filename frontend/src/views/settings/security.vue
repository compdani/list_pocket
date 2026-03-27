<template>
  <div class="settings-section">
    <v-card variant="outlined" class="pa-5">
      <div class="section-head">
        <div>
          <h2 class="text-h6 mb-1">{{ $t('settings.security.enableCaptcha') }}</h2>
          <p class="text-body-2 text-medium-emphasis">{{ $t('settings.security.enableCaptchaHelp') }}</p>
        </div>
        <v-switch
          v-model="captchaEnabled"
          color="primary"
          hide-details
          inset
          name="security.captcha"
        />
      </div>

      <template v-if="captchaEnabled">
        <v-radio-group v-model="selectedProvider" inline class="mt-4" hide-details>
          <v-radio label="ALTCHA" value="altcha" />
          <v-radio label="hCaptcha (deprecated)" value="hcaptcha" />
        </v-radio-group>

        <div v-if="selectedProvider === 'altcha'" class="mt-4">
          <v-text-field
            v-model.number="data['security.captcha'].altcha.complexity"
            :hint="$t('settings.security.altchaComplexityHelp')"
            :label="$t('settings.security.altchaComplexity')"
            max="1000000"
            min="1000"
            name="altcha_complexity"
            persistent-hint
            required
            type="number"
          />
        </div>

        <div v-else class="mt-4">
          <v-text-field
            v-model="data['security.captcha'].hcaptcha.key"
            :hint="$t('settings.security.captchaKeyHelp')"
            :label="$t('settings.security.captchaKey')"
            maxlength="200"
            name="hcaptcha_key"
            persistent-hint
            required
          />
          <v-text-field
            v-model="data['security.captcha'].hcaptcha.secret"
            :label="$t('settings.security.captchaSecret')"
            maxlength="200"
            name="hcaptcha_secret"
            required
            type="password"
          />
        </div>
      </template>
    </v-card>

    <v-card variant="outlined" class="pa-5">
      <h2 class="text-h6 mb-4">CORS</h2>
      <v-textarea
        v-model="corsDomains"
        :hint="$t('settings.security.CORSDomainsHelp')"
        :label="$t('settings.security.CORSDomains')"
        auto-grow
        name="cors_origins"
        persistent-hint
        placeholder="https://example.com"
        rows="5"
      />
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
      data: this.form,
    };
  },

  computed: {
    corsDomains: {
      get() {
        const domains = this.data['security.cors_origins'];
        return domains && Array.isArray(domains) ? domains.join('\n') : '';
      },
      set(value) {
        this.data['security.cors_origins'] = value.split('\n');
      },
    },

    captchaEnabled: {
      get() {
        return this.data['security.captcha'].altcha.enabled || this.data['security.captcha'].hcaptcha.enabled;
      },
      set(value) {
        this.data['security.captcha'].altcha.enabled = !!value;
        this.data['security.captcha'].hcaptcha.enabled = false;
      },
    },

    selectedProvider: {
      get() {
        if (this.data['security.captcha'].hcaptcha.enabled) {
          return 'hcaptcha';
        }
        return 'altcha';
      },
      set(value) {
        this.data['security.captcha'].hcaptcha.enabled = value === 'hcaptcha';
        this.data['security.captcha'].altcha.enabled = value === 'altcha';
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

.section-head,
.toggle-field {
  align-items: start;
  display: flex;
  gap: 16px;
  justify-content: space-between;
}

.toggle-field.slim {
  border: 1px solid rgba(15, 76, 129, 0.14);
  border-radius: 16px;
  padding: 18px 20px;
}

</style>
