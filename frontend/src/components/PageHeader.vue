<template>
  <header class="page-header">
    <div class="page-header-main">
      <h1 class="text-h5 font-weight-semibold mb-0">
        <slot name="title">{{ title }}</slot>
        <span
          v-if="count != null && !Number.isNaN(Number(count))"
          class="text-medium-emphasis"
        >
          (<slot name="count">{{ count }}</slot>)
        </span>
      </h1>
      <div v-if="$slots.subtitle" class="text-caption mt-2">
        <slot name="subtitle" />
      </div>
    </div>
    <div class="page-header-actions d-flex flex-wrap ga-2">
      <slot name="actions" />
    </div>
  </header>
</template>

<script setup>
defineProps({
  title: { type: String, default: '' },
  count: { type: [Number, String], default: null },
});
</script>

<style scoped>
.page-header {
  align-items: flex-start;
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  justify-content: space-between;
  margin-bottom: 16px;
}

.page-header-main {
  min-width: 0;
  flex: 1 1 auto;
}

.page-header-actions {
  align-items: center;
  justify-content: flex-end;
}
</style>
