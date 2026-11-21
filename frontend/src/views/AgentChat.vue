<template>
  <section class="agent-host" data-cy="agent-page">
    <v-alert v-if="actionError" type="error" variant="tonal" density="compact" closable class="agent-notice" data-cy="agent-action-error" @click:close="actionError = ''">
      {{ actionError }}
    </v-alert>

    <lp-agent-page
      ref="page"
      class="agent-page"
      mention-trigger="@"
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
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, onBeforeUnmount, onMounted, ref, watch,
} from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { pb } from '../api';
import {
  bindAgentPanel, renderContextMarkerEntry,
} from '@pocketbase-mcp/chat-client/agent-panel';
import { ChatController } from '@pocketbase-mcp/chat-client';
import { ChatClient } from '@pocketbase-mcp/chat-client';
import { assistantContext } from '../assistant/context';
import {
  AGENT_EDITOR_PROPOSAL_TEXT, chatErrorText, choiceIdFor, contextReferencesFromTranscript,
  navigationWrite, pageContext, referencesFromContext, reviewPartOf, storedModelFor,
} from '../assistant/chat-host.mjs';
import {
  clearHostComposer, createSubmitCapture, decideModuleProposal, holdComposerUntilReady,
  proposalFor, renderModuleProposal, renderNoticeWithOpen,
} from '../assistant/chat-ui.mjs';
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
const reviewProposal = ref(null);
const reviewKey = ref(0);
const reviewError = ref('');
const busyId = ref('');
let binding = null;
let unsubscribeAuth = null;
let unmounted = false;
let accountEpoch = 0;
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
// The @ chips of the submit being sent. Registered before the binding's
// agent-submit listener, because a message with files has its chips cleared
// before the binding reads the context.
const submitCapture = createSubmitCapture();

const reviewPart = computed(() => (reviewProposal.value ? reviewPartOf(reviewProposal.value) : null));
const formatDate = shortStamp;
const suggestions = computed(() => [$t('agent.suggest.overview'), $t('agent.suggest.replies'), $t('agent.suggest.workflow')]);

