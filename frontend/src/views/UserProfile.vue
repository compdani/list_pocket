<template>
  <section class="user-profile">
    <div v-if="loading.users" class="section-loading">
      <v-progress-circular indeterminate color="primary" size="48" />
    </div>

    <header class="profile-header">
      <h1 class="profile-title">
        @{{ data.username }}
      </h1>
      <v-chip v-if="data.userRole" size="small" variant="tonal" color="primary">
        {{ data.userRole.name }}
      </v-chip>
    </header>

    <v-form class="profile-form" @submit.prevent="onSubmit">
      <v-text-field
        v-if="data.type !== 'api'"
        v-model="form.email"
        :label="$t('subscribers.email')"
        :placeholder="$t('subscribers.email')"
        :disabled="!data.passwordLogin"
        maxlength="200"
        name="email"
        type="email"
        required
        autofocus
        variant="outlined"
        density="comfortable"
        class="mb-2"
      />

      <v-text-field
        v-model="form.name"
        :label="$t('globals.fields.name')"
        :placeholder="$t('globals.fields.name')"
        maxlength="200"
        name="name"
        variant="outlined"
        density="comfortable"
        class="mb-2"
      />

      <v-row v-if="data.passwordLogin">
        <v-col cols="12" md="6">
          <password-field
            v-model="form.password"
            :label="$t('users.password')"
            autocomplete="new-password"
          />
        </v-col>
        <v-col cols="12" md="6">
          <password-field
            v-model="form.password2"
            :label="$t('users.passwordRepeat')"
            autocomplete="new-password"
          />
        </v-col>
      </v-row>

      <v-btn
        color="primary"
        variant="flat"
        type="submit"
        data-cy="btn-save"
        prepend-icon="mdi-content-save-outline"
        class="mt-2"
      >
        {{ $t('globals.buttons.save') }}
      </v-btn>
    </v-form>

    <section v-if="data.passwordLogin" class="twofa-section">
      <v-card v-if="data.twofaType === 'none'" variant="outlined" class="twofa-card">
        <div class="twofa-card-head">
          <h3 class="twofa-title">{{ $t('users.twoFA') }}</h3>
          <v-switch
            v-if="!isTotpVisible"
            :model-value="twofaEnabled"
            hide-details
            density="compact"
            color="primary"
            @update:model-value="onEnableSwitch"
          />
        </div>

        <p class="twofa-desc">{{ $t('users.twoFANotEnabled') }}</p>

        <div v-if="isTotpVisible" class="totp-setup">
          <div v-if="totpQR" class="qr-section">
            <p class="text-medium-emphasis">{{ $t('users.totpScanQR') }}</p>

            <img class="qr-image" :src="'data:image/png;base64,' + totpQR" alt="QR Code" />

            <p class="mt-4">
              <strong>{{ $t('users.totpSecret') }}</strong><br />
              <code><copy-text :text="`${totpSecret}`" /></code>
            </p>

            <v-form class="mt-4" @submit.prevent="confirmTOTP">
              <v-text-field
                ref="totpCodeInput"
                v-model="totpCode"
                :label="$t('users.totpCode')"
                maxlength="6"
                pattern="[0-9]{6}"
                placeholder="000000"
                required
                variant="outlined"
                density="comfortable"
                class="mb-3"
              />
              <div class="d-flex ga-3">
                <v-btn color="primary" variant="flat" type="submit">
                  {{ $t('globals.buttons.enable') }}
                </v-btn>
                <v-btn type="button" variant="outlined" @click="onCancelTOTPSetup">
                  {{ $t('globals.buttons.cancel') }}
                </v-btn>
              </div>
            </v-form>
          </div>
        </div>
      </v-card>

      <v-card v-if="data.twofaType === 'totp'" variant="outlined" class="twofa-card">
        <div class="twofa-card-head">
          <h3 class="twofa-title">
            <v-icon icon="mdi-check-circle-outline" color="success" class="mr-1" />
            {{ $t('users.twoFAEnabled') }}
          </h3>
          <v-switch
            v-if="!showDisableTOTP"
            :model-value="twofaEnabled"
            hide-details
            density="compact"
            color="primary"
            @update:model-value="onDisableSwitch"
          />
        </div>

        <p class="twofa-desc">{{ $t('users.twoFAEnabledDesc', { type: data.twofaType.toUpperCase() }) }}</p>

        <v-form v-if="showDisableTOTP" class="mt-4" @submit.prevent="confirmDisableTOTP">
          <password-field
            v-model="disableTOTPPassword"
            :label="$t('users.password')"
            autocomplete="current-password"
            field-class="mb-3"
            :rules="[(v) => Boolean(v) || $t('auth.requiredField')]"
          />
          <div class="d-flex ga-3">
            <v-btn color="error" variant="flat" type="submit">
              {{ $t('globals.buttons.disable') }}
            </v-btn>
            <v-btn type="button" variant="outlined" @click="onCancelTOTPSetup">
              {{ $t('globals.buttons.cancel') }}
            </v-btn>
          </div>
        </v-form>
      </v-card>
    </section>
  </section>
</template>

<script>
import { mapState } from 'vuex';
import CopyText from '../components/CopyText.vue';
import PasswordField from '../components/PasswordField.vue';

