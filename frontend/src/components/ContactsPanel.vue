<script setup>
/* eslint-disable */
defineProps({
  contacts: { type: Array, default: () => [] },
});
</script>

<template>
  <v-card class="panel">
    <v-card-title class="panel-header">
      <div>
        <h2>Contacts</h2>
        <p>CRM records available to workflows and manual review steps.</p>
      </div>
    </v-card-title>
    <v-list class="contact-list">
      <v-list-item
        v-for="contact in contacts"
        :key="contact.id"
        class="contact-card"
      >
        <template #prepend>
          <div>
            <strong>{{ contact.fullName }}</strong>
            <span>{{ contact.company }}</span>
          </div>
        </template>
        <template #append>
          <div class="contact-meta">
            <span>{{ contact.stage }}</span>
            <span>{{ contact.email }}</span>
            <span>{{ contact.lastActivity }}</span>
          </div>
        </template>
      </v-list-item>
    </v-list>
  </v-card>
</template>
