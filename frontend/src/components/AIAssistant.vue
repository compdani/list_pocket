<script setup>
import { computed, getCurrentInstance, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import { useRoute } from 'vue-router';
import AppRightSidebar from './AppRightSidebar.vue';
import Media from '../views/Media.vue';
import { pb } from '../api';
import { assistantContext } from '../assistant/context';
import { assistantJSON, streamAssistant } from '../assistant/client';
import { anchorLegacyActivity, appendAssistantEvent, conversationTranscript } from '../assistant/transcript.mjs';
import { renderAssistantMarkdown } from '../assistant/markdown.mjs';
import {
  groupToolRows, removeThread, toolGroupStatus, toolLabel,
} from '../assistant/threads.mjs';
import { proposalInfo, proposalTitle } from '../agent/parts.mjs';
import { shortStamp } from '../utils/crm.mjs';
import ModelPicker from './agent/ModelPicker.vue';
import ReviewModal from './agent/ReviewModal.vue';
import { applyEditorPatch, revision } from '../assistant/patches.mjs';

const route = useRoute();
const { proxy } = getCurrentInstance();
const threads = ref([]); const active = ref(''); const draft = ref(''); const running = ref(false);
const files = ref([]); const mediaOpen = ref(false);
const scrollArea = ref(null);
const composer = ref(null);
const fileInput = ref(null);
const chatsOpen = ref(false);
const confirmingDelete = ref('');
const reviewing = ref(null);
const reviewError = ref('');
const followOutput = ref(true);
const error = ref(''); const controller = ref(null); const busy = ref(false); const undo = shallowRef(null);
const current = computed(() => threads.value.find((thread) => thread.id === active.value));
const messages = computed(() => current.value?.messages || []);
const tools = computed(() => current.value?.tools || []);
const proposals = computed(() => current.value?.proposals || []);
const transcript = computed(() => groupToolRows(conversationTranscript(current.value)));
const MODEL_KEY = 'listpocket_agent_model';
const contextLabel = computed(() => {
  if (assistantContext.editor?.label) return assistantContext.editor.label;
  const title = route.meta?.title;
  return typeof title === 'string' && title ? proxy.$t(title) : '';
});
const hasLegacyChats = computed(() => {
  try {
    return Boolean(localStorage.getItem('listpocket_ai_builder_threads_v1'));
  } catch {
    return false;
  }
});
const key = () => `listpocket_assistant_v1:${pb.authStore.record?.id || 'signed-out'}`;
function persist() {
  try { localStorage.setItem(key(), JSON.stringify({ active: active.value, threads: threads.value })); } catch { error.value = 'Browser storage is full. This conversation is available until you close the page.'; }
}
function newThread() {
  // A new chat starts on the model picked last, here or on the Agent page.
  const thread = { id: crypto.randomUUID(), title: 'New conversation', messages: [], tools: [], proposals: [], after: 0, turn: null, ai: lastModel(), updated: new Date().toISOString() };
  threads.value.unshift(thread); active.value = thread.id; persist();
}
function lastModel() {
  try { return JSON.parse(localStorage.getItem(MODEL_KEY) || 'null'); } catch { return null; }
}
function setModel(next) {
  if (current.value) current.value.ai = next;
  try { localStorage.setItem(MODEL_KEY, JSON.stringify(next)); } catch { /* It still applies to this chat. */ }
  persist();
}
function openThread(id) {
  active.value = id; chatsOpen.value = false; confirmingDelete.value = ''; undo.value = null; error.value = '';
}
function startNewChat() {
  chatsOpen.value = false; undo.value = null; error.value = '';
  // Don't pile up empty chats.
  if (current.value && !current.value.messages.length) return;
  newThread();
}
// Chats live in this browser, so deleting one is final. The menu asks once first.
function deleteThread(id) {
  if (confirmingDelete.value !== id) { confirmingDelete.value = id; return; }
  confirmingDelete.value = '';
  const out = removeThread(threads.value, id, active.value);
  threads.value = out.threads;
  undo.value = null; error.value = '';
  if (out.active) active.value = out.active;
  else newThread();
  persist();
}
function load() {
  try {
    const saved = JSON.parse(localStorage.getItem(key()) || 'null'); threads.value = saved?.threads || []; active.value = saved?.active || '';
  } catch { threads.value = []; }
  threads.value.forEach(anchorLegacyActivity);
  if (!threads.value.length) newThread();
  if (!current.value) active.value = threads.value[0]?.id;
}
load();
let loadedOwner = pb.authStore.record?.id;
let sessionEpoch = 0;
const unsubscribe = pb.authStore.onChange(() => {
  if (loadedOwner === pb.authStore.record?.id) return;
  loadedOwner = pb.authStore.record?.id; sessionEpoch += 1;
  controller.value?.abort(); running.value = false; undo.value = null; files.value = []; load();
});
onBeforeUnmount(() => { controller.value?.abort(); unsubscribe(); });
watch(active, () => { followOutput.value = true; persist(); });
function trackScroll() {
  const area = scrollArea.value;
  if (area) followOutput.value = area.scrollHeight - area.scrollTop - area.clientHeight < 80;
}
watch(() => [messages.value.at(-1)?.content, proposals.value.length, tools.value.length], async () => { await nextTick(); if (scrollArea.value && followOutput.value) scrollArea.value.scrollTop = scrollArea.value.scrollHeight; });
async function listen(thread) {
  const epoch = sessionEpoch;
  controller.value = new AbortController(); running.value = true;
  let completed = false;
  for (let attempt = 0; attempt < 4 && !completed; attempt += 1) {
    try {
      await streamAssistant(thread.turn, thread.after || 0, (event) => {
        if (epoch !== sessionEpoch || event.id <= thread.after) return;
        thread.after = event.id;
        appendAssistantEvent(thread, event);
        if (event.type === 'error') error.value = event.data.message;
        if (event.type === 'done') { completed = true; thread.turn = null; }
        persist();
      }, controller.value.signal);
    } catch (err) {
      if (epoch !== sessionEpoch || err.name === 'AbortError') return;
      if (attempt === 3) error.value = `${err.message}. Use Resume to reconnect.`;
    }
  }
  if (epoch !== sessionEpoch) return;
  running.value = false; persist();
}
function growComposer() {
  const el = composer.value;
  if (!el) return;
  el.style.height = 'auto';
  el.style.height = `${Math.min(el.scrollHeight, 180)}px`;
}
watch(draft, () => nextTick(growComposer));
async function send(text = draft.value) {
  if (!text.trim() || running.value) return;
  const epoch = sessionEpoch;
  followOutput.value = true;
  const thread = current.value; error.value = ''; draft.value = ''; undo.value = null;
  thread.messages.push({ role: 'user', content: text });
  thread.updated = new Date().toISOString();
  if (thread.title === 'New conversation') thread.title = text.slice(0, 50);
  const editor = assistantContext.editor; const snapshot = editor?.snapshot();
  thread.editorKey = editor?.key;
  try {
    running.value = true;
    const result = await assistantJSON('turns', {
      ...(thread.ai?.model ? { ai: thread.ai } : {}),
      attachments: files.value.map(({ size, ...file }) => file), conversation: thread.id, messages: thread.messages.filter((message) => message.content).slice(-60),
      context: { pictureReferences: thread.pictures || [], path: route.path, page: route.name, params: route.params, filters: route.query,
        editor: snapshot ? { key: editor.key, unsaved: true, revision: revision(snapshot), current: snapshot, selection: editor.selection?.() || window.getSelection()?.toString().slice(0, 4000) } : null },
    });
    if (epoch !== sessionEpoch) return;
    thread.attachments = [...(thread.attachments || []), ...files.value.map(({ data, ...file }) => file)];
    thread.turn = result.id; thread.after = 0; thread.messages.push({ role: 'assistant', content: '', parts: [] }); persist(); await listen(thread);
  } catch (err) { if (epoch !== sessionEpoch) return; error.value = err.message; running.value = false; persist(); }
}
async function stop() {
  try { if (current.value.turn) await assistantJSON(`turns/${current.value.turn}/cancel`, {}); } catch (err) { error.value = err.message; }
}
async function decide(proposal, approve, edited) {
  busy.value = true; error.value = ''; reviewError.value = '';
  try {
    const args = proposal.arguments;
    const editor = assistantContext.editor;
    let before; let next;
    if (approve && proposal.tool === 'propose_editor_patch') {
      if (!editor || editor.key !== proposal.editorKey) throw new Error('Open the original editor before applying this proposal.');
      before = editor.snapshot(); next = applyEditorPatch(before, args);
    }
    const result = await assistantJSON(`turns/${proposal.turn}/decision`, { proposal: proposal.id, approve, ...(edited ? { arguments: edited } : {}) });
    if (approve && next && result.status === 'approved') {
      if (revision(editor.snapshot()) !== revision(before)) throw new Error('The editor changed during approval. Ask for a fresh proposal.');
      await editor.apply(next);
      undo.value = assistantContext.editor === editor && editor.key === proposal.editorKey
        ? { editor, before, after: revision(editor.snapshot()) } : null;
    }
    Object.assign(proposal, result);
    if (approve && next && result.status !== 'approved') throw new Error('This proposal is no longer available. Request a fresh proposal.');
    if (result.status === 'failed') {
      const message = result.result?.error || 'This operation failed. Check saved data before requesting a new proposal.';
      if (reviewing.value) reviewError.value = message;
      else error.value = message;
    }
    if (result.status === 'applied') window.dispatchEvent(new CustomEvent('assistant-applied', { detail: { tool: proposal.tool, result: result.result } }));
    persist();
    return result.status !== 'failed';
  } catch (err) {
    if (reviewing.value) reviewError.value = err.message;
    else error.value = err.message;
    return false;
  } finally { busy.value = false; }
}
async function applyFromReview(edited) {
  if (await decide(reviewing.value, true, edited)) closeReview();
}
async function discardFromReview() {
  if (await decide(reviewing.value, false)) closeReview();
}
function closeReview() { reviewing.value = null; reviewError.value = ''; }
async function undoEdit() {
  try {
    const entry = undo.value;
    if (assistantContext.editor !== entry.editor || revision(entry.editor.snapshot()) !== entry.after) throw new Error('The editor changed after this edit; use its undo controls.');
    await entry.editor.apply(entry.before); undo.value = null;
  } catch (err) { error.value = err.message; }
}
async function attach(event) {
  try {
    for (const file of event.target.files || []) {
      if (file.size > 6 * 1024 * 1024 || files.value.reduce((total, item) => total + item.size, 0) + file.size > 20 * 1024 * 1024) throw new Error('Use files up to 6 MiB each and 20 MiB total.');
      const data = await new Promise((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result).split(',')[1]); reader.onerror = reject; reader.readAsDataURL(file); });
      files.value.push({ filename: file.name, mimeType: file.type || 'text/plain', data, size: file.size });
    }
  } catch (err) { error.value = err.message; }
  event.target.value = '';
}
// Media library pictures go to the model as name + hosted URL, so it can place them in content.
function addPicture(media) {
  mediaOpen.value = false;
  const thread = current.value;
  if (!media?.url || !thread) return;
  thread.pictures ||= [];
  if (!thread.pictures.some((picture) => picture.url === media.url)) thread.pictures.push({ name: media.filename || '', url: media.url });
  persist();
}
function removePicture(index) { current.value.pictures.splice(index, 1); persist(); }
function importLegacy() {
  try {
    const ownerKey = 'listpocket_ai_builder_legacy_owner';
    const owner = localStorage.getItem(ownerKey);
    if (owner && owner !== pb.authStore.record?.id) throw new Error('Previous chats were already assigned to a different account.');
    const legacy = JSON.parse(localStorage.getItem('listpocket_ai_builder_threads_v1') || '{}');
    for (const [session, bucket] of Object.entries(legacy.bySessionKey || {})) {
      for (const thread of bucket.threads || []) {
        const id = `legacy:${session}:${thread.id}`;
        if (!threads.value.some(item => item.id === id)) threads.value.push({ ...thread, id, title: thread.title || session, messages: thread.messages || [], tools: [], proposals: [], after: 0, turn: null });
      }
    }
    localStorage.setItem(ownerKey, pb.authStore.record.id); persist();
    chatsOpen.value = false;
  } catch (err) { error.value = err.message; }
}
const suggestions = computed(() => {
  if (route.name === 'plugins') return ['Build a plugin that adds a workflow tool', 'Why did a plugin fail to load?', 'What can a plugin do?'];
  if (route.name === 'workflowBuilder') return ['Explain this workflow', 'What could go wrong with this workflow?', 'Add a review step before anything is sent'];
  if (route.name === 'workflowRun') return ['Why did this run do what it did?', 'How do I fix this?'];
  if (route.path.includes('workflow')) return ['Build a workflow that follows up on unanswered emails', 'Which workflows failed recently, and why?'];
  if (route.path.includes('inbox')) return ['Which conversations need a reply most?', 'Draft a reply to this conversation'];
  if (route.path.includes('crm')) return ['Summarize my overdue tasks', 'Who on my boards needs attention?'];
  if (route.path.includes('subscribers')) return ['Summarize this contact', 'Suggest the next step for this contact'];
  return ['Analyze recent campaign performance', 'Suggest a campaign for an engaged audience', 'Build a workflow for new subscribers'];
});
const statusColors = { pending: 'warning', applying: 'info', applied: 'success', approved: 'success', failed: 'error', discarded: 'default' };
</script>

