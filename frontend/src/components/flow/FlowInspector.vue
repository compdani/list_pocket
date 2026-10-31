<template>
  <div class="inspector" data-cy="flow-inspector">
    <header class="inspector-head">
      <v-icon :icon="info.icon" size="18" />
      <strong>{{ info.label }}</strong>
      <v-spacer />
      <v-btn
        v-if="node.type !== 'start' && !readonly"
        :aria-label="$t('globals.buttons.delete')"
        icon="mdi-trash-can-outline"
        size="x-small"
        variant="text"
        data-cy="flow-node-delete"
        @click="$emit('remove', node.id)"
      />
    </header>
    <p class="text-caption text-medium-emphasis mb-3">{{ info.hint }}</p>

    <v-text-field v-model="node.name" :label="$t('workflows.field.label')" :placeholder="info.label" v-bind="field" />
    <v-text-field
      :model-value="node.id"
      :label="$t('workflows.field.id')"
      :hint="$t('workflows.field.idHint', { id: node.id })"
      persistent-hint
      v-bind="field"
      @change="$emit('rename', node.id, $event.target.value)"
    />

    <!-- start -->
    <template v-if="node.type === 'start'">
      <h3 class="inspector-section">{{ $t('workflows.trigger.title') }}</h3>
      <v-select v-model="trigger.type" :items="triggerTypes" :label="$t('workflows.trigger.type')" v-bind="field" data-cy="flow-trigger-type" />
      <template v-if="trigger.type === 'cron'">
        <v-text-field v-model="trigger.spec" :label="$t('workflows.trigger.spec')" hint="0 9 * * 1-5" persistent-hint v-bind="field" />
        <v-text-field v-model="trigger.timezone" :label="$t('workflows.trigger.timezone')" hint="America/Chicago" persistent-hint v-bind="field" />
        <v-select v-model="trigger.overlap" :items="overlapItems" :label="$t('workflows.trigger.overlap')" v-bind="field" />
      </template>
      <template v-if="trigger.type === 'event'">
        <v-select v-model="eventSource" :items="eventSources" :label="$t('workflows.trigger.source')" v-bind="field" />
        <v-select
          v-if="eventSource === 'event'"
          v-model="trigger.event"
          :items="catalog.events || []"
          item-title="name"
          item-value="name"
          :hint="eventHint(trigger.event)"
          persistent-hint
          :label="$t('workflows.trigger.event')"
          v-bind="field"
          data-cy="flow-trigger-event"
        />
        <template v-else>
          <v-select v-model="trigger.collection" :items="catalog.collections || []" :label="$t('workflows.trigger.collection')" v-bind="field" />
          <v-select v-model="trigger.on" :items="['create', 'update']" multiple :label="$t('workflows.trigger.on')" v-bind="field" />
        </template>
      </template>
      <div v-if="trigger.type === 'webhook'" class="mb-3 text-caption">
        <div class="text-medium-emphasis mb-1">{{ $t('workflows.trigger.webhookHelp') }}</div>
        <copy-text v-if="fullWebhookUrl" :text="fullWebhookUrl" />
      </div>
      <template v-if="trigger.type === 'event' || trigger.type === 'webhook'">
        <v-text-field v-model="trigger.filter" :label="$t('workflows.trigger.filter')" hint="event.kind == 'reply'" persistent-hint v-bind="field" />
        <json-field v-model="trigger.input_map" :label="$t('workflows.trigger.inputMap')" hint='{"contact": "{{ event.contact_id }}"}' :readonly="readonly" class="mb-3" />
      </template>
      <h3 class="inspector-section">{{ $t('workflows.field.runInput') }}</h3>
      <json-field v-model="definition.input_schema" :label="$t('workflows.field.inputSchema')" :readonly="readonly" class="mb-3" />
      <json-field v-model="definition.default_input" :label="$t('workflows.field.defaultInput')" :readonly="readonly" class="mb-3" />
    </template>

    <!-- agent -->
    <template v-if="node.type === 'agent'">
      <v-select
        v-if="(catalog.presets || []).length || node.agent.preset"
        v-model="agentPreset"
        :items="catalog.presets || []"
        item-title="name"
        item-value="name"
        clearable
        :label="$t('workflows.field.preset')"
        :hint="presetHint"
        persistent-hint
        v-bind="field"
        data-cy="flow-agent-preset"
      />
      <v-textarea v-model="node.agent.system_prompt" :label="$t('workflows.field.instructions')" :hint="$t(node.agent.preset ? 'workflows.field.instructionsPresetHint' : 'workflows.field.instructionsHint')" persistent-hint rows="6" auto-grow v-bind="field" data-cy="flow-agent-prompt" />
      <v-autocomplete
        v-model="node.agent.tools"
        :items="toolItems"
        multiple
        chips
        closable-chips
        :label="$t('workflows.field.tools')"
        :hint="$t('workflows.field.toolsHint')"
        persistent-hint
        v-bind="field"
      />
      <v-select v-model="modelRole" :items="modelItems" :label="$t('workflows.field.model')" v-bind="field" />
      <v-text-field v-model.number="node.agent.max_steps" type="number" min="1" max="50" :label="$t('workflows.field.maxSteps')" v-bind="field" />
      <json-field v-model="node.agent.output_schema" :label="$t('workflows.field.outputSchema')" :hint="$t('workflows.field.outputSchemaHint')" :readonly="readonly" class="mb-3" />
      <json-field v-model="node.input" :label="$t('workflows.field.input')" :readonly="readonly" class="mb-3" />
    </template>

    <!-- tool -->
    <template v-if="node.type === 'tool'">
      <v-autocomplete v-model="node.tool" :items="toolItems" :label="$t('workflows.field.tool')" v-bind="field" data-cy="flow-tool-select" />
      <p v-if="selectedTool" class="text-caption text-medium-emphasis">{{ selectedTool.description }}</p>
      <v-alert v-if="selectedTool?.requires_approval" type="info" variant="tonal" density="compact" class="mb-3 text-caption">
        {{ $t('workflows.field.toolApproval') }}
      </v-alert>
      <template v-if="toolFields.length">
        <template v-for="prop in toolFields" :key="prop.name">
          <component
            :is="prop.editor"
            v-if="prop.editor"
            :model-value="(node.input || {})[prop.name]"
            :label="prop.label"
            :hint="prop.description"
            :readonly="readonly"
            :schema="prop.schema"
            :options="prop.widgetOptions"
            :data-cy="`flow-tool-input-${prop.name}`"
            @update:model-value="setInput(prop.name, $event)"
          />
          <v-select
            v-else-if="prop.options"
            :model-value="(node.input || {})[prop.name]"
            :items="prop.options"
            :label="prop.label"
            :hint="prop.description"
            persistent-hint
            clearable
            v-bind="field"
            :data-cy="`flow-tool-input-${prop.name}`"
            @update:model-value="setInput(prop.name, $event ?? undefined)"
          />
          <json-field
            v-else-if="prop.complex"
            :model-value="(node.input || {})[prop.name]"
            :label="prop.label"
            :hint="prop.description"
            :readonly="readonly"
            allow-text
            class="mb-3"
            @update:model-value="setInput(prop.name, $event)"
          />
          <v-text-field
            v-else
            :model-value="(node.input || {})[prop.name]"
            :label="prop.label"
            :hint="prop.description"
            persistent-hint
            v-bind="field"
            :data-cy="`flow-tool-input-${prop.name}`"
            @update:model-value="setInput(prop.name, $event)"
          />
        </template>
      </template>

      <!-- board and stage -->
      <template v-if="isBoardTool">
        <v-combobox
          :model-value="node.input?.board"
          :items="boardItems"
          item-title="name"
          item-value="key"
          :return-object="false"
          :label="$t('workflows.board.board')"
          :hint="$t('workflows.board.hint')"
          persistent-hint
          v-bind="field"
          data-cy="flow-board"
          @update:model-value="changeBoard"
        />
        <v-combobox
          v-if="node.tool === BOARD_MOVE_TOOL"
          :model-value="node.input?.stage"
          :items="stageItems"
          item-title="name"
          item-value="key"
          :return-object="false"
          :label="$t('workflows.board.stage')"
          v-bind="field"
          data-cy="flow-board-stage"
          @update:model-value="setInput('stage', $event ?? undefined)"
        />
      </template>

      <!-- email message -->
      <template v-if="isEmailTool">
        <v-alert v-if="node.input?.template_id" type="info" variant="tonal" density="compact" class="mb-3 text-caption" data-cy="flow-email-template-note">
          {{ $t('workflows.email.usesTemplate') }}
        </v-alert>
        <template v-else>
          <v-select
            :model-value="emailFormat"
            :items="emailFormatItems"
            :label="$t('workflows.email.format')"
            v-bind="field"
            data-cy="flow-email-format"
            @update:model-value="changeEmailFormat"
          />
          <p class="email-summary text-body-2" :class="{ 'text-medium-emphasis': !emailSummary }" data-cy="flow-email-summary">
            {{ emailSummary || $t('workflows.email.empty') }}
          </p>
          <v-btn block variant="tonal" color="primary" class="text-none mb-3" prepend-icon="mdi-email-edit-outline" data-cy="flow-email-compose" @click="openCompose()">
            {{ readonly ? $t('workflows.email.view') : (emailSummary ? $t('workflows.email.edit') : $t('workflows.email.write')) }}
          </v-btn>
          <v-alert v-if="emailNeedsApproval" type="info" variant="tonal" density="compact" class="mb-3 text-caption" data-cy="flow-email-approval">
            {{ $t('workflows.email.needsApproval') }}
          </v-alert>
        </template>
        <email-compose-modal
          v-if="composing"
          :input="node.input || {}"
          :format="composeFormat"
          :readonly="readonly"
          @close="composing = false"
          @save="saveEmail"
        />
      </template>
    </template>

    <!-- branch -->
    <template v-if="node.type === 'branch'">
      <div v-for="(item, i) in node.cases" :key="i" class="d-flex ga-1 align-start">
        <v-text-field v-model="item.if" :label="$t('workflows.field.caseIf', { n: i + 1 })" v-bind="field" />
        <v-btn v-if="!readonly && node.cases.length > 1" :aria-label="$t('globals.buttons.delete')" icon="mdi-close" size="x-small" variant="text" @click="node.cases.splice(i, 1)" />
      </div>
      <v-btn v-if="!readonly" size="small" variant="text" class="text-none mb-2" prepend-icon="mdi-plus" @click="node.cases.push({ if: '', to: '' })">
        {{ $t('workflows.field.addCase') }}
      </v-btn>
      <p class="text-caption text-medium-emphasis">{{ $t('workflows.field.branchHelp') }}</p>
    </template>

    <!-- foreach -->
    <template v-if="node.type === 'foreach'">
      <v-text-field v-model="node.items" :label="$t('workflows.field.items')" hint="nodes.search.results" persistent-hint v-bind="field" />
      <v-text-field v-model.number="node.concurrency" type="number" min="1" max="20" :label="$t('workflows.field.concurrency')" v-bind="field" />
    </template>

    <!-- loop -->
    <template v-if="node.type === 'loop'">
      <v-text-field v-model="node.until" :label="$t('workflows.field.until')" hint="loop.last.status == 'done'" persistent-hint v-bind="field" />
      <v-text-field v-model.number="node.max_iterations" type="number" min="1" max="100" :label="$t('workflows.field.maxIterations')" v-bind="field" />
    </template>

    <!-- review -->
    <template v-if="node.type === 'review'">
      <v-text-field v-model="node.review.title" :label="$t('workflows.field.reviewTitle')" v-bind="field" />
      <v-textarea v-model="node.review.instructions" :label="$t('workflows.field.reviewInstructions')" rows="2" auto-grow v-bind="field" />
      <json-field v-model="node.review.data" :label="$t('workflows.field.reviewData')" :hint="$t('workflows.field.reviewDataHint')" :readonly="readonly" class="mb-3" />
      <v-text-field v-model="node.review.auto_approve" :label="$t('workflows.field.autoApprove')" :hint="$t('workflows.field.autoApproveHint')" persistent-hint v-bind="field" />
      <v-select v-model="node.review.on_deny" :items="denyItems" :label="$t('workflows.field.onDeny')" v-bind="field" />
      <v-text-field v-model="node.review.timeout" :label="$t('workflows.field.timeout')" hint="72h" persistent-hint v-bind="field" />
    </template>

    <!-- wait -->
    <template v-if="node.type === 'wait'">
      <v-text-field v-model="node.wait.delay" :label="$t('workflows.field.delay')" hint="10m, 2h, 72h" persistent-hint v-bind="field" />
      <v-select v-model="node.wait.event" :items="catalog.events || []" item-title="name" item-value="name" clearable :label="$t('workflows.field.waitEvent')" :hint="eventHint(node.wait.event)" persistent-hint v-bind="field" />
      <template v-if="node.wait.event">
        <v-text-field v-model="node.wait.match" :label="$t('workflows.field.match')" hint="event.contact_id == input.contact" persistent-hint v-bind="field" />
        <v-text-field v-model="node.wait.timeout" :label="$t('workflows.field.timeout')" hint="72h" persistent-hint v-bind="field" />
      </template>
    </template>

    <!-- subworkflow -->
    <template v-if="node.type === 'subworkflow'">
      <v-select v-model="node.workflow" :items="otherWorkflows" :label="$t('workflows.field.workflow')" v-bind="field" />
      <json-field v-model="node.input" :label="$t('workflows.field.input')" :readonly="readonly" class="mb-3" />
      <v-checkbox v-model="node.detach" :label="$t('workflows.field.detach')" density="compact" hide-details :readonly="readonly" class="mb-3" />
    </template>

    <template v-if="node.type === 'notify'">
      <v-textarea v-model="node.message" :label="$t('workflows.field.message')" :hint="$t('workflows.field.notifyHelp')" persistent-hint rows="3" auto-grow v-bind="field" />
    </template>
    <json-field v-if="node.type === 'transform'" v-model="node.value" :label="$t('workflows.field.value')" :readonly="readonly" allow-text class="mb-3" />
    <json-field v-if="node.type === 'end'" v-model="node.output" :label="$t('workflows.field.output')" :readonly="readonly" allow-text class="mb-3" />

    <!-- errors -->
    <template v-if="canFail">
      <h3 class="inspector-section">{{ $t('workflows.field.ifFails') }}</h3>
      <v-select v-model="node.on_error" :items="errorItems" :label="$t('workflows.field.onError')" v-bind="field" />
      <v-text-field v-model.number="node.retries" type="number" min="0" max="5" :label="$t('workflows.field.retries')" v-bind="field" />
    </template>

    <p class="text-caption text-medium-emphasis mt-2 mb-0">{{ $t('workflows.field.expressionHelp') }}</p>
  </div>
