<template>
  <section class="logs-page">
    <div class="logs-header">
      <h1 class="text-h4 font-weight-bold">
        {{ $t('logs.title') }}
      </h1>
    </div>

    <v-card variant="outlined" class="logs-shell">
      <v-card-text class="pa-0">
        <log-view :loading="loading.logs" :lines="lines" />
      </v-card-text>
    </v-card>
  </section>
</template>

<script>
import { mapState } from 'vuex';
import LogView from '../components/LogView.vue';

export default {
  components: {
    LogView,
  },

  data() {
    return {
      lines: [],
      pollId: null,
    };
  },

  methods: {
    getLogs() {
      this.$api.getLogs().then((data) => {
        this.lines = data;
      });
    },
  },

  computed: {
    ...mapState(['loading']),
  },

  mounted() {
    this.getLogs();

    // Update the logs every 10 seconds.
    this.pollId = setInterval(() => this.getLogs(), 10000);
  },

  beforeUnmount() {
    clearInterval(this.pollId);
  },
};
</script>

<style scoped>
.logs-page {
  position: relative;
}

.logs-header {
  margin-bottom: 24px;
}

.logs-shell :deep(.log-view .lines) {
  border: 0;
  height: min(70vh, 680px);
}
</style>
