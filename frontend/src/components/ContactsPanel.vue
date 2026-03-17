<script setup>
/* eslint-disable */
defineProps({
  contacts: { type: Array, default: () => [] },
});
</script>

<template>
  <section class="panel">
    <div class="panel-header">
      <h2>Contacts</h2>
      <p>CRM records available to workflows and manual review steps.</p>
    </div>
    <div class="contact-list">
      <article v-for="contact in contacts" :key="contact.id" class="contact-card">
        <div>
          <strong>{{ contact.fullName }}</strong>
          <span>{{ contact.company }}</span>
        </div>
        <div class="contact-meta">
          <span>{{ contact.stage }}</span>
          <span>{{ contact.email }}</span>
          <span>{{ contact.lastActivity }}</span>
        </div>
      </article>
    </div>
  </section>
</template>