</template>

<script setup>
import {
  computed, defineAsyncComponent, getCurrentInstance, ref,
} from 'vue';
import CopyText from '../CopyText.vue';
import JsonField from './JsonField.vue';
import { nodeInfo } from '../../utils/flowGraph.mjs';
import { EMAIL_FORMATS, bodySummary, needsSendApproval } from '../../utils/emailCompose.mjs';
import { pluginRegistry } from '../../pluginHost';
import { pluginFieldEditor, schemaWidget } from '../../pluginHost/registry.mjs';

// It brings every email editor with it, so it loads when it opens.
const EmailComposeModal = defineAsyncComponent(() => import('./EmailComposeModal.vue'));
const EMAIL_TOOL = 'messages.send_email';
const EMAIL_MESSAGE_FIELDS = ['body', 'body_source', 'content_type'];
const BOARD_MOVE_TOOL = 'boards.move_contact';
const BOARD_TOOLS = [BOARD_MOVE_TOOL, 'boards.remove_contact'];
const BOARD_FIELDS = ['board', 'stage'];

const props = defineProps({
  node: { type: Object, required: true },
  definition: { type: Object, required: true },
  catalog: { type: Object, default: () => ({}) },
  webhookUrl: { type: String, default: '' },
  readonly: { type: Boolean, default: false },
});
defineEmits(['remove', 'rename']);

