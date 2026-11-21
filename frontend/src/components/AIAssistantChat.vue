<template>
  <AppRightSidebar v-model="assistantContext.open" :width="440" overlay disable-route-watcher :show-resize-controls="false">
    <v-alert v-if="actionError" type="error" variant="tonal" density="compact" closable class="mx-3 mt-3" data-cy="assistant-action-error" @click:close="actionError = ''">
      {{ actionError }}
    </v-alert>

    <lp-agent-panel
      ref="panel"
      class="assistant-panel"
      conversations-menu
      closable
      sticky-attachments
      :addMenu="addMenu"
      :formatDate="formatDate"
      :suggestions="suggestions"
      placeholder="Message the assistant…"
      hint="You review changes before they apply"
      empty-title="How can I help?"
      empty-text="Ask about your data, plan a campaign, or edit what you're working on. You review every change first."
      data-cy="assistant-panel"
      @agent-menu-select="onMenuSelect"
    >
      <span slot="header-start" class="ai-mark" aria-hidden="true"><v-icon icon="mdi-creation-outline" size="16" /></span>
      <div slot="switcher-end" class="pa-1">
        <v-btn v-if="conversationOpen" block size="small" variant="text" class="text-none justify-start" prepend-icon="mdi-pencil-outline" data-cy="assistant-rename" @click="openRename">Rename</v-btn>
        <v-btn block size="small" variant="text" class="text-none justify-start" prepend-icon="mdi-open-in-new" :to="{ name: 'agent' }" data-cy="assistant-open-agent" @click="assistantContext.open = false">Open the full Agent page</v-btn>
      </div>
      <div slot="subheader" class="ai-subhead">
        <span v-if="contextLabel" class="ai-context text-truncate" :title="`Looking at: ${contextLabel}`" data-cy="assistant-context">{{ contextLabel }}</span>
      </div>
      <div v-if="undo || canRetry" slot="transcript-end" class="d-flex flex-wrap ga-2">
        <v-btn v-if="undo" size="small" variant="tonal" class="text-none" prepend-icon="mdi-undo" data-cy="assistant-undo" @click="undoEdit">Undo last edit</v-btn>
        <v-btn v-if="canRetry" size="small" variant="tonal" class="text-none" data-cy="assistant-retry" @click="retry">Retry</v-btn>
      </div>
      <span v-if="pictures.length" slot="composer-chips" class="d-contents" data-cy="assistant-attached">
        <v-chip v-for="(picture, index) in pictures" :key="picture.url" closable size="small" prepend-icon="mdi-image-outline" :title="picture.url" data-cy="assistant-picture" @click:close="removePicture(index)">{{ picture.name || picture.url.split('/').pop() }}</v-chip>
      </span>
    </lp-agent-panel>

    <v-dialog v-model="mediaOpen" max-width="900"><v-card><v-card-text class="pt-0"><Media v-if="mediaOpen" is-modal type="pictures" @selected="addPicture" @close="mediaOpen = false" /></v-card-text></v-card></v-dialog>
    <review-modal
      v-if="reviewPart"
      :key="reviewKey"
      :part="reviewPart"
      :busy="Boolean(busyId && reviewPart && busyId === reviewPart.id)"
      :error="reviewError"
      @close="closeReview"
      @apply="applyFromReview"
      @reject="rejectFromReview"
    />
    <v-dialog v-model="renaming" max-width="420">
      <v-card>
        <v-card-title>Rename</v-card-title>
        <v-card-text>
          <v-text-field v-model="renameTitle" label="Title" density="compact" variant="outlined" hide-details data-cy="assistant-rename-title" @keydown.enter.prevent="saveRename" />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn class="text-none" @click="renaming = false">Cancel</v-btn>
          <v-btn color="primary" variant="flat" class="text-none" :disabled="!renameTitle.trim()" data-cy="assistant-rename-save" @click="saveRename">Save</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </AppRightSidebar>
</template>