<template>
  <AppRightSidebar v-model="assistantContext.open" :width="480" disable-route-watcher>
    <template #header>
      <div class="ai-head">
        <span class="ai-mark"><v-icon icon="mdi-creation-outline" size="16" /></span>
        <v-menu v-model="chatsOpen" :close-on-content-click="false" location="bottom start" max-width="400">
          <template #activator="{ props: menuProps }">
            <button v-bind="menuProps" type="button" class="ai-title" :disabled="running" aria-label="Switch chat" data-cy="assistant-chats">
              <span class="text-truncate">{{ current?.title || 'New conversation' }}</span>
              <v-icon icon="mdi-chevron-down" size="16" />
            </button>
          </template>
          <v-card class="ai-chats" elevation="8" rounded="lg" data-cy="assistant-chat-list">
            <div class="ai-chats-list">
              <div
                v-for="thread in threads"
                :key="thread.id"
                :class="['ai-chat', { 'ai-chat--active': thread.id === active, 'ai-chat--confirm': confirmingDelete === thread.id }]"
                data-cy="assistant-chat"
              >
                <button type="button" class="ai-chat-open" @click="openThread(thread.id)">
                  <span class="text-truncate">{{ thread.title }}</span>
                  <span v-if="thread.updated" class="ai-chat-when">{{ shortStamp(thread.updated) }}</span>
                </button>
                <template v-if="confirmingDelete === thread.id">
                  <v-btn size="x-small" color="error" variant="flat" class="text-none" data-cy="assistant-chat-delete-confirm" @click="deleteThread(thread.id)">Delete</v-btn>
                  <v-btn size="x-small" variant="text" class="text-none" @click="confirmingDelete = ''">Keep</v-btn>
                </template>
                <v-btn
                  v-else
                  :aria-label="`Delete ${thread.title}`"
                  icon="mdi-trash-can-outline"
                  size="x-small"
                  variant="text"
                  :disabled="running && thread.id === active"
                  data-cy="assistant-chat-delete"
                  @click="deleteThread(thread.id)"
                />
              </div>
            </div>
            <v-divider />
            <div class="pa-1">
              <v-btn v-if="hasLegacyChats" block size="small" variant="text" class="text-none justify-start" prepend-icon="mdi-import" :disabled="running" @click="importLegacy">Import previous campaign chats</v-btn>
              <v-btn block size="small" variant="text" class="text-none justify-start" prepend-icon="mdi-open-in-new" :to="{ name: 'agent' }" data-cy="assistant-open-agent" @click="chatsOpen = false; assistantContext.open = false">Open the full Agent page</v-btn>
            </div>
          </v-card>
        </v-menu>
        <v-spacer />
        <v-btn icon="mdi-plus" size="small" variant="text" aria-label="New chat" title="New chat" :disabled="running" data-cy="assistant-new" @click="startNewChat" />
        <v-btn icon="mdi-close" size="small" variant="text" aria-label="Close assistant" @click="assistantContext.open = false" />
      </div>
      <div class="ai-subhead">
        <model-picker v-if="assistantContext.open" :value="current?.ai || null" @change="setModel" />
        <span v-if="contextLabel" class="ai-context text-truncate" :title="`Looking at: ${contextLabel}`" data-cy="assistant-context">{{ contextLabel }}</span>
      </div>
    </template>

    <div class="assistant-panel">
      <div ref="scrollArea" class="assistant-scroll" aria-live="polite" @scroll="trackScroll">
        <div v-if="!messages.length" class="ai-empty" data-cy="assistant-empty">
          <span class="ai-empty-mark"><v-icon icon="mdi-creation-outline" size="26" /></span>
          <h2 class="ai-empty-title">How can I help?</h2>
          <p class="ai-empty-text">Ask about your data, plan a campaign, or edit what you're working on. You review every change first.</p>
          <button v-for="suggestion in suggestions" :key="suggestion" type="button" class="ai-suggestion" @click="send(suggestion)">
            <span>{{ suggestion }}</span>
            <v-icon icon="mdi-arrow-up" size="14" />
          </button>
        </div>

        <template v-for="entry in transcript" :key="entry.id">
          <div v-if="entry.type === 'text' && entry.role === 'user'" class="ai-bubble" data-cy="assistant-user">{{ entry.content }}</div>
          <!-- eslint-disable-next-line vue/no-v-html -->
          <div v-else-if="entry.type === 'text'" class="assistant-message" data-cy="assistant-reply" v-html="renderAssistantMarkdown(entry.content)" />

          <details v-else-if="entry.type === 'tools'" class="ai-tools" data-cy="assistant-tools">
            <summary>
              <v-progress-circular v-if="toolGroupStatus(entry.tools) === 'running'" indeterminate size="12" width="2" />
              <v-icon v-else :icon="toolGroupStatus(entry.tools) === 'failed' ? 'mdi-alert-circle-outline' : 'mdi-check'" :color="toolGroupStatus(entry.tools) === 'failed' ? 'error' : undefined" size="14" />
              <span>{{ entry.tools.length === 1 ? toolLabel(entry.tools[0].name) : `Used ${entry.tools.length} tools` }}</span>
              <v-icon class="ai-tools-chevron" icon="mdi-chevron-right" size="14" />
            </summary>
            <details v-for="tool in entry.tools" :key="tool.key || tool.id" class="ai-tool">
              <summary :class="{ 'text-error': tool.status === 'failed' }">{{ toolLabel(tool.name) }}</summary>
              <pre>{{ JSON.stringify(tool.result, null, 2) }}</pre>
            </details>
          </details>

          <details v-else-if="entry.type === 'legacy_tools'" class="ai-tools">
            <summary><v-icon icon="mdi-check" size="14" /><span>Earlier activity · {{ entry.tools.length }} tools</span><v-icon class="ai-tools-chevron" icon="mdi-chevron-right" size="14" /></summary>
            <details v-for="tool in entry.tools" :key="tool.id" class="ai-tool"><summary>{{ toolLabel(tool.name) }}</summary><pre>{{ JSON.stringify(tool.result, null, 2) }}</pre></details>
          </details>

          <article v-else-if="entry.type === 'proposal'" class="ai-card" data-cy="assistant-proposal">
            <div class="d-flex align-start ga-2">
              <div class="flex-grow-1" style="min-width: 0">
                <div class="ai-card-title">{{ proposalTitle(entry.proposal) === entry.proposal.tool ? 'Proposed edit' : proposalTitle(entry.proposal) }}</div>
                <div v-if="proposalInfo(entry.proposal.tool).label !== entry.proposal.tool" class="ai-card-kind">{{ proposalInfo(entry.proposal.tool).label }}</div>
              </div>
              <v-chip v-if="entry.proposal.status !== 'pending'" size="x-small" label :color="statusColors[entry.proposal.status]" data-cy="assistant-proposal-status">{{ entry.proposal.status }}</v-chip>
            </div>
            <v-alert v-for="warning in entry.proposal.warnings || []" :key="warning" type="warning" variant="tonal" density="compact" class="mt-2 text-caption">{{ warning }}</v-alert>
            <!-- Edits to the open editor are short, so they show in place. -->
            <div v-for="(op, index) in entry.proposal.arguments.operations || []" :key="index" class="ai-op">
              <div class="ai-op-field">{{ op.field || op.blockId || op.id || op.op }}</div>
              <pre v-if="op.before" class="ai-op-before">{{ op.before }}</pre>
              <pre class="ai-op-after">{{ op.after || JSON.stringify(op.value) }}</pre>
            </div>
            <div class="ai-card-actions">
              <template v-if="entry.proposal.status === 'pending'">
                <v-btn v-if="!proposalInfo(entry.proposal.tool).reviewFirst" size="small" color="primary" variant="flat" class="text-none" :loading="busy" :disabled="running" data-cy="assistant-apply" @click="decide(entry.proposal, true)">{{ proposalInfo(entry.proposal.tool).apply }}</v-btn>
                <v-btn v-if="!entry.proposal.arguments.operations" size="small" variant="tonal" class="text-none" prepend-icon="mdi-eye-outline" data-cy="assistant-review" @click="reviewing = entry.proposal">Review</v-btn>
                <v-btn size="small" variant="text" class="text-none" :disabled="busy || running" data-cy="assistant-discard" @click="decide(entry.proposal, false)">Discard</v-btn>
              </template>
              <template v-else>
                <v-btn v-if="!entry.proposal.arguments.operations" size="small" variant="text" class="text-none" @click="reviewing = entry.proposal">View</v-btn>
                <v-btn v-if="entry.proposal.status === 'applied' && entry.proposal.result?.source?.href" :href="entry.proposal.result.source.href" size="small" variant="text" class="text-none" append-icon="mdi-open-in-new" data-cy="assistant-open-result">Open</v-btn>
              </template>
            </div>
          </article>
        </template>

        <div v-if="running" class="ai-working" data-cy="assistant-working" role="status" aria-label="Working">
          <span /><span /><span />
        </div>
        <v-btn v-if="undo" size="small" variant="tonal" class="text-none mt-2 align-self-start" prepend-icon="mdi-undo" @click="undoEdit">Undo last edit</v-btn>
        <v-alert v-if="error" type="error" variant="tonal" density="compact" class="ai-error mt-2 text-body-2" data-cy="assistant-error">{{ error }}</v-alert>
        <div v-if="!running && (current?.turn || error)" class="d-flex ga-2">
          <v-btn v-if="current?.turn" size="small" variant="tonal" class="text-none" @click="listen(current)">Resume</v-btn>
          <v-btn v-if="error" size="small" variant="tonal" class="text-none" @click="send([...messages].reverse().find(m => m.role === 'user')?.content || '')">Retry</v-btn>
        </div>
      </div>

      <form class="ai-composer" @submit.prevent="send()">
        <div v-if="files.length || current?.pictures?.length" class="d-flex flex-wrap ga-1 mb-2" data-cy="assistant-attached">
          <v-chip v-for="(file, index) in files" :key="index" closable size="small" prepend-icon="mdi-paperclip" @click:close="files.splice(index, 1)">{{ file.filename }}</v-chip>
          <v-chip v-for="(picture, index) in current?.pictures || []" :key="picture.url" closable size="small" prepend-icon="mdi-image-outline" :title="picture.url" @click:close="removePicture(index)">{{ picture.name || picture.url.split('/').pop() }}</v-chip>
        </div>
        <div class="ai-box">
          <textarea
            ref="composer"
            v-model="draft"
            class="ai-input"
            rows="1"
            placeholder="Message the assistant…"
            aria-label="Message assistant"
            :disabled="running"
            data-cy="assistant-input"
            @keydown.enter.exact.prevent="send()"
          />
          <div class="ai-box-row">
            <v-btn icon="mdi-paperclip" size="small" variant="text" density="comfortable" aria-label="Attach a file" title="Attach a file" :disabled="running" data-cy="assistant-attach" @click="fileInput.click()" />
            <v-btn icon="mdi-image-plus-outline" size="small" variant="text" density="comfortable" aria-label="Add a picture from media" title="Add a picture from media" :disabled="running" @click="mediaOpen = true" />
            <input ref="fileInput" type="file" multiple hidden accept="image/*,application/pdf,text/*" data-cy="assistant-file-input" @change="attach" />
            <span class="ai-hint">You review changes before they apply</span>
            <v-spacer />
            <v-btn v-if="running" icon="mdi-stop" size="small" variant="tonal" density="comfortable" aria-label="Stop" title="Stop" data-cy="assistant-stop" @click="stop" />
            <v-btn v-else type="submit" icon="mdi-arrow-up" size="small" color="primary" variant="flat" density="comfortable" aria-label="Send" title="Send" :disabled="!draft.trim()" data-cy="assistant-send" />
          </div>
        </div>
      </form>
    </div>
    <v-dialog v-model="mediaOpen" max-width="900"><v-card><v-card-text class="pt-0"><Media is-modal type="pictures" @selected="addPicture" @close="mediaOpen = false" /></v-card-text></v-card></v-dialog>
    <review-modal
      v-if="reviewing"
      :key="reviewing.id"
      :part="reviewing"
      :busy="busy"
      :error="reviewError"
      @close="closeReview"
      @apply="applyFromReview"
      @reject="discardFromReview"
    />
  </AppRightSidebar>
