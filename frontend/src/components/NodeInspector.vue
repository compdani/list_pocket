<script setup>
/* eslint-disable */
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { getSubscriber, getTemplate, getTemplates, searchSubscribers } from "../api";
import RichtextEditor from "./RichtextEditor.vue";

const props = defineProps({
  contacts: { type: Array, default: () => [] },
  node: { type: Object, default: null },
});

const emit = defineEmits(["save", "saveLabel"]);

const triggerMode = computed(() => String(configValue("mode") ?? "manual"));
const httpBodyMode = computed(() => String(configValue("bodyMode") ?? "source_path"));
const isTransactionalEmailNode = computed(() => props.node?.data?.type === "send_transactional_email");
const showDemoContactPicker = computed(() => props.node?.data?.type === "trigger" && (triggerMode.value === "tag_added" || triggerMode.value === "tag_removed"));
const demoContactItems = ref([]);
const demoContactLoading = ref(false);
const demoContactSearch = ref("");
const templateItems = ref([]);
const templateLoading = ref(false);
const localMapDrafts = ref({});
const localRichtextDrafts = ref({});
let demoContactSearchTimer;
let richtextSaveTimers = {};

const sortedContacts = computed(() => [...props.contacts].sort((left, right) => String(left.fullName || left.email || "").localeCompare(String(right.fullName || right.email || ""))));
const selectedDemoContactId = computed(() => String(configValue("demoContactId") ?? "").trim());
const demoContactOptions = computed(() => {
  const options = [...demoContactItems.value];
  for (const contact of sortedContacts.value) {
    if (!options.some((entry) => entry.id === contact.id)) {
      options.push(normalizeContactOption(contact));
    }
  }
  return options;
});
const selectedTemplateId = computed(() => String(configValue("templateId") ?? "").trim());
const selectedTemplateLabel = computed(() => {
  const match = templateItems.value.find((item) => item.id === selectedTemplateId.value);
  return match?.title ?? "";
});
const fields = computed(() => {
  const schema = props.node?.data?.schema ?? [];
  if (props.node?.data?.type === "http_request") {
    const hidden = new Set();
    if (httpBodyMode.value === "custom_map") {
      hidden.add("sourcePath");
    } else {
      hidden.add("bodyMap");
    }
    return schema.filter((field) => !hidden.has(field.key));
  }

  if (props.node?.data?.type === "send_transactional_email") {
    const hidden = new Set(["templateId"]);
    return schema.filter((field) => !hidden.has(field.key));
  }

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

function saveValue(key, event) {
  emit("save", key, event.target?.value ?? "");
}

function saveLabel(event) {
  emit("saveLabel", event.target?.value ?? "");
}

function normalizeContactOption(contact) {
  if (!contact) {
    return { id: "", title: "" };
  }

  const fullName = String(contact.fullName ?? contact.name ?? "").trim();
  const email = String(contact.email ?? "").trim();
  const tagSummary = Array.isArray(contact.tags) && contact.tags.length ? ` · ${contact.tags.slice(0, 3).join(", ")}` : "";
  return {
    id: String(contact.id ?? ""),
    title: fullName || email || "Unnamed contact",
    subtitle: email && email !== fullName ? `${email}${tagSummary}` : tagSummary.replace(/^ · /, ""),
  };
}

function normalizeTemplateOption(template) {
  if (!template) {
    return { id: "", title: "" };
  }

  const subject = String(template.subject ?? "").trim();
  return {
    id: String(template.id ?? ""),
    title: String(template.name ?? "").trim() || String(template.id ?? ""),
    subtitle: subject,
  };
}

async function ensureSelectedDemoContactLoaded() {
  if (!selectedDemoContactId.value || demoContactOptions.value.some((entry) => entry.id === selectedDemoContactId.value)) {
    return;
  }

  try {
    const subscriber = await getSubscriber(selectedDemoContactId.value);
    demoContactItems.value = [normalizeContactOption(subscriber), ...demoContactItems.value];
  } catch {
    // Keep the picker usable even if the stored contact no longer exists.
  }
}

async function ensureSelectedTemplateLoaded() {
  if (!selectedTemplateId.value || templateItems.value.some((entry) => entry.id === selectedTemplateId.value)) {
    return;
  }

  try {
    const template = await getTemplate(selectedTemplateId.value);
    templateItems.value = [normalizeTemplateOption(template), ...templateItems.value];
  } catch {
    // Keep manual record-id entry valid even if the template is gone.
  }
}

async function loadTransactionalTemplates() {
  if (!isTransactionalEmailNode.value) {
    return;
  }

  templateLoading.value = true;
  try {
    const response = await getTemplates();
    templateItems.value = Array.isArray(response)
      ? response.filter((item) => item.type === "tx").map(normalizeTemplateOption)
      : [];
    await ensureSelectedTemplateLoaded();
  } finally {
    templateLoading.value = false;
  }
}

async function loadDemoContactOptions(search = "") {
  if (!showDemoContactPicker.value) {
    return;
  }

  demoContactLoading.value = true;
  try {
    const response = await searchSubscribers({
      search,
      page: 1,
      per_page: 10,
      order_by: "updated_at",
      order: "DESC",
    });
    demoContactItems.value = Array.isArray(response?.results)
      ? response.results.map(normalizeContactOption)
      : [];
    await ensureSelectedDemoContactLoaded();
  } finally {
    demoContactLoading.value = false;
  }
}

function queueDemoContactSearch(value) {
  demoContactSearch.value = typeof value === "string" ? value : "";
  window.clearTimeout(demoContactSearchTimer);
  demoContactSearchTimer = window.setTimeout(() => {
    void loadDemoContactOptions(demoContactSearch.value.trim());
  }, 250);
}

function saveDemoContact(value) {
  emit("save", "demoContactId", value ? String(value) : "");
}

function saveTemplateSelection(value) {
  if (value && typeof value === "object") {
    const objectID = String(value.id ?? value.value ?? "").trim();
    emit("save", "templateId", objectID);
    return;
  }
  if (typeof value === "string") {
    emit("save", "templateId", value.trim());
    return;
  }
  emit("save", "templateId", value ? String(value) : "");
}

function openTemplatesWindow(query = "") {
  window.open(`/admin/campaigns/templates${query}`, "_blank", "noopener");
}

function createTransactionalTemplate() {
  openTemplatesWindow("?open=new&type=tx");
}

function editTransactionalTemplate() {
  if (!selectedTemplateId.value) {
    return;
  }
  openTemplatesWindow(`?edit=${encodeURIComponent(selectedTemplateId.value)}`);
}

function browseTransactionalTemplates() {
  openTemplatesWindow("");
}

function configValue(key) {
  return props.node?.data?.config?.[key];
}

function configMapEntries(key) {
  const value = configValue(key);
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return [];
  }

  return Object.entries(value).map(([entryKey, entryValue]) => ({
    key: entryKey,
    value: typeof entryValue === "string" ? entryValue : JSON.stringify(entryValue),
  }));
}

function richtextValue(key) {
  if (Object.prototype.hasOwnProperty.call(localRichtextDrafts.value, key)) {
    return localRichtextDrafts.value[key];
  }
  return String(configValue(key) ?? "");
}

function queueRichtextSave(key, value) {
  localRichtextDrafts.value = {
    ...localRichtextDrafts.value,
    [key]: value,
  };

  window.clearTimeout(richtextSaveTimers[key]);
  richtextSaveTimers[key] = window.setTimeout(() => {
    emit("save", key, value);
  }, 250);
}

function mapEntries(key) {
  if (Object.prototype.hasOwnProperty.call(localMapDrafts.value, key)) {
    return localMapDrafts.value[key];
  }
  return configMapEntries(key);
}

function setMapEntries(configKey, entries) {
  localMapDrafts.value = {
    ...localMapDrafts.value,
    [configKey]: entries,
  };
}

function updateMapEntry(configKey, index, field, event) {
  const entries = mapEntries(configKey);
  const nextEntries = entries.map((entry, entryIndex) => (
    entryIndex === index ? { ...entry, [field]: event.target?.value ?? "" } : entry
  ));
  setMapEntries(configKey, nextEntries);
  emitMap(configKey, nextEntries);
}

function addMapEntry(configKey) {
  const nextEntries = [...mapEntries(configKey), { key: "", value: "" }];
  setMapEntries(configKey, nextEntries);
  emitMap(configKey, nextEntries);
}

function removeMapEntry(configKey, index) {
  const nextEntries = mapEntries(configKey).filter((_, entryIndex) => entryIndex !== index);
  setMapEntries(configKey, nextEntries);
  emitMap(configKey, nextEntries);
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
    case "campaign_launch":
      return ["ctx.previous.campaignId", "ctx.run.triggerPayload", "ctx.contact"];
    case "send_transactional_email":
      return [
        "contact.id",
        "contact.email",
        "contact.firstName",
        "previous",
        "run.triggerPayload",
        "run.events",
        "{{ template \"content\" . }}",
        "{{contact.firstName}}",
        "{{data.orderId}}",
      ];
    default:
      return shared;
  }
}