const { proxy } = getCurrentInstance();
const t = (key) => proxy.$t(key);

const field = computed(() => ({
  density: 'compact', variant: 'outlined', class: 'mb-3', readonly: props.readonly,
}));

const info = computed(() => nodeInfo(props.node));
const trigger = computed(() => props.definition.trigger);
const canFail = computed(() => ['agent', 'tool', 'foreach', 'loop', 'subworkflow', 'transform', 'notify'].includes(props.node.type));

const triggerTypes = computed(() => ['manual', 'event', 'cron', 'webhook'].map((value) => ({ value, title: t(`workflows.trigger.types.${value}`) })));
const overlapItems = computed(() => [{ value: 'skip', title: t('workflows.trigger.overlapSkip') }, { value: 'queue', title: t('workflows.trigger.overlapQueue') }]);
const eventSources = computed(() => [{ value: 'event', title: t('workflows.trigger.fromEvent') }, { value: 'collection', title: t('workflows.trigger.fromCollection') }]);
const denyItems = computed(() => [{ value: 'skip', title: t('workflows.field.denySkip') }, { value: 'continue', title: t('workflows.field.denyContinue') }]);
const errorItems = computed(() => [{ value: 'fail', title: t('workflows.field.errorFail') }, { value: 'continue', title: t('workflows.field.errorContinue') }]);
const modelItems = computed(() => [
  { value: '', title: t('workflows.field.modelDefault') },
  { value: '@fast', title: t('workflows.field.modelFast') },
  { value: '@research', title: t('workflows.field.modelResearch') },
]);

