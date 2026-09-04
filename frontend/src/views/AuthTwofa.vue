<template>
  <auth-card-layout
    :title="$t('users.twoFA')"
    :description="$t('auth.twoFaHelp')"
  >
    <v-alert v-if="error" type="error" variant="tonal" class="mb-4">{{ error }}</v-alert>

    <v-form ref="formRef" @submit.prevent="submit">
      <v-text-field
        v-model="totpCode"
        :label="$t('users.totpCode')"
        autocomplete="one-time-code"
        variant="outlined"
        density="comfortable"
        maxlength="6"
        class="mb-2"
        hide-details="auto"
        :rules="requiredRule"
        autofocus
      />
      <v-btn block color="primary" size="large" type="submit" :loading="loading">
        {{ $t('globals.buttons.continue') }}
      </v-btn>
    </v-form>

    <div class="auth-actions">
      <router-link :to="{ name: 'login', query: { next } }">{{ $t('auth.backToSignIn') }}</router-link>
    </div>
  </auth-card-layout>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';
import AuthCardLayout from '../components/AuthCardLayout.vue';
import { verifyTwoFA } from '../api';
import { getRouteNext, toAdminPath } from '../utils/authRoutes';

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

const formRef = ref(null);
const totpCode = ref('');
const loading = ref(false);
const error = ref('');

const next = computed(() => getRouteNext(route));
const challengeToken = computed(() => (
  typeof route.query.token === 'string' && route.query.token ? route.query.token : ''
));
const requiredRule = computed(() => [
  (v) => Boolean(v && String(v).trim()) || t('auth.requiredField'),
]);

function getErrorMessage(err) {
  return err?.response?.message || err?.message || t('auth.verificationFailed');
}

onMounted(() => {
  if (!challengeToken.value) {
    router.replace({ name: 'login', query: { next: next.value } });
  }
});

async function submit() {
  error.value = '';
  const result = await formRef.value?.validate?.();
  if (result && result.valid === false) {
    return;
  }

  loading.value = true;

  try {
    const verifyResult = await verifyTwoFA({
      token: challengeToken.value,
      totpCode: totpCode.value,
      next: next.value,
    });
    window.location.assign(toAdminPath(verifyResult.next || next.value));
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
