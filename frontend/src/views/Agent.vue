<template>
  <section class="agent-host" data-cy="agent-page">
    <lp-agent-page
      ref="page"
      class="agent-page"
      mention-trigger="@"
      :accept="AGENT_PAGE_ACCEPT"
      :max-file-size="MAX_BINARY_BYTES"
      :heading="title"
      :renamable="Boolean(convId)"
      :active="convId"
      :conversations="conversationRows"
      :contextReferences="inContext"
      :formatDate="shortStamp"
      :labels="labels"
      :models="models"
      :model="modelKey(ai)"
      :default-model="defaultModel"
      :entries="entries"
      :suggestions="suggestions"
      :running="busy"
      :error="error"
      :placeholder="$t('agent.placeholder', { at: '@' })"
      :empty-title="$t('agent.welcomeTitle')"
      :empty-text="$t('agent.welcomeHelp', { at: '@' })"
      @agent-submit="onSubmit"
      @agent-stop="stop"
      @agent-action="onAction"
      @agent-error-dismiss="error = ''"
      @agent-rename="rename($event.detail.title)"
      @agent-new-conversation="newChat"
      @agent-select-conversation="openConversation"
      @agent-delete-conversation="removeConversation($event.detail.id)"
      @agent-mention="picker = $event.detail.open ? { query: $event.detail.query } : null"
      @agent-mention-key="onMentionKey"
      @agent-reference-open="previewing = $event.detail.reference"
      @agent-model-change="setModel(modelFromKey($event.detail.model))"
    >
      <div slot="mention">
        <context-picker v-if="picker" ref="pickerRef" :query="picker.query" @pick="pickContext" @close="page?.closeMention()" />
      </div>
    </lp-agent-page>

    <review-modal
      v-if="reviewing"
      :key="`${reviewing.msg.id}:${reviewing.index}`"
      :part="reviewPart"
      :busy="busyPart === `${reviewing.msg.id}:${reviewing.index}`"
      :error="reviewError"
      @close="closeReview"
      @apply="applyFromReview"
      @reject="rejectFromReview"
    />
    <context-preview-modal v-if="previewing" :item="previewing" @close="previewing = null" />

  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, onBeforeUnmount, onMounted, ref, watch,
} from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  agentDelete, agentGet, agentPatch, agentPost,
} from '../agent/client';
import {
  CONTEXT_ICONS, MAX_BINARY_BYTES, contextInUse, isBinaryFile,
} from '../agent/parts.mjs';
import { streamAssistant } from '../assistant/client';
import { assistantContext } from '../assistant/context';
// Defines <lp-agent-page>, which draws the page; this component feeds it.
import { AGENT_PAGE_ACCEPT } from '../assistant/lp-agent-page';
import { modelFromKey, modelKey, useAgentModels } from '../assistant/models';
import ContextPicker from '../components/agent/ContextPicker.vue';
import ContextPreviewModal from '../components/agent/ContextPreviewModal.vue';
import ReviewModal from '../components/agent/ReviewModal.vue';
import { shortStamp } from '../utils/crm.mjs';

const MODEL_KEY = 'listpocket_agent_model';

const { proxy } = getCurrentInstance();
const $t = (...args) => proxy.$t(...args);
const route = useRoute();
const router = useRouter();

const page = ref(null);
const pickerRef = ref(null);
const conversations = ref([]);
const convId = ref(String(route.params.id || ''));
const title = ref('');
const ai = ref(lastModel());
const messages = ref([]);
const busy = ref(false);
const error = ref('');
const live = ref(null);
const picker = ref(null);
const reviewing = ref(null);
const reviewError = ref('');
const busyPart = ref('');
const previewing = ref(null);
// Context, Paste data and the model picker are the page's built-in composer menus.
const { models, defaultModel } = useAgentModels();
let turnId = '';
let controller = null;
// A new conversation gets its ID mid-send, and that mustn't reload the page.
let ownNavigation = false;

