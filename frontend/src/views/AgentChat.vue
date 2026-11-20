<template>
  <section class="agent-host" data-cy="agent-page">
    <v-alert v-if="actionError" type="error" variant="tonal" density="compact" closable class="agent-notice" data-cy="agent-action-error" @click:close="actionError = ''">
      {{ actionError }}
    </v-alert>

    <lp-agent-page
      ref="page"
      class="agent-page"
      mention-trigger="@"
      plain-text
      no-add-menu
      :renamable="renamable"
      :contextReferences="inContext"
      :formatDate="formatDate"
      :labels="labels"
      :suggestions="suggestions"
      :placeholder="$t('agent.placeholder', { at: '@' })"
      :empty-title="$t('agent.welcomeTitle')"
      :empty-text="$t('agent.welcomeHelp', { at: '@' })"
    >
      <div slot="mention">
        <context-picker v-if="picker" ref="pickerRef" :query="picker.query" @pick="pickContext" @close="page?.closeMention()" />
      </div>
      <span slot="composer-actions" class="agent-actions">
        <v-btn size="small" variant="text" class="text-none" prepend-icon="mdi-at" data-cy="agent-context" @click="toggleMention">{{ $t('agent.context') }}</v-btn>
        <v-btn size="small" variant="text" class="text-none" prepend-icon="mdi-content-paste" data-cy="agent-paste" @click="pasting = { name: '', content: '' }">{{ $t('agent.pasteData') }}</v-btn>
      </span>
    </lp-agent-page>

    <review-modal
      v-if="reviewPart"
      :key="reviewKey"
      :part="reviewPart"
      :busy="Boolean(busyId && busyId === reviewPart.id)"
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
  computed, getCurrentInstance, onBeforeUnmount, onMounted, ref, watch,
} from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { pb } from '../api';
import {
  bindAgentPanel, renderContextMarkerEntry, renderNoticeEntry, proposalErrorText,
} from '@pocketbase-mcp/chat-client/agent-panel';
import { ChatController } from '@pocketbase-mcp/chat-client';
import { ChatClient } from '@pocketbase-mcp/chat-client';
import { assistantContext } from '../assistant/context';
import {
  cardProposalOf, chatErrorText, choiceIdFor, contextReferencesFromTranscript,
  navigationWrite, pageContext, referencesFromContext, reviewPartOf, storedModelFor,
} from '../assistant/chat-host.mjs';
import { renderProposal } from '../assistant/lp-agent-common';
import { CONTEXT_ICONS } from '../agent/parts.mjs';
// Defines <lp-agent-page>; this host feeds it through the chat binding.
import '../assistant/lp-agent-page';
import ContextPicker from '../components/agent/ContextPicker.vue';
import ContextPreviewModal from '../components/agent/ContextPreviewModal.vue';
import ReviewModal from '../components/agent/ReviewModal.vue';
import { shortStamp } from '../utils/crm.mjs';

const MODEL_KEY = 'listpocket_agent_model';
const CHAT_MOUNT = '/api/mcp/chat/v1';

const { proxy } = getCurrentInstance();
const $t = (...args) => proxy.$t(...args);
const route = useRoute();
const router = useRouter();

const page = ref(null);
const pickerRef = ref(null);
const picker = ref(null);
const inContext = ref([]);
const renamable = ref(false);
const actionError = ref('');
const previewing = ref(null);
const pasting = ref(null);
const reviewProposal = ref(null);
const reviewKey = ref(0);
const reviewError = ref('');
const busyId = ref('');
let binding = null;
let unsubscribeAuth = null;
let aligned = false;
let switchingAccount = false;
// A route change that arrived before the first open finished. The URL wins:
// that id is applied once, and the load it interrupted must not write itself back.
let sawQueuedRoute = false;
let queuedRoute = '';
// 'push' for a person selecting a conversation, starting a new chat, or
// deleting the open one. The id a draft's first send assigns stays 'replace'.
let urlIntent = 'replace';
// The id of a push/replace that has not finished. A later state change for
// that same id must not write again: the first write is the one that counts,
// and a replace behind a push would drop the history step.
let pendingNavId = null;
// The composer stays disabled until /status has answered. The binding only
// disables it once features.ready is false, and it will not re-enable a
// composer it did not disable itself.
let heldUntilReady = true;
// The @ chips of the submit being sent. The binding reads the context during
// its own agent-submit handler, but with files attached it clears the
// composer's chips first, so the element's references are empty by then.
// This listener runs before the binding's and keeps the event's copy.
let submitReferences = null;
function captureSubmitReferences(event) {
  submitReferences = Array.isArray(event.detail?.references) ? event.detail.references : null;
}

