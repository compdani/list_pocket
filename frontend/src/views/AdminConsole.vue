<template>
  <section class="admin-console">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h4 font-weight-bold">
          Admin API Console
        </h1>
        <p class="text-medium-emphasis mt-1">
          Send an arbitrary authenticated request to the backend. Restricted to
          super admins. Use for one-off maintenance, debugging and internal
          endpoints.
        </p>
      </div>
    </header>

    <v-alert
      v-if="!canUse"
      type="error"
      variant="tonal"
      class="mb-4"
      title="Not authorized"
      text="This console is only available to super admin users."
    />

    <v-card v-else variant="outlined" class="console-card">
      <v-card-text>
        <v-form @submit.prevent="send">
          <div class="request-row">
            <v-select
              v-model="form.method"
              :items="methods"
              label="Method"
              density="comfortable"
              variant="outlined"
              hide-details
              class="method-field"
            />
            <v-text-field
              v-model="form.url"
              label="URL"
              placeholder="/api/campaigns/123/ledger/resolve-inflight"
              density="comfortable"
              variant="outlined"
              hide-details
              class="url-field"
              autocomplete="off"
              spellcheck="false"
            />
            <v-btn
              type="submit"
              color="primary"
              :loading="loading"
              :disabled="!isValidRequest"
              prepend-icon="mdi-send"
            >
              Send
            </v-btn>
          </div>

          <v-textarea
            v-model="form.body"
            label="Body (JSON, optional)"
            :placeholder="bodyPlaceholder"
            rows="10"
            auto-grow
            variant="outlined"
            density="comfortable"
            spellcheck="false"
            class="mt-4 body-field"
            :error-messages="bodyError ? [bodyError] : []"
            hint='Example: {"action":"mark_sent"}'
            persistent-hint
          />

          <div class="helpers mt-2">
            <v-btn
              size="small"
              variant="text"
              prepend-icon="mdi-code-json"
              :disabled="!form.body"
              @click="prettyPrintBody"
            >
              Format JSON
            </v-btn>
            <v-btn
              size="small"
              variant="text"
              prepend-icon="mdi-close"
              :disabled="!form.body && !form.url"
              @click="clearForm"
            >
              Clear
            </v-btn>
          </div>
        </v-form>
      </v-card-text>
    </v-card>

    <v-card
      v-if="canUse && lastResponse"
      variant="outlined"
      class="console-card mt-4 response-card"
    >
      <v-card-title class="response-title">
        <v-icon
          :color="lastResponse.ok ? 'success' : 'error'"
          class="mr-2"
        >
          {{ lastResponse.ok ? 'mdi-check-circle' : 'mdi-alert-circle' }}
        </v-icon>
        <span>
          {{ lastResponse.ok ? 'Success' : 'Error' }}
        </span>
        <span class="response-meta ml-2">
          <v-chip v-if="lastResponse.status" size="x-small" variant="tonal" class="mr-1">
            HTTP {{ lastResponse.status }}
          </v-chip>
          <v-chip size="x-small" variant="tonal">
            {{ lastResponse.durationMs }} ms
          </v-chip>
        </span>
        <v-spacer />
        <v-btn
          size="small"
          variant="text"
          prepend-icon="mdi-content-copy"
          @click="copyResponse"
        >
          Copy
        </v-btn>
      </v-card-title>
      <v-divider />
      <v-card-text class="pa-0">
        <div v-if="!lastResponse.ok && lastResponse.error" class="error-line">
          {{ lastResponse.error }}
        </div>
        <pre class="response-body">{{ responseText }}</pre>
      </v-card-text>
    </v-card>
  </section>
</template>

<script>
export default {
  data() {
    return {
      methods: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'],
      form: {
        method: 'POST',
        url: '',
        body: '',
      },
      bodyError: '',
      loading: false,
      lastResponse: null,
    };
  },

  computed: {
    canUse() {
      return Boolean(this.$isSuperAdmin && this.$isSuperAdmin());
    },

    bodyPlaceholder() {
      return '{\n  "key": "value"\n}';
    },

    isValidRequest() {
      return Boolean(this.form.method && this.form.url && this.form.url.trim());
    },

    responseText() {
      if (!this.lastResponse) {
        return '';
      }
      const payload = this.lastResponse.data;
      if (payload === null || payload === undefined) {
        return '';
      }
      if (typeof payload === 'string') {
        return payload;
      }
      try {
        return JSON.stringify(payload, null, 2);
      } catch (_e) {
        return String(payload);
      }
    },
  },

  methods: {
    parseBody() {
      const raw = (this.form.body || '').trim();
      if (!raw) {
        return { ok: true, value: undefined };
      }
      try {
        return { ok: true, value: JSON.parse(raw) };
      } catch (err) {
        return { ok: false, error: `Invalid JSON: ${err.message}` };
      }
    },

    prettyPrintBody() {
      const parsed = this.parseBody();
      if (!parsed.ok) {
        this.bodyError = parsed.error;
        return;
      }
      this.bodyError = '';
      if (parsed.value === undefined) {
        return;
      }
      this.form.body = JSON.stringify(parsed.value, null, 2);
    },

    clearForm() {
      this.form.url = '';
      this.form.body = '';
      this.bodyError = '';
      this.lastResponse = null;
    },

    async send() {
      if (!this.canUse) {
        return;
      }
      const parsed = this.parseBody();
      if (!parsed.ok) {
        this.bodyError = parsed.error;
        return;
      }
      this.bodyError = '';
      this.loading = true;
      try {
        const res = await this.$api.sendRawRequest({
          method: this.form.method,
          url: this.form.url.trim(),
          body: parsed.value,
        });
        this.lastResponse = res;
        if (res.ok) {
          this.$utils.toast(`${this.form.method} ${this.form.url} — ${res.durationMs}ms`);
        } else {
          this.$utils.toast(
            res.error || `Request failed (HTTP ${res.status || 'unknown'})`,
            'is-danger',
            4000,
            false,
          );
        }
      } finally {
        this.loading = false;
      }
    },

    async copyResponse() {
      if (!this.lastResponse) {
        return;
      }
      try {
        await navigator.clipboard.writeText(this.responseText);
        this.$utils.toast('Response copied to clipboard');
      } catch (_e) {
        this.$utils.toast('Could not copy to clipboard', 'is-danger');
      }
    },
  },
};
</script>

<style scoped>
.admin-console {
  position: relative;
}

.page-header {
  margin-bottom: 24px;
}

.console-card {
  padding: 4px;
}

.request-row {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}

.method-field {
  flex: 0 0 140px;
}

.url-field {
  flex: 1 1 auto;
}

.body-field :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  line-height: 1.5;
}

.helpers {
  display: flex;
  gap: 8px;
}

.response-title {
  display: flex;
  align-items: center;
}

.response-meta {
  display: inline-flex;
  gap: 4px;
}

.response-body {
  margin: 0;
  padding: 16px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 60vh;
  overflow: auto;
}

.error-line {
  padding: 12px 16px;
  color: rgb(var(--v-theme-error));
  font-weight: 500;
  border-bottom: 1px solid rgba(var(--v-border-color), 0.12);
  background: rgba(var(--v-theme-error), 0.06);
}
</style>