export default {
  name: 'UserProfile',

  components: {
    CopyText,
    PasswordField,
  },

  data() {
    return {
      form: {},
      data: {},
      isTotpVisible: false,
      totpQR: null,
      totpSecret: null,
      totpCode: '',
      showDisableTOTP: false,
      disableTOTPPassword: '',
      twofaEnabled: false,
    };
  },

  methods: {
    onSubmit() {
      const params = {
        name: this.form.name,
        email: this.form.email,
      };

      if (this.data.passwordLogin && this.form.password) {
        if (this.form.password !== this.form.password2) {
          this.$utils.toast(this.$t('users.passwordMismatch'), 'is-danger');
          return;
        }

        params.password = this.form.password;
        params.password2 = this.form.password2;
      }

      this.$api.updateUserProfile(params).then(() => {
        this.form.password = '';
        this.form.password2 = '';
        this.$utils.toast(this.$t('globals.messages.updated', { name: this.data.username }));
      });
    },

    onEnableSwitch(enabled) {
      this.twofaEnabled = enabled;
      if (enabled) {
        this.onToggleEnableTotp();
      } else {
        this.onCancelTOTPSetup();
      }
    },

    onDisableSwitch(enabled) {
      this.twofaEnabled = enabled;
      if (!enabled) {
        this.toggleDisableTOTP();
      }
    },

    onToggleEnableTotp() {
      this.$api.getTOTPQR(this.data.id).then((data) => {
        this.totpQR = data.qr;
        this.totpSecret = data.secret;
        this.isTotpVisible = true;

        this.$nextTick(() => {
          if (this.$refs.totpCodeInput) {
            this.$refs.totpCodeInput.focus();
          }
        });
      }).catch(() => {
        this.twofaEnabled = false;
        this.$utils.toast(this.$t('globals.messages.errorFetching'), 'is-danger');
      });
    },

    onCancelTOTPSetup() {
      this.isTotpVisible = false;
      this.totpQR = null;
      this.totpSecret = null;
      this.totpCode = '';
      this.twofaEnabled = this.data.twofaType === 'totp';
      this.showDisableTOTP = false;
      this.disableTOTPPassword = '';
    },

    confirmTOTP() {
      if (!this.totpCode || this.totpCode.length !== 6) {
        this.$utils.toast(this.$t('globals.messages.invalidValue'), 'is-danger');
        return;
      }

      const d = new FormData();
      d.append('secret', this.totpSecret);
      d.append('code', this.totpCode);

      this.$api.enableTOTP(this.data.id, d).then(() => {
        this.$utils.toast(this.$t('users.twoFAEnabled'));
        this.onCancelTOTPSetup();

        this.$api.refreshUserProfile().then((data) => {
          this.data = { ...data };
          this.twofaEnabled = data.twofaType === 'totp';
        });
      }).catch(() => {
        this.$utils.toast(this.$t('globals.messages.invalidValue'), 'is-danger');
      });
    },

    toggleDisableTOTP() {
      this.showDisableTOTP = true;

      this.$nextTick(() => {
        if (this.$refs.disablePasswordInput) {
          this.$refs.disablePasswordInput.focus();
        }
      });
    },

    confirmDisableTOTP() {
      if (!this.disableTOTPPassword) {
        this.$utils.toast(this.$t('globals.messages.invalidFields'), 'is-danger');
        return;
      }

      const formData = new FormData();
      formData.append('password', this.disableTOTPPassword);

      this.$api.disableTOTP(this.data.id, formData).then(() => {
        this.$utils.toast(this.$t('globals.messages.done'));
        this.showDisableTOTP = false;
        this.disableTOTPPassword = '';
        this.$api.refreshUserProfile().then((data) => {
          this.data = { ...data };
          this.twofaEnabled = data.twofaType === 'totp';
        });
      }).catch(() => {
        this.$utils.toast(this.$t('users.invalidPassword'), 'is-danger');
      });
    },
  },

  mounted() {
    this.$api.refreshUserProfile().then((data) => {
      this.data = { ...data };
      this.form = { name: data.name, email: data.email };
      this.twofaEnabled = data.twofaType === 'totp';
    });
  },

  computed: {
    ...mapState(['loading']),
  },
};
</script>

<style scoped>
.user-profile {
  max-width: 720px;
  position: relative;
}

.section-loading {
  align-items: center;
  background: rgba(255, 255, 255, 0.72);
  display: flex;
  inset: 0;
  justify-content: center;
  position: absolute;
  z-index: 2;
}

.profile-header {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 28px;
}

.profile-title {
  font-size: 1.75rem;
  font-weight: 700;
  line-height: 1.2;
  margin: 0;
}

.profile-form {
  margin-bottom: 32px;
}

.twofa-section {
  margin-top: 8px;
}

.twofa-card {
  padding: 20px;
}

.twofa-card-head {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
  margin-bottom: 12px;
}

.twofa-title {
  align-items: center;
  display: flex;
  font-size: 1.1rem;
  font-weight: 600;
  margin: 0;
}

.twofa-desc {
  color: #64748b;
  margin: 0;
}

.qr-image {
  display: block;
  margin-top: 12px;
  max-width: 200px;
}
</style>