const reviewPart = computed(() => (reviewProposal.value ? reviewPartOf(reviewProposal.value) : null));
const formatDate = shortStamp;
const suggestions = computed(() => [$t('agent.suggest.overview'), $t('agent.suggest.replies'), $t('agent.suggest.workflow')]);

const labels = computed(() => ({
  attach: $t('agent.attachFile'),
  attachHint: $t('agent.attachFile'),
  cancel: $t('globals.buttons.cancel'),
  conversations: $t('agent.conversations'),
  delete: $t('globals.buttons.delete'),
  deleteConversation: (name) => `${$t('globals.buttons.delete')} ${name}`,
  discard: $t('agent.reject'),
  inContext: $t('agent.inContext'),
  loadMore: $t('agent.loadMore'),
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

function accountKey() {
  return {
    appUrl: pb.baseURL,
    mount: CHAT_MOUNT,
    authCollectionId: pb.authStore.record?.collectionId || 'users',
    userId: pb.authStore.record?.id || 'signed-out',
  };
}

const client = new ChatClient({
  baseUrl: pb.baseURL,
  // Read per request: a renewed or cleared session is picked up immediately.
  getToken: () => pb.authStore.token,
});
const chat = new ChatController({
  client,
  storageKey: accountKey(),
  context: () => {
    const references = submitReferences ?? page.value?.references;
    submitReferences = null;
    return pageContext(route.fullPath, references);
  },
});

function lastModel() {
  try {
    return JSON.parse(localStorage.getItem(MODEL_KEY) || 'null');
  } catch {
    return null;
  }
}

function preselectModel() {
  const id = choiceIdFor(lastModel(), chat.state.choices);
  if (id && page.value) page.value.model = id;
}

function activeId() {
  return chat.state.activeConversationId ? String(chat.state.activeConversationId) : '';
}

function routeId() {
  return String(route.params.id || '');
}

function syncUrl() {
  const held = !aligned || switchingAccount || (sawQueuedRoute && queuedRoute !== activeId());
  const change = navigationWrite({
    activeId: activeId(),
    routeId: routeId(),
    intent: urlIntent,
    suspended: held,
  });
  if (!change || pendingNavId === change.id) return;
  urlIntent = 'replace';
  pendingNavId = change.id;
  const loc = { name: 'agent', params: change.id ? { id: change.id } : {} };
  Promise.resolve(router[change.method](loc)).catch(() => {}).finally(() => {
    if (pendingNavId === change.id) pendingNavId = null;
  });
}

function holdComposerUntilReady(state) {
  const el = page.value;
  if (!el) return;
  if (state.status !== 'ready') {
    el.disabled = true;
    heldUntilReady = true;
    return;
  }
  if (!heldUntilReady) return;
  heldUntilReady = false;
  if (state.features?.ready !== false) el.disabled = false;
}

function onChange() {
  const state = chat.state;
  syncUrl();
  inContext.value = contextReferencesFromTranscript(state.transcript);
  renamable.value = Boolean(state.activeConversationId);
  holdComposerUntilReady(state);
}

function armHistoryPush() {
  urlIntent = 'push';
  // A new chat opened while the URL is already /agent does not navigate, so
  // the push must not ride along with that draft's first send.
  queueMicrotask(() => {
    if (urlIntent === 'push' && activeId() === routeId()) urlIntent = 'replace';
  });
}

function onSelectConversation() {
  armHistoryPush();
}

function onNewConversation() {
  armHistoryPush();
}

function onDeleteConversation(event) {
  const id = event.detail?.id;
  if (id == null || String(id) !== activeId()) return;
  urlIntent = 'push';
}

function onModelChange(event) {
  const choiceId = typeof event.detail?.model === 'string' ? event.detail.model : '';
  try {
    localStorage.setItem(MODEL_KEY, JSON.stringify(storedModelFor(choiceId, chat.state.choices)));
  } catch {
    // The choice still applies to this turn.
  }
}

async function decideProposal(id, decision, edited) {
  if (!chat || !id || busyId.value === id) return { ok: false };
  busyId.value = id;
  page.value?.busyProposals.add(id);
  page.value?.repaint();
  actionError.value = '';
  try {
    const out = await chat.decide(id, decision, edited ? { arguments: edited } : undefined);
    const stored = out?.proposal ?? null;
    if (out?.error) {
      return { ok: false, proposal: stored, error: proposalErrorText(out.error) };
    }
    if (decision === 'approve' && stored?.state === 'succeeded') {
      window.dispatchEvent(new CustomEvent('assistant-applied', { detail: { tool: stored.tool, result: stored.result } }));
    }
    const problem = stored ? reviewPartOf(stored)?.error || '' : '';
    return {
      ok: Boolean(stored) && stored.state !== 'failed',
      proposal: stored,
      error: stored?.state === 'failed' ? problem : '',
    };
  } catch (err) {
    return { ok: false, error: proposalErrorText(err) };
  } finally {
    busyId.value = '';
    page.value?.busyProposals.delete(id);
    page.value?.repaint();
  }
}

// entry.proposal is the stored proposal. The binding omits the controller's
// `error` (a 409 conflict or a 422 refusal), so that one field still comes
// from state; everything else the card and the review modal show is on the entry.
function proposalFor(id, stored) {
  const live = [...(chat.state.proposals || []), ...(chat.state.decidedProposals || [])]
    .find((item) => item && String(item.id) === String(id));
  if (stored && typeof stored === 'object') return { ...stored, error: stored.error ?? live?.error };
  return live || null;
}

function openReview(id) {
  const entries = page.value?.entries || [];
  const entry = entries.find((item) => item
    && item.type === 'proposal'
    && (item.proposalId === id || item.proposal?.id === id));
  const proposal = proposalFor(id, entry?.proposal);
  if (!proposal) return;
  reviewProposal.value = JSON.parse(JSON.stringify(proposal));
  reviewError.value = '';
  reviewKey.value += 1;
}

function closeReview() {
  reviewProposal.value = null;
  reviewError.value = '';
}

async function approve(id) {
  const out = await decideProposal(id, 'approve');
  // A refusal or a failed effect is shown on its card; a request that threw
  // before the server answered has no card update, so it gets the banner.
  if (!out.ok && out.error && !out.proposal) actionError.value = out.error;
}

async function reject(id) {
  const out = await decideProposal(id, 'reject');
  if (!out.ok && out.error && !out.proposal) actionError.value = out.error;
}

async function applyFromReview(args) {
  const id = reviewProposal.value?.id;
  if (!id) return;
  const out = await decideProposal(id, 'approve', args);
  if (out.ok) closeReview();
  else if (out.error) {
    reviewError.value = out.error;
    if (out.proposal) reviewProposal.value = JSON.parse(JSON.stringify(out.proposal));
  }
}

async function rejectFromReview() {
  const id = reviewProposal.value?.id;
  if (!id) return;
  const out = await decideProposal(id, 'reject');
  if (out.ok) closeReview();
  else if (out.error) reviewError.value = out.error;
}

function onAction(event) {
  const { action, id } = event.detail || {};
  if (typeof id !== 'string' || id === '') return;
  if (action === 'apply') approve(id);
  else if (action === 'discard') reject(id);
  else if (action === 'review') openReview(id);
}

function onReferenceOpen(event) {
  const reference = event.detail?.reference;
  // An attachment chip downloads through the binding; only app references
  // open the preview modal here.
  if (!reference || reference.type === 'attachment') return;
  previewing.value = { type: reference.type, id: reference.id, label: reference.label || String(reference.id) };
}

function onMention(event) {
  picker.value = event.detail?.open ? { query: event.detail.query } : null;
}

function onMentionKey(event) {
  if (pickerRef.value?.key(event.detail.event)) event.preventDefault();
}

function toggleMention() {
  if (page.value?.mention.open) page.value.closeMention();
  else page.value?.openMention();
}

function pickContext(item) {
  page.value?.addReference({ type: item.type, id: item.id, label: item.label });
}

function savePaste() {
  const { name, content } = pasting.value;
  pasting.value = null;
  page.value?.addFiles([new File([content], name.trim() || 'pasted.txt', { type: 'text/plain' })]);
}

// One card per proposal the binding reports. The entry carries the stored
// proposal (arguments, warnings, result), including decided ones after a reopen.
function renderProposalEntry(entry, panel) {
  const full = proposalFor(entry.proposalId, entry.proposal) || {
    id: entry.proposalId,
    tool: entry.tool,
    state: entry.state,
    revision: entry.revision,
    arguments: {},
    warnings: [],
  };
  const card = cardProposalOf(full, {
    busy: entry.busy === true || busyId.value === entry.proposalId,
    running: entry.running === true || panel.chat?.state?.running === true,
  });
  return card ? renderProposal(panel, card) : null;
}

// The binding's notice is the outcome line. A decided proposal's stored result
// rides on that entry, so the same Open link the applied card shows is still
// there after the conversation is reopened.
function renderNotice(entry, panel) {
  const node = renderNoticeEntry(entry);
  const href = entry?.result?.source?.href;
  if (!node || typeof href !== 'string' || href === '') return node;
  const link = document.createElement('a');
  link.className = 'btn btn--text';
  link.href = href;
  link.setAttribute('data-cy', 'assistant-open-result');
  link.textContent = panel?.labels?.open || 'Open';
  node.append(document.createTextNode(' '), link);
  return node;
}

function preAttach() {
  const { ref: type, id, label } = route.query;
  if (typeof type !== 'string' || typeof id !== 'string' || !CONTEXT_ICONS[type]) return;
  if (page.value) page.value.references = [{ type, id, label: typeof label === 'string' && label ? label : id }];
}

function followRoute(id) {
  const next = String(id || '');
  if (next === activeId()) return;
  if (page.value) page.value.references = [];
  if (next) void chat.open(next).then(() => page.value?.scrollToBottom({ force: true }));
  else chat.startDraft();
}

const stopRouteWatch = watch(() => route.params.id, (id) => {
  const next = String(id || '');
  if (!aligned) {
    sawQueuedRoute = true;
    queuedRoute = next;
    return;
  }
  if (next === activeId()) return;
  followRoute(next);
});

onMounted(async () => {
  // The full page does what the side panel does, with more room.
  assistantContext.open = false;
  const el = page.value;
  el.chat = chat;
  el.disabled = true;
  el.defineEntry('proposal', renderProposalEntry);
  el.defineEntry('notice', renderNotice);
  el.defineEntry('context-marker', renderContextMarkerEntry);

  // Before bindAgentPanel(), so it runs before the binding's agent-submit.
  el.addEventListener('agent-submit', captureSubmitReferences);
  el.addEventListener('agent-select-conversation', onSelectConversation);
  el.addEventListener('agent-new-conversation', onNewConversation);
  el.addEventListener('agent-delete-conversation', onDeleteConversation);
  el.addEventListener('agent-action', onAction);
  el.addEventListener('agent-mention', onMention);
  el.addEventListener('agent-mention-key', onMentionKey);
  el.addEventListener('agent-reference-open', onReferenceOpen);
  el.addEventListener('agent-model-change', onModelChange);

  binding = bindAgentPanel(el, chat, {
    registerEntries: false,
    errorText: (code) => chatErrorText(code, $t('agent.noProvider')),
    contextReferences: referencesFromContext,
  });
  const removeConversation = chat.remove.bind(chat);
  chat.remove = (id) => {
    const deletingOpen = String(id) === activeId();
    if (deletingOpen) urlIntent = 'push';
    const pending = removeConversation(id);
    Promise.resolve(pending).then((ok) => {
      if (deletingOpen && ok !== true && activeId() === routeId()) urlIntent = 'replace';
    });
    return pending;
  };
  chat.addEventListener('change', onChange);
  unsubscribeAuth = pb.authStore.onChange(async () => {
    // setAccount clears the state, which would look like "no conversation";
    // don't rewrite the URL while the route's own conversation is reloaded.
    switchingAccount = true;
    const id = String(route.params.id || '');
    try {
      await chat.setAccount(accountKey());
      if (id) await chat.open(id);
    } finally {
      switchingAccount = false;
      syncUrl();
    }
  });

  await chat.init();
  const id = String(route.params.id || '');
  if (id) await chat.open(id);
  else chat.startDraft();
  aligned = true;
  const queued = sawQueuedRoute ? queuedRoute : undefined;
  sawQueuedRoute = false;
  preselectModel();
  preAttach();
  if (queued !== undefined && queued !== activeId()) followRoute(queued);
  else onChange();
  if (routeId()) page.value?.scrollToBottom({ force: true });
});

onBeforeUnmount(() => {
  stopRouteWatch();
  binding?.unbind();
  chat.dispose();
  unsubscribeAuth?.();
});
</script>

<style scoped>
.agent-host {
  height: calc(100vh - 112px);
  min-height: 520px;
}

.agent-notice {
  margin-bottom: 8px;
}

.agent-page {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 12px;
  display: block;
  height: 100%;
  overflow: hidden;
}

.agent-actions {
  align-items: center;
  display: inline-flex;
  flex-wrap: wrap;
  gap: 2px;
}
</style>