</template>

<style scoped>
.ai-head { align-items: center; display: flex; gap: 8px; padding: 10px 8px 6px 14px; }
.ai-mark { align-items: center; background: rgba(var(--v-theme-primary), 0.14); border-radius: 8px; color: rgb(var(--v-theme-primary)); display: flex; flex: none; height: 28px; justify-content: center; width: 28px; }
.ai-title { align-items: center; background: none; border: 0; border-radius: 6px; color: inherit; cursor: pointer; display: flex; font: inherit; font-size: 14px; font-weight: 600; gap: 2px; max-width: 280px; min-width: 0; padding: 4px 6px; }
.ai-title:hover:not(:disabled) { background: rgb(var(--v-theme-surface-light)); }
.ai-title:focus-visible { outline: 2px solid rgb(var(--v-theme-primary)); }
.ai-subhead { align-items: center; display: flex; gap: 8px; padding: 0 14px 10px; }
.ai-context { color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity)); font-size: 12px; margin-left: auto; min-width: 0; }
.ai-chats { width: min(380px, 88vw); }
.ai-chats-list { max-height: 340px; overflow-y: auto; padding: 4px; }
.ai-chat { align-items: center; border-radius: 6px; display: flex; gap: 2px; min-height: 34px; padding-right: 4px; }
.ai-chat:hover, .ai-chat--active, .ai-chat--confirm { background: rgb(var(--v-theme-surface-light)); }
.ai-chat-open { align-items: center; background: none; border: 0; color: inherit; cursor: pointer; display: flex; flex: 1; font: inherit; font-size: 13px; gap: 8px; justify-content: space-between; min-width: 0; padding: 6px 8px; text-align: left; }
.ai-chat--active .ai-chat-open { color: rgb(var(--v-theme-primary)); font-weight: 500; }
.ai-chat-when { color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity)); flex: none; font-size: 11px; font-weight: 400; }