watch(showDemoContactPicker, (visible) => {
  if (visible) {
    void loadDemoContactOptions(demoContactSearch.value.trim());
  }
});

watch(isTransactionalEmailNode, (visible) => {
  if (visible) {
    void loadTransactionalTemplates();
  }
});

watch(
  () => props.node?.id,
  () => {
    localMapDrafts.value = {};
    localRichtextDrafts.value = {};
  },
  { immediate: true }
);

watch(selectedDemoContactId, () => {
  void ensureSelectedDemoContactLoaded();
});

watch(selectedTemplateId, () => {
  void ensureSelectedTemplateLoaded();
});

onMounted(() => {
  if (showDemoContactPicker.value) {
    void loadDemoContactOptions("");
  }
  if (isTransactionalEmailNode.value) {
    void loadTransactionalTemplates();
  }
});

onBeforeUnmount(() => {
  window.clearTimeout(demoContactSearchTimer);
  Object.values(richtextSaveTimers).forEach((timer) => window.clearTimeout(timer));
  richtextSaveTimers = {};
});
</script>

<template>
  <aside class="panel detail-panel node-inspector">
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

      <div v-if="isTransactionalEmailNode" class="form-field-card form-field-card-wide">
        <div class="form-field-header">
          <label class="form-field-label" for="node-field-templateId">Template</label>
          <p class="field-help">Pick a transactional template, or open the template manager to create or edit one in a new tab.</p>
        </div>
        <v-combobox
          id="node-field-templateId"
          :model-value="selectedTemplateId || null"
          :items="templateItems"
          :loading="templateLoading"
          :menu-props="{ maxHeight: 320 }"
          item-title="title"
          item-value="id"
          variant="outlined"
          density="comfortable"
          placeholder="Select or paste a transactional template record id"
          :return-object="false"
          clearable
          hide-details
          @update:model-value="saveTemplateSelection"
        >
          <template #item="{ props: itemProps, item }">
            <v-list-item v-bind="itemProps" :title="item.raw.title" :subtitle="item.raw.subtitle || item.raw.id" />
          </template>
          <template #selection="{ item }">
            <span class="demo-contact-selection">{{ item.raw?.title || item.title || selectedTemplateLabel }}</span>
          </template>
        </v-combobox>
        <div class="template-picker-actions">
          <button type="button" class="ghost-button" @click="browseTransactionalTemplates">Browse Templates</button>
          <button type="button" class="ghost-button" @click="createTransactionalTemplate">New Template</button>
          <button type="button" class="ghost-button" :disabled="!selectedTemplateId" @click="editTransactionalTemplate">Edit Selected</button>
        </div>
      </div>

      <div
        v-for="field in fields"
        :key="field.key"
        class="form-field-card"
        :class="{ 'form-field-card-wide': field.kind === 'textarea' || field.kind === 'kv_map' || field.kind === 'richtext' }"
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
        <div v-else-if="field.kind === 'richtext'" class="richtext-field-wrap">
          <RichtextEditor
            :model-value="richtextValue(field.key)"
            :preserve-go-template="true"
            @update:model-value="queueRichtextSave(field.key, $event)"
          />
        </div>
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

      <div v-if="showDemoContactPicker" class="form-field-card form-field-card-wide">
        <div class="form-field-header">
          <label class="form-field-label" for="node-field-demoContactId">Demo Contact</label>
          <p class="field-help">Search subscribers by name or email. The selected contact will be used for tag-added and tag-removed test runs.</p>
        </div>
        <v-autocomplete
          id="node-field-demoContactId"
          :model-value="selectedDemoContactId || null"
          :items="demoContactOptions"
          :loading="demoContactLoading"
          :search="demoContactSearch"
          item-title="title"
          item-value="id"
          variant="outlined"
          density="comfortable"
          placeholder="Search contacts by name or email"
          clearable
          hide-details
          no-filter
          @update:search="queueDemoContactSearch"
          @update:model-value="saveDemoContact"
        >
          <template #prepend-item>
            <div class="demo-contact-hint">Leave this blank to use the most recently updated contact.</div>
          </template>
          <template #item="{ props: itemProps, item }">
            <v-list-item v-bind="itemProps" :title="item.raw.title" :subtitle="item.raw.subtitle" />
          </template>
          <template #selection="{ item }">
            <span class="demo-contact-selection">{{ item.raw.title }}</span>
          </template>
          <template #no-data>
            <div class="demo-contact-hint">No subscribers matched that search.</div>
          </template>
        </v-autocomplete>
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
