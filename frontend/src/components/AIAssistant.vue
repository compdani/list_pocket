<script setup>
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import { useRoute } from 'vue-router';
import AppRightSidebar from './AppRightSidebar.vue';
import { pb } from '../api';
import { assistantContext } from '../assistant/context';
import { assistantJSON, streamAssistant } from '../assistant/client';
import { anchorLegacyActivity, appendAssistantEvent, conversationTranscript } from '../assistant/transcript.mjs';
import { renderAssistantMarkdown } from '../assistant/markdown.mjs';
import { applyEditorPatch, revision } from '../assistant/patches.mjs';

const route = useRoute();
const threads = ref([]); const active = ref(''); const draft = ref(''); const running = ref(false);
const files = ref([]);
const scrollArea = ref(null);
const followOutput = ref(true);
const error = ref(''); const controller = ref(null); const busy = ref(false); const undo = shallowRef(null);
const current = computed(() => threads.value.find((thread) => thread.id === active.value));
const messages = computed(() => current.value?.messages || []);
const tools = computed(() => current.value?.tools || []);
const proposals = computed(() => current.value?.proposals || []);
const transcript = computed(() => conversationTranscript(current.value));
const editorLabel = computed(() => assistantContext.editor?.label || 'Saved application data');
const key = () => `listpocket_assistant_v1:${pb.authStore.record?.id || 'signed-out'}`;
function persist() {
  try { localStorage.setItem(key(), JSON.stringify({ active: active.value, threads: threads.value })); } catch { error.value = 'Browser storage is full. This conversation is available until you close the page.'; }
}
function newThread() {
  const thread = { id: crypto.randomUUID(), title: 'New conversation', messages: [], tools: [], proposals: [], after: 0, turn: null };
  threads.value.unshift(thread); active.value = thread.id; persist();
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
async function send(text = draft.value) {
  if (!text.trim() || running.value) return;
  const epoch = sessionEpoch;
  followOutput.value = true;
  const thread = current.value; error.value = ''; draft.value = ''; undo.value = null;
  thread.messages.push({ role: 'user', content: text });
  if (thread.title === 'New conversation') thread.title = text.slice(0, 50);
  const editor = assistantContext.editor; const snapshot = editor?.snapshot();
  thread.editorKey = editor?.key;
  try {
    running.value = true;
    const result = await assistantJSON('turns', {
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
async function decide(proposal, approve) {
  busy.value = true; error.value = '';
  try {
    const args = proposal.arguments;
    const editor = assistantContext.editor;
    let before; let next;
    if (approve && proposal.tool === 'propose_editor_patch') {
      if (!editor || editor.key !== proposal.editorKey) throw new Error('Open the original editor before applying this proposal.');
      before = editor.snapshot(); next = applyEditorPatch(before, args);
    }
    const result = await assistantJSON(`turns/${proposal.turn}/decision`, { proposal: proposal.id, approve });
    if (approve && next && result.status === 'approved') {
      if (revision(editor.snapshot()) !== revision(before)) throw new Error('The editor changed during approval. Ask for a fresh proposal.');
      await editor.apply(next);
      undo.value = assistantContext.editor === editor && editor.key === proposal.editorKey
        ? { editor, before, after: revision(editor.snapshot()) } : null;
    }
    Object.assign(proposal, result);
    if (approve && next && result.status !== 'approved') throw new Error('This proposal is no longer available. Request a fresh proposal.');
    if (result.status === 'failed') error.value = result.result?.error || 'This operation failed. Check saved data before requesting a new proposal.';
    persist();
  } catch (err) { error.value = err.message; } finally { busy.value = false; }
}
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
  } catch (err) { error.value = err.message; }
}
const suggestions = computed(() => route.path.includes('workflow')
  ? ['Explain this workflow', 'Add a condition to this workflow']
  : ['Analyze recent campaign performance', 'Suggest a campaign for an engaged audience', 'Shorten the selected CTA']);
</script>

<template>
  <AppRightSidebar v-model="assistantContext.open" :width="520" disable-route-watcher>
    <template #header><div class="pa-4 d-flex align-center ga-2"><v-icon>mdi-creation-outline</v-icon><strong>AI assistant</strong><v-spacer /><v-btn icon="mdi-close" variant="text" aria-label="Close assistant" @click="assistantContext.open = false" /></div></template>
    <div class="assistant-panel">
      <div class="pa-3 border-b">
        <div class="d-flex ga-2"><v-select v-model="active" :items="threads" item-title="title" item-value="id" label="Conversation" density="compact" hide-details :disabled="running" /><v-btn variant="text" :disabled="running" @click="newThread">New</v-btn></div>
        <v-btn size="x-small" variant="text" :disabled="running" @click="importLegacy">Import previous campaign chats</v-btn>
        <div class="d-flex flex-wrap ga-1 mt-2"><v-chip size="small">{{ route.path }}</v-chip><v-chip size="small" color="primary">{{ editorLabel }}</v-chip></div>
      </div>
      <div ref="scrollArea" class="assistant-scroll pa-4" aria-live="polite" @scroll="trackScroll">
        <div v-if="!messages.length" class="text-medium-emphasis my-6">Ask about your data, plan a campaign, or edit what you’re working on.<v-btn v-for="suggestion in suggestions" :key="suggestion" block variant="tonal" class="mt-2" @click="send(suggestion)">{{ suggestion }}</v-btn></div>
        <template v-for="entry in transcript" :key="entry.id">
          <article v-if="entry.type === 'text'" class="mb-4" :data-role="entry.role">
            <div class="text-caption text-medium-emphasis">{{ entry.role === 'user' ? 'You' : 'Assistant' }}</div>
            <div v-if="entry.role === 'user'" class="assistant-user-message">{{ entry.content }}</div>
            <div v-else class="assistant-message" v-html="renderAssistantMarkdown(entry.content)" />
          </article>
          <details v-else-if="entry.type === 'tool'" class="assistant-tool mb-3" :data-tool-id="entry.tool.id">
            <summary>{{ entry.tool.name }} · {{ entry.tool.status }}</summary><pre>{{ JSON.stringify(entry.tool.result, null, 2) }}</pre>
          </details>
          <details v-else-if="entry.type === 'legacy_tools'" class="assistant-tool mb-3">
            <summary>Earlier tool activity · {{ entry.tools.length }} calls</summary>
            <details v-for="tool in entry.tools" :key="tool.id" class="mt-2"><summary>{{ tool.name }} · {{ tool.status }}</summary><pre>{{ JSON.stringify(tool.result, null, 2) }}</pre></details>
          </details>
        <v-card v-if="entry.type === 'proposal'" variant="outlined" class="my-3">
          <v-card-title class="text-body-1">{{ entry.proposal.arguments.summary || entry.proposal.tool }}</v-card-title>
          <v-card-text><div v-for="(op, index) in entry.proposal.arguments.operations || []" :key="index" class="mb-3"><strong>{{ op.field || op.blockId || op.id || op.op }}</strong><pre class="text-error">{{ op.before }}</pre><pre class="text-success">{{ op.after || JSON.stringify(op.value) }}</pre></div><details v-if="!entry.proposal.arguments.operations"><summary>Review draft</summary><pre>{{ JSON.stringify(entry.proposal.arguments, null, 2) }}</pre></details><div v-if="entry.proposal.status !== 'pending'">{{ entry.proposal.status }}</div><a v-if="entry.proposal.status === 'applied' && entry.proposal.result?.source?.href" :href="entry.proposal.result.source.href">Open saved draft</a></v-card-text>
          <v-card-actions v-if="entry.proposal.status === 'pending'"><v-btn :disabled="busy || running" @click="decide(entry.proposal, false)">Discard</v-btn><v-btn color="primary" :loading="busy" :disabled="running" @click="decide(entry.proposal, true)">Apply</v-btn></v-card-actions>
        </v-card>
        </template>
        <div v-if="running" class="text-caption text-medium-emphasis py-2"><v-progress-circular indeterminate size="14" width="2" class="mr-2" />Working…</div>
        <v-btn v-if="undo" variant="text" @click="undoEdit">Undo last edit</v-btn>
        <v-alert v-if="error" type="error" variant="tonal" class="my-2">{{ error }}</v-alert>
        <v-btn v-if="current?.turn && !running" @click="listen(current)">Resume</v-btn>
        <v-btn v-if="error && !running" @click="send([...messages].reverse().find(m => m.role === 'user')?.content || '')">Retry</v-btn>
      </div>
      <div class="px-3"><v-chip v-for="(file, index) in files" :key="index" closable size="small" @click:close="files.splice(index, 1)">{{ file.filename }}</v-chip></div>
      <form class="pa-3 border-t" @submit.prevent="send()"><v-textarea v-model="draft" label="Message assistant" rows="3" auto-grow hide-details :disabled="running" @keydown.enter.exact.prevent="send()" /><label class="text-caption">Attach reference<input type="file" multiple accept="image/*,application/pdf,text/*" :disabled="running" @change="attach" /></label><div class="d-flex justify-space-between mt-2"><span class="text-caption text-medium-emphasis">Changes are reviewed before applying.</span><v-btn v-if="running" variant="tonal" @click="stop">Stop</v-btn><v-btn v-else type="submit" color="primary" :disabled="!draft.trim()">Send</v-btn></div></form>
    </div>
  </AppRightSidebar>
</template>
<style scoped>
.assistant-panel { height: 100%; display: flex; flex-direction: column; min-height: 0; }
.assistant-scroll { flex: 1; overflow-y: auto; min-height: 0; }
.assistant-user-message { white-space: pre-wrap; overflow-wrap: anywhere; }
.assistant-message { overflow-wrap: anywhere; line-height: 1.6; overflow-x: auto; }
.assistant-message :deep(p), .assistant-message :deep(ul), .assistant-message :deep(ol), .assistant-message :deep(blockquote) { margin-bottom: 12px; }
.assistant-message :deep(h1), .assistant-message :deep(h2), .assistant-message :deep(h3) { font-size: 1.1rem; line-height: 1.4; margin: 18px 0 10px; }
.assistant-message :deep(:first-child) { margin-top: 0; }
.assistant-message :deep(ul), .assistant-message :deep(ol) { padding-left: 24px; }
.assistant-message :deep(table) { border-collapse: collapse; font-size: 0.85rem; width: 100%; margin: 12px 0; }
.assistant-message :deep(th), .assistant-message :deep(td) { padding: 8px 10px; border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity)); vertical-align: top; }
.assistant-message :deep(th) { background: rgba(var(--v-theme-on-surface), 0.04); font-weight: 600; }
.assistant-message :deep(pre) { padding: 12px; background: rgba(var(--v-theme-on-surface), 0.04); border-radius: 6px; }
.assistant-message :deep(blockquote) { padding-left: 12px; border-left: 3px solid rgba(var(--v-border-color), var(--v-border-opacity)); }
.assistant-tool { padding: 10px; border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity)); border-radius: 8px; }
pre { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 240px; overflow: auto; font-size: 12px; }
</style>