.assistant-panel { display: flex; flex-direction: column; height: 100%; min-height: 0; }
.assistant-scroll { display: flex; flex: 1; flex-direction: column; gap: 12px; min-height: 0; overflow-y: auto; padding: 16px; }

.ai-empty { align-items: center; display: flex; flex-direction: column; margin: auto 0; padding: 16px 4px; text-align: center; }
.ai-empty-mark { align-items: center; background: rgba(var(--v-theme-primary), 0.12); border-radius: 14px; color: rgb(var(--v-theme-primary)); display: flex; height: 52px; justify-content: center; margin-bottom: 12px; width: 52px; }
.ai-empty-title { font-size: 18px; font-weight: 600; margin-bottom: 4px; }
.ai-empty-text { color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity)); font-size: 13px; line-height: 1.5; margin-bottom: 16px; max-width: 340px; }
.ai-suggestion { align-items: center; background: rgb(var(--v-theme-surface)); border: 1px solid rgba(var(--v-border-color), 0.22); border-radius: 10px; color: inherit; cursor: pointer; display: flex; font: inherit; font-size: 13px; gap: 10px; justify-content: space-between; margin-bottom: 8px; padding: 10px 12px; text-align: left; transition: background-color 0.15s, border-color 0.15s; width: 100%; }
.ai-suggestion .v-icon { color: rgba(var(--v-theme-on-surface), 0.4); flex: none; }
.ai-suggestion:hover { background: rgba(var(--v-theme-primary), 0.06); border-color: rgba(var(--v-theme-primary), 0.4); }
.ai-suggestion:focus-visible { outline: 2px solid rgb(var(--v-theme-primary)); }