const eventSource = computed({
  get: () => (trigger.value.collection ? 'collection' : 'event'),
  set(value) {
    if (value === 'collection') {
      delete trigger.value.event;
      trigger.value.collection = (props.catalog.collections || [])[0] || 'subscribers';
    } else {
      delete trigger.value.collection;
      delete trigger.value.on;
    }
  },
});

const toolItems = computed(() => (props.catalog.tools || []).map((tool) => ({
  value: tool.name,
  title: tool.read_only ? tool.name : `${tool.name} ✎`,
})));
const selectedTool = computed(() => (props.catalog.tools || []).find((tool) => tool.name === props.node.tool));
const otherWorkflows = computed(() => (props.catalog.workflows || []).filter((name) => name !== props.definition.name));

const toolFields = computed(() => {
  const schema = selectedTool.value?.input_schema;
  if (!schema?.properties) return [];
  const required = schema.required || [];
  let hidden = [];
  if (isEmailTool.value) hidden = EMAIL_MESSAGE_FIELDS;
  if (isBoardTool.value) hidden = BOARD_FIELDS;
  return Object.entries(schema.properties).filter(([name]) => !hidden.includes(name)).map(([name, prop]) => ({
    name,
    label: required.includes(name) ? `${name} *` : name,
    description: prop.description || '',
    complex: prop.type === 'object' || prop.type === 'array' || !prop.type,
    options: Array.isArray(prop.enum) ? prop.enum : null,
    editor: pluginFieldEditor(pluginRegistry, prop),
    schema: prop,
    widgetOptions: schemaWidget(prop).options,
  }));
});