const inContext = computed(() => contextInUse(messages.value).map(({ type, id, label }) => ({ type, id, label })));
const reviewPart = computed(() => (reviewing.value ? reviewing.value.msg.parts[reviewing.value.index] : null));
const conversationRows = computed(() => conversations.value.map((c) => ({
  id: c.id, title: c.title, updated: c.updated, href: router.resolve({ name: 'agent', params: { id: c.id } }).href,
})));
const suggestions = computed(() => [$t('agent.suggest.overview'), $t('agent.suggest.replies'), $t('agent.suggest.workflow')]);
const labels = computed(() => ({
  addContext: $t('agent.context'),
  addFile: $t('agent.attach'),
  attach: $t('agent.attachFile'),
  attachFiles: $t('agent.attachFile'),
  defaultModel: $t('agent.defaultModel'),
  defaultModelNamed: (model) => $t('agent.defaultNamed', { model }),
  pasteContent: $t('agent.pasteContent'),
  pasteName: $t('agent.pasteName'),
  pasteText: $t('agent.pasteData'),
  searchModels: $t('agent.searchModels'),
  attachHint: $t('agent.attachFile'),
  cancel: $t('globals.buttons.cancel'),
  conversations: $t('agent.conversations'),
  delete: $t('globals.buttons.delete'),
  deleteConversation: (name) => `${$t('globals.buttons.delete')} ${name}`,
  discard: $t('agent.reject'),
  inContext: $t('agent.inContext'),
  newChat: $t('agent.newChat'),
  noConversations: $t('agent.noConversations'),
  open: $t('agent.open'),
  rename: $t('agent.rename'),
  review: $t('agent.review.title'),
  send: $t('agent.send'),
  stop: $t('agent.stop'),
  titleLabel: $t('agent.titleLabel'),
  untitled: $t('agent.newChat'),
  usedTools: (n) => $t('agent.usedTools', { n }),
  view: $t('agent.view'),
}));

// The page compares entries by value, so they're plain copies.
const entries = computed(() => {
  const rows = [];
  messages.value.forEach((msg) => {
    if (msg.role === 'user') {
      rows.push({
        id: msg.id, type: 'text', role: 'user', content: msg.content, references: (msg.context || []).map(({ type, id, label }) => ({ type, id, label })),
      });
      return;
    }
    const tools = (msg.parts || []).filter((p) => p.type === 'tool');
    if (tools.length) {
      rows.push({
        id: `${msg.id}:tools`,
        type: 'tools',
        label: $t('agent.usedTools', { n: tools.length }),
        tools: tools.map((t, i) => ({
          id: String(i), name: t.name, status: t.error ? 'failed' : 'completed', ...(t.error ? { detail: t.error } : {}),
        })),
      });
    }
    if (msg.content) rows.push({ id: `${msg.id}:text`, type: 'text', role: 'assistant', content: msg.content });
    (msg.parts || []).forEach((part, index) => {
      if (part.type !== 'proposal') return;
      const key = `${msg.id}:${index}`;
      rows.push({
        id: `${msg.id}:proposal:${index}`, type: 'proposal', actionId: key, proposal: JSON.parse(JSON.stringify(part)), busy: busyPart.value === key, running: busy.value,
      });
    });
  });
  if (live.value) {
    if (live.value.tools.length) {
      rows.push({
        id: 'live:tools', type: 'tools', label: $t('agent.usedTools', { n: live.value.tools.length }), tools: live.value.tools.map((t) => ({ id: t.id, name: t.name, status: t.status })),
      });
    }
    rows.push({ id: 'live:text', type: 'text', role: 'assistant', content: live.value.text });
  }
  return rows;
});

function lastModel() {
  try {
    return JSON.parse(localStorage.getItem(MODEL_KEY) || 'null');
  } catch {
    return null;
  }
}

async function loadList() {
  try {
    const data = await agentGet('agent/conversations');
    conversations.value = Array.isArray(data) ? data : [];
  } catch (err) {
    error.value = err.message;
  }
}

