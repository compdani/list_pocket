<template>
  <auth-card-layout
    title="Two-factor authentication"
    description="Enter the 6-digit code from your authenticator app to finish signing in."
  >
    <v-alert v-if="error" type="error" variant="tonal" class="mb-4">{{ error }}</v-alert>

    <v-form @submit.prevent="submit">
      <v-text-field
        v-model="totpCode"
        label="Authentication code"
        autocomplete="one-time-code"
        variant="outlined"
        density="comfortable"
        maxlength="6"
        class="mb-2"
        autofocus
      />
      <v-btn block color="primary" size="large" type="submit" :loading="loading">
        Continue
      </v-btn>
    </v-form>

    <div class="auth-actions">
      <router-link :to="{ name: 'login', query: { next } }">Back to sign in</router-link>
    </div>
  </auth-card-layout>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import AuthCardLayout from '../components/AuthCardLayout.vue';
import { verifyTwoFA } from '../api';
import { getRouteNext, toAdminPath } from '../utils/authRoutes';

const route = useRoute();
const router = useRouter();

const totpCode = ref('');
const loading = ref(false);
const error = ref('');

const next = computed(() => getRouteNext(route));
const challengeToken = computed(() => (
  typeof route.query.token === 'string' && route.query.token ? route.query.token : ''
));

function getErrorMessage(err) {
  return err?.response?.message || err?.message || 'Verification failed';
}

onMounted(() => {
  if (!challengeToken.value) {
    router.replace({ name: 'login', query: { next: next.value } });
  }
});

async function submit() {
  error.value = '';
  loading.value = true;

  try {
    const result = await verifyTwoFA({
      token: challengeToken.value,
      totpCode: totpCode.value,
      next: next.value,
    });
    window.location.assign(toAdminPath(result.next || next.value));
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
