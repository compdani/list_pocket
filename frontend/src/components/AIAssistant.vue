<script setup>
import { computed, defineAsyncComponent, getCurrentInstance, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import { useRoute } from 'vue-router';
import AppRightSidebar from './AppRightSidebar.vue';
import { pb } from '../api';
import { assistantContext } from '../assistant/context';
import { assistantJSON, streamAssistant } from '../assistant/client';
import { anchorLegacyActivity, appendAssistantEvent, conversationTranscript } from '../assistant/transcript.mjs';
import { groupToolRows, removeThread, toolLabel } from '../assistant/threads.mjs';
// Defines <lp-agent-panel>, which draws the chat itself; this component feeds it.
import { ASSISTANT_ACCEPT } from '../assistant/lp-agent-panel';
import { shortStamp } from '../utils/crm.mjs';
import ModelPicker from './agent/ModelPicker.vue';
import ReviewModal from './agent/ReviewModal.vue';
import { applyEditorPatch, revision } from '../assistant/patches.mjs';

const Media = defineAsyncComponent(() => import('../views/Media.vue'));

const route = useRoute();
const { proxy } = getCurrentInstance();
const threads = ref([]); const active = ref(''); const running = ref(false);
const mediaOpen = ref(false);
const panel = ref(null);
const chatsOpen = ref(false);
const confirmingDelete = ref('');
const reviewing = ref(null);
const reviewError = ref('');
const error = ref(''); const controller = ref(null); const busy = ref(false); const undo = shallowRef(null);
const current = computed(() => threads.value.find((thread) => thread.id === active.value));
const messages = computed(() => current.value?.messages || []);
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
// The panel compares entries by value, so they're plain copies; reading every field here
// also re-renders when a tool or proposal changes in place.
const entries = computed(() => transcript.value.map((entry) => {
  if (entry.type === 'text') return { id: entry.id, type: 'text', role: entry.role, content: entry.content };
  if (entry.type === 'tools' || entry.type === 'legacy_tools') {
    const legacy = entry.type === 'legacy_tools';
    return {
      id: entry.id,
      type: 'tools',
      label: legacy ? `Earlier activity · ${entry.tools.length} tools` : undefined,
      tools: entry.tools.map((tool) => ({
        id: tool.id, key: tool.key || tool.id, name: tool.name, label: toolLabel(tool.name), status: legacy ? 'completed' : tool.status, detail: tool.result ?? null,
      })),
    };
  }
  if (entry.type === 'proposal') {
    return { id: entry.id, type: 'proposal', proposal: JSON.parse(JSON.stringify(entry.proposal)), busy: busy.value, running: running.value };
  }
  return null;
}).filter(Boolean));
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
  controller.value?.abort(); running.value = false; undo.value = null; panel.value?.clearAttachments(); load();
});
onBeforeUnmount(() => { controller.value?.abort(); unsubscribe(); });
watch(active, async () => { persist(); await nextTick(); panel.value?.scrollToBottom({ force: true }); });
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
// Attachments stay in the composer after sending (sticky-attachments), so each turn
// sends them again and follow-up questions can still see them.
async function send(text, attachments = []) {
  if (!text.trim() || running.value) return;
  const epoch = sessionEpoch;
  const thread = current.value; error.value = ''; undo.value = null;
  running.value = true;
  try {
    const files = await Promise.all(attachments.map(async (file) => ({ filename: file.name, mimeType: file.type || 'text/plain', data: await file.base64() })));
    if (epoch !== sessionEpoch) return;
    thread.messages.push({ role: 'user', content: text });
    thread.updated = new Date().toISOString();
    if (thread.title === 'New conversation') thread.title = text.slice(0, 50);
    const editor = assistantContext.editor; const snapshot = editor?.snapshot();
    thread.editorKey = editor?.key;
    const result = await assistantJSON('turns', {
      ...(thread.ai?.model ? { ai: thread.ai } : {}),
      attachments: files, conversation: thread.id, messages: thread.messages.filter((message) => message.content).slice(-60),
      context: { pictureReferences: thread.pictures || [], path: route.path, page: route.name, params: route.params, filters: route.query,
        editor: snapshot ? { key: editor.key, unsaved: true, revision: revision(snapshot), current: snapshot, selection: editor.selection?.() || window.getSelection()?.toString().slice(0, 4000) } : null },
    });
    if (epoch !== sessionEpoch) return;
    thread.attachments = [...(thread.attachments || []), ...files.map(({ data, ...file }) => file)];
    thread.turn = result.id; thread.after = 0; thread.messages.push({ role: 'assistant', content: '', parts: [] }); persist(); await listen(thread);
  } catch (err) { if (epoch !== sessionEpoch) return; error.value = err.message; running.value = false; persist(); }
}
function onSubmit(event) {
  if (running.value || !current.value) { event.preventDefault(); return; }
  send(event.detail.text, event.detail.attachments);
}
function retry() {
  send([...messages.value].reverse().find((m) => m.role === 'user')?.content || '', panel.value?.attachments || []);
}
async function stop() {
  try { if (current.value.turn) await assistantJSON(`turns/${current.value.turn}/cancel`, {}); } catch (err) { error.value = err.message; }
}
function onAction(event) {
  const proposal = current.value?.proposals.find((p) => p.id === event.detail.id);
  if (!proposal) return;
  if (event.detail.action === 'apply') decide(proposal, true);
  else if (event.detail.action === 'discard') decide(proposal, false);
  else if (event.detail.action === 'review') reviewing.value = proposal;
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
</script>

<template>
  <AppRightSidebar v-model="assistantContext.open" :width="440" overlay disable-route-watcher :show-resize-controls="false">
    <lp-agent-panel
      ref="panel"
      class="assistant-panel"
      closable
      sticky-attachments
      plain-text
      no-add-menu
      :accept="ASSISTANT_ACCEPT"
      :entries="entries"
      :suggestions="suggestions"
      :running="running"
      :error="error"
      placeholder="Message the assistant…"
      hint="You review changes before they apply"
      empty-title="How can I help?"
      empty-text="Ask about your data, plan a campaign, or edit what you're working on. You review every change first."
      data-cy="assistant-panel"
      @agent-submit="onSubmit"
      @agent-stop="stop"
      @agent-close="assistantContext.open = false"
      @agent-action="onAction"
      @agent-error-dismiss="error = ''"
    >
      <div slot="header-start" class="ai-head">
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
      </div>
      <span slot="header-actions">
        <v-btn icon="mdi-plus" size="small" variant="text" aria-label="New chat" title="New chat" :disabled="running" data-cy="assistant-new" @click="startNewChat" />
      </span>
      <div slot="subheader" class="ai-subhead">
        <model-picker v-if="assistantContext.open" :value="current?.ai || null" @change="setModel" />
        <span v-if="contextLabel" class="ai-context text-truncate" :title="`Looking at: ${contextLabel}`" data-cy="assistant-context">{{ contextLabel }}</span>
      </div>

      <div v-if="undo || (!running && (current?.turn || error))" slot="transcript-end" class="d-flex flex-wrap ga-2">
        <v-btn v-if="undo" size="small" variant="tonal" class="text-none" prepend-icon="mdi-undo" @click="undoEdit">Undo last edit</v-btn>
        <template v-if="!running">
          <v-btn v-if="current?.turn" size="small" variant="tonal" class="text-none" @click="listen(current)">Resume</v-btn>
          <v-btn v-if="error" size="small" variant="tonal" class="text-none" @click="retry">Retry</v-btn>
        </template>
      </div>

      <span v-if="current?.pictures?.length" slot="composer-chips" class="d-contents" data-cy="assistant-attached">
        <v-chip v-for="(picture, index) in current.pictures" :key="picture.url" closable size="small" prepend-icon="mdi-image-outline" :title="picture.url" @click:close="removePicture(index)">{{ picture.name || picture.url.split('/').pop() }}</v-chip>
      </span>
      <span slot="composer-actions">
        <v-btn icon="mdi-image-plus-outline" size="small" variant="text" density="comfortable" aria-label="Add a picture from media" title="Add a picture from media" :disabled="running" @click="mediaOpen = true" />
      </span>
    </lp-agent-panel>

    <v-dialog v-model="mediaOpen" max-width="900"><v-card><v-card-text class="pt-0"><Media v-if="mediaOpen" is-modal type="pictures" @selected="addPicture" @close="mediaOpen = false" /></v-card-text></v-card></v-dialog>
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
.assistant-panel { display: block; height: 100%; }
.ai-head { align-items: center; display: flex; gap: 8px; min-width: 0; }
.ai-mark { align-items: center; background: rgba(var(--v-theme-primary), 0.14); border-radius: 8px; color: rgb(var(--v-theme-primary)); display: flex; flex: none; height: 28px; justify-content: center; width: 28px; }
.ai-title { align-items: center; background: none; border: 0; border-radius: 6px; color: inherit; cursor: pointer; display: flex; font: inherit; font-size: 14px; font-weight: 600; gap: 2px; max-width: 280px; min-width: 0; padding: 4px 6px; }
.ai-title:hover:not(:disabled) { background: rgb(var(--v-theme-surface-light)); }
.ai-title:focus-visible { outline: 2px solid rgb(var(--v-theme-primary)); }
.ai-subhead { align-items: center; display: flex; gap: 8px; }
.ai-context { color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity)); font-size: 12px; margin-left: auto; min-width: 0; }
.ai-chats { width: min(380px, 88vw); }
.ai-chats-list { max-height: 340px; overflow-y: auto; padding: 4px; }
.ai-chat { align-items: center; border-radius: 6px; display: flex; gap: 2px; min-height: 34px; padding-right: 4px; }
.ai-chat:hover, .ai-chat--active, .ai-chat--confirm { background: rgb(var(--v-theme-surface-light)); }
.ai-chat-open { align-items: center; background: none; border: 0; color: inherit; cursor: pointer; display: flex; flex: 1; font: inherit; font-size: 13px; gap: 8px; justify-content: space-between; min-width: 0; padding: 6px 8px; text-align: left; }
.ai-chat--active .ai-chat-open { color: rgb(var(--v-theme-primary)); font-weight: 500; }
.ai-chat-when { color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity)); flex: none; font-size: 11px; font-weight: 400; }
.d-contents { display: contents; }
</style>
