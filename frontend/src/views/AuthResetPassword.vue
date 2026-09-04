<template>
  <auth-card-layout
    :title="$t('users.resetPassword')"
    :description="$t('auth.resetHelp')"
  >
    <v-alert v-if="error" type="error" variant="tonal" class="mb-4">{{ error }}</v-alert>

    <template v-if="hasResetLink">
      <v-form ref="formRef" @submit.prevent="submit">
        <password-field
          v-model="password"
          :label="$t('users.newPassword')"
          autocomplete="new-password"
          field-class="mb-3"
          :rules="requiredRule"
        />
        <password-field
          v-model="password2"
          :label="$t('users.passwordRepeat')"
          autocomplete="new-password"
          field-class="mb-2"
          :rules="matchRules"
        />
        <v-btn block color="primary" size="large" type="submit" :loading="loading">
          {{ $t('users.resetPassword') }}
        </v-btn>
      </v-form>
    </template>

    <template v-else>
      <p class="mb-4">{{ $t('auth.resetLinkMissing') }}</p>
    </template>

    <div class="auth-actions">
      <router-link :to="{ name: 'login' }">{{ $t('auth.backToSignIn') }}</router-link>
    </div>
  </auth-card-layout>
</template>

<script setup>
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute } from 'vue-router';
import AuthCardLayout from '../components/AuthCardLayout.vue';
import PasswordField from '../components/PasswordField.vue';
import { resetPassword } from '../api';
import { toAdminPath } from '../utils/authRoutes';

const { t } = useI18n();
const route = useRoute();

const formRef = ref(null);
const password = ref('');
const password2 = ref('');
const loading = ref(false);
const error = ref('');

const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''));
const email = computed(() => (typeof route.query.email === 'string' ? route.query.email : ''));
const hasResetLink = computed(() => Boolean(token.value && email.value));
const requiredRule = computed(() => [
  (v) => Boolean(v && String(v).trim()) || t('auth.requiredField'),
]);
const matchRules = computed(() => [
  (v) => Boolean(v && String(v).trim()) || t('auth.requiredField'),
  (v) => v === password.value || t('users.passwordMismatch'),
]);

function getErrorMessage(err) {
  return err?.response?.message || err?.message || t('auth.resetFailed');
}

async function submit() {
  error.value = '';
  const result = await formRef.value?.validate?.();
  if (result && result.valid === false) {
    return;
  }

  loading.value = true;

  try {
    const resetResult = await resetPassword({
      token: token.value,
      email: email.value,
      password: password.value,
      password2: password2.value,
    });
    window.location.assign(toAdminPath(resetResult.next || '/'));
  } catch (err) {
    error.value = getErrorMessage(err);
  } finally {
    loading.value = false;
  }
}
</script>

<style scoped>
.auth-actions {
  margin-top: 16px;
  text-align: right;
}
</style>
