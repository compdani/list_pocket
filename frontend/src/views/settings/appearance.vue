<template>
  <div class="items">
    <div class="settings-tabs">
      <button type="button" class="settings-tab" :class="{ 'is-active': tab === 0 }" @click="tab = 0">
        {{ $t('settings.appearance.adminName') }}
      </button>
      <button type="button" class="settings-tab" :class="{ 'is-active': tab === 1 }" @click="tab = 1">
        {{ $t('settings.appearance.publicName') }}
      </button>
    </div>

    <section v-show="tab === 0">
      <div class="block">
        {{ $t('settings.appearance.adminHelp') }}
      </div>

      <b-field :label="$t('settings.appearance.customCSS')" label-position="on-border">
        <code-editor lang="css" v-model="data['appearance.admin.custom_css']" name="body" key="editor-admin-css" />
      </b-field>

      <b-field :label="$t('settings.appearance.customJS')" label-position="on-border">
        <code-editor lang="javascript" v-model="data['appearance.admin.custom_js']" name="body"
          key="editor-admin-js" />
      </b-field>
    </section>

    <section v-show="tab === 1">
      <div class="block">
        {{ $t('settings.appearance.publicHelp') }}
      </div>

      <b-field :label="$t('settings.appearance.customCSS')" label-position="on-border">
        <code-editor lang="css" v-model="data['appearance.public.custom_css']" name="body" key="editor-public-css" />
      </b-field>

      <b-field :label="$t('settings.appearance.customJS')" label-position="on-border">
        <code-editor lang="javascript" v-model="data['appearance.public.custom_js']" name="body"
          key="editor-public-js" />
      </b-field>
    </section>
  </div>
</template>

<script>
import { mapState } from 'vuex';
import CodeEditor from '../../components/CodeEditor.vue';

export default {
  components: {
    'code-editor': CodeEditor,
  },

  props: {
    form: {
      type: Object, default: () => { },
    },
  },

  data() {
    return {
      data: this.form,
      tab: 0,
    };
  },

  mounted() {
    this.tab = this.$utils.getPref('settings.apperanceTab') || 0;
  },

  watch: {
    tab(t) {
      this.$utils.setPref('settings.apperanceTab', t);
    },
  },

  computed: {
    ...mapState(['settings']),
  },
};

</script>

<style scoped>
.settings-tabs {
  border-bottom: 1px solid #d8dfec;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 20px;
}

.settings-tab {
  background: #fff;
  border: 1px solid #d8dfec;
  border-bottom: 0;
  border-radius: 12px 12px 0 0;
  color: #667085;
  cursor: pointer;
  font-size: 0.95rem;
  padding: 10px 16px;
}

.settings-tab.is-active {
  background: #f8fbff;
  color: #0f5bd8;
  font-weight: 600;
}
</style>