const labels = computed(() => ({
  addContext: $t('agent.context'),
  addFile: $t('agent.attach'),
  attach: $t('agent.attachFile'),
  attachFiles: $t('agent.attachFile'),
  attachHint: $t('agent.attachFile'),
  cancel: $t('globals.buttons.cancel'),
  conversations: $t('agent.conversations'),
  defaultModel: $t('agent.defaultModel'),
  defaultModelNamed: (model) => $t('agent.defaultNamed', { model }),
  delete: $t('globals.buttons.delete'),
  deleteConversation: (name) => `${$t('globals.buttons.delete')} ${name}`,
  discard: $t('agent.reject'),
  inContext: $t('agent.inContext'),
  loadMore: $t('agent.loadMore'),
  newChat: $t('agent.newChat'),
  noConversations: $t('agent.noConversations'),
  open: $t('agent.open'),
  pasteContent: $t('agent.pasteContent'),
  pasteName: $t('agent.pasteName'),
  pasteText: $t('agent.pasteData'),
  rename: $t('agent.rename'),
  review: $t('agent.review.title'),
  searchModels: $t('agent.searchModels'),
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
  context: () => pageContext(route.fullPath, submitCapture.consume(page.value?.references)),
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
  if (id && page.value) {
    page.value.model = id;
    binding?.refresh();
  }
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

function syncComposer(state) {
  heldUntilReady = holdComposerUntilReady(page.value, state, heldUntilReady);
}

function onChange() {
  const state = chat.state;
  syncUrl();
  inContext.value = contextReferencesFromTranscript(state.transcript);
  renamable.value = Boolean(state.activeConversationId);
  syncComposer(state);
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

function decideProposal(id, decision, edited) {
  return decideModuleProposal({
    chat,
    element: page.value,
    id,
    decision,
    edited,
    isBusy: (value) => busyId.value === value,
    setBusy: (value) => { busyId.value = value; },
    onStart: () => { actionError.value = ''; },
  });
}

// entry.proposal is the stored proposal. The binding omits the controller's
// `error` (a 409 conflict or a 422 refusal), so that one field still comes
// from state; everything else the card and the review modal show is on the entry.

function openReview(id) {
  const entries = page.value?.entries || [];
  const entry = entries.find((item) => item
    && item.type === 'proposal'
    && (item.proposalId === id || item.proposal?.id === id));
  const proposal = proposalFor(chat, id, entry?.proposal);
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
  const entries = page.value?.entries || [];
  const entry = entries.find((item) => item
    && item.type === 'proposal'
    && (item.proposalId === id || item.proposal?.id === id));
  const proposal = proposalFor(chat, id, entry?.proposal);
  if (proposal && (proposal.location === 'editor' || proposal.tool === 'propose_editor_patch')) {
    actionError.value = AGENT_EDITOR_PROPOSAL_TEXT;
    return;
  }
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

function pickContext(item) {
  page.value?.addReference({ type: item.type, id: item.id, label: item.label });
}

function renderProposalEntry(entry, panel) {
  return renderModuleProposal(entry, panel, { chat, busyId: busyId.value });
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

async function refreshIfUnavailable(force = false) {
  if (unmounted || chat.state.status === 'loading') return;
  if (!force && chat.state.status !== 'error' && chat.state.features?.ready !== false) return;
  const epoch = accountEpoch;
  await chat.init();
  if (unmounted || epoch !== accountEpoch) return;
  if (chat.state.status === 'ready') preselectModel();
  onChange();
}

function onRefreshTrigger(event) {
  // Settings can change a healthy catalog's default model and effort levels too.
  void refreshIfUnavailable(event?.type === 'ai-settings-updated');
}

onMounted(async () => {
  const mountEpoch = accountEpoch;
  // The full page does what the side panel does, with more room.
  assistantContext.open = false;
  const el = page.value;
  el.chat = chat;
  el.disabled = true;
  el.defineEntry('proposal', renderProposalEntry);
  el.defineEntry('notice', renderNoticeWithOpen);
  el.defineEntry('context-marker', renderContextMarkerEntry);

  // Before bindAgentPanel(), so it runs before the binding's agent-submit.
  el.addEventListener('agent-submit', submitCapture.onSubmit);
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
  window.addEventListener('focus', onRefreshTrigger);
  window.addEventListener('ai-settings-updated', onRefreshTrigger);
  let loadedAccount = `${accountKey().authCollectionId}:${accountKey().userId}`;
  unsubscribeAuth = pb.authStore.onChange(async () => {
    const key = accountKey();
    const next = `${key.authCollectionId}:${key.userId}`;
    if (next === loadedAccount) return;
    loadedAccount = next;
    accountEpoch += 1;
    const epoch = accountEpoch;
    submitCapture.clear();
    clearHostComposer(page.value);
    actionError.value = '';
    closeReview();
    previewing.value = null;
    picker.value = null;
    inContext.value = [];
    // setAccount clears the state, which would look like "no conversation";
    // don't rewrite the URL while the route's own conversation is reloaded.
    switchingAccount = true;
    const id = String(route.params.id || '');
    try {
      await chat.setAccount(key);
      if (unmounted || epoch !== accountEpoch) return;
      if (chat.state.status === 'ready') {
        if (id) await chat.open(id);
        if (unmounted || epoch !== accountEpoch) return;
        preselectModel();
      }
    } finally {
      if (epoch === accountEpoch) {
        switchingAccount = false;
        if (!unmounted) syncUrl();
      }
    }
  });

  await chat.init();
  if (unmounted || mountEpoch !== accountEpoch) return;
  const id = String(route.params.id || '');
  if (chat.state.status === 'ready') {
    if (id) await chat.open(id);
    else chat.startDraft();
  }
  if (unmounted || mountEpoch !== accountEpoch) return;
  aligned = true;
  const queued = sawQueuedRoute ? queuedRoute : undefined;
  sawQueuedRoute = false;
  if (chat.state.status === 'ready') preselectModel();
  preAttach();
  if (queued !== undefined && queued !== activeId()) followRoute(queued);
  else onChange();
  if (routeId()) page.value?.scrollToBottom({ force: true });
});

onBeforeUnmount(() => {
  unmounted = true;
  window.removeEventListener('focus', onRefreshTrigger);
  window.removeEventListener('ai-settings-updated', onRefreshTrigger);
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
</style>
