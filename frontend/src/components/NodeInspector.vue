<script setup>
/* eslint-disable */
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useStore } from "vuex";
import dayjs from "dayjs";
import dayjsTimezone from "dayjs/plugin/timezone";
import dayjsUtc from "dayjs/plugin/utc";
import {
  ensureSubscriberTags,
  getSubscriber,
  getSubscriberTagsCatalog,
  getTemplate,
  getTemplates,
  searchSubscribers,
} from "../api";
import { useSenderLookup } from "../composables/useSenderLookup";
import RichtextEditor from "./RichtextEditor.vue";

dayjs.extend(dayjsUtc);
dayjs.extend(dayjsTimezone);

function isValidIanaTimeZone(id) {
  const trimmed = String(id ?? "").trim();
  if (!trimmed) {
    return false;
  }
  try {
    Intl.DateTimeFormat(undefined, { timeZone: trimmed });
    return true;
  } catch {
    return false;
  }
}

const props = defineProps({
  contacts: { type: Array, default: () => [] },
  node: { type: Object, default: null },
});

const emit = defineEmits(["save", "saveLabel"]);
const store = useStore();

const triggerMode = computed(() => String(configValue("mode") ?? "manual"));
const httpBodyMode = computed(() => String(configValue("bodyMode") ?? "source_path"));
const isTransactionalEmailNode = computed(() => props.node?.data?.type === "send_transactional_email");
const showDemoContactPicker = computed(() => props.node?.data?.type === "trigger" && (triggerMode.value === "tag_added" || triggerMode.value === "tag_removed"));
const demoContactItems = ref([]);
const demoContactLoading = ref(false);
const demoContactSearch = ref("");
const templateItems = ref([]);
const templateLoading = ref(false);
const lastAutoFromEmail = ref("");
const subscriberTagOptions = ref([]);
const pendingCatalogTag = ref(null);
const tagCatalogConfirmLoading = ref(false);
const localMapDrafts = ref({});
const localRichtextDrafts = ref({});
const activeRichtextDraftNodeId = ref("");
let demoContactSearchTimer;
let tagCatalogCheckTimer;

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
const selectedMessenger = computed(() => String(configValue("messenger") ?? "").trim() || "email");
const selectedFromEmail = computed(() => String(configValue("fromEmail") ?? "").trim());
const serverConfig = computed(() => store.state.serverConfig || {});
const { availableMessengers, availableFromAddresses, defaultFromEmail } = useSenderLookup(serverConfig, selectedMessenger);
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
    const hidden = new Set(["templateId", "messenger", "fromEmail"]);
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

const currentTagNameNormalized = computed(() => normalizeTagInput(configValue("tagName")));

const showTagCatalogActions = computed(() => {
  const pending = pendingCatalogTag.value;
  if (!pending) {
    return false;
  }
  return pending === currentTagNameNormalized.value && !tagExistsInCatalog(pending);
});

const isEventStartNode = computed(() => props.node?.data?.type === "event_start");

const eventStartTimezone = computed(() => {
  const tz = String(configValue("timezone") ?? "").trim();
  if (tz && isValidIanaTimeZone(tz)) {
    return tz;
  }
  return "UTC";
});

const fallbackAtDatetimeLocal = computed(() => {
  if (!isEventStartNode.value) {
    return "";
  }
  const raw = String(configValue("fallbackAt") ?? "").trim();
  if (!raw) {
    return "";
  }
  const asUtc = dayjs.utc(raw);
  if (!asUtc.isValid()) {
    return "";
  }
  return asUtc.tz(eventStartTimezone.value).format("YYYY-MM-DDTHH:mm");
});

function isEventStartFallbackField(field) {
  return props.node?.data?.type === "event_start" && field?.key === "fallbackAt";
}

