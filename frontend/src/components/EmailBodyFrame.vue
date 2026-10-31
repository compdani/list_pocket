<template>
  <div class="email-frame">
    <div v-if="hasRemoteImages && !showImages" class="email-frame-notice text-caption text-medium-emphasis">
      <v-icon icon="mdi-image-off-outline" size="16" />
      <span>{{ $t('conversations.imagesBlocked') }}</span>
      <v-btn size="x-small" variant="text" class="text-none" @click="showImages = true">
        {{ $t('conversations.showImages') }}
      </v-btn>
    </div>
    <!-- allow-same-origin only lets this page measure the frame. Scripts stay disabled,
         because this HTML comes from whoever sent the email. -->
    <iframe
      ref="frame"
      class="email-frame-doc"
      :title="title"
      sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
      referrerpolicy="no-referrer"
      :srcdoc="doc"
      :style="{ height: `${height}px` }"
      @load="measure"
    />
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue';
import { emailFrameDoc } from '../utils/crm.mjs';

const props = defineProps({
  html: { type: String, default: '' },
  title: { type: String, default: 'Email' },
});

const frame = ref(null);
const height = ref(200);
const showImages = ref(false);

const doc = computed(() => emailFrameDoc(props.html, { showImages: showImages.value }));
const hasRemoteImages = computed(() => /<img\b[^>]*\bsrc\s*=\s*["']?https?:|url\(\s*["']?https?:/i.test(props.html));

function measure() {
  const body = frame.value?.contentDocument?.documentElement;
  if (body) {
    height.value = Math.min(Math.max(body.scrollHeight, 60), 2000);
  }
}

watch(() => props.html, () => {
  showImages.value = false;
});
</script>

<style scoped>
.email-frame-notice {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 6px;
}

.email-frame-doc {
  background: #fff;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 8px;
  display: block;
  width: 100%;
}
</style>
