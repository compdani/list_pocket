<template>
  <auth-card-layout
    title="Sign in"
    description="Use your admin credentials to access the dashboard."
  >
    <v-alert v-if="error" type="error" variant="tonal" class="mb-4">{{ error }}</v-alert>

    <v-form @submit.prevent="submit">
      <v-text-field
        v-model="username"
        label="Username"
        autocomplete="username"
        variant="outlined"
        density="comfortable"
        class="mb-3"
        autofocus
      />
      <v-text-field
        v-model="password"
        label="Password"
        autocomplete="current-password"
        type="password"
        variant="outlined"
        density="comfortable"
        class="mb-2"
      />
      <v-btn block color="primary" size="large" type="submit" :loading="loading">
        Sign in
      </v-btn>
    </v-form>

    <div class="auth-actions">
      <router-link :to="{ name: 'forgotPassword', query: { next } }">Forgot your password?</router-link>
    </div>
  </auth-card-layout>
</template>

<script setup>
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import AuthCardLayout from '../components/AuthCardLayout.vue';
import { login } from '../api';
import { getRouteNext, toAdminPath } from '../utils/authRoutes';

const route = useRoute();
const router = useRouter();

const username = ref('');
const password = ref('');
const loading = ref(false);
const error = ref('');

const next = computed(() => getRouteNext(route));

function getErrorMessage(err) {
  return err?.response?.message || err?.message || 'Sign in failed';
}

async function submit() {
  error.value = '';
  loading.value = true;

  try {
    const result = await login({
      username: username.value,
      password: password.value,
      next: next.value,
    });

    if (result.status === 'twofa_required') {
      router.push({
        name: 'loginTwofa',
        query: {
          token: result.token,
          next: result.next || next.value,
        },
      });
      return;
    }

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
