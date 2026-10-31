<template>
  <section class="agent-page" data-cy="agent-page" @dragover.prevent @drop.prevent="onDrop">
    <aside class="agent-convs" :aria-label="$t('agent.conversations')">
      <v-btn color="primary" variant="flat" class="text-none mb-2 agent-new" prepend-icon="mdi-plus" data-cy="agent-new" @click="newChat">{{ $t('agent.newChat') }}</v-btn>
      <div class="agent-conv-list">
        <p v-if="!conversations.length" class="text-caption text-medium-emphasis pa-2 mb-0">{{ $t('agent.noConversations') }}</p>
        <div v-for="c in conversations" :key="c.id" :class="['agent-conv', { 'agent-conv--active': c.id === convId }]" data-cy="agent-conv">
          <router-link :to="{ name: 'agent', params: { id: c.id } }" class="agent-conv-link" :title="c.title">{{ c.title }}</router-link>
          <template v-if="deleting === c.id">
            <v-btn size="x-small" color="error" variant="flat" class="text-none" data-cy="agent-conv-delete-confirm" @click="removeConversation(c)">{{ $t('globals.buttons.delete') }}</v-btn>
            <v-btn :aria-label="$t('globals.buttons.cancel')" icon="mdi-close" size="x-small" variant="text" @click="deleting = ''" />
          </template>
          <template v-else>
            <span class="agent-conv-when">{{ shortStamp(c.updated) }}</span>
            <v-btn class="agent-conv-del" :aria-label="$t('globals.buttons.delete')" icon="mdi-trash-can-outline" size="x-small" variant="text" data-cy="agent-conv-delete" @click="deleting = c.id" />
          </template>
        </div>
      </div>
    </aside>

    <div class="agent-chat">
      <header class="agent-head">
        <v-text-field
          v-if="renaming"
          v-model="title"
          density="compact"
          variant="outlined"
          hide-details
          autofocus
          class="agent-title-input"
          :label="$t('agent.titleLabel')"
          @blur="rename"
          @keydown.enter="rename"
        />
        <button v-else type="button" class="agent-title" :disabled="!convId" :title="$t('agent.rename')" data-cy="agent-title" @click="renaming = true">{{ title || $t('agent.newChat') }}</button>
        <v-spacer />
        <model-picker :value="ai" @change="setModel" />
      </header>
      <div v-if="inContext.length" class="agent-in-context" data-cy="agent-in-context">
        <span class="text-caption text-medium-emphasis">{{ $t('agent.inContext') }}</span>
        <button v-for="ref_ in inContext" :key="refKey(ref_)" type="button" class="agent-chip" @click="previewing = ref_">
          <v-icon :icon="icons[ref_.type]" size="13" /> <span class="text-truncate">{{ ref_.label || ref_.id }}</span>
        </button>
      </div>

      <div ref="scroller" class="agent-log" aria-live="polite">
        <div class="agent-inner">
          <div v-if="!messages.length && !live" class="agent-welcome">
            <h1 class="text-h5 mb-2">{{ $t('agent.welcomeTitle') }}</h1>
            <p class="text-body-2 text-medium-emphasis">{{ $t('agent.welcomeHelp', { at: '@' }) }}</p>
            <v-btn v-for="s in suggestions" :key="s" variant="tonal" class="text-none agent-suggestion" block @click="send(s)">{{ s }}</v-btn>
          </div>

          <article v-for="msg in messages" :key="msg.id" :class="['agent-msg', `agent-msg--${msg.role}`]" :data-cy="`agent-msg-${msg.role}`">
            <template v-if="msg.role === 'user'">
              <div v-if="msg.context?.length" class="agent-chips agent-chips--end">
                <button v-for="ref_ in msg.context" :key="refKey(ref_)" type="button" class="agent-chip" @click="previewing = ref_">
                  <v-icon :icon="icons[ref_.type]" size="13" /> <span class="text-truncate">{{ ref_.label || ref_.id }}</span>
                </button>
              </div>
              <div class="agent-bubble">{{ msg.content }}</div>
            </template>
            <template v-else>
              <details v-if="toolsOf(msg).length" class="agent-tools">
                <summary>{{ $t('agent.usedTools', { n: toolsOf(msg).length }) }}</summary>
                <div v-for="(t, i) in toolsOf(msg)" :key="i" :class="{ 'text-error': t.error }">{{ t.name }}<template v-if="t.error">: {{ t.error }}</template></div>
              </details>
              <!-- eslint-disable-next-line vue/no-v-html -->
              <div v-if="msg.content" class="agent-md" v-html="renderMarkdown(msg.content)" />
              <div v-for="p in proposalsOf(msg)" :key="p.index" class="agent-card" data-cy="agent-proposal">
                <div class="d-flex align-center flex-wrap ga-2">
                  <strong class="text-body-2 agent-wrap">{{ proposalTitle(p.part) }}</strong>
                  <v-spacer />
                  <v-chip size="x-small" label :color="statusColors[p.part.status]" data-cy="agent-proposal-status">{{ p.part.status }}</v-chip>
                </div>
                <div class="text-caption text-medium-emphasis">{{ proposalInfo(p.part.tool).label }}</div>
                <div v-if="p.part.error" class="text-caption text-error mt-1">{{ p.part.error }}</div>
                <div class="d-flex align-center flex-wrap ga-2 mt-2">
                  <v-btn size="small" variant="tonal" class="text-none" prepend-icon="mdi-eye-outline" data-cy="agent-proposal-review" @click="reviewing = { msg, index: p.index }">{{ p.part.status === 'pending' ? $t('agent.review.title') : $t('agent.view') }}</v-btn>
                  <template v-if="p.part.status === 'pending'">
                    <v-btn v-if="!proposalInfo(p.part.tool).reviewFirst" size="small" color="primary" variant="flat" class="text-none" :loading="busyPart === `${msg.id}:${p.index}`" data-cy="agent-proposal-apply" @click="applyPart(msg, p.index)">{{ proposalInfo(p.part.tool).apply }}</v-btn>
                    <v-btn size="small" variant="text" class="text-none" data-cy="agent-proposal-reject" @click="rejectPart(msg, p.index)">{{ $t('agent.reject') }}</v-btn>
                  </template>
                  <a v-if="p.part.result?.source?.href" :href="p.part.result.source.href" class="text-caption">{{ $t('agent.open') }}</a>
                </div>
              </div>
            </template>
          </article>

          <article v-if="live" class="agent-msg agent-msg--assistant" data-cy="agent-live">
            <div v-if="live.tools.length" class="agent-tools">
              <div v-for="t in live.tools" :key="t.id" :class="{ 'text-error': t.status === 'failed' }">{{ t.name }} · {{ t.status }}</div>
            </div>
            <!-- eslint-disable-next-line vue/no-v-html -->
            <div class="agent-md" v-html="renderMarkdown(live.text)" />
            <v-progress-circular indeterminate size="14" width="2" color="primary" />
          </article>
        </div>
      </div>

      <div class="agent-composer">
        <div class="agent-inner">
          <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-2 text-body-2" closable data-cy="agent-error" @click:close="error = ''">{{ error }}</v-alert>
          <div v-if="chips.length" class="agent-chips" data-cy="agent-chips">
            <span v-for="(chip, i) in chips" :key="refKey(chip)" class="agent-chip">
              <button type="button" class="agent-chip-open" @click="previewing = chip"><v-icon :icon="icons[chip.type]" size="13" /> <span class="text-truncate">{{ chip.label || chip.id }}</span></button>
              <button type="button" class="agent-chip-x" :aria-label="$t('globals.buttons.delete')" @click="chips.splice(i, 1)"><v-icon icon="mdi-close" size="12" /></button>
            </span>
          </div>
          <div class="agent-box">
            <context-picker v-if="picker" ref="pickerRef" :query="picker.query" @pick="addChip" @close="picker = null" />
            <textarea
              ref="textarea"
              v-model="input"
              class="agent-input"
              rows="3"
              :placeholder="$t('agent.placeholder', { at: '@' })"
              :aria-label="$t('agent.placeholder', { at: '@' })"
              data-cy="agent-input"
              @input="onInput"
              @keydown="onKeydown"
            />
            <div class="d-flex align-center flex-wrap ga-1">
              <v-btn size="small" variant="text" class="text-none" prepend-icon="mdi-at" data-cy="agent-context" @click="togglePicker">{{ $t('agent.context') }}</v-btn>
              <v-btn size="small" variant="text" class="text-none" prepend-icon="mdi-paperclip" :loading="uploading" data-cy="agent-attach" @click="fileInput.click()">{{ $t('agent.attachFile') }}</v-btn>
              <v-btn size="small" variant="text" class="text-none" prepend-icon="mdi-content-paste" data-cy="agent-paste" @click="pasting = { name: '', content: '' }">{{ $t('agent.pasteData') }}</v-btn>
              <input ref="fileInput" type="file" multiple hidden accept="image/*,application/pdf,text/*,.csv,.json,.txt,.md,.tsv,.html,.xml,.yaml,.yml" data-cy="agent-file-input" @change="onFiles" />
              <v-spacer />
              <v-btn v-if="busy" size="small" variant="outlined" class="text-none" data-cy="agent-stop" @click="stop">{{ $t('agent.stop') }}</v-btn>
              <v-btn v-else size="small" color="primary" variant="flat" class="text-none" prepend-icon="mdi-send" :disabled="!input.trim()" data-cy="agent-send" @click="send()">{{ $t('agent.send') }}</v-btn>
            </div>
          </div>
        </div>
      </div>
    </div>

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

    <v-dialog :model-value="Boolean(pasting)" max-width="680" @update:model-value="pasting = null">
      <v-card v-if="pasting">
        <v-card-title>{{ $t('agent.pasteData') }}</v-card-title>
        <v-card-text>
          <v-text-field v-model="pasting.name" :label="$t('agent.pasteName')" placeholder="leads.csv" density="compact" variant="outlined" class="mb-2" hide-details data-cy="agent-paste-name" />
          <v-textarea v-model="pasting.content" :label="$t('agent.pasteContent')" rows="12" density="compact" variant="outlined" hide-details data-cy="agent-paste-content" />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn class="text-none" @click="pasting = null">{{ $t('globals.buttons.cancel') }}</v-btn>
          <v-btn color="primary" variant="flat" class="text-none" :disabled="!pasting.content.trim()" data-cy="agent-paste-attach" @click="savePaste">{{ $t('agent.attach') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, nextTick, onBeforeUnmount, onMounted, ref, watch,
} from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  agentDelete, agentGet, agentPatch, agentPost,
} from '../agent/client';
import {
  CONTEXT_ICONS, contextInUse, fileProblem, isBinaryFile, proposalInfo, proposalTitle, refKey,
} from '../agent/parts.mjs';
import { streamAssistant } from '../assistant/client';
import { assistantContext } from '../assistant/context';
import { renderAssistantMarkdown as renderMarkdown } from '../assistant/markdown.mjs';
import ContextPicker from '../components/agent/ContextPicker.vue';
import ContextPreviewModal from '../components/agent/ContextPreviewModal.vue';
import ModelPicker from '../components/agent/ModelPicker.vue';
import ReviewModal from '../components/agent/ReviewModal.vue';
import { shortStamp } from '../utils/crm.mjs';

