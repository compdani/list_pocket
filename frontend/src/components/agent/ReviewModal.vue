<template>
  <v-dialog :model-value="true" max-width="1000" scrollable @update:model-value="$emit('close')">
    <v-card data-cy="agent-review">
      <v-card-title class="d-flex align-center ga-2">
        <span class="text-truncate">{{ info.label }}</span>
        <v-spacer />
        <v-btn :aria-label="$t('globals.buttons.close')" icon="mdi-close" size="small" variant="text" @click="$emit('close')" />
      </v-card-title>
      <v-card-subtitle v-if="args.summary" class="agent-wrap">{{ args.summary }}</v-card-subtitle>
      <v-card-text>
        <v-alert v-if="pending && info.warning" type="warning" variant="tonal" density="compact" class="mb-3">{{ info.warning }}</v-alert>
        <v-alert v-for="warning in part.warnings || []" :key="warning" type="info" variant="tonal" density="compact" class="mb-2 text-body-2">{{ warning }}</v-alert>

        <!-- workflows -->
        <template v-if="info.view === 'workflow' || info.view === 'publish'">
          <v-tabs v-if="info.view === 'workflow'" v-model="tab" density="compact" color="primary" class="mb-2">
            <v-tab value="graph" class="text-none">{{ $t('agent.review.graph') }}</v-tab>
            <v-tab value="json" class="text-none" data-cy="agent-review-json">{{ pending ? $t('agent.review.editJson') : $t('agent.review.json') }}</v-tab>
          </v-tabs>
          <template v-if="tab === 'graph'">
            <p v-if="previewError" class="text-error text-body-2">{{ previewError }}</p>
            <p v-if="diff && !diff.isNew" class="text-caption text-medium-emphasis mb-2" data-cy="agent-review-diff">
              {{ $t('agent.review.changes', { added: countOf('added'), changed: countOf('changed'), removed: diff.removed.length }) }}
              <template v-if="diff.removed.length"> ({{ diff.removed.join(', ') }})</template>
            </p>
            <flow-preview v-if="preview?.definition" :key="previewKey" :definition="preview.definition" :current="preview.current" :diff="diff?.nodes" />
            <v-progress-linear v-else-if="!previewError" indeterminate color="primary" />
          </template>
          <json-field v-else :key="jsonKey" v-model="edited" :label="$t('agent.review.proposal')" :rows="16" :readonly="!pending" data-cy="agent-review-editor" />
          <v-text-field v-if="info.view === 'publish' && pending" v-model="edited.note" :label="$t('agent.review.note')" density="compact" variant="outlined" hide-details class="mt-3" />
          <template v-if="preview?.validation">
            <p v-if="preview.validation.ok" class="text-caption text-success mt-2 mb-0" data-cy="agent-review-valid">{{ $t('agent.review.valid') }}</p>
            <v-alert v-else type="error" variant="tonal" density="compact" class="mt-2 text-body-2">
              <ul class="agent-list"><li v-for="item in preview.validation.errors" :key="item">{{ item }}</li></ul>
            </v-alert>
          </template>
        </template>

        <!-- a message to someone -->
        <template v-else-if="info.view === 'reply'">
          <p class="text-caption text-medium-emphasis">{{ $t('agent.review.replyTo', { key: edited.body.key, channel: edited.body.channel }) }}</p>
          <v-text-field v-if="edited.body.channel === 'email'" v-model="edited.body.subject" :label="$t('agent.review.subject')" :readonly="!pending" density="compact" variant="outlined" class="mb-2" hide-details data-cy="agent-review-subject" />
          <v-textarea v-model="edited.body.body" :label="$t('agent.review.message')" :readonly="!pending" rows="8" auto-grow density="compact" variant="outlined" hide-details data-cy="agent-review-body" />
        </template>

        <!-- campaigns and templates -->
        <template v-else-if="info.view === 'content'">
          <v-text-field v-if="content.name !== undefined" v-model="content.name" :label="$t('agent.review.name')" :readonly="!pending" density="compact" variant="outlined" class="mb-2" hide-details />
          <v-text-field v-if="content.subject !== undefined" v-model="content.subject" :label="$t('agent.review.subject')" :readonly="!pending" density="compact" variant="outlined" class="mb-2" hide-details data-cy="agent-review-subject" />
          <v-tabs v-model="tab" density="compact" color="primary" class="mb-2">
            <v-tab value="graph" class="text-none">{{ $t('agent.review.preview') }}</v-tab>
            <v-tab value="json" class="text-none">{{ $t('agent.review.editBody') }}</v-tab>
          </v-tabs>
          <email-body-frame v-if="tab === 'graph'" :html="contentHtml" :title="content.subject || info.label" data-cy="agent-review-frame" />
          <v-textarea v-else v-model="content.body" :label="$t('agent.review.body')" :readonly="!pending" rows="14" density="compact" variant="outlined" hide-details />
        </template>

        <!-- test runs and runs -->
        <template v-else-if="info.view === 'run'">
          <p class="text-body-2">{{ $t('agent.review.runOf', { workflow: edited.workflow }) }}</p>
          <json-field v-model="edited.input" :label="$t('agent.review.input')" :rows="8" :readonly="!pending" data-cy="agent-review-input" />
        </template>

        <!-- plugin source -->
        <template v-else-if="info.view === 'plugin'">
          <p v-if="previewError" class="text-error text-body-2">{{ previewError }}</p>
          <p v-if="preview?.replaces" class="text-body-2" data-cy="agent-review-replaces">{{ $t('agent.review.pluginReplaces', { version: preview.replaces }) }}</p>
          <div v-if="addons.length" class="d-flex flex-wrap ga-1 mb-2">
            <v-chip v-for="addon in addons" :key="addon.key" :title="addon.description" size="small" variant="outlined" data-cy="agent-review-addon">{{ $t(addon.kind) }}: {{ addon.name }}</v-chip>
          </div>
          <p v-if="changes.removed.length" class="text-caption text-medium-emphasis mb-2">{{ $t('agent.review.pluginRemoved', { files: changes.removed.join(', ') }) }}</p>
          <v-tabs v-model="pluginFile" density="compact" color="primary" show-arrows class="mb-2">
            <v-tab v-for="file in changes.files" :key="file.path" :value="file.path" class="text-none" data-cy="agent-review-file">
              {{ file.path }}
              <span v-if="preview?.current && file.status !== 'same'" :class="['plugin-mark', `plugin-mark--${file.status}`]">{{ $t(`agent.review.plugin_${file.status}`) }}</span>
            </v-tab>
          </v-tabs>
          <v-textarea v-if="openFile" v-model="openFile.content" :label="openFile.path" :readonly="!pending" rows="18" density="compact" variant="outlined" hide-details spellcheck="false" class="plugin-code" data-cy="agent-review-code" />
          <details v-if="fileDiff" class="mt-2" data-cy="agent-review-diff">
            <summary class="text-caption">{{ $t('agent.review.pluginChanges') }}</summary>
            <pre class="plugin-diff"><div v-for="(line, i) in fileDiff" :key="i" :class="`plugin-diff--${line.kind}`">{{ { added: '+ ', removed: '- ', same: '  ' }[line.kind] }}{{ line.text }}</div></pre>
          </details>
        </template>

        <!-- everything else -->
        <template v-else>
          <table v-if="rows.length" class="agent-table mb-3">
            <tbody>
              <tr v-for="row in rows" :key="row.key"><th>{{ row.key }}</th><td>{{ row.value }}</td></tr>
            </tbody>
          </table>
          <details>
            <summary class="text-caption mb-2">{{ pending ? $t('agent.review.editJson') : $t('agent.review.json') }}</summary>
            <json-field :key="jsonKey" v-model="edited" :label="$t('agent.review.proposal')" :rows="10" :readonly="!pending" />
          </details>
        </template>

        <v-alert v-if="part.error || error" type="error" variant="tonal" density="compact" class="mt-3 text-body-2">{{ part.error || error }}</v-alert>
      </v-card-text>
      <v-card-actions>
        <a v-if="resultHref" :href="resultHref" class="text-body-2 ml-2" data-cy="agent-review-open">{{ $t('agent.open') }}</a>
        <v-spacer />
        <v-btn class="text-none" @click="$emit('close')">{{ $t('globals.buttons.close') }}</v-btn>
        <v-btn v-if="pending" class="text-none" :disabled="busy" data-cy="agent-review-reject" @click="$emit('reject')">{{ $t('agent.reject') }}</v-btn>
        <v-btn v-if="pending" color="primary" variant="flat" class="text-none" :loading="busy" data-cy="agent-review-apply" @click="apply">{{ info.apply }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue';
import { agentPost } from '../../agent/client';
import {
  diffNodes, lineDiff, pluginAddons, pluginFileChanges, proposalInfo,
} from '../../agent/parts.mjs';
import { sameDefinition } from '../../utils/flowGraph.mjs';
import EmailBodyFrame from '../EmailBodyFrame.vue';
import JsonField from '../flow/JsonField.vue';
import FlowPreview from './FlowPreview.vue';

const props = defineProps({
  part: { type: Object, required: true },
  busy: { type: Boolean, default: false },
  error: { type: String, default: '' },
});
const emit = defineEmits(['close', 'apply', 'reject']);

const args = computed(() => props.part.arguments || {});
const info = computed(() => proposalInfo(props.part.tool));
const pending = computed(() => props.part.status === 'pending');
const resultHref = computed(() => props.part.result?.source?.href || '');

// The modal edits its own copy, so closing it throws edits away.
const edited = ref(JSON.parse(JSON.stringify(props.part.arguments || {})));
if (info.value.view === 'reply') edited.value.body ||= {};
if (info.value.view === 'content') edited.value[info.value.path] ||= {};
if (info.value.view === 'publish') edited.value.note ||= '';

const tab = ref('graph');
const preview = ref(null);
const previewError = ref('');
const previewKey = ref(0);
const jsonKey = ref(0);

const content = computed(() => edited.value[info.value.path] || {});
const contentHtml = computed(() => {
  const body = content.value.body || '';
  // Markdown and plain bodies are shown as text, so they read as they'll be written.
  if (['plain', 'markdown'].includes(content.value.content_type)) {
    const safe = String(body).replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
    return `<pre style="white-space:pre-wrap;font:inherit">${safe}</pre>`;
  }
  return body;
});

const pluginFile = ref(edited.value.files?.[0]?.path || '');
const openFile = computed(() => (edited.value.files || []).find((f) => f.path === pluginFile.value));
const changes = computed(() => pluginFileChanges(preview.value?.current?.files, edited.value.files));
const addons = computed(() => pluginAddons(preview.value?.manifest));
const fileDiff = computed(() => {
  const before = preview.value?.current?.files?.find((f) => f.path === pluginFile.value);
  return before && openFile.value && before.content !== openFile.value.content ? lineDiff(before.content, openFile.value.content) : null;
});

const rows = computed(() => {
  const source = edited.value.body && typeof edited.value.body === 'object' ? edited.value.body : edited.value;
  return Object.entries(source)
    .filter(([key]) => key !== 'summary')
    .map(([key, value]) => ({ key, value: typeof value === 'object' ? JSON.stringify(value) : String(value) }));
});

const diff = computed(() => (preview.value?.definition
  ? diffNodes(preview.value.current || null, preview.value.definition, sameDefinition)
  : null));

function countOf(kind) {
  return Object.values(diff.value?.nodes || {}).filter((v) => v === kind).length;
}

async function loadPreview() {
  previewError.value = '';
  try {
    preview.value = await agentPost('agent/preview-proposal', { tool: props.part.tool, arguments: edited.value });
    previewKey.value += 1;
  } catch (err) {
    previewError.value = err.message;
  }
}

function apply() {
  emit('apply', edited.value);
}

// Switching back to the graph shows what the edited JSON would do.
watch(tab, (value) => {
  if (value === 'graph' && (info.value.view === 'workflow')) loadPreview();
  if (value === 'json') jsonKey.value += 1;
});

if (info.value.view === 'workflow' || info.value.view === 'publish') loadPreview();
// Once a plugin is installed, the files are the installed ones, so there's nothing to compare.
if (info.value.view === 'plugin' && pending.value) loadPreview();
</script>

<style scoped>
.agent-wrap {
  overflow-wrap: anywhere;
  white-space: normal;
}

.plugin-code :deep(textarea),
.plugin-diff {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.5;
}

.plugin-diff {
  max-height: 320px;
  overflow: auto;
}

.plugin-diff--added {
  background: rgba(var(--v-theme-success), 0.12);
}

.plugin-diff--removed {
  background: rgba(var(--v-theme-error), 0.1);
}

.plugin-mark {
  font-size: 11px;
  margin-left: 6px;
  opacity: 0.75;
}

.agent-list {
  margin: 0;
  padding-left: 18px;
}

.agent-table {
  border-collapse: collapse;
  font-size: 13px;
  width: 100%;
}

.agent-table th,
.agent-table td {
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  overflow-wrap: anywhere;
  padding: 6px 8px;
  text-align: left;
  vertical-align: top;
}

.agent-table th {
  font-weight: 500;
  white-space: nowrap;
  width: 1%;
}
</style>
