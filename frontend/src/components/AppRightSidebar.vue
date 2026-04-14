<template>
  <v-navigation-drawer
    :model-value="modelValue"
    :location="location"
    :width="currentWidth"
    :border="border"
    :color="color"
    :temporary="isCompact && temporaryOnMobile"
    :mobile="isCompact && temporaryOnMobile"
    :scrim="isCompact && temporaryOnMobile && scrimOnMobile"
    @update:model-value="onDrawerToggle"
  >
    <div
      v-if="showDragHandle && !isCompact"
      class="app-right-sidebar__resize-handle"
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize sidebar"
      @mousedown.prevent="onResizeStart"
    />
    <v-sheet flat class="app-right-sidebar d-flex flex-column h-100 bg-surface">
      <div v-if="$slots.header || title || subtitle || showDefaultClose || showResizeControls" class="app-right-sidebar__header border-b">
        <slot
          name="header"
          :narrow="narrow"
          :widen="widen"
          :can-narrow="canNarrow"
          :can-widen="canWiden"
          :current-width="currentWidth"
        >
          <div class="d-flex align-start ga-2 py-2">
            <div class="d-flex flex-column flex-grow-1 py-1" style="min-width: 0">
              <span v-if="title" class="text-subtitle-1 font-weight-semibold text-truncate">{{ title }}</span>
              <span v-if="subtitle" class="text-caption text-medium-emphasis">{{ subtitle }}</span>
            </div>
            <div v-if="showResizeControls" class="d-flex align-center ga-1">
              <v-btn
                size="small"
                icon
                variant="text"
                :disabled="!canNarrow"
                aria-label="Narrow panel"
                @click="narrow"
              >
                <v-icon>mdi-arrow-collapse-right</v-icon>
              </v-btn>
              <v-btn
                size="small"
                icon
                variant="text"
                :disabled="!canWiden"
                aria-label="Widen panel"
                @click="widen"
              >
                <v-icon>mdi-arrow-expand-right</v-icon>
              </v-btn>
            </div>
            <v-btn
              v-if="showDefaultClose"
              size="small"
              icon
              variant="text"
              aria-label="Close panel"
              @click="requestClose"
            >
              <v-icon>mdi-close</v-icon>
            </v-btn>
          </div>
        </slot>
      </div>

      <div class="app-right-sidebar__body flex-grow-1">
        <slot />
      </div>

      <div v-if="$slots.footer" class="app-right-sidebar__footer border-t">
        <slot name="footer" />
      </div>
    </v-sheet>
  </v-navigation-drawer>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';

defineOptions({
  name: 'AppRightSidebar',
});

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  width: { type: [Number, String], default: 520 },
  location: { type: String, default: 'right' },
  border: { type: String, default: 'start' },
  color: { type: String, default: 'surface' },
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' },
  showDefaultClose: { type: Boolean, default: false },
  temporaryOnMobile: { type: Boolean, default: true },
  scrimOnMobile: { type: Boolean, default: true },
  mobileBreakpoint: { type: Number, default: 960 },
  showResizeControls: { type: Boolean, default: true },
  minWidth: { type: Number, default: 360 },
  maxWidth: { type: Number, default: 900 },
  widthStep: { type: Number, default: 40 },
  showDragHandle: { type: Boolean, default: true },
});

const emit = defineEmits(['update:modelValue', 'close', 'width-change']);

const layoutWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1200);
const currentWidth = ref(520);
const isResizing = ref(false);
const resizeStartX = ref(0);
const resizeStartWidth = ref(520);

const isCompact = computed(() => layoutWidth.value <= props.mobileBreakpoint);
const canNarrow = computed(() => currentWidth.value > props.minWidth);
const canWiden = computed(() => currentWidth.value < props.maxWidth);

function parseWidth(value) {
  const n = typeof value === 'number' ? value : Number.parseInt(String(value || ''), 10);
  return Number.isFinite(n) ? n : 520;
}

function clampWidth(value) {
  return Math.min(props.maxWidth, Math.max(props.minWidth, value));
}

function narrow() {
  const next = clampWidth(currentWidth.value - props.widthStep);
  if (next === currentWidth.value) return;
  currentWidth.value = next;
  emit('width-change', next);
}

function widen() {
  const next = clampWidth(currentWidth.value + props.widthStep);
  if (next === currentWidth.value) return;
  currentWidth.value = next;
  emit('width-change', next);
}

function onResizeStart(e) {
  if (isCompact.value) {
    return;
  }
  isResizing.value = true;
  resizeStartX.value = e.clientX;
  resizeStartWidth.value = currentWidth.value;
  document.body.style.cursor = 'col-resize';
  document.body.style.userSelect = 'none';
  window.addEventListener('mousemove', onResizeMove);
  window.addEventListener('mouseup', onResizeEnd);
}

function onResizeMove(e) {
  if (!isResizing.value) {
    return;
  }
  const delta = resizeStartX.value - e.clientX;
  const next = clampWidth(resizeStartWidth.value + delta);
  if (next === currentWidth.value) {
    return;
  }
  currentWidth.value = next;
  emit('width-change', next);
}

function onResizeEnd() {
  if (!isResizing.value) {
    return;
  }
  isResizing.value = false;
  document.body.style.cursor = '';
  document.body.style.userSelect = '';
  window.removeEventListener('mousemove', onResizeMove);
  window.removeEventListener('mouseup', onResizeEnd);
}

function onDrawerToggle(next) {
  emit('update:modelValue', next);
  if (!next) {
    emit('close');
  }
}

function requestClose() {
  emit('close');
  emit('update:modelValue', false);
}

watch(
  () => props.width,
  (next) => {
    const parsed = parseWidth(next);
    currentWidth.value = clampWidth(parsed);
  },
  { immediate: true }
);

let onWindowResize = null;

onMounted(() => {
  onWindowResize = () => {
    layoutWidth.value = window.innerWidth;
  };
  window.addEventListener('resize', onWindowResize);
});

onBeforeUnmount(() => {
  onResizeEnd();
  if (onWindowResize) {
    window.removeEventListener('resize', onWindowResize);
  }
});
</script>

<style scoped>
.app-right-sidebar {
  min-height: 100%;
}

.app-right-sidebar__resize-handle {
  position: absolute;
  top: 0;
  bottom: 0;
  left: -4px;
  width: 8px;
  cursor: col-resize;
  z-index: 2;
}

.app-right-sidebar__resize-handle::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 3px;
  width: 2px;
  background: rgba(var(--v-theme-outline), 0.35);
  transition: background-color 120ms ease;
}

.app-right-sidebar__resize-handle:hover::after {
  background: rgba(var(--v-theme-primary), 0.6);
}

.app-right-sidebar__header,
.app-right-sidebar__footer {
  flex-shrink: 0;
}

.app-right-sidebar__body {
  min-height: 0;
  overflow-y: auto;
}
</style>