<script setup>
import {
  computed, defineAsyncComponent, getCurrentInstance, onBeforeUnmount, onMounted, ref, shallowRef, watch,
} from 'vue';
import { useRoute } from 'vue-router';
import AppRightSidebar from './AppRightSidebar.vue';
import { pb } from '../api';
import {
  bindAgentPanel, renderContextMarkerEntry,
} from '@pocketbase-mcp/chat-client/agent-panel';
import { ChatController } from '@pocketbase-mcp/chat-client';
import { ChatClient } from '@pocketbase-mcp/chat-client';
import { assistantContext } from '../assistant/context';
import {
  chatErrorText, choiceIdFor, cleanPictures, clearLegacyAssistantStorage,
  createEditorAdapter, editorContextOf, editorPrecheckError, moveDraftPictures,
  panelPageContext, picturesFor, referencesFromContext, retryPayload, reviewPartOf,
  savePictures, storedModelFor, undoLastEdit,
} from '../assistant/chat-host.mjs';
import {
  clearHostComposer, createSubmitCapture, decideModuleProposal, holdComposerUntilReady,
  proposalFor, renderModuleProposal, renderNoticeWithOpen,
} from '../assistant/chat-ui.mjs';
import { applyEditorPatch, revision } from '../assistant/patches.mjs';
// Defines <lp-agent-panel>. The module host feeds it through the chat binding.
import '../assistant/lp-agent-panel';
import ReviewModal from './agent/ReviewModal.vue';
import { shortStamp } from '../utils/crm.mjs';

const Media = defineAsyncComponent(() => import('../views/Media.vue'));

const MODEL_KEY = 'listpocket_agent_model';
const CHAT_MOUNT = '/api/mcp/chat/v1';

const { proxy } = getCurrentInstance();
const route = useRoute();
const panel = ref(null);
const pictures = ref([]);
const conversationOpen = ref(false);
const canRetry = ref(false);
const sending = ref(false);
const actionError = ref('');
const mediaOpen = ref(false);
const renaming = ref(false);
const renameTitle = ref('');
const undo = shallowRef(null);
const reviewProposal = ref(null);
const reviewKey = ref(0);
const reviewError = ref('');
const busyId = ref('');
let binding = null;
let unsubscribeAuth = null;
let unmounted = false;
let heldUntilReady = true;
let loadedAccount = '';
let accountEpoch = 0;
// Set when a submit starts with no conversation, so that draft's pictures
// follow the id the first send creates and are not left on the draft.
let pendingDraftMove = false;
let pictureConversation = '';
const submitCapture = createSubmitCapture();
const formatDate = shortStamp;
const addMenu = computed(() => [
  'attach',
  'paste',
  { id: 'media', label: 'Add a picture from media', icon: 'image', disabled: sending.value },
]);

function onMenuSelect(event) {
  if (event.detail?.id !== 'media') return;
  event.preventDefault();
  if (!sending.value) mediaOpen.value = true;
}

const reviewPart = computed(() => (reviewProposal.value ? reviewPartOf(reviewProposal.value) : null));
const contextLabel = computed(() => {
  if (assistantContext.editor?.label) return assistantContext.editor.label;
  const title = route.meta?.title;
  return typeof title === 'string' && title ? proxy.$t(title) : '';
});
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

function userId() {
  return pb.authStore.record?.id || 'signed-out';
}

function accountKey() {
  return {
    appUrl: pb.baseURL,
    mount: CHAT_MOUNT,
    authCollectionId: pb.authStore.record?.collectionId || 'users',
    userId: userId(),
  };
}

function selectionText() {
  try {
    return window.getSelection?.()?.toString?.() || '';
  } catch {
    return '';
  }
}

function activePictureId() {
  return chat.state.activeConversationId ? String(chat.state.activeConversationId) : '';
}

const editorAdapter = createEditorAdapter({
  getEditor: () => assistantContext.editor,
  revisionOf: revision,
  applyPatch: applyEditorPatch,
  onApplied: (entry) => {
    undo.value = entry;
    actionError.value = '';
  },
});

