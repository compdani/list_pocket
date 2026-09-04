<template>
  <auth-card-layout
    :title="$t('users.forgotPassword')"
    :description="$t('auth.forgotHelp')"
  >
    <v-alert v-if="success" type="success" variant="tonal" class="mb-4">{{ success }}</v-alert>
    <v-alert v-if="error" type="error" variant="tonal" class="mb-4">{{ error }}</v-alert>

    <v-form ref="formRef" @submit.prevent="submit">
      <v-text-field
        v-model="email"
        :label="$t('globals.fields.email')"
        type="email"
        autocomplete="email"
        variant="outlined"
        density="comfortable"
        class="mb-2"
        hide-details="auto"
        :rules="emailRules"
        autofocus
      />
      <v-btn block color="primary" size="large" type="submit" :loading="loading">
        {{ $t('auth.sendResetLink') }}
      </v-btn>
    </v-form>

    <div class="auth-actions">
      <router-link :to="{ name: 'login', query: { next } }">{{ $t('auth.backToSignIn') }}</router-link>
    </div>
  </auth-card-layout>
</template>

<script setup>
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute } from 'vue-router';
import AuthCardLayout from '../components/AuthCardLayout.vue';
import { requestPasswordReset } from '../api';
import { getRouteNext } from '../utils/authRoutes';

const { t } = useI18n();
const route = useRoute();

const formRef = ref(null);
const email = ref('');
const loading = ref(false);
const error = ref('');
const success = ref('');

const next = computed(() => getRouteNext(route));
const emailRules = computed(() => [
  (v) => Boolean(v && String(v).trim()) || t('auth.requiredField'),
  (v) => /.+@.+\..+/.test(String(v || '')) || t('globals.fields.email'),
]);

function getErrorMessage(err) {
  return err?.response?.message || err?.message || t('auth.resetEmailFailed');
}

async function submit() {
  error.value = '';
  success.value = '';
  const result = await formRef.value?.validate?.();
  if (result && result.valid === false) {
    return;
  }

  loading.value = true;

  try {
    const resetResult = await requestPasswordReset(email.value);
    success.value = resetResult.message || t('users.resetLinkSent');
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