.ai-bubble { align-self: flex-end; background: rgba(var(--v-theme-primary), 0.13); border-radius: 14px 14px 4px 14px; font-size: 14px; line-height: 1.5; max-width: 88%; overflow-wrap: anywhere; padding: 8px 12px; white-space: pre-wrap; }
.assistant-message { font-size: 14px; line-height: 1.6; overflow-wrap: anywhere; overflow-x: auto; }
.assistant-message :deep(p), .assistant-message :deep(ul), .assistant-message :deep(ol), .assistant-message :deep(blockquote) { margin-bottom: 10px; }
.assistant-message :deep(:last-child) { margin-bottom: 0; }
.assistant-message :deep(h1), .assistant-message :deep(h2), .assistant-message :deep(h3) { font-size: 15px; font-weight: 600; line-height: 1.4; margin: 14px 0 6px; }
.assistant-message :deep(:first-child) { margin-top: 0; }
.assistant-message :deep(ul), .assistant-message :deep(ol) { padding-left: 22px; }
.assistant-message :deep(table) { border-collapse: collapse; font-size: 13px; margin: 10px 0; width: 100%; }
.assistant-message :deep(th), .assistant-message :deep(td) { border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity)); padding: 6px 8px; vertical-align: top; }
.assistant-message :deep(th) { background: rgba(var(--v-theme-on-surface), 0.04); font-weight: 600; }
.assistant-message :deep(code) { background: rgba(var(--v-theme-on-surface), 0.07); border-radius: 4px; font-size: 12px; padding: 1px 4px; }
.assistant-message :deep(pre) { background: rgba(var(--v-theme-on-surface), 0.05); border-radius: 8px; margin-bottom: 10px; overflow-x: auto; padding: 10px 12px; }
.assistant-message :deep(pre code) { background: none; padding: 0; }
.assistant-message :deep(blockquote) { border-left: 3px solid rgba(var(--v-border-color), 0.3); padding-left: 12px; }

