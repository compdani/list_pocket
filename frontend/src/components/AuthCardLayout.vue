<template>
  <section class="auth-layout">
    <div class="auth-backdrop" />
    <v-container class="fill-height d-flex align-center justify-center">
      <v-card class="auth-card" elevation="8">
        <div class="auth-brand">
          <img :src="logo" alt="listpocket" class="auth-logo" />
          <p class="auth-kicker">Admin</p>
          <h1>{{ title }}</h1>
          <p v-if="description" class="auth-description">{{ description }}</p>
        </div>

        <slot />
      </v-card>
    </v-container>
  </section>
</template>

<script setup>
import logo from '@/assets/logo.svg';

defineProps({
  title: {
    type: String,
    required: true,
  },
  description: {
    type: String,
    default: '',
  },
});
</script>

<style scoped>
.auth-layout {
  position: relative;
  min-height: 100dvh;
  overflow: hidden;
  background:
    radial-gradient(circle at top left, rgba(0, 85, 212, 0.18), transparent 28rem),
    linear-gradient(135deg, #f5f8ff 0%, #eef3fb 45%, #f7fafc 100%);
}

.auth-backdrop {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(120deg, rgba(255, 255, 255, 0.72), rgba(255, 255, 255, 0.35)),
    repeating-linear-gradient(
      -45deg,
      rgba(0, 85, 212, 0.03),
      rgba(0, 85, 212, 0.03) 18px,
      transparent 18px,
      transparent 36px
    );
}

.auth-card {
  position: relative;
  z-index: 1;
  width: min(100%, 460px);
  padding: 32px;
  border-radius: 24px;
}

.auth-brand {
  margin-bottom: 24px;
}

.auth-logo {
  height: 28px;
  margin-bottom: 18px;
}

.auth-kicker {
  margin: 0 0 6px;
  font-size: 0.78rem;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: #5f6b7a;
}

.auth-brand h1 {
  margin: 0;
  font-size: 1.8rem;
  line-height: 1.1;
}

.auth-description {
  margin: 10px 0 0;
  color: #586679;
}
</style>
