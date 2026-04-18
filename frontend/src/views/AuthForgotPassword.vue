<template>
  <auth-card-layout
    title="Forgot password"
    description="Enter the email address tied to your admin account and we will send a reset link."
  >
    <v-alert v-if="success" type="success" variant="tonal" class="mb-4">{{ success }}</v-alert>
    <v-alert v-if="error" type="error" variant="tonal" class="mb-4">{{ error }}</v-alert>

    <v-form @submit.prevent="submit">
      <v-text-field
        v-model="email"
        label="Email"
        type="email"
        autocomplete="email"
        variant="outlined"
        density="comfortable"
        class="mb-2"
        autofocus
      />
      <v-btn block color="primary" size="large" type="submit" :loading="loading">
        Send reset link
      </v-btn>
    </v-form>

    <div class="auth-actions">
      <router-link :to="{ name: 'login', query: { next } }">Back to sign in</router-link>
    </div>
  </auth-card-layout>
</template>

<script setup>
import { computed, ref } from 'vue';
import { useRoute } from 'vue-router';
import AuthCardLayout from '../components/AuthCardLayout.vue';
import { requestPasswordReset } from '../api';
import { getRouteNext } from '../utils/authRoutes';

const route = useRoute();

const email = ref('');
const loading = ref(false);
const error = ref('');
const success = ref('');

const next = computed(() => getRouteNext(route));

function getErrorMessage(err) {
  return err?.response?.message || err?.message || 'Unable to send reset email';
}

async function submit() {
  error.value = '';
  success.value = '';
  loading.value = true;

  try {
    const result = await requestPasswordReset(email.value);
    success.value = result.message || 'If an account exists for that email, a reset link has been sent.';
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
