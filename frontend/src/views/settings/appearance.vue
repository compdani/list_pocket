<template>
  <div class="settings-section">
    <v-tabs
      v-model="tab"
      color="primary"
      density="comfortable"
      align-tabs="start"
      class="mb-4"
    >
      <v-tab value="admin">
        {{ $t('settings.appearance.adminName') }}
      </v-tab>
      <v-tab value="public">
        {{ $t('settings.appearance.publicName') }}
      </v-tab>
    </v-tabs>

    <v-window v-model="tab" :touch="false">
      <v-window-item value="admin">
        <section class="tab-panel">
          <p class="text-body-1 text-medium-emphasis mb-4">
            {{ $t('settings.appearance.adminHelp') }}
          </p>

          <div class="editor-field">
            <label class="text-subtitle-2 mb-2 d-block">{{ $t('settings.appearance.customCSS') }}</label>
            <code-editor v-model="data['appearance.admin.custom_css']" lang="css" name="body" key="editor-admin-css" />
          </div>

          <div class="editor-field">
            <label class="text-subtitle-2 mb-2 d-block">{{ $t('settings.appearance.customJS') }}</label>
            <code-editor
              v-model="data['appearance.admin.custom_js']"
              lang="javascript"
              name="body"
              key="editor-admin-js"
            />
          </div>
        </section>
      </v-window-item>

      <v-window-item value="public">
        <section class="tab-panel">
          <p class="text-body-1 text-medium-emphasis mb-4">
            {{ $t('settings.appearance.publicHelp') }}
          </p>

          <div class="editor-field">
            <label class="text-subtitle-2 mb-2 d-block">{{ $t('settings.appearance.customCSS') }}</label>
            <code-editor v-model="data['appearance.public.custom_css']" lang="css" name="body" key="editor-public-css" />
          </div>

          <div class="editor-field">
            <label class="text-subtitle-2 mb-2 d-block">{{ $t('settings.appearance.customJS') }}</label>
            <code-editor
              v-model="data['appearance.public.custom_js']"
              lang="javascript"
              name="body"
              key="editor-public-js"
            />
          </div>
        </section>
      </v-window-item>
    </v-window>
  </div>
</template>

<script>
import CodeEditor from '../../components/CodeEditor.vue';

export default {
  components: {
    'code-editor': CodeEditor,
  },

  props: {
    form: {
      type: Object, default: () => {},
    },
  },

  data() {
    return {
      data: this.form,
      tab: 'admin',
    };
  },

  mounted() {
    this.tab = this.$utils.getPref('settings.apperanceTab') || 'admin';
  },

  watch: {
    tab(t) {
      this.$utils.setPref('settings.apperanceTab', t);
    },
  },
};
</script>

<style scoped>
.settings-section,
.tab-panel {
  display: grid;
  gap: 24px;
}

.editor-field {
  display: grid;
  gap: 8px;
}
</style>
