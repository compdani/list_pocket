<template>
  <div :id="containerId" class="grapes-editor-root" />
</template>

<script>
import 'grapesjs/dist/css/grapes.min.css';
import grapesJS from 'grapesjs';
import grapesJSMJML from 'grapesjs-mjml';

export default {
  name: 'GrapesMjmlEditor',
  props: {
    source: { type: String, default: '' },
    data: { type: String, default: '' },
    height: { type: String, default: '65vh' },
  },
  emits: ['change', 'ready'],
  data() {
    return {
      editor: null,
      containerId: `gjs-${Math.random().toString(36).slice(2, 10)}`,
    };
  },
  methods: {
    loadContent() {
      if (!this.editor) {
        return;
      }

      const source = (this.source || '').trim();
      const data = (this.data || '').trim();
      if (source) {
        this.editor.setComponents(source);
      } else if (data) {
        this.editor.setComponents(data);
      }
    },
    getMjml() {
      if (!this.editor) {
        return '';
      }
      return this.editor.runCommand('mjml-code') || '';
    },
    getHTML() {
      if (!this.editor) {
        return '';
      }
      const htmlWithCss = this.editor.runCommand('mjml-code-to-html');
      return htmlWithCss?.html || '';
    },
  },
  mounted() {
    this.editor = grapesJS.init({
      container: `#${this.containerId}`,
      fromElement: false,
      height: this.height,
      width: 'auto',
      plugins: [grapesJSMJML],
      storageManager: false,
    });

    this.loadContent();

    this.editor.on('update', () => {
      this.$emit('change', { body: this.getHTML(), source: this.getMjml() });
    });

    this.$emit('ready', true);
  },
  beforeUnmount() {
    if (this.editor) {
      this.editor.destroy();
      this.editor = null;
    }
  },
};
</script>

<style scoped>
.grapes-editor-root {
  min-height: 420px;
}
</style>
