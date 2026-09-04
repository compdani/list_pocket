<template>
  <button
    type="button"
    class="copy-text"
    :aria-label="$t('globals.buttons.copy')"
    @click="onClick"
  >
    <template v-if="!hideText">{{ text }}</template>
    <v-icon icon="mdi-content-copy" size="small" />
  </button>
</template>

<script>
export default {
  name: 'CopyText',

  props: {
    text: { type: String, default: '' },
    hideText: { type: Boolean, default: false },
  },

  methods: {
    async onClick(e) {
      e.preventDefault();
      e.stopPropagation();

      try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
          await navigator.clipboard.writeText(this.text);
        } else {
          const input = document.createElement('textarea');
          input.value = this.text;
          input.setAttribute('readonly', '');
          input.style.position = 'absolute';
          input.style.left = '-9999px';
          document.body.appendChild(input);
          input.select();
          document.execCommand('copy');
          document.body.removeChild(input);
        }
        this.$utils.toast(this.$t('globals.messages.copied'));
      } catch {
        this.$utils.toast(this.$t('globals.messages.copied'), 'error');
      }
    },
  },
};
</script>

<style scoped>
.copy-text {
  align-items: center;
  background: none;
  border: 0;
  color: inherit;
  cursor: pointer;
  display: inline-flex;
  font: inherit;
  gap: 4px;
  padding: 0;
}

.copy-text :deep(.v-icon) {
  opacity: 0.7;
}

.copy-text:hover :deep(.v-icon),
.copy-text:focus-visible :deep(.v-icon) {
  opacity: 1;
}
</style>
