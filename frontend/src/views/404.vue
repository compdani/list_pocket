<template>
  <section class="page-404">
    <v-card class="page-404-card" variant="outlined">
      <v-card-text class="text-center py-12">
        <h1 class="text-h4 font-weight-semibold mb-2">{{ $t('notFound.title') }}</h1>
        <p class="text-medium-emphasis mb-6">{{ $t('notFound.help') }}</p>
        <v-btn color="primary" variant="flat" :to="{ name: 'dashboard' }">
          {{ $t('notFound.home') }}
        </v-btn>
      </v-card-text>
    </v-card>
  </section>
</template>

<script>
export default {
  name: 'NotFound',
};
</script>

<style scoped>
.page-404 {
  display: flex;
  justify-content: center;
  padding: 48px 16px;
}

.page-404-card {
  max-width: 480px;
  width: 100%;
}
</style>
