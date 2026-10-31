<template>
  <section class="plugins-page">
    <page-header :title="$t('menu.plugins')">
      <template #actions>
        <v-btn
          variant="tonal"
          prepend-icon="mdi-creation-outline"
          class="me-2"
          data-cy="btn-build-plugin"
          @click="buildWithAssistant"
        >
          {{ $t('plugins.buildWithAssistant') }}
        </v-btn>
        <v-btn
          color="primary"
          prepend-icon="mdi-plus"
          :loading="isUploading"
          data-cy="btn-install-plugin"
          @click="$refs.upload.click()"
        >
          {{ $t('plugins.install') }}
        </v-btn>
        <input
          ref="upload"
          type="file"
          accept=".zip,application/zip"
          hidden
          data-cy="plugin-upload"
          @change="onFile"
        >
      </template>
    </page-header>

    <v-alert type="warning" variant="tonal" density="comfortable" class="mb-4">
      {{ $t('plugins.trustWarning') }}
    </v-alert>

    <v-alert
      v-for="failure in failures"
      :key="failure.plugin"
      type="error"
      variant="tonal"
      density="comfortable"
      class="mb-4"
      :title="$t('plugins.componentFailed', { name: failure.plugin })"
      :text="failure.message"
    />

    <v-progress-linear v-if="isLoading" indeterminate color="primary" class="mb-4" />

    <empty-placeholder v-if="!isLoading && plugins.length === 0" />

    <v-card v-for="item in plugins" :key="item.manifest.id" class="mb-4" :data-cy="`plugin-${item.manifest.id}`">
      <v-card-title class="d-flex align-center flex-wrap ga-2">
        <span>{{ item.manifest.name }}</span>
        <span class="text-caption text-medium-emphasis">{{ item.version }}</span>
        <v-chip size="small" :color="statusColor(item.status)" data-cy="plugin-status">
          {{ $t(`plugins.status.${item.status}`) }}
        </v-chip>
      </v-card-title>
      <v-card-text>
        <v-alert
          v-if="item.last_error"
          type="error"
          variant="tonal"
          density="comfortable"
          class="mb-3"
          :text="item.last_error"
        />
        <div class="d-flex flex-wrap ga-2 mb-3">
          <v-chip v-for="c in item.manifest.channels || []" :key="c.id" size="small" variant="outlined">
            {{ $t('plugins.channel') }}: {{ c.label }}
          </v-chip>
          <v-chip v-for="w in item.manifest.webhooks || []" :key="w.path" size="small" variant="outlined">
            {{ webhookUrl(item.manifest.id, w.path) }}
          </v-chip>
          <v-chip
            v-for="addon in addons(item.manifest)"
            :key="addon.key"
            :title="addon.description"
            size="small"
            variant="outlined"
            data-cy="plugin-addon"
          >
            {{ $t(addon.kind) }}: {{ addon.name }}
          </v-chip>
        </div>
        <div class="d-flex flex-wrap ga-2">
          <v-btn
            v-if="item.enabled"
            variant="tonal"
            :disabled="isBusy"
            data-cy="btn-disable-plugin"
            @click="setEnabled(item, false)"
          >
            {{ $t('plugins.disable') }}
          </v-btn>
          <v-btn
            v-else
            variant="tonal"
            color="primary"
            :disabled="isBusy"
            data-cy="btn-enable-plugin"
            @click="setEnabled(item, true)"
          >
            {{ $t('plugins.enable') }}
          </v-btn>
          <v-btn
            variant="text"
            color="error"
            :disabled="isBusy || item.enabled"
            data-cy="btn-uninstall-plugin"
            @click="uninstall(item)"
          >
            {{ $t('plugins.uninstall') }}
          </v-btn>
        </div>
      </v-card-text>
    </v-card>
  </section>
</template>

<script>
import {
  disablePlugin, enablePlugin, getPlugins, installPlugin, uninstallPlugin,
} from '../api';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import PageHeader from '../components/PageHeader.vue';
import { pluginRegistry } from '../pluginHost';
import { pluginAddons } from '../agent/parts.mjs';
import { assistantContext } from '../assistant/context';

export default {
  components: { EmptyPlaceholder, PageHeader },

  data() {
    return {
      plugins: [],
      isLoading: true,
      isUploading: false,
      isBusy: false,
    };
  },

  computed: {
    failures() {
      return pluginRegistry.failures;
    },
  },

  mounted() {
    this.load();
  },

  methods: {
    apiError(err) {
      return (err.response && err.response.message) || err.message;
    },

    statusColor(status) {
      return {
        loaded: 'success', failed: 'error', pending_restart: 'warning', safe_mode: 'warning',
      }[status] || undefined;
    },

    addons: pluginAddons,

    buildWithAssistant() {
      assistantContext.open = true;
    },

    webhookUrl(id, path) {
      return `${window.location.origin}/webhooks/plugins/${id}/${path}`;
    },

    async load() {
      this.isLoading = true;
      try {
        this.plugins = await getPlugins();
      } catch (err) {
        this.$utils.toast(this.apiError(err), 'error');
      } finally {
        this.isLoading = false;
      }
    },

    async onFile(event) {
      const [file] = event.target.files;
      event.target.value = '';
      if (!file) {
        return;
      }
      this.isUploading = true;
      try {
        const data = await installPlugin(file);
        if (data && data.needs_restart !== undefined && !data.manifest) {
          await this.$root.awaitRestart({ needsRestart: data.needs_restart });
        }
        this.$utils.toast(this.$t('plugins.installed'));
        await this.load();
      } catch (err) {
        this.$utils.toast(this.apiError(err), 'error');
      } finally {
        this.isUploading = false;
      }
    },

    // Enabling and disabling restart the app, so reload to pick up its routes and nav.
    async setEnabled(item, enabled) {
      this.isBusy = true;
      try {
        const data = await (enabled ? enablePlugin : disablePlugin)(item.manifest.id);
        const result = await this.$root.awaitRestart({ needsRestart: data && data.needs_restart });
        if (result.needsRestart) {
          await this.load();
          return;
        }
        window.location.reload();
      } catch (err) {
        this.$utils.toast(this.apiError(err), 'error');
      } finally {
        this.isBusy = false;
      }
    },

    uninstall(item) {
      this.$utils.confirm(this.$t('plugins.uninstallConfirm', { name: item.manifest.name }), async () => {
        this.isBusy = true;
        try {
          await uninstallPlugin(item.manifest.id);
          await this.load();
        } catch (err) {
          this.$utils.toast(this.apiError(err), 'error');
        } finally {
          this.isBusy = false;
        }
      });
    },
  },
};
</script>
