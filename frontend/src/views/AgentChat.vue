<template>
  <section class="agent-host" data-cy="agent-page">
    <v-alert v-if="noProvider" type="info" variant="tonal" density="compact" class="agent-notice" data-cy="agent-no-provider">
      {{ $t('agent.noProvider') }}
    </v-alert>
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
      :disabled="noProvider"
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
      <span slot="sidebar-end" class="agent-sidebar-end">
        <v-btn v-if="hasMore" block size="small" variant="text" class="text-none" data-cy="agent-load-more" @click="loadMore">{{ $t('agent.loadMore') }}</v-btn>
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
import { ChatClient } from '@pocketbase-mcp/chat-client/client';
import { assistantContext } from '../assistant/context';
import {
  cardProposalOf, choiceIdFor, contextReferencesFromTranscript,
  pageContext, referencesOf, reviewPartOf, storedModelFor,
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
const hasMore = ref(false);
const renamable = ref(false);
const noProvider = ref(false);
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
// The references of the message being submitted. The controller reads them at
// send time, after the binding's agent-submit handler has started the turn.
let submittedReferences = [];
let pendingSubmit = null;

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
  context: () => pageContext(route.fullPath, submittedReferences),
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

function syncUrl() {
  if (!aligned || switchingAccount) return;
  const id = chat.state.activeConversationId ? String(chat.state.activeConversationId) : '';
  const current = String(route.params.id || '');
  if (id === current) return;
  router.replace({ name: 'agent', params: id ? { id } : {} });
}

// A send that failed before the server accepted it puts the draft and its
// references back, as the legacy page did. Upload failures are reported by the
// binding itself and keep their chips.
function restoreFailedSend(submit) {
  const el = page.value;
  if (!el || !submit) return;
  if (!el.draft) el.draft = submit.text;
  if (el.references.length === 0 && submit.references.length > 0) el.references = submit.references;
}

function onChange() {
  const state = chat.state;
  syncUrl();
  inContext.value = contextReferencesFromTranscript(state.transcript);
  hasMore.value = Boolean(state.nextCursor);
  renamable.value = Boolean(state.activeConversationId);
  noProvider.value = state.status === 'ready'
    && state.models.length === 0 && state.choices.length === 0;
  if (pendingSubmit) {
    if (state.running) {
      // The controller has read this turn's context; the next send captures its own.
      pendingSubmit = null;
      submittedReferences = [];
    } else if (state.error) {
      restoreFailedSend(pendingSubmit);
      pendingSubmit = null;
    }
  }
}

// Registered before bindAgentPanel(), so it runs before the binding's own
// agent-submit handler and captures this message's chips before any await.
function onSubmit(event) {
  submittedReferences = referencesOf(event.detail?.references);
  pendingSubmit = {
    text: typeof event.detail?.text === 'string' ? event.detail.text : '',
    references: submittedReferences,
  };
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

function openReview(id) {
  const proposal = chat.state.proposals.find((item) => item && item.id === id);
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

function loadMore() {
  void binding?.loadMore();
}

// One card per proposal the binding reports; the full proposal (arguments,
// warnings, result, error) lives in the controller's state.
function renderProposalEntry(entry, panel) {
  const proposals = panel.chat?.state?.proposals || [];
  const full = proposals.find((item) => item && item.id === entry.proposalId) || {
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

function preAttach() {
  const { ref: type, id, label } = route.query;
  if (typeof type !== 'string' || typeof id !== 'string' || !CONTEXT_ICONS[type]) return;
  if (page.value) page.value.references = [{ type, id, label: typeof label === 'string' && label ? label : id }];
}

const stopRouteWatch = watch(() => route.params.id, (id) => {
  if (!aligned) return;
  const next = String(id || '');
  const current = chat.state.activeConversationId ? String(chat.state.activeConversationId) : '';
  if (next === current) return;
  if (page.value) page.value.references = [];
  if (next) void chat.open(next).then(() => page.value?.scrollToBottom({ force: true }));
  else chat.startDraft();
});

onMounted(async () => {
  // The full page does what the side panel does, with more room.
  assistantContext.open = false;
  const el = page.value;
  el.chat = chat;
  el.defineEntry('proposal', renderProposalEntry);
  el.defineEntry('notice', renderNoticeEntry);
  el.defineEntry('context-marker', renderContextMarkerEntry);

  // Before bindAgentPanel(): this listener must run before the binding's
  // agent-submit, which starts the turn that reads the page context.
  el.addEventListener('agent-submit', onSubmit);
  el.addEventListener('agent-action', onAction);
  el.addEventListener('agent-mention', onMention);
  el.addEventListener('agent-mention-key', onMentionKey);
  el.addEventListener('agent-reference-open', onReferenceOpen);
  el.addEventListener('agent-model-change', onModelChange);

  binding = bindAgentPanel(el, chat, { registerEntries: false });
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
  preselectModel();
  preAttach();
  onChange();
  if (id) page.value?.scrollToBottom({ force: true });
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

.agent-sidebar-end {
  display: block;
  padding: 4px;
}
</style>