/* Vuetify's alert has a flex basis of 0 and hides overflow, so in this column it collapses. */
.ai-error { flex: none; }

.ai-tools { color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity)); font-size: 12px; }
.ai-tools > summary { align-items: center; cursor: pointer; display: inline-flex; gap: 6px; list-style: none; }
.ai-tools > summary::-webkit-details-marker { display: none; }
.ai-tools > summary:hover { color: rgb(var(--v-theme-on-surface)); }
.ai-tools-chevron { transition: transform 0.15s; }
.ai-tools[open] .ai-tools-chevron { transform: rotate(90deg); }
.ai-tool { border-left: 2px solid rgba(var(--v-border-color), 0.2); margin: 6px 0 0 6px; padding-left: 10px; }
.ai-tool summary { cursor: pointer; }
.ai-tool pre { background: rgba(var(--v-theme-on-surface), 0.04); border-radius: 6px; font-size: 11px; margin-top: 4px; max-height: 200px; overflow: auto; padding: 8px; white-space: pre-wrap; word-break: break-word; }

.ai-card { background: rgb(var(--v-theme-surface)); border: 1px solid rgba(var(--v-border-color), 0.22); border-radius: 12px; box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05); padding: 12px; }
.ai-card-title { font-size: 13px; font-weight: 600; overflow-wrap: anywhere; }
.ai-card-kind { color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity)); font-size: 12px; }
.ai-card-actions { align-items: center; display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }
.ai-card-actions:empty { display: none; }
.ai-op { margin-top: 10px; }
.ai-op-field { font-size: 12px; font-weight: 500; margin-bottom: 4px; }
.ai-op-before, .ai-op-after { border-radius: 6px; font-family: inherit; font-size: 12px; line-height: 1.5; margin: 0 0 4px; max-height: 180px; overflow: auto; padding: 6px 8px; white-space: pre-wrap; word-break: break-word; }
.ai-op-before { background: rgba(var(--v-theme-error), 0.08); text-decoration: line-through; text-decoration-color: rgba(var(--v-theme-error), 0.5); }
.ai-op-after { background: rgba(var(--v-theme-success), 0.1); }