async function loadConversation() {
  if (!convId.value) {
    messages.value = [];
    title.value = '';
    ai.value = lastModel();
    return;
  }
  try {
    const data = await agentGet(`agent/conversations/${convId.value}`);
    messages.value = data.messages || [];
    title.value = data.title;
    ai.value = data.ai || null;
    page.value?.scrollToBottom({ force: true });
  } catch (err) {
    error.value = err.message;
  }
}

function newChat() {
  router.push({ name: 'agent' });
}

function openConversation(event) {
  // The page renders real links; navigate inside the app instead of reloading.
  event.preventDefault();
  router.push({ name: 'agent', params: { id: event.detail.id } });
}

async function setModel(next) {
  ai.value = next;
  try {
    localStorage.setItem(MODEL_KEY, JSON.stringify(next));
  } catch {
    // The choice still applies to this conversation.
  }
  if (!convId.value) return;
  try {
    await agentPatch(`agent/conversations/${convId.value}`, next ? { ai: next } : { reset_ai: true });
  } catch (err) {
    error.value = err.message;
  }
}

async function ensureConversation() {
  if (convId.value) return convId.value;
  const conv = await agentPost('agent/conversations', ai.value ? { ai: ai.value } : {});
  convId.value = conv.id;
  ownNavigation = true;
  router.replace({ name: 'agent', params: { id: conv.id } });
  conversations.value = [conv, ...conversations.value];
  return conv.id;
}

async function listen(id) {
  let after = 0;
  let finished = false;
  for (let attempt = 0; attempt < 4 && !finished; attempt += 1) {
    try {
      // eslint-disable-next-line no-await-in-loop
      await streamAssistant(id, after, (event) => {
        after = event.id;
        if (event.type === 'text_delta') live.value.text += event.data.text;
        else if (event.type === 'tool') {
          const known = live.value.tools.find((t) => t.id === event.data.id);
          if (known) Object.assign(known, event.data);
          else live.value.tools.push({ ...event.data });
        } else if (event.type === 'error') error.value = event.data.message;
        else if (event.type === 'done') finished = true;
      }, controller.signal);
    } catch (err) {
      if (err.name === 'AbortError') return;
      if (attempt === 3) error.value = err.message;
    }
  }
}

// Files are uploaded when the message is sent, then go to the agent as context.
async function upload(attachment, conversation) {
  const { file } = attachment;
  const binary = isBinaryFile(file);
  const content = binary ? await attachment.base64() : await file.text();
  const out = await agentPost('agent/attachments', {
    name: attachment.name, mime: file.type || 'text/plain', content, binary, conversation,
  });
  return { type: 'file', id: out.id, label: out.name };
}

function onSubmit(event) {
  if (busy.value) {
    event.preventDefault();
    return;
  }
  const { text, attachments, references } = event.detail;
  send(text, attachments, references);
}

async function send(message, attachments = [], references = []) {
  error.value = '';
  busy.value = true;
  const refs = references.map(({ type, id, label }) => ({ type, id, label }));
  messages.value = [...messages.value, {
    id: `local-${Date.now()}`, role: 'user', content: message, context: [...refs, ...attachments.map((a) => ({ type: 'file', id: a.id, label: a.name }))],
  }];
  live.value = { text: '', tools: [] };
  controller = new AbortController();
  let uploaded = false;
  try {
    const id = await ensureConversation();
    const files = [];
    for (const attachment of attachments) {
      // eslint-disable-next-line no-await-in-loop
      files.push(await upload(attachment, id));
    }
    uploaded = true;
    const context = [...refs, ...files];
    const out = await agentPost(`agent/conversations/${id}/turns`, { message, context, path: route.path });
    turnId = out.turn;
    if (out.title) title.value = out.title;
    await listen(out.turn);
  } catch (err) {
    if (err.name !== 'AbortError') {
      error.value = err.message;
      // Give the message back so it can be sent again.
      if (page.value && !page.value.draft) page.value.draft = message;
      if (page.value && !page.value.references.length) page.value.references = refs;
      if (page.value && !uploaded && !page.value.attachments.length) page.value.addFiles(attachments.map((a) => a.file));
    }
  } finally {
    live.value = null;
    busy.value = false;
    turnId = '';
    await loadConversation();
    loadList();
  }
}

