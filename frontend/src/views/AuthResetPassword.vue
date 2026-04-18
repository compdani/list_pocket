<template>
  <auth-card-layout
    title="Reset password"
    description="Choose a new password to regain access to the admin."
  >
    <v-alert v-if="error" type="error" variant="tonal" class="mb-4">{{ error }}</v-alert>

    <template v-if="hasResetLink">
      <v-form @submit.prevent="submit">
        <v-text-field
          v-model="password"
          label="New password"
          type="password"
          autocomplete="new-password"
          variant="outlined"
          density="comfortable"
          class="mb-3"
          autofocus
        />
        <v-text-field
          v-model="password2"
          label="Repeat password"
          type="password"
          autocomplete="new-password"
          variant="outlined"
          density="comfortable"
          class="mb-2"
        />
        <v-btn block color="primary" size="large" type="submit" :loading="loading">
          Reset password
        </v-btn>
      </v-form>
    </template>

    <template v-else>
      <p class="mb-4">This reset link is missing required information.</p>
    </template>

    <div class="auth-actions">
      <router-link :to="{ name: 'login' }">Back to sign in</router-link>
    </div>
  </auth-card-layout>
</template>

<script setup>
import { computed, ref } from 'vue';
import { useRoute } from 'vue-router';
import AuthCardLayout from '../components/AuthCardLayout.vue';
import { resetPassword } from '../api';
import { toAdminPath } from '../utils/authRoutes';

const route = useRoute();

const password = ref('');
const password2 = ref('');
const loading = ref(false);
const error = ref('');

const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''));
const email = computed(() => (typeof route.query.email === 'string' ? route.query.email : ''));
const hasResetLink = computed(() => Boolean(token.value && email.value));

function getErrorMessage(err) {
  return err?.response?.message || err?.message || 'Unable to reset password';
}

async function submit() {
  error.value = '';
  loading.value = true;

  try {
    const result = await resetPassword({
      token: token.value,
      email: email.value,
      password: password.value,
      password2: password2.value,
    });
    window.location.assign(toAdminPath(result.next || '/'));
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