.ai-working { display: flex; gap: 4px; padding: 4px 0; }
.ai-working span { animation: ai-pulse 1.2s infinite ease-in-out; background: rgb(var(--v-theme-primary)); border-radius: 50%; height: 6px; width: 6px; }
.ai-working span:nth-child(2) { animation-delay: 0.15s; }
.ai-working span:nth-child(3) { animation-delay: 0.3s; }
@keyframes ai-pulse {
  0%, 60%, 100% { opacity: 0.25; transform: translateY(0); }
  30% { opacity: 1; transform: translateY(-2px); }
}
@media (prefers-reduced-motion: reduce) {
  .ai-working span { animation: none; opacity: 0.6; }
}

.ai-composer { flex: none; padding: 8px 12px 12px; }
.ai-box { background: rgb(var(--v-theme-surface)); border: 1px solid rgba(var(--v-border-color), 0.28); border-radius: 14px; padding: 8px 8px 6px; transition: border-color 0.15s, box-shadow 0.15s; }
.ai-box:focus-within { border-color: rgb(var(--v-theme-primary)); box-shadow: 0 0 0 3px rgba(var(--v-theme-primary), 0.12); }
.ai-input { background: none; border: 0; color: inherit; display: block; font: inherit; font-size: 14px; line-height: 1.5; max-height: 180px; min-height: 44px; outline: none; padding: 2px 6px; resize: none; width: 100%; }
.ai-box-row { align-items: center; display: flex; gap: 2px; margin-top: 2px; }
.ai-hint { color: rgba(var(--v-theme-on-surface), 0.45); font-size: 11px; margin-left: 6px; white-space: nowrap; }
@media (max-width: 420px) {
  .ai-hint { display: none; }
}
</style>