async function stop() {
  try {
    if (turnId) await agentPost(`ai/assistant/turns/${turnId}/cancel`);
  } catch (err) {
    error.value = err.message;
  }
}

function onMentionKey(event) {
  if (pickerRef.value?.key(event.detail.event)) event.preventDefault();
}

function pickContext(item) {
  page.value?.addReference({ type: item.type, id: item.id, label: item.label });
}

async function rename(next) {
  title.value = next;
  if (!convId.value) return;
  try {
    await agentPatch(`agent/conversations/${convId.value}`, { title: next });
    loadList();
  } catch (err) {
    error.value = err.message;
  }
}

async function removeConversation(id) {
  try {
    await agentDelete(`agent/conversations/${id}`);
    conversations.value = conversations.value.filter((x) => x.id !== id);
    if (id === convId.value) newChat();
  } catch (err) {
    error.value = err.message;
  }
}

function findPart(actionId) {
  const cut = actionId.lastIndexOf(':');
  const msg = messages.value.find((m) => m.id === actionId.slice(0, cut));
  const index = Number(actionId.slice(cut + 1));
  return msg && msg.parts?.[index] ? { msg, index } : null;
}

function onAction(event) {
  const target = findPart(event.detail.id);
  if (!target) return;
  if (event.detail.action === 'apply') applyPart(target.msg, target.index);
  else if (event.detail.action === 'discard') rejectPart(target.msg, target.index);
  else if (event.detail.action === 'review') reviewing.value = target;
}

async function applyPart(msg, index, args) {
  const key = `${msg.id}:${index}`;
  busyPart.value = key;
  reviewError.value = '';
  try {
    const part = await agentPost(`agent/messages/${msg.id}/parts/${index}/apply`, args ? { arguments: args } : {});
    msg.parts[index] = part;
    if (part.status === 'failed') {
      throw new Error(part.error || $t('agent.applyFailed'));
    }
    window.dispatchEvent(new CustomEvent('assistant-applied', { detail: { tool: part.tool } }));
    return true;
  } catch (err) {
    if (reviewing.value) reviewError.value = err.message;
    else error.value = err.message;
    return false;
  } finally {
    busyPart.value = '';
  }
}

async function rejectPart(msg, index) {
  try {
    msg.parts[index] = await agentPost(`agent/messages/${msg.id}/parts/${index}/reject`);
  } catch (err) {
    error.value = err.message;
  }
}

function closeReview() {
  reviewing.value = null;
  reviewError.value = '';
}

async function applyFromReview(args) {
  const { msg, index } = reviewing.value;
  if (await applyPart(msg, index, args)) closeReview();
}

async function rejectFromReview() {
  const { msg, index } = reviewing.value;
  await rejectPart(msg, index);
  closeReview();
}

watch(() => route.params.id, (id) => {
  if (ownNavigation) {
    ownNavigation = false;
    return;
  }
  convId.value = String(id || '');
  if (page.value) page.value.references = [];
  loadConversation();
});

onMounted(() => {
  // This page does what the side panel does, with more room.
  assistantContext.open = false;
  loadList();
  loadConversation();
  // A page can link here with something already attached.
  const { ref: type, id, label } = route.query;
  if (type && id && CONTEXT_ICONS[type]) page.value.references = [{ type, id, label: label || id }];
});

onBeforeUnmount(() => controller?.abort());
</script>

<style scoped>
.agent-host {
  height: calc(100vh - 112px);
  min-height: 520px;
}

.agent-page {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 12px;
  display: block;
  height: 100%;
  overflow: hidden;
}

</style>