function saveFallbackAtPicker(value) {
  if (value === null || value === undefined || String(value).trim() === "") {
    emit("save", "fallbackAt", "");
    return;
  }
  const tz = eventStartTimezone.value;
  const parsed = dayjs.tz(String(value).trim(), "YYYY-MM-DDTHH:mm", tz);
  if (!parsed.isValid()) {
    return;
  }
  emit("save", "fallbackAt", parsed.utc().format("YYYY-MM-DDTHH:mm:ss[Z]"));
}

function saveValue(key, event) {
  const value = typeof event === "string" ? event : event?.target?.value ?? "";
  emit("save", key, value);
}

function normalizeTagInput(value) {
  if (value === null || value === undefined) {
    return "";
  }
  return String(value).trim();
}

function tagExistsInCatalog(normalized) {
  return subscriberTagOptions.value.some((tag) => tag.toLowerCase() === normalized.toLowerCase());
}

function queueTagCatalogCreateCheck() {
  window.clearTimeout(tagCatalogCheckTimer);
  tagCatalogCheckTimer = window.setTimeout(() => {
    const normalized = normalizeTagInput(configValue("tagName"));
    if (!normalized) {
      pendingCatalogTag.value = null;
      return;
    }
    if (tagExistsInCatalog(normalized)) {
      pendingCatalogTag.value = null;
      return;
    }
    pendingCatalogTag.value = normalized;
  }, 400);
}

function saveTagName(value) {
  const normalized = normalizeTagInput(value);
  emit("save", "tagName", normalized);
  window.clearTimeout(tagCatalogCheckTimer);
  pendingCatalogTag.value = null;
  if (!normalized) {
    return;
  }
  queueTagCatalogCreateCheck();
}

async function confirmCreateCatalogTag() {
  const name = normalizeTagInput(pendingCatalogTag.value || configValue("tagName"));
  if (!name || name !== currentTagNameNormalized.value) {
    pendingCatalogTag.value = null;
    return;
  }
  tagCatalogConfirmLoading.value = true;
  try {
    await ensureSubscriberTags([name], subscriberTagOptions.value);
    if (!tagExistsInCatalog(name)) {
      subscriberTagOptions.value = [...subscriberTagOptions.value, name].sort((left, right) => left.localeCompare(right));
    }
    pendingCatalogTag.value = null;
  } catch {
    // Keep editing uninterrupted if catalog sync fails.
  } finally {
    tagCatalogConfirmLoading.value = false;
  }
}

function dismissPendingCatalogTag() {
  pendingCatalogTag.value = null;
}

function saveLabel(event) {
  const value = typeof event === "string" ? event : event?.target?.value ?? "";
  emit("saveLabel", value);
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
    isDefault: template.isDefault === true,
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
    applyDefaultTemplateSelection();
    await ensureSelectedTemplateLoaded();
  } finally {
    templateLoading.value = false;
  }
}