const client = new ChatClient({
  baseUrl: pb.baseURL,
  getToken: () => pb.authStore.token,
});
const chat = new ChatController({
  client,
  storageKey: accountKey(),
  editor: editorAdapter,
  context: () => panelPageContext({
    path: route.path,
    page: typeof route.name === 'string' ? route.name : '',
    params: route.params,
    filters: route.query,
    references: submitCapture.consume(panel.value?.references),
    editor: editorContextOf(assistantContext.editor, revision, selectionText()),
    pictures: pictures.value,
  }),
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
  if (id && panel.value) {
    panel.value.model = id;
    binding?.refresh();
  }
}

function loadPictures(conversationId) {
  pictureConversation = conversationId || '';
  pictures.value = picturesFor(localStorage, userId(), pictureConversation);
}

function syncPictures(state) {
  const id = state.activeConversationId ? String(state.activeConversationId) : '';
  if (pendingDraftMove && id) {
    pendingDraftMove = false;
    pictures.value = moveDraftPictures(localStorage, userId(), id);
    pictureConversation = id;
    return;
  }
  if (id === pictureConversation) return;
  loadPictures(id);
}

function onChange() {
  const state = chat.state;
  conversationOpen.value = Boolean(state.activeConversationId);
  canRetry.value = retryPayload(state) !== null;
  sending.value = state.running === true || state.sending === true;
  syncPictures(state);
  heldUntilReady = holdComposerUntilReady(panel.value, state, heldUntilReady);
}

function onPanelSubmit(event) {
  submitCapture.onSubmit(event);
  pendingDraftMove = !chat.state.activeConversationId;
  undo.value = null;
}

function onSelectConversation() {
  pendingDraftMove = false;
  undo.value = null;
}

function onNewConversation() {
  pendingDraftMove = false;
  undo.value = null;
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
    element: panel.value,
    id,
    decision,
    edited,
    isBusy: (value) => busyId.value === value,
    setBusy: (value) => { busyId.value = value; },
    onStart: () => { actionError.value = ''; },
  });
}

