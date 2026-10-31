<template>
  <section class="mb-4" data-cy="whatsapp-template-picker">
    <div class="d-flex ga-2 align-start">
      <v-select
        :model-value="selectedKey"
        :items="items"
        :label="$t('whatsapp.template')"
        :loading="loading"
        :disabled="disabled"
        :error-messages="error"
        :no-data-text="$t('whatsapp.noTemplates')"
        variant="outlined"
        density="comfortable"
        data-cy="whatsapp-template"
        @update:model-value="select"
      />
      <v-btn variant="text" :loading="loading" :disabled="disabled" data-cy="whatsapp-template-refresh" @click="load(true)">
        {{ $t('whatsapp.refresh') }}
      </v-btn>
    </div>

    <p v-if="preview" class="form-help mb-3" data-cy="whatsapp-template-preview">{{ preview }}</p>

    <template v-for="(section, i) in sections" :key="i">
      <div class="text-subtitle-2 mb-2">{{ sectionLabel(section) }}</div>
      <v-text-field
        v-if="section.format"
        v-model="section.link"
        :label="$t('whatsapp.mediaLink')"
        :disabled="disabled"
        variant="outlined"
        density="comfortable"
        class="mb-2"
        @update:model-value="emitChoice"
      />
      <v-row v-for="(param, j) in section.parameters" :key="j" dense>
        <v-col cols="12" sm="7">
          <v-text-field
            v-model="param.value"
            :label="`{{${param.label}}}`"
            :hint="$t('whatsapp.valueHelp')"
            :disabled="disabled"
            :data-cy="`whatsapp-param-${i}-${j}`"
            variant="outlined"
            density="comfortable"
            @update:model-value="emitChoice"
          />
        </v-col>
        <v-col cols="12" sm="5">
          <v-text-field
            v-model="param.fallback"
            :label="$t('whatsapp.fallback')"
            :disabled="disabled"
            :data-cy="`whatsapp-fallback-${i}-${j}`"
            variant="outlined"
            density="comfortable"
            @update:model-value="emitChoice"
          />
        </v-col>
      </v-row>
    </template>

    <v-alert
      v-if="selected && incomplete"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mb-2"
      :text="$t('whatsapp.incomplete')"
      data-cy="whatsapp-template-incomplete"
    />
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, onMounted, ref,
} from 'vue';
import { getWhatsAppTemplates } from '../api';
import {
  choiceFromSections,
  missingValues,
  restoreSections,
  templateKey,
  templatePreview,
} from '../utils/whatsappTemplates.mjs';

const props = defineProps({
  modelValue: { type: Object, default: () => ({}) },
  disabled: { type: Boolean, default: false },
});

const emit = defineEmits(['update:modelValue']);
const { proxy } = getCurrentInstance();

const templates = ref([]);
const loading = ref(false);
const error = ref('');
const selected = ref(null);
const sections = ref([]);

const items = computed(() => templates.value.map((t) => ({
  title: `${t.name} (${t.language})`,
  value: templateKey(t),
})));
const selectedKey = computed(() => (selected.value ? templateKey(selected.value) : null));
const preview = computed(() => (selected.value ? templatePreview(selected.value) : ''));
const incomplete = computed(() => missingValues(sections.value));

function sectionLabel(section) {
  if (section.type === 'button') {
    return proxy.$t('whatsapp.sectionButton', { name: section.label });
  }
  return proxy.$t(section.type === 'header' ? 'whatsapp.sectionHeader' : 'whatsapp.sectionBody');
}

function emitChoice() {
  emit('update:modelValue', {
    ...props.modelValue,
    template: selected.value ? choiceFromSections(selected.value, sections.value) : undefined,
  });
}

function select(key) {
  selected.value = templates.value.find((t) => templateKey(t) === key) || null;
  sections.value = selected.value ? restoreSections(selected.value, props.modelValue.template) : [];
  emitChoice();
}

async function load(refresh = false) {
  loading.value = true;
  error.value = '';
  try {
    templates.value = (await getWhatsAppTemplates(refresh)) || [];
    const saved = props.modelValue.template;
    if (saved && !selected.value) {
      const match = templates.value.find((t) => t.name === saved.name && t.language === saved.language);
      if (match) {
        selected.value = match;
        sections.value = restoreSections(match, saved);
      } else {
        error.value = proxy.$t('whatsapp.templateGone', { name: saved.name });
      }
    }
  } catch (err) {
    error.value = (err.response && err.response.message) || err.message;
  } finally {
    loading.value = false;
  }
}

onMounted(() => load());
</script>