const MODEL_KEY = 'listpocket_agent_model';

const { proxy } = getCurrentInstance();
const route = useRoute();
const router = useRouter();
const icons = CONTEXT_ICONS;
const statusColors = {
  pending: 'warning', applying: 'info', applied: 'success', failed: 'error', rejected: 'default',
};

const conversations = ref([]);
const convId = ref(String(route.params.id || ''));
const title = ref('');
const ai = ref(lastModel());
const messages = ref([]);
const input = ref('');
const chips = ref([]);
const busy = ref(false);
const error = ref('');
const live = ref(null);
const picker = ref(null);
const pickerRef = ref(null);
const reviewing = ref(null);
const reviewError = ref('');
const busyPart = ref('');
const previewing = ref(null);
const pasting = ref(null);
const renaming = ref(false);
const deleting = ref('');
const uploading = ref(false);
const scroller = ref(null);
const textarea = ref(null);
const fileInput = ref(null);
let turnId = '';
let controller = null;
// A new conversation gets its ID mid-send, and that mustn't reload the page.
let ownNavigation = false;

const inContext = computed(() => contextInUse(messages.value));
const reviewPart = computed(() => (reviewing.value ? reviewing.value.msg.parts[reviewing.value.index] : null));
const suggestions = computed(() => [
  proxy.$t('agent.suggest.overview'),
  proxy.$t('agent.suggest.replies'),
  proxy.$t('agent.suggest.workflow'),
]);