function openReview(id) {
  const entries = panel.value?.entries || [];
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
  const entries = panel.value?.entries || [];
  const entry = entries.find((item) => item
    && item.type === 'proposal'
    && (item.proposalId === id || item.proposal?.id === id));
  const proposal = proposalFor(chat, id, entry?.proposal);
  const precheck = editorPrecheckError(proposal, assistantContext.editor);
  if (precheck) {
    actionError.value = precheck;
    return;
  }
  const out = await decideProposal(id, 'approve');
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

function renderProposalEntry(entry, host) {
  return renderModuleProposal(entry, host, { chat, busyId: busyId.value });
}

function openRename() {
  const id = chat.state.activeConversationId;
  if (!id) return;
  const row = (chat.state.conversations || []).find((item) => item && String(item.id) === String(id));
  renameTitle.value = row?.title || '';
  renaming.value = true;
}

async function saveRename() {
  const id = chat.state.activeConversationId;
  const title = renameTitle.value.trim();
  renaming.value = false;
  if (!id || !title) return;
  try {
    await chat.rename(String(id), title);
  } catch (err) {
    actionError.value = err?.message || 'Could not rename this chat.';
  }
}

async function undoEdit() {
  const entry = undo.value;
  const verdict = undoLastEdit(entry, assistantContext.editor, revision);
  if (!verdict.ok) {
    actionError.value = verdict.message;
    undo.value = null;
    return;
  }
  try {
    await entry.editor.apply(verdict.before);
    undo.value = null;
    actionError.value = '';
  } catch (err) {
    actionError.value = err?.message || verdict.message;
    undo.value = null;
  }
}

function retry() {
  const payload = retryPayload(chat.state);
  if (!payload) return;
  undo.value = null;
  const model = typeof panel.value?.model === 'string' ? panel.value.model : '';
  const effort = typeof panel.value?.optionValues?.effort === 'string' ? panel.value.optionValues.effort : '';
  const options = {
    ...(model ? { model } : {}),
    ...(effort ? { effort } : {}),
    ...(payload.attachments.length ? { attachments: payload.attachments } : {}),
  };
  if (payload.references.length) {
    options.context = panelPageContext({
      path: route.path,
      page: typeof route.name === 'string' ? route.name : '',
      params: route.params,
      filters: route.query,
      references: payload.references,
      editor: editorContextOf(assistantContext.editor, revision, selectionText()),
      pictures: pictures.value,
    });
  }
  void chat.send(payload.text, options);
}

function addPicture(media) {
  mediaOpen.value = false;
  if (!media?.url) return;
  const next = cleanPictures([...(pictures.value || []), { name: media.filename || media.name || '', url: media.url }]);
  pictures.value = next;
  savePictures(localStorage, userId(), activePictureId(), next);
}

function removePicture(index) {
  const next = pictures.value.filter((_, at) => at !== index);
  pictures.value = next;
  savePictures(localStorage, userId(), activePictureId(), next);
}

async function refreshIfUnavailable() {
  if (unmounted || chat.state.status === 'loading') return;
  if (chat.state.status !== 'error' && chat.state.features?.ready !== false) return;
  const epoch = accountEpoch;
  await chat.init();
  if (unmounted || epoch !== accountEpoch) return;
  if (chat.state.status === 'ready') preselectModel();
  onChange();
}

function onRefreshTrigger() {
  void refreshIfUnavailable();
}

watch(() => assistantContext.editor, (next, prev) => {
  if (next !== prev) undo.value = null;
});

watch(() => assistantContext.open, (open) => {
  if (open) void refreshIfUnavailable();
});

onMounted(async () => {
  const mountEpoch = accountEpoch;
  clearLegacyAssistantStorage(localStorage);
  const el = panel.value;
  el.chat = chat;
  el.disabled = true;
  el.formatDate = formatDate;
  el.defineEntry('proposal', renderProposalEntry);
  el.defineEntry('notice', renderNoticeWithOpen);
  el.defineEntry('context-marker', renderContextMarkerEntry);

  el.addEventListener('agent-submit', onPanelSubmit);
  el.addEventListener('agent-select-conversation', onSelectConversation);
  el.addEventListener('agent-new-conversation', onNewConversation);
  el.addEventListener('agent-action', onAction);
  el.addEventListener('agent-model-change', onModelChange);
  el.addEventListener('agent-close', () => { assistantContext.open = false; });

  binding = bindAgentPanel(el, chat, {
    registerEntries: false,
    errorText: (code) => chatErrorText(code, proxy.$t('agent.noProvider')),
    contextReferences: referencesFromContext,
  });
  const removeConversation = chat.remove.bind(chat);
  chat.remove = (id) => {
    const key = id == null ? '' : String(id);
    const deletingOpen = key !== '' && key === activePictureId();
    const owner = userId();
    const pending = removeConversation(id);
    Promise.resolve(pending).then((ok) => {
      if (ok !== true || key === '') return;
      savePictures(localStorage, owner, key, []);
      if (deletingOpen) {
        pendingDraftMove = false;
        undo.value = null;
      }
    });
    return pending;
  };
  chat.addEventListener('change', onChange);
  window.addEventListener('focus', onRefreshTrigger);
  window.addEventListener('ai-settings-updated', onRefreshTrigger);
  const currentAccountId = () => {
    const key = accountKey();
    return `${key.authCollectionId}:${key.userId}`;
  };
  loadedAccount = currentAccountId();
  unsubscribeAuth = pb.authStore.onChange(async () => {
    const next = currentAccountId();
    if (next === loadedAccount) return;
    loadedAccount = next;
    accountEpoch += 1;
    const epoch = accountEpoch;
    submitCapture.clear();
    clearHostComposer(panel.value);
    undo.value = null;
    pendingDraftMove = false;
    actionError.value = '';
    closeReview();
    mediaOpen.value = false;
    renaming.value = false;
    renameTitle.value = '';
    pictures.value = [];
    pictureConversation = '';
    await chat.setAccount(accountKey());
    if (unmounted || epoch !== accountEpoch) return;
    if (chat.state.status === 'ready') {
      chat.startDraft();
      preselectModel();
    }
    loadPictures('');
    onChange();
  });

  await chat.init();
  if (unmounted || mountEpoch !== accountEpoch) return;
  if (chat.state.status === 'ready') {
    chat.startDraft();
    preselectModel();
  }
  loadPictures(chat.state.activeConversationId ? String(chat.state.activeConversationId) : '');
  onChange();
});

onBeforeUnmount(() => {
  unmounted = true;
  window.removeEventListener('focus', onRefreshTrigger);
  window.removeEventListener('ai-settings-updated', onRefreshTrigger);
  binding?.unbind();
  chat.dispose();
  unsubscribeAuth?.();
});
</script>

<style scoped>
.assistant-panel { display: block; height: 100%; }
.ai-mark { align-items: center; background: rgba(var(--v-theme-primary), 0.14); border-radius: 8px; color: rgb(var(--v-theme-primary)); display: flex; flex: none; height: 28px; justify-content: center; width: 28px; }
.ai-subhead { align-items: center; display: flex; gap: 8px; min-width: 0; }
.ai-context { color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity)); font-size: 12px; margin-left: auto; min-width: 0; }
.d-contents { display: contents; }
</style>
