<template>
  <div class="grapes-editor-wrapper">
    <div :id="containerId" class="grapes-editor-root" />

    <v-dialog v-model="isMediaVisible" max-width="900">
      <v-card>
        <v-card-text class="pt-0">
          <media
            is-modal
            type="pictures"
            @selected="onMediaSelect"
            @close="isMediaVisible = false"
          />
        </v-card-text>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup>
import { ref, onBeforeUnmount, onMounted } from 'vue';
import 'grapesjs/dist/css/grapes.min.css';
import grapesJS from 'grapesjs';
import grapesJSMJML from 'grapesjs-mjml';
import Media from '../views/Media.vue';

const BASIC_MJML_ROOT = '<mjml><mj-body><mj-section><mj-column><mj-text></mj-text></mj-column></mj-section></mj-body></mjml>';

const props = defineProps({
  source: { type: String, default: '' },
  data: { type: String, default: '' },
  height: { type: String, default: '65vh' },
});

const emit = defineEmits(['ready']);

let editor = null;
const containerId = `gjs-${Math.random().toString(36).slice(2, 10)}`;
const isMediaVisible = ref(false);
let assetSelectHandler = null;

function runCommandSafe(command, fallbackValue) {
  if (!editor) {
    return fallbackValue;
  }
  try {
    return editor.runCommand(command);
  } catch {
    return fallbackValue;
  }
}

function loadMjml(newMjml) {
  if (!editor) {
    return;
  }
  const next = String(newMjml || '').trim() || BASIC_MJML_ROOT;
  editor.setComponents(next);
}

function getMjml() {
  return runCommandSafe('mjml-code', '') || '';
}

function getHTML() {
  const htmlWithCss = runCommandSafe('mjml-code-to-html', {});
  return htmlWithCss?.html || '';
}

function getCompiledContent() {
  return {
    source: getMjml(),
    body: getHTML(),
  };
}

function onMediaSelect(media) {
  if (assetSelectHandler && media?.url) {
    assetSelectHandler({ src: media.url }, true);
  }
  isMediaVisible.value = false;
  assetSelectHandler = null;
}

onMounted(() => {
  editor = grapesJS.init({
    container: `#${containerId}`,
    fromElement: false,
    height: props.height,
    width: 'auto',
    plugins: [grapesJSMJML],
    storageManager: false,
    assetManager: {
      custom: true,
    },
  });

  editor.on('asset:custom', (propsAM = {}) => {
    if (!propsAM.open) {
      isMediaVisible.value = false;
      assetSelectHandler = null;
      return;
    }

    assetSelectHandler = propsAM.select || null;
    isMediaVisible.value = true;
  });

  const initial = (props.source || props.data || '').trim() || BASIC_MJML_ROOT;
  editor.setComponents(initial);

  emit('ready', true);
});

onBeforeUnmount(() => {
  if (editor) {
    editor.destroy();
    editor = null;
  }
});

defineExpose({ loadMjml, getMjml, getHTML, getCompiledContent });
</script>

<style scoped>
.grapes-editor-wrapper {
  width: 100%;
}

.grapes-editor-root {
  min-height: 420px;
}
</style>