function lastModel() {
  try {
    return JSON.parse(localStorage.getItem(MODEL_KEY) || 'null');
  } catch {
    return null;
  }
}

function toolsOf(msg) {
  return (msg.parts || []).filter((p) => p.type === 'tool');
}

function proposalsOf(msg) {
  return (msg.parts || []).map((part, index) => ({ part, index })).filter((p) => p.part.type === 'proposal');
}

function scrollDown() {
  nextTick(() => {
    if (scroller.value) scroller.value.scrollTop = scroller.value.scrollHeight;
  });
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
    scrollDown();
  } catch (err) {
    error.value = err.message;
  }
}

function newChat() {
  router.push({ name: 'agent' });
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
        scrollDown();
      }, controller.signal);
    } catch (err) {
      if (err.name === 'AbortError') return;
      if (attempt === 3) error.value = err.message;
    }
  }
}

async function send(text = input.value) {
  const message = String(text).trim();
  if (!message || busy.value) return;
  error.value = '';
  busy.value = true;
  const context = chips.value.map(({ type, id, label }) => ({ type, id, label }));
  input.value = '';
  chips.value = [];
  picker.value = null;
  messages.value = [...messages.value, { id: `local-${Date.now()}`, role: 'user', content: message, context }];
  live.value = { text: '', tools: [] };
  scrollDown();
  controller = new AbortController();
  try {
    const id = await ensureConversation();
    const out = await agentPost(`agent/conversations/${id}/turns`, { message, context, path: route.path });
    turnId = out.turn;
    if (out.title) title.value = out.title;
    await listen(out.turn);
  } catch (err) {
    if (err.name !== 'AbortError') {
      error.value = err.message;
      if (!input.value) input.value = message;
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

function onInput() {
  const caret = textarea.value.selectionStart;
  const m = input.value.slice(0, caret).match(/(^|\s)@([^\s@]*)$/);
  if (m) picker.value = { query: m[2], fromAt: caret - m[2].length - 1 };
  else if (picker.value && picker.value.fromAt >= 0) picker.value = null;
}

function onKeydown(e) {
  if (picker.value && pickerRef.value?.key(e)) return;
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    send();
  }
}

function togglePicker() {
  picker.value = picker.value ? null : { query: '', fromAt: -1 };
  textarea.value?.focus();
}

function addChip(item) {
  if (picker.value && picker.value.fromAt >= 0) {
    const end = picker.value.fromAt + 1 + picker.value.query.length;
    input.value = input.value.slice(0, picker.value.fromAt) + input.value.slice(end);
  }
  if (!chips.value.some((c) => refKey(c) === refKey(item))) {
    chips.value.push({ type: item.type, id: item.id, label: item.label });
  }
  picker.value = null;
  textarea.value?.focus();
}

async function attach(name, mime, content, binary) {
  const conversation = await ensureConversation();
  const out = await agentPost('agent/attachments', {
    name, mime, content, binary, conversation,
  });
  chips.value.push({ type: 'file', id: out.id, label: out.name });
}

function readBase64(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result).split(',')[1]);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

async function addFiles(files) {
  uploading.value = true;
  error.value = '';
  try {
    for (const file of files) {
      const problem = fileProblem(file);
      if (problem) {
        error.value = problem;
        continue;
      }
      const binary = isBinaryFile(file);
      // eslint-disable-next-line no-await-in-loop
      const content = binary ? await readBase64(file) : await file.text();
      // eslint-disable-next-line no-await-in-loop
      await attach(file.name, file.type || 'text/plain', content, binary);
    }
  } catch (err) {
    error.value = err.message;
  } finally {
    uploading.value = false;
  }
}

function onFiles(e) {
  addFiles([...(e.target.files || [])]);
  e.target.value = '';
}

function onDrop(e) {
  if (e.dataTransfer?.files?.length) addFiles([...e.dataTransfer.files]);
}

async function savePaste() {
  const { name, content } = pasting.value;
  pasting.value = null;
  try {
    await attach(name.trim() || 'pasted.txt', 'text/plain', content, false);
  } catch (err) {
    error.value = err.message;
  }
}

async function rename() {
  renaming.value = false;
  if (!convId.value || !title.value.trim()) return;
  try {
    await agentPatch(`agent/conversations/${convId.value}`, { title: title.value });
    loadList();
  } catch (err) {
    error.value = err.message;
  }
}

async function removeConversation(c) {
  deleting.value = '';
  try {
    await agentDelete(`agent/conversations/${c.id}`);
    conversations.value = conversations.value.filter((x) => x.id !== c.id);
    if (c.id === convId.value) newChat();
  } catch (err) {
    error.value = err.message;
  }
}

async function applyPart(msg, index, args) {
  const key = `${msg.id}:${index}`;
  busyPart.value = key;
  reviewError.value = '';
  try {
    const part = await agentPost(`agent/messages/${msg.id}/parts/${index}/apply`, args ? { arguments: args } : {});
    msg.parts[index] = part;
    if (part.status === 'failed') {
      throw new Error(part.error || proxy.$t('agent.applyFailed'));
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
  chips.value = [];
  loadConversation();
});

onMounted(() => {
  // This page does what the side panel does, with more room.
  assistantContext.open = false;
  loadList();
  loadConversation();
  // A page can link here with something already attached.
  const { ref: type, id, label } = route.query;
  if (type && id && icons[type]) chips.value.push({ type, id, label: label || id });
});

onBeforeUnmount(() => controller?.abort());
</script>

<style scoped>
.agent-page {
  --agent-border: rgba(var(--v-border-color), var(--v-border-opacity));
  background: rgb(var(--v-theme-surface));
  border: 1px solid var(--agent-border);
  border-radius: 12px;
  display: grid;
  grid-template-columns: 250px minmax(0, 1fr);
  height: calc(100vh - 112px);
  min-height: 520px;
  overflow: hidden;
}

.agent-convs {
  border-right: 1px solid var(--agent-border);
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding: 12px 8px;
}

.agent-new {
  flex: none;
  width: 100%;
}

.agent-conv-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.agent-conv {
  align-items: center;
  border-radius: 6px;
  display: flex;
  gap: 4px;
  min-height: 34px;
  padding: 2px 4px 2px 10px;
}

.agent-conv:hover,
.agent-conv--active {
  background: rgb(var(--v-theme-surface-light));
}

.agent-conv-link {
  color: inherit;
  flex: 1;
  font-size: 13px;
  min-width: 0;
  overflow: hidden;
  text-decoration: none;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-conv--active .agent-conv-link {
  color: rgb(var(--v-theme-primary));
  font-weight: 500;
}

.agent-conv-when {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  flex: none;
  font-size: 11px;
}

.agent-conv-del {
  display: none;
}

.agent-conv:hover .agent-conv-when,
.agent-conv:focus-within .agent-conv-when {
  display: none;
}

.agent-conv:hover .agent-conv-del,
.agent-conv:focus-within .agent-conv-del {
  display: inline-flex;
}

.agent-chat {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
}

.agent-head {
  align-items: center;
  border-bottom: 1px solid var(--agent-border);
  display: flex;
  flex: none;
  gap: 10px;
  padding: 8px 16px;
}

.agent-title {
  background: none;
  border: 0;
  color: inherit;
  cursor: text;
  font: inherit;
  font-size: 15px;
  font-weight: 600;
  max-width: 60%;
  overflow: hidden;
  padding: 0;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-title-input {
  max-width: 420px;
}

.agent-in-context {
  align-items: center;
  border-bottom: 1px solid var(--agent-border);
  display: flex;
  flex: none;
  flex-wrap: wrap;
  gap: 6px;
  padding: 6px 16px;
}

.agent-log {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.agent-inner {
  margin: 0 auto;
  max-width: 860px;
  padding: 0 20px;
}

.agent-log .agent-inner {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding-bottom: 16px;
  padding-top: 16px;
}

.agent-welcome {
  padding-top: 6vh;
}

.agent-suggestion {
  height: auto !important;
  justify-content: flex-start;
  margin-top: 8px;
  min-height: 36px;
  padding: 8px 12px;
  text-align: left;
  white-space: normal;
}

.agent-suggestion :deep(.v-btn__content) {
  white-space: normal;
}

.agent-msg--user {
  align-items: flex-end;
  align-self: flex-end;
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 85%;
}

.agent-bubble {
  background: rgba(var(--v-theme-primary), 0.14);
  border-radius: 12px;
  overflow-wrap: anywhere;
  padding: 8px 12px;
  white-space: pre-wrap;
}

.agent-md {
  line-height: 1.55;
  overflow-wrap: anywhere;
}

.agent-md :deep(p),
.agent-md :deep(ul),
.agent-md :deep(ol) {
  margin-bottom: 8px;
}

.agent-md :deep(ul),
.agent-md :deep(ol) {
  padding-left: 22px;
}

.agent-md :deep(pre) {
  background: rgb(var(--v-theme-surface-light));
  border-radius: 6px;
  margin-bottom: 8px;
  overflow-x: auto;
  padding: 10px;
}

.agent-tools {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  font-size: 12px;
  margin-bottom: 6px;
}

.agent-tools summary {
  cursor: pointer;
}

.agent-card {
  border: 1px solid var(--agent-border);
  border-radius: 10px;
  margin-top: 8px;
  padding: 10px 12px;
}

.agent-wrap {
  overflow-wrap: anywhere;
}

.agent-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 6px;
}

.agent-chips--end {
  justify-content: flex-end;
  margin-bottom: 0;
}

.agent-chip {
  align-items: center;
  background: rgb(var(--v-theme-surface-light));
  border: 1px solid var(--agent-border);
  border-radius: 999px;
  color: inherit;
  cursor: pointer;
  display: inline-flex;
  font: inherit;
  font-size: 12px;
  gap: 4px;
  max-width: 280px;
  padding: 2px 9px;
}

.agent-chip-open {
  align-items: center;
  background: none;
  border: 0;
  color: inherit;
  cursor: pointer;
  display: inline-flex;
  font: inherit;
  gap: 4px;
  min-width: 0;
  padding: 0;
}

.agent-chip-x {
  background: none;
  border: 0;
  color: inherit;
  cursor: pointer;
  line-height: 1;
  padding: 0;
}

.agent-composer {
  border-top: 1px solid var(--agent-border);
  flex: none;
  padding: 10px 0 12px;
}

.agent-box {
  border: 1px solid rgba(var(--v-border-color), 0.3);
  border-radius: 10px;
  padding: 6px 8px;
  position: relative;
}

.agent-box:focus-within {
  border-color: rgb(var(--v-theme-primary));
}

.agent-input {
  background: none;
  border: 0;
  color: inherit;
  font: inherit;
  font-size: 14px;
  outline: none;
  padding: 4px;
  resize: vertical;
  width: 100%;
}

@container (max-width: 700px) {
  .agent-page {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 760px) {
  .agent-page {
    grid-template-columns: minmax(0, 1fr);
  }

  .agent-convs {
    display: none;
  }

  .agent-inner {
    padding: 0 12px;
  }
}
</style>