const isEmailTool = computed(() => props.node.type === 'tool' && props.node.tool === EMAIL_TOOL);
const isBoardTool = computed(() => props.node.type === 'tool' && BOARD_TOOLS.includes(props.node.tool));
const boardItems = computed(() => props.catalog.boards || []);
const stageItems = computed(() => boardItems.value.find((b) => b.key === props.node.input?.board)?.stages || []);

// A stage belongs to its board, so choosing another board clears it.
function changeBoard(board) {
  if (board === props.node.input?.board) return;
  const input = { ...(props.node.input || {}) };
  delete input.stage;
  if (board) {
    input.board = board;
  } else {
    delete input.board;
  }
  // eslint-disable-next-line vue/no-mutating-props
  props.node.input = input;
}

const composing = ref(false);
const composeFormat = ref('');
const emailFormat = computed(() => props.node.input?.content_type || 'plain');
const emailFormatItems = computed(() => EMAIL_FORMATS.map((f) => ({ value: f.value, title: f.title || t(f.label) })));
const emailSummary = computed(() => bodySummary(props.node.input || {}));
const emailNeedsApproval = computed(() => needsSendApproval(props.node.input || {}));

function openCompose(format = '') {
  composeFormat.value = format;
  composing.value = true;
}

// Changing an existing body's format converts it, which the editor does and confirms.
function changeEmailFormat(format) {
  if (format === emailFormat.value) return;
  if (emailSummary.value) {
    openCompose(format);
    return;
  }
  setInput('content_type', format === 'plain' ? undefined : format);
}

function saveEmail(input) {
  // eslint-disable-next-line vue/no-mutating-props
  props.node.input = input;
  composing.value = false;
}

function setInput(name, value) {
  const input = { ...(props.node.input || {}) };
  if (value === undefined || value === '') {
    delete input[name];
  } else {
    input[name] = value;
  }
  // eslint-disable-next-line vue/no-mutating-props
  props.node.input = input;
}

const agentPreset = computed({
  get: () => props.node.agent?.preset || null,
  set(value) {
    const { agent } = props.node;
    if (value) {
      agent.preset = value;
    } else {
      delete agent.preset;
    }
  },
});
const presetHint = computed(() => (props.catalog.presets || []).find((p) => p.name === props.node.agent?.preset)?.description || t('workflows.field.presetHint'));

const modelRole = computed({
  get: () => props.node.agent?.ai?.model || '',
  set(value) {
    const { agent } = props.node;
    if (value) {
      agent.ai = { ...(agent.ai || {}), model: value };
    } else if (agent.ai) {
      delete agent.ai.model;
    }
  },
});

// Without a root URL in Settings, the server can only give the path.
const fullWebhookUrl = computed(() => (props.webhookUrl.startsWith('/') ? `${window.location.origin}${props.webhookUrl}` : props.webhookUrl));

function eventHint(name) {
  return (props.catalog.events || []).find((e) => e.name === name)?.description || '';
}
</script>

<style scoped>
.inspector {
  padding: 14px;
}

.inspector-head {
  align-items: center;
  display: flex;
  gap: 8px;
  margin-bottom: 4px;
}

.email-summary {
  background: rgba(var(--v-theme-on-surface), 0.04);
  border-radius: 6px;
  margin-bottom: 8px;
  overflow-wrap: anywhere;
  padding: 8px 10px;
}

.inspector-section {
  font-size: 13px;
  font-weight: 600;
  margin: 12px 0 8px;
}
</style>
