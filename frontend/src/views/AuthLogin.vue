<template>
  <auth-card-layout
    :title="$t('users.login')"
    :description="$t('auth.signInHelp')"
  >
    <v-alert v-if="error" type="error" variant="tonal" class="mb-4">{{ error }}</v-alert>

    <v-form ref="formRef" @submit.prevent="submit">
      <v-text-field
        v-model="username"
        :label="$t('users.username')"
        autocomplete="username"
        variant="outlined"
        density="comfortable"
        class="mb-3"
        hide-details="auto"
        :rules="requiredRule"
        autofocus
      />
      <password-field
        v-model="password"
        :label="$t('users.password')"
        autocomplete="current-password"
        field-class="mb-2"
        :rules="requiredRule"
      />
      <v-btn block color="primary" size="large" type="submit" :loading="loading">
        {{ $t('users.login') }}
      </v-btn>
    </v-form>

    <div class="auth-actions">
      <router-link :to="{ name: 'forgotPassword', query: { next } }">{{ $t('users.forgotPassword') }}</router-link>
    </div>
  </auth-card-layout>
</template>

<script setup>
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';
import AuthCardLayout from '../components/AuthCardLayout.vue';
import PasswordField from '../components/PasswordField.vue';
import { login } from '../api';
import { getRouteNext, toAdminPath } from '../utils/authRoutes';

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

const formRef = ref(null);
const username = ref('');
const password = ref('');
const loading = ref(false);
const error = ref('');

const next = computed(() => getRouteNext(route));
const requiredRule = computed(() => [
  (v) => Boolean(v && String(v).trim()) || t('auth.requiredField'),
]);

function getErrorMessage(err) {
  return err?.response?.message || err?.message || t('auth.signInFailed');
}

async function submit() {
  error.value = '';
  const result = await formRef.value?.validate?.();
  if (result && result.valid === false) {
    return;
  }

  loading.value = true;

  try {
    const loginResult = await login({
      username: username.value,
      password: password.value,
      next: next.value,
    });

    if (loginResult.status === 'twofa_required') {
      router.push({
        name: 'loginTwofa',
        query: {
          token: loginResult.token,
          next: loginResult.next || next.value,
        },
      });
      return;
    }

    window.location.assign(toAdminPath(loginResult.next || next.value));
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
