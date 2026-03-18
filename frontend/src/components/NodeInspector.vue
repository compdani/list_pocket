<script setup>
/* eslint-disable */
import { computed } from "vue";

const props = defineProps({
  node: { type: Object, default: null },
});

const emit = defineEmits(["captureSchema", "remove", "save", "saveLabel"]);

const triggerMode = computed(() => String(configValue("mode") ?? "manual"));
const fields = computed(() => {
  const schema = props.node?.data?.schema ?? [];
  if (props.node?.data?.type !== "trigger") {
    return schema;
  }

  const hiddenByMode = new Map([
    ["manual", ["path", "tagName", "payloadField", "samplePayload", "payloadSchema", "signatureHeader", "secretRef", "contactStrategy", "contactKey", "ownerField"]],
    ["webhook", ["tagName"]],
    ["tag_added", ["path", "payloadField", "samplePayload", "payloadSchema", "signatureHeader", "secretRef", "contactStrategy", "contactKey", "ownerField"]],
    ["tag_removed", ["path", "payloadField", "samplePayload", "payloadSchema", "signatureHeader", "secretRef", "contactStrategy", "contactKey", "ownerField"]],
  ]);

  const hidden = new Set(hiddenByMode.get(triggerMode.value) ?? []);
  return schema.filter((field) => !hidden.has(field.key));
});
const contextTokens = computed(() => describeContextTokens(props.node?.data?.type, triggerMode.value));
const canCaptureSchema = computed(() => props.node?.data?.type === "trigger" && triggerMode.value === "webhook");

function saveValue(key, event) {
  emit("save", key, event.target?.value ?? "");
}

function saveLabel(event) {
  emit("saveLabel", event.target?.value ?? "");
}

function configValue(key) {
  return props.node?.data?.config?.[key];
}

function mapEntries(key) {
  const value = configValue(key);
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return [];
  }

  return Object.entries(value).map(([entryKey, entryValue]) => ({
    key: entryKey,
    value: typeof entryValue === "string" ? entryValue : JSON.stringify(entryValue),
  }));
}

function updateMapEntry(configKey, index, field, event) {
  const entries = mapEntries(configKey);
  const nextEntries = entries.map((entry, entryIndex) => (
    entryIndex === index ? { ...entry, [field]: event.target?.value ?? "" } : entry
  ));
  emitMap(configKey, nextEntries);
}

function addMapEntry(configKey) {
  emitMap(configKey, [...mapEntries(configKey), { key: "", value: "" }]);
}

function removeMapEntry(configKey, index) {
  emitMap(configKey, mapEntries(configKey).filter((_, entryIndex) => entryIndex !== index));
}

function emitMap(configKey, entries) {
  const nextValue = entries.reduce((accumulator, entry) => {
    const trimmedKey = entry.key.trim();
    if (!trimmedKey) {
      return accumulator;
    }

    accumulator[trimmedKey] = entry.value;
    return accumulator;
  }, {});

  emit("save", configKey, nextValue);
}

function describeContextTokens(type, mode) {
  const shared = ["ctx.run.triggerPayload", "ctx.previous", "ctx.contact", "ctx.company", "ctx.run.users", "ctx.run.events"];

  switch (type) {
    case "trigger":
      if (mode === "tag_added" || mode === "tag_removed") {
        return ["ctx.contact", "ctx.run.triggerPayload.tag", "ctx.run.triggerPayload.tagsBefore", "ctx.run.triggerPayload.tagsAfter", "ctx.run.users"];
      }
      if (mode === "manual") {
        return ["ctx.run.triggerPayload", "ctx.run.users", "ctx.contact (optional)"];
      }
      return ["ctx.run.triggerPayload", "ctx.run.triggerSchema", "ctx.run.security", "ctx.run.users", "ctx.contact (optional)"];
    case "event_start":
      return ["ctx.run.events", "ctx.run.triggerPayload", "ctx.previous"];
    case "wait_until":
      return ["ctx.run.events", "ctx.previous", "ctx.run.triggerPayload"];
    default:
      return shared;
  }
}
</script>

<template>
  <aside class="panel detail-panel node-inspector">
    <div class="panel-header node-summary">
      <div class="node-summary-copy">
        <span class="builder-eyebrow">Node</span>
        <h2>{{ node?.data?.label ?? "Node" }}</h2>
        <p v-if="node">{{ node.data?.description }}</p>
        <p v-else>Select a step to edit its configuration.</p>
      </div>
      <div v-if="node" class="node-summary-actions">
        <button v-if="canCaptureSchema" type="button" class="ghost-button" @click="emit('captureSchema')">
          Infer Schema
        </button>
        <button type="button" class="danger-button" @click="emit('remove')">Delete Node</button>
      </div>
    </div>

    <div v-if="node" class="inspector-fields">
      <div class="form-field-card">
        <div class="form-field-header">
          <label class="form-field-label" for="node-field-label">Nickname</label>
          <p class="field-help">This is the display name shown on the canvas and in run logs.</p>
        </div>
        <input
          id="node-field-label"
          :value="node.data?.label ?? ''"
          placeholder="Set Event Start"
          @input="saveLabel($event)"
        />
      </div>

      <div
        v-for="field in fields"
        :key="field.key"
        class="form-field-card"
        :class="{ 'form-field-card-wide': field.kind === 'textarea' || field.kind === 'kv_map' }"
      >
        <div class="form-field-header">
          <label class="form-field-label" :for="`node-field-${field.key}`">{{ field.label }}</label>
          <p v-if="field.description" class="field-help">{{ field.description }}</p>
        </div>

        <textarea
          v-if="field.kind === 'textarea'"
          :id="`node-field-${field.key}`"
          :value="configValue(field.key) ?? ''"
          rows="6"
          :placeholder="field.placeholder"
          @input="saveValue(field.key, $event)"
        />
        <div v-else-if="field.kind === 'kv_map'" class="map-field-editor">
          <div v-if="mapEntries(field.key).length" class="map-field-list">
            <div v-for="(entry, index) in mapEntries(field.key)" :key="`${field.key}:${index}`" class="map-field-row">
              <input :value="entry.key" placeholder="fieldName" @input="updateMapEntry(field.key, index, 'key', $event)" />
              <input :value="entry.value" :placeholder="field.placeholder ?? 'previous.someField'" @input="updateMapEntry(field.key, index, 'value', $event)" />
              <button type="button" class="ghost-button map-remove-button" @click="removeMapEntry(field.key, index)">
                Remove
              </button>
            </div>
          </div>
          <button type="button" class="ghost-button" @click="addMapEntry(field.key)">Add Field</button>
        </div>
        <select
          v-else-if="field.kind === 'select'"
          :id="`node-field-${field.key}`"
          :value="configValue(field.key) ?? ''"
          @change="saveValue(field.key, $event)"
        >
          <option v-for="option in field.options" :key="option" :value="option">{{ option }}</option>
        </select>
        <input
          v-else
          :id="`node-field-${field.key}`"
          :value="configValue(field.key) ?? ''"
          :placeholder="field.placeholder"
          @input="saveValue(field.key, $event)"
        />
      </div>

      <div class="context-help">
        <div class="context-help-header">
          <strong>Available Context</strong>
          <p>Use these values in scripts, branches, and field mappings.</p>
        </div>
        <div class="context-token-list">
          <span v-for="token in contextTokens" :key="token" class="context-token">{{ token }}</span>
        </div>
      </div>
    </div>
  </aside>
</template>