function applyDefaultTemplateSelection() {
  if (!isTransactionalEmailNode.value || selectedTemplateId.value) {
    return;
  }
  const defaultTemplate = templateItems.value.find((item) => item.isDefault) || templateItems.value[0];
  if (defaultTemplate?.id) {
    emit("save", "templateId", defaultTemplate.id);
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

async function loadSubscriberTagOptions() {
  try {
    const tags = await getSubscriberTagsCatalog();
    subscriberTagOptions.value = [...new Set(
      tags
        .map((tag) => String(tag || "").trim())
        .filter(Boolean),
    )].sort((left, right) => left.localeCompare(right));
  } catch {
    subscriberTagOptions.value = [];
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

function saveMessengerSelection(value) {
  const nextMessenger = String(value || "").trim() || "email";
  emit("save", "messenger", nextMessenger);
}

function saveFromEmailSelection(value) {
  if (value && typeof value === "object") {
    const fromEmail = String(value.id ?? value.value ?? value.title ?? "").trim();
    emit("save", "fromEmail", fromEmail);
    return;
  }
  emit("save", "fromEmail", String(value || "").trim());
}

function applyDefaultFromEmailForMessenger(force = false) {
  if (!isTransactionalEmailNode.value) {
    return;
  }
  const current = selectedFromEmail.value;
  const nextDefault = defaultFromEmail.value || "";
  const shouldReplace = force || !current || current === lastAutoFromEmail.value;
  if (!shouldReplace) {
    return;
  }
  emit("save", "fromEmail", nextDefault);
  lastAutoFromEmail.value = nextDefault;
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

function buildRichtextDraftsForNode(node) {
  if (!node?.data?.schema?.length) {
    return {};
  }

  const draftEntries = node.data.schema
    .filter((field) => field.kind === "richtext")
    .map((field) => [field.key, String(node?.data?.config?.[field.key] ?? "")]);

  return Object.fromEntries(draftEntries);
}

function syncRichtextDraftsForNode(node, forceReset = false) {
  const nodeId = String(node?.id ?? "");
  const nextDrafts = buildRichtextDraftsForNode(node);

  if (!nodeId) {
    localRichtextDrafts.value = {};
    activeRichtextDraftNodeId.value = "";
    return;
  }

  if (forceReset || activeRichtextDraftNodeId.value !== nodeId) {
    localRichtextDrafts.value = nextDrafts;
    activeRichtextDraftNodeId.value = nodeId;
    return;
  }

  // Keep in-progress editor text stable for existing keys, only seed new keys.
  const mergedDrafts = {};
  let hasChanges = false;
  Object.entries(nextDrafts).forEach(([key, value]) => {
    const hasLocalValue = Object.prototype.hasOwnProperty.call(localRichtextDrafts.value, key);
    mergedDrafts[key] = hasLocalValue ? localRichtextDrafts.value[key] : value;
    if (!hasLocalValue) {
      hasChanges = true;
    }
  });

  // Avoid replacing the draft object when schema keys did not change.
  const previousKeys = Object.keys(localRichtextDrafts.value);
  const nextKeys = Object.keys(mergedDrafts);
  if (previousKeys.length !== nextKeys.length) {
    hasChanges = true;
  } else if (!hasChanges) {
    hasChanges = previousKeys.some((key) => !Object.prototype.hasOwnProperty.call(mergedDrafts, key));
  }

  if (hasChanges) {
    localRichtextDrafts.value = mergedDrafts;
  }
}

async function flushPendingChanges() {
  Object.entries(localRichtextDrafts.value).forEach(([key, value]) => {
    emit("save", key, value);
  });
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
  const value = typeof event === "string" ? event : event?.target?.value ?? "";
  const nextEntries = entries.map((entry, entryIndex) => (
    entryIndex === index ? { ...entry, [field]: value } : entry
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
  const shared = ["run.triggerPayload", "previous", "contact", "company", "run.users", "run.events"];

  switch (type) {
    case "trigger":
      if (mode === "tag_added" || mode === "tag_removed") {
        return ["contact", "run.triggerPayload.tag", "run.triggerPayload.tagsBefore", "run.triggerPayload.tagsAfter", "run.users"];
      }
      if (mode === "manual") {
        return ["run.triggerPayload", "run.users", "contact (optional)"];
      }
      return ["run.triggerPayload", "run.triggerSchema", "run.security", "run.users", "contact (optional)"];
    case "event_start":
      return ["run.events", "run.triggerPayload", "previous"];
    case "wait_until":
      return ["run.events", "previous", "run.triggerPayload"];
    case "campaign_launch":
      return ["previous.campaignId", "run.triggerPayload", "contact"];
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
  (nextNodeId, previousNodeId) => {
    localMapDrafts.value = {};
    syncRichtextDraftsForNode(props.node, nextNodeId !== previousNodeId);
  },
  { immediate: true }
);

watch(
  () => props.node?.data?.schema,
  () => {
    if (!props.node?.id) {
      return;
    }
    syncRichtextDraftsForNode(props.node, false);
  }
);

watch(selectedDemoContactId, () => {
  void ensureSelectedDemoContactLoaded();
});

watch(selectedTemplateId, () => {
  void ensureSelectedTemplateLoaded();
});

watch(selectedMessenger, (nextMessenger, previousMessenger) => {
  if (!isTransactionalEmailNode.value || nextMessenger === previousMessenger) {
    return;
  }
  applyDefaultFromEmailForMessenger(true);
});

watch(
  () => props.node?.id,
  () => {
    if (!isTransactionalEmailNode.value) {
      return;
    }
    lastAutoFromEmail.value = selectedFromEmail.value;
    applyDefaultTemplateSelection();
  },
);

watch(triggerMode, (mode) => {
  if (mode !== "tag_added" && mode !== "tag_removed") {
    window.clearTimeout(tagCatalogCheckTimer);
    pendingCatalogTag.value = null;
  }
});

watch(
  () => props.node?.id,
  () => {
    window.clearTimeout(tagCatalogCheckTimer);
    pendingCatalogTag.value = null;
  },
);

onMounted(() => {
  if (showDemoContactPicker.value) {
    void loadDemoContactOptions("");
  }
  if (isTransactionalEmailNode.value) {
    if (!selectedMessenger.value) {
      emit("save", "messenger", "email");
    }
    applyDefaultFromEmailForMessenger(true);
    void loadTransactionalTemplates();
  }
  void loadSubscriberTagOptions();
});

onBeforeUnmount(() => {
  window.clearTimeout(demoContactSearchTimer);
  window.clearTimeout(tagCatalogCheckTimer);
});

defineExpose({
  flushPendingChanges,
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
        <v-text-field
          id="node-field-label"
          :model-value="node.data?.label ?? ''"
          placeholder="Set Event Start"
          hide-details
          @update:model-value="saveLabel($event || '')"
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
            <v-list-item
              v-bind="itemProps"
              :title="item.raw.title"
              :subtitle="item.raw.subtitle || item.raw.id"
            >
              <template v-if="item.raw.isDefault" #append>
                <v-chip size="x-small" color="primary" label>Default</v-chip>
              </template>
            </v-list-item>
          </template>
          <template #selection="{ item }">
            <span class="demo-contact-selection">{{ item.raw?.title || item.title || selectedTemplateLabel }}</span>
          </template>
        </v-combobox>
        <div class="template-picker-actions">
          <v-btn type="button" color="primary" variant="tonal" size="small" @click="browseTransactionalTemplates">Browse Templates</v-btn>
          <v-btn type="button" color="primary" variant="tonal" size="small" @click="createTransactionalTemplate">New Template</v-btn>
          <v-btn type="button" color="primary" variant="tonal" size="small" :disabled="!selectedTemplateId" @click="editTransactionalTemplate">Edit Selected</v-btn>
        </div>
      </div>

      <div v-if="isTransactionalEmailNode" class="form-field-card">
        <div class="form-field-header">
          <label class="form-field-label" for="node-field-messenger">Messenger</label>
          <p class="field-help">Select the configured messenger backend for this send.</p>
        </div>
        <v-select
          id="node-field-messenger"
          :model-value="selectedMessenger"
          :items="availableMessengers"
          variant="outlined"
          density="comfortable"
          hide-details
          @update:model-value="saveMessengerSelection"
        />
      </div>

      <div v-if="isTransactionalEmailNode" class="form-field-card form-field-card-wide">
        <div class="form-field-header">
          <label class="form-field-label" for="node-field-fromEmail">From Email Override</label>
          <p class="field-help">Defaults to the selected messenger sender. You can pick or paste a value.</p>
        </div>
        <v-combobox
          id="node-field-fromEmail"
          :model-value="selectedFromEmail || null"
          :items="availableFromAddresses"
          :menu-props="{ maxHeight: 280 }"
          variant="outlined"
          density="comfortable"
          clearable
          hide-details
          @update:model-value="saveFromEmailSelection"
        />
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

        <v-textarea
          v-if="field.kind === 'textarea'"
          :id="`node-field-${field.key}`"
          :model-value="configValue(field.key) ?? ''"
          rows="6"
          :placeholder="field.placeholder"
          hide-details
          @update:model-value="saveValue(field.key, $event || '')"
        />
        <div v-else-if="field.kind === 'richtext'" class="richtext-field-wrap">
          <RichtextEditor
            :key="`richtext:${props.node?.id ?? 'none'}:${field.key}`"
            v-model="localRichtextDrafts[field.key]"
            :preserve-go-template="true"
          />
        </div>
        <div v-else-if="field.kind === 'kv_map'" class="map-field-editor">
          <div v-if="mapEntries(field.key).length" class="map-field-list">
            <div v-for="(entry, index) in mapEntries(field.key)" :key="`${field.key}:${index}`" class="map-field-row">
              <v-text-field :model-value="entry.key" placeholder="fieldName" hide-details @update:model-value="updateMapEntry(field.key, index, 'key', $event || '')" />
              <v-text-field :model-value="entry.value" :placeholder="field.placeholder ?? 'previous.someField'" hide-details @update:model-value="updateMapEntry(field.key, index, 'value', $event || '')" />
              <v-btn type="button" color="primary" variant="outlined" density="comfortable" class="map-remove-button map-action-btn" @click="removeMapEntry(field.key, index)">Remove</v-btn>
            </div>
          </div>
          <v-btn type="button" color="primary" variant="tonal" density="comfortable" class="map-action-btn" @click="addMapEntry(field.key)">Add field</v-btn>
        </div>
        <v-text-field
          v-else-if="isEventStartFallbackField(field)"
          :id="`node-field-${field.key}`"
          :model-value="fallbackAtDatetimeLocal"
          type="datetime-local"
          variant="outlined"
          density="comfortable"
          hide-details
          clearable
          @update:model-value="saveFallbackAtPicker"
        />
        <v-combobox
          v-else-if="field.key === 'tagName'"
          :id="`node-field-${field.key}`"
          :model-value="configValue(field.key) ?? ''"
          :items="subscriberTagOptions"
          variant="outlined"
          density="comfortable"
          hide-details
          clearable
          @update:model-value="saveTagName"
        >
          <template v-if="showTagCatalogActions" #append-inner>
            <div class="tag-catalog-append" @click.stop @mousedown.stop>
              <v-btn
                type="button"
                icon="mdi-check"
                size="x-small"
                variant="text"
                color="success"
                :loading="tagCatalogConfirmLoading"
                aria-label="Add tag to catalog"
                @click="confirmCreateCatalogTag"
              />
              <v-btn
                type="button"
                icon="mdi-close"
                size="x-small"
                variant="text"
                :disabled="tagCatalogConfirmLoading"
                aria-label="Dismiss"
                @click="dismissPendingCatalogTag"
              />
            </div>
          </template>
        </v-combobox>
        <v-select
          v-else-if="field.kind === 'select'"
          :id="`node-field-${field.key}`"
          :model-value="configValue(field.key) ?? ''"
          :items="field.options || []"
          hide-details
          @update:model-value="saveValue(field.key, $event || '')"
        >
        </v-select>
        <v-text-field
          v-else
          :id="`node-field-${field.key}`"
          :model-value="configValue(field.key) ?? ''"
          :placeholder="field.placeholder"
          type="text"
          hide-details
          @update:model-value="saveValue(field.key, $event || '')"
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

<style scoped>
.tag-catalog-append {
  display: inline-flex;
  align-items: center;
  margin-inline-end: 2px;
}

.node-inspector :deep(.v-input) {
  width: 100%;
}

.inspector-fields {
  align-items: start;
}

.node-inspector :deep(.form-field-card) {
  align-content: start;
}

/* Keep label/help copy height consistent so controls align. */
.node-inspector :deep(.form-field-header) {
  min-height: 72px;
}

.map-action-btn {
  align-self: start;
}

.template-picker-actions :deep(.v-btn__content) {
  text-decoration: none;
}
</style>
