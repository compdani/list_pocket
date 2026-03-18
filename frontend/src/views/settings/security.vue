<template>
  <div class="settings-section">
    <v-card variant="outlined" class="pa-5">
      <div class="section-head">
        <div>
          <h2 class="text-h6 mb-1">{{ $t('settings.security.enableOIDC') }}</h2>
          <p class="text-body-2 text-medium-emphasis">{{ $t('settings.security.OIDCHelp') }}</p>
        </div>
        <v-switch
          v-model="data['security.oidc'].enabled"
          color="primary"
          hide-details
          inset
          name="security.oidc"
        />
      </div>

      <v-row class="mt-2">
        <v-col cols="12" md="7">
          <v-text-field
            v-model="data['security.oidc'].provider_url"
            :disabled="!data['security.oidc'].enabled"
            :label="$t('settings.security.OIDCURL')"
            maxlength="300"
            name="oidc.provider_url"
            placeholder="https://login.yoursite.com"
            required
            type="url"
          />

          <div class="quick-links" :class="{ disabled: !data['security.oidc'].enabled }">
            <v-btn variant="text" size="small" :disabled="!data['security.oidc'].enabled" @click="setProvider('google')">Google</v-btn>
            <v-btn variant="text" size="small" :disabled="!data['security.oidc'].enabled" @click="setProvider('microsoft')">Microsoft</v-btn>
            <v-btn variant="text" size="small" :disabled="!data['security.oidc'].enabled" @click="setProvider('apple')">Apple</v-btn>
          </div>
        </v-col>
        <v-col cols="12" md="5">
          <v-text-field
            ref="provider_name"
            v-model="data['security.oidc'].provider_name"
            :disabled="!data['security.oidc'].enabled"
            :label="$t('settings.security.OIDCName')"
            maxlength="200"
            name="oidc.provider_name"
          />
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" md="6">
          <v-text-field
            ref="client_id"
            v-model="data['security.oidc'].client_id"
            :disabled="!data['security.oidc'].enabled"
            :label="$t('settings.security.OIDCClientID')"
            maxlength="200"
            name="oidc.client_id"
            required
          />
        </v-col>
        <v-col cols="12" md="6">
          <v-text-field
            v-model="data['security.oidc'].client_secret"
            :disabled="!data['security.oidc'].enabled"
            :label="$t('settings.security.OIDCClientSecret')"
            maxlength="200"
            name="oidc.client_secret"
            required
            type="password"
          />
        </v-col>
      </v-row>

      <v-divider class="my-4" />

      <v-row>
        <v-col cols="12" md="4">
          <div class="toggle-field slim">
            <div>
              <div class="text-subtitle-2">{{ $t('settings.security.OIDCAutoCreateUsers') }}</div>
              <div class="text-body-2 text-medium-emphasis">{{ $t('settings.security.OIDCAutoCreateUsersHelp') }}</div>
            </div>
            <v-switch
              v-model="data['security.oidc'].auto_create_users"
              :disabled="!data['security.oidc'].enabled"
              color="primary"
              hide-details
              inset
              name="oidc.auto_create_users"
            />
          </div>
        </v-col>
        <v-col cols="12" md="4">
          <v-select
            v-model="data['security.oidc'].default_user_role_id"
            :disabled="!data['security.oidc'].enabled || !data['security.oidc'].auto_create_users"
            :items="userRoles"
            :hint="$t('settings.security.OIDCDefaultRoleHelp')"
            item-title="name"
            item-value="id"
            :label="$t('settings.security.OIDCDefaultUserRole')"
            name="oidc.default_user_role_id"
            persistent-hint
          />
        </v-col>
        <v-col cols="12" md="4">
          <v-select
            v-model="data['security.oidc'].default_list_role_id"
            :disabled="!data['security.oidc'].enabled || !data['security.oidc'].auto_create_users"
            :items="listRoleOptions"
            :hint="$t('settings.security.OIDCDefaultRoleHelp')"
            item-title="name"
            item-value="id"
            :label="$t('settings.security.OIDCDefaultListRole')"
            name="oidc.default_list_role_id"
            persistent-hint
          />
        </v-col>
      </v-row>

      <v-divider class="my-4" />

      <div class="redirect-row">
        <div>
          <div class="text-subtitle-2">{{ $t('settings.security.OIDCRedirectURL') }}</div>
          <code><copy-text :text="`${serverConfig.root_url}/auth/oidc`" /></code>
        </div>
      </div>

      <v-alert
        v-if="data['security.oidc'].enabled && !isURLOk"
        type="warning"
        variant="tonal"
        class="mt-4"
      >
        {{ $t('settings.security.OIDCRedirectWarning') }}
      </v-alert>
    </v-card>

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
import { mapState } from 'vuex';
import CopyText from '../../components/CopyText.vue';

const OIDC_PROVIDERS = {
  google: 'https://accounts.google.com',
  microsoft: 'https://login.microsoftonline.com/{TENANT_HERE}/v2.0',
  apple: 'https://appleid.apple.com',
};

export default {
  components: {
    CopyText,
  },

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
    ...mapState(['serverConfig', 'userRoles', 'listRoles']),

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

    listRoleOptions() {
      return [{ id: null, name: `— ${this.$t('globals.terms.none')} —` }, ...this.listRoles];
    },

    isURLOk() {
      try {
        const u = new URL(this.serverConfig.root_url);
        return u.hostname !== 'localhost' && u.hostname !== '127.0.0.1';
      } catch (e) {
        return false;
      }
    },
  },

  mounted() {
    this.$api.getUserRoles();
    this.$api.getListRoles();
  },

  methods: {
    setProvider(provider) {
      this.data['security.oidc'].provider_url = OIDC_PROVIDERS[provider];
      this.data['security.oidc'].provider_name = provider.charAt(0).toUpperCase() + provider.slice(1);

      this.$nextTick(() => {
        this.$refs.client_id?.focus?.();
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

.section-head,
.toggle-field,
.redirect-row {
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

.quick-links {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}

.quick-links.disabled {
  opacity: 0.6;
}
</style>
