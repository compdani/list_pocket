<template>
  <AppRightSidebar
    :model-value="modelValue"
    :width="520"
    @update:model-value="onDialogToggle"
  >
    <template #header>
      <v-toolbar density="comfortable" flat class="ai-chat-toolbar px-2">
        <div class="d-flex flex-column flex-grow-1 py-1" style="min-width: 0">
          <span class="text-subtitle-1 font-weight-semibold text-truncate">AI assistant</span>
          <span class="text-caption text-medium-emphasis">Campaign threads saved in this browser</span>
        </div>
        <v-btn size="small" variant="text" :disabled="isRunning" @click="clearCurrentThread">
          Clear chat
        </v-btn>
      </v-toolbar>
    </template>

    <div class="ai-chat-body d-flex flex-column h-100">
      <div class="ai-chat-thread-row px-2 py-2 border-b d-flex align-center ga-2 flex-wrap">
        <v-select
          :model-value="activeThreadId"
          :items="threadsSorted"
          item-title="title"
          item-value="id"
          density="compact"
          variant="outlined"
          hide-details
          label="Thread"
          class="thread-select flex-grow-1"
          style="min-width: 140px; max-width: 260px"
          :disabled="isRunning"
          @update:model-value="onSelectThread"
        />
        <v-btn size="small" color="primary" variant="tonal" :disabled="isRunning" @click="newThread">
          New thread
        </v-btn>
        <v-menu location="bottom end">
          <template #activator="{ props }">
            <v-btn v-bind="props" icon size="small" variant="text" :disabled="isRunning">
              <v-icon>mdi-dots-vertical</v-icon>
            </v-btn>
          </template>
          <v-list density="compact">
            <v-list-item
              title="Delete this thread"
              prepend-icon="mdi-trash-can-outline"
              base-color="error"
              @click="deleteActiveThread"
            />
          </v-list>
        </v-menu>
      </div>

      <div class="ai-chat-messages flex-grow-1 px-3 py-3">
        <div v-if="messages.length === 0" class="chat-welcome text-body-2 text-medium-emphasis text-center pa-6">
          Start a thread for this campaign. Each thread keeps its own history, reference files, and picture URLs.
        </div>

        <div
          v-for="(message, index) in messages"
          :key="`${message.role}-${index}-${message.content?.slice(0, 12)}`"
          class="chat-row"
          :class="message.role === 'user' ? 'chat-row--user' : 'chat-row--assistant'"
        >
          <div class="chat-bubble" :class="message.role === 'user' ? 'chat-bubble--user' : 'chat-bubble--assistant'">
            <div class="chat-bubble-meta">{{ message.role === 'user' ? 'You' : 'Assistant' }}</div>
            <div class="chat-bubble-text">{{ message.content }}</div>
          </div>
        </div>

        <v-alert v-if="errorMessage" type="error" variant="tonal" density="compact" class="mt-3">
          {{ errorMessage }}
        </v-alert>

        <v-alert
          v-if="job.status && job.status !== 'success' && job.status !== 'failed' && job.status !== 'canceled'"
          type="info"
          variant="tonal"
          density="compact"
          class="mt-2"
        >
          Generation: {{ job.status }} ({{ job.progress || 0 }}%)
        </v-alert>
        <v-alert v-if="job.status === 'canceled'" type="warning" variant="tonal" density="compact" class="mt-2">
          Generation cancelled.
        </v-alert>
        <v-alert v-if="job.status === 'success' && job.result.notes" type="success" variant="tonal" density="compact" class="mt-2">
          {{ job.result.notes }}
        </v-alert>

        <v-card v-if="pendingResult" variant="outlined" class="mt-4 draft-preview-card">
          <v-card-title class="text-subtitle-2">Draft preview</v-card-title>
          <v-card-text>
            <div v-if="pendingResult.subject" class="mb-2">
              <strong>Subject:</strong> {{ pendingResult.subject }}
            </div>
            <div v-if="pendingResult.preheader" class="mb-2">
              <strong>Preheader:</strong> {{ pendingResult.preheader }}
            </div>
            <div class="text-caption mb-1">Body (before → after)</div>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  :model-value="(current && current.body) || ''"
                  label="Before"
                  readonly
                  rows="6"
                  variant="outlined"
                  density="compact"
                />
              </v-col>
              <v-col cols="12">
                <v-textarea
                  :model-value="pendingResult.body || ''"
                  label="After"
                  readonly
                  rows="6"
                  variant="outlined"
                  density="compact"
                />
              </v-col>
            </v-row>
          </v-card-text>
          <v-card-actions class="justify-end">
            <v-btn variant="text" @click="discardDraft">Discard</v-btn>
            <v-btn color="primary" @click="applyDraft">Apply to editor</v-btn>
          </v-card-actions>
        </v-card>

        <div ref="chatAnchor" class="chat-anchor" aria-hidden="true" />
      </div>

      <v-expansion-panels v-model="attachmentsPanel" class="ai-chat-panels border-t" flat>
        <v-expansion-panel rounded="0">
          <v-expansion-panel-title class="text-body-2">
            Reference files &amp; pictures
          </v-expansion-panel-title>
          <v-expansion-panel-text class="pa-2">
            <p class="text-caption text-medium-emphasis mb-3">
              Reference files go to the model as context. Pictures below add hosted URLs into the written instructions (for the email), not the same as attaching an image file.
              Paste files or screenshots with Ctrl+V / ⌘V while the cursor is in the message box below.
            </p>
            <div class="text-caption font-weight-medium mb-1">Reference files</div>
            <v-file-input
              v-model="statusFilePickerModel"
              label="Add files"
              multiple
              chips
              :disabled="isRunning"
              prepend-icon="mdi-paperclip"
              accept=".pdf,.doc,.docx,.txt,.rtf,image/*"
              show-size
              clearable
              variant="outlined"
              density="compact"
              hide-details="auto"
              class="mb-2"
              @update:model-value="onStatusFilesPicked"
            />
            <div v-if="statusFiles.length" class="d-flex flex-wrap ga-1 mb-4">
              <v-chip
                v-for="(f, i) in statusFiles"
                :key="f.id"
                size="small"
                closable
                @click:close="removeStatusFile(i)"
              >
                {{ f.name }} ({{ formatStatusFileSize(f.size) }})
              </v-chip>
            </div>
            <div class="d-flex align-center justify-space-between mb-1">
              <span class="text-caption font-weight-medium">Pictures (URLs in instructions)</span>
              <v-btn size="x-small" variant="outlined" prepend-icon="mdi-image-plus" @click="isMediaVisible = true">
                Add
              </v-btn>
            </div>
            <div v-if="pictures.length === 0" class="text-caption text-medium-emphasis mb-0">
              None selected.
            </div>
            <div v-for="(picture, index) in pictures" :key="`${picture.url}-${index}`" class="mb-2">
              <v-text-field :model-value="picture.url" readonly density="compact" variant="outlined" label="URL" />
              <v-text-field
                v-model="picture.note"
                label="Note (your reference only)"
                density="compact"
                variant="outlined"
                hide-details
              />
              <div class="d-flex justify-end">
                <v-btn size="x-small" color="error" variant="text" @click="removePicture(index)">Remove</v-btn>
              </div>
            </div>
          </v-expansion-panel-text>
        </v-expansion-panel>
      </v-expansion-panels>
    </div>

    <template #footer>
      <div class="ai-chat-composer pa-3 bg-surface">
        <v-textarea
          v-model="draftMessage"
          placeholder="Message…"
          auto-grow
          rows="2"
          variant="outlined"
          density="comfortable"
          hide-details
          :disabled="isRunning"
          :maxlength="20000"
          class="mb-2 composer-input"
          @paste="onComposerPaste"
          @keydown.enter.exact.prevent="onComposerEnter"
        />
        <div class="d-flex align-center ga-2 flex-wrap">
          <v-btn variant="outlined" @click="closeDialog">Close</v-btn>
          <v-spacer />
          <v-btn
            v-if="isRunning && job.id"
            color="warning"
            variant="tonal"
            :loading="isCancelling"
            @click="cancelGeneration"
          >
            Cancel
          </v-btn>
          <v-btn
            color="primary"
            :loading="isRunning"
            :disabled="!draftMessage.trim()"
            prepend-icon="mdi-send"
            @click="generate"
          >
            Send
          </v-btn>
        </div>
      </div>
    </template>

    <v-dialog v-model="isMediaVisible" max-width="900">
      <v-card>
        <v-card-text class="pt-0">
          <media
            is-modal
            type="pictures"
            @selected="onPictureSelected"
            @close="isMediaVisible = false"
          />
        </v-card-text>
      </v-card>
    </v-dialog>
  </AppRightSidebar>
</template>

<script>
import Media from '../views/Media.vue';
import AppRightSidebar from './AppRightSidebar.vue';

const STATUS_MAX_FILES = 12;
const STATUS_MAX_BYTES = 6 * 1024 * 1024;
const STATUS_NAME_RE = /\.(pdf|doc|docx|txt|rtf|jpe?g|png|gif|webp)$/i;

export default {
  name: 'AICampaignBuilderDialog',
  components: {
    Media,
    AppRightSidebar,
  },
  props: {
    modelValue: { type: Boolean, default: false },
    context: { type: Object, default: () => ({}) },
    current: { type: Object, default: () => ({}) },
    sessionKey: { type: String, default: '' },
  },
  emits: ['update:modelValue', 'apply'],
  data() {
    return {
      draftMessage: '',
      messages: [],
      pendingResult: null,
      pictures: [],
      statusFilePickerModel: null,
      statusFiles: [],
      isMediaVisible: false,
      attachmentsPanel: [0],
      _hydrating: false,
      _persistTimer: null,
      job: {
        id: '',
        status: '',
        progress: 0,
        result: {},
      },
      isRunning: false,
      isCancelling: false,
      errorMessage: '',
      streamAbortController: null,
      lastHandledSuccessJobId: null,
    };
  },

  computed: {
    effectiveSessionKey() {
      const k = (this.sessionKey && String(this.sessionKey).trim()) || '';
      return k || '_default';
    },
    sessionBucket() {
      return this.$store.state.aiCampaignBuilderThreads.bySessionKey[this.effectiveSessionKey] || null;
    },
    activeThreadId() {
      return this.sessionBucket?.activeThreadId || null;
    },
    threadsSorted() {
      const list = this.sessionBucket?.threads || [];
      return [...list].sort((a, b) => (b.updatedAt || 0) - (a.updatedAt || 0));
    },
  },

  watch: {
    messages: {
      deep: true,
      handler() {
        this.schedulePersist();
      },
    },
    pictures: {
      deep: true,
      handler() {
        this.schedulePersist();
      },
    },
    async modelValue(next, prev) {
      this.$events?.$emit('layout.aiPanel', next);
      if (next) {
        this.$store.commit('aiCampaignBuilderThreads/ensureSession', this.effectiveSessionKey);
        this.$nextTick(() => {
          this.hydrateActiveThread();
          this.scrollChatToBottom();
        });
      }
      if (prev && !next) {
        this.persistThread();
        if (this.isRunning && this.job.id) {
          await this.cancelGeneration(true);
        }
      }
      if (!next) {
        this.isMediaVisible = false;
      }
    },
    sessionKey(to, from) {
      if (from) {
        this.flushToSession(from);
      }
      this.$store.commit('aiCampaignBuilderThreads/ensureSession', this.effectiveSessionKey);
      this.$nextTick(() => this.hydrateActiveThread());
    },
  },

  methods: {
    onDialogToggle(next) {
      this.$emit('update:modelValue', next);
    },
    async closeDialog() {
      if (this.isRunning && this.job.id) {
        await this.cancelGeneration(true);
      }
      this.persistThread();
      this.$emit('update:modelValue', false);
    },
    resetJobState() {
      if (this.streamAbortController) {
        this.streamAbortController.abort();
        this.streamAbortController = null;
      }
      this.job = {
        id: '',
        status: '',
        progress: 0,
        result: {},
      };
      this.errorMessage = '';
    },
    deriveThreadTitle() {
      const first = this.messages.find((m) => m.role === 'user');
      if (!first || !String(first.content || '').trim()) {
        return 'New chat';
      }
      const line = String(first.content).trim().split('\n')[0];
      return line.length > 52 ? `${line.slice(0, 49)}…` : line;
    },
    persistThread() {
      if (this._hydrating) {
        return;
      }
      const sid = this.effectiveSessionKey;
      const tid = this.activeThreadId;
      if (!sid || !tid) {
        return;
      }
      this.$store.commit('aiCampaignBuilderThreads/saveThreadData', {
        sessionKey: sid,
        threadId: tid,
        messages: JSON.parse(JSON.stringify(this.messages)),
        pictures: this.pictures.map((p) => ({ url: p.url, note: p.note || '' })),
        title: this.deriveThreadTitle(),
      });
    },
    flushToSession(sessionKey) {
      if (!sessionKey || this._hydrating) {
        return;
      }
      const sess = this.$store.state.aiCampaignBuilderThreads.bySessionKey[sessionKey];
      const tid = sess?.activeThreadId;
      if (!tid) {
        return;
      }
      this.$store.commit('aiCampaignBuilderThreads/saveThreadData', {
        sessionKey,
        threadId: tid,
        messages: JSON.parse(JSON.stringify(this.messages)),
        pictures: this.pictures.map((p) => ({ url: p.url, note: p.note || '' })),
        title: this.deriveThreadTitle(),
      });
    },
    schedulePersist() {
      if (this._hydrating || !this.modelValue) {
        return;
      }
      clearTimeout(this._persistTimer);
      this._persistTimer = setTimeout(() => this.persistThread(), 350);
    },
    hydrateActiveThread() {
      const sess = this.sessionBucket;
      if (!sess) {
        return;
      }
      let tid = sess.activeThreadId;
      let t = sess.threads.find((x) => x.id === tid);
      if (!t && sess.threads.length) {
        t = sess.threads[0];
        tid = t.id;
        this.$store.commit('aiCampaignBuilderThreads/setActiveThread', {
          sessionKey: this.effectiveSessionKey,
          threadId: tid,
        });
      }
      if (!t) {
        return;
      }
      this._hydrating = true;
      this.messages = JSON.parse(JSON.stringify(t.messages || []));
      this.pictures = (t.pictures || []).map((p) => ({ url: p.url, note: p.note || '' }));
      this.draftMessage = '';
      this.pendingResult = null;
      this.statusFiles = [];
      this.statusFilePickerModel = null;
      this.resetJobState();
      this.$nextTick(() => {
        this._hydrating = false;
        this.scrollChatToBottom();
      });
    },
    onSelectThread(threadId) {
      if (!threadId || threadId === this.activeThreadId) {
        return;
      }
      this.persistThread();
      this.$store.commit('aiCampaignBuilderThreads/setActiveThread', {
        sessionKey: this.effectiveSessionKey,
        threadId,
      });
      this.hydrateActiveThread();
    },
    newThread() {
      this.persistThread();
      const now = Date.now();
      const id = `th-${now}-${Math.random().toString(36).slice(2, 10)}`;
      this.$store.commit('aiCampaignBuilderThreads/addThread', {
        sessionKey: this.effectiveSessionKey,
        thread: {
          id,
          title: 'New chat',
          createdAt: now,
          updatedAt: now,
          messages: [],
          pictures: [],
        },
      });
      this._hydrating = true;
      this.messages = [];
      this.pictures = [];
      this.draftMessage = '';
      this.pendingResult = null;
      this.statusFiles = [];
      this.statusFilePickerModel = null;
      this.resetJobState();
      this.errorMessage = '';
      this.$nextTick(() => {
        this._hydrating = false;
      });
    },
    deleteActiveThread() {
      const sid = this.effectiveSessionKey;
      const tid = this.activeThreadId;
      if (!sid || !tid) {
        return;
      }
      this.$store.commit('aiCampaignBuilderThreads/removeThread', {
        sessionKey: sid,
        threadId: tid,
      });
      this.hydrateActiveThread();
      this.resetJobState();
    },
    clearCurrentThread() {
      this.messages = [];
      this.pictures = [];
      this.draftMessage = '';
      this.pendingResult = null;
      this.statusFiles = [];
      this.statusFilePickerModel = null;
      this.resetJobState();
      this.errorMessage = '';
      this.persistThread();
    },
    scrollChatToBottom() {
      this.$nextTick(() => {
        const el = this.$refs.chatAnchor;
        if (el && typeof el.scrollIntoView === 'function') {
          el.scrollIntoView({ block: 'end', behavior: 'smooth' });
        }
      });
    },
    onComposerEnter() {
      if (!this.isRunning && this.draftMessage.trim()) {
        this.generate();
      }
    },
    applyDraft() {
      if (!this.pendingResult) {
        return;
      }
      this.$emit('apply', this.pendingResult);
      this.messages.push({
        role: 'assistant',
        content: 'Draft applied to the editor.',
      });
      this.pendingResult = null;
      this.persistThread();
      this.scrollChatToBottom();
    },
    discardDraft() {
      this.pendingResult = null;
    },
    buildInstructionsFromMessages() {
      const lines = this.messages.map((m) => `${m.role === 'user' ? 'User' : 'Assistant'}: ${m.content}`);
      if (this.pictures.length > 0) {
        lines.push('Selected image links:');
        this.pictures.forEach((picture) => {
          if (picture.url) {
            const note = String(picture.note || '').trim();
            if (note) {
              lines.push(`- ${picture.url} (reference: ${note})`);
            } else {
              lines.push(`- ${picture.url}`);
            }
          }
        });
      }
      return lines.join('\n\n');
    },
    onPictureSelected(media) {
      const url = media?.url || '';
      if (!url) {
        this.isMediaVisible = false;
        return;
      }
      if (!this.pictures.some((picture) => picture.url === url)) {
        this.pictures.push({ url, note: '' });
      }
      this.isMediaVisible = false;
    },
    removePicture(index) {
      this.pictures.splice(index, 1);
    },
    mimeFromFilename(name) {
      const n = String(name || '').toLowerCase();
      if (n.endsWith('.pdf')) return 'application/pdf';
      if (n.endsWith('.docx')) return 'application/vnd.openxmlformats-officedocument.wordprocessingml.document';
      if (n.endsWith('.doc')) return 'application/msword';
      if (n.endsWith('.txt')) return 'text/plain';
      if (n.endsWith('.rtf')) return 'application/rtf';
      if (n.endsWith('.jpg') || n.endsWith('.jpeg')) return 'image/jpeg';
      if (n.endsWith('.png')) return 'image/png';
      if (n.endsWith('.gif')) return 'image/gif';
      if (n.endsWith('.webp')) return 'image/webp';
      return '';
    },
    formatStatusFileSize(n) {
      if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
      if (n >= 1024) return `${Math.round(n / 1024)} KB`;
      return `${n} B`;
    },
    validateStatusFileCandidate(file) {
      if (!file || typeof file.size !== 'number') {
        return 'Invalid file';
      }
      if (this.statusFiles.length >= STATUS_MAX_FILES) {
        return `At most ${STATUS_MAX_FILES} reference files`;
      }
      if (file.size > STATUS_MAX_BYTES) {
        return `Each file must be at most ${STATUS_MAX_BYTES / (1024 * 1024)} MB`;
      }
      const t = (file.type || '').toLowerCase();
      if (t.startsWith('image/')) {
        return '';
      }
      if (!STATUS_NAME_RE.test(file.name)) {
        return 'Allowed: images, PDF, Word (.doc/.docx), .txt, .rtf';
      }
      return '';
    },
    readFileBase64(file) {
      return new Promise((resolve, reject) => {
        const r = new FileReader();
        r.onload = () => {
          const s = String(r.result || '');
          const comma = s.indexOf(',');
          resolve(comma >= 0 ? s.slice(comma + 1) : s);
        };
        r.onerror = () => reject(new Error('Failed to read file'));
        r.readAsDataURL(file);
      });
    },
    /** Clipboard screenshots often have an empty name; give a stable filename for validation and the API. */
    normalizeFileForStatusUpload(file) {
      if (!file) {
        return null;
      }
      const name = String(file.name || '').trim();
      if (name) {
        return file;
      }
      const mime = String(file.type || 'application/octet-stream').toLowerCase();
      let ext = 'bin';
      if (mime === 'image/png') ext = 'png';
      else if (mime === 'image/jpeg' || mime === 'image/jpg') ext = 'jpg';
      else if (mime === 'image/gif') ext = 'gif';
      else if (mime === 'image/webp') ext = 'webp';
      else if (mime === 'application/pdf') ext = 'pdf';
      else if (mime === 'text/plain') ext = 'txt';
      else if (mime === 'application/rtf' || mime === 'text/rtf') ext = 'rtf';
      else if (mime.includes('wordprocessingml') || mime.includes('msword')) ext = 'docx';
      else if (mime.startsWith('image/')) ext = (mime.split('/')[1] || 'png').replace('jpeg', 'jpg');
      try {
        return new File([file], `pasted.${ext}`, { type: file.type || mime });
      } catch {
        return file;
      }
    },
    collectClipboardFiles(dataTransfer) {
      if (!dataTransfer) {
        return [];
      }
      const out = [];
      const seen = new Set();
      const push = (f) => {
        if (!f) return;
        const k = `${f.name}|${f.size}|${f.type}`;
        if (seen.has(k)) return;
        seen.add(k);
        out.push(f);
      };
      if (dataTransfer.files && dataTransfer.files.length) {
        for (let i = 0; i < dataTransfer.files.length; i += 1) {
          push(dataTransfer.files[i]);
        }
      }
      if (dataTransfer.items && dataTransfer.items.length) {
        for (let i = 0; i < dataTransfer.items.length; i += 1) {
          const it = dataTransfer.items[i];
          if (it.kind === 'file') {
            const f = it.getAsFile();
            push(f);
          }
        }
      }
      return out;
    },
    onComposerPaste(e) {
      if (!this.modelValue || this.isRunning) {
        return;
      }
      const raw = this.collectClipboardFiles(e.clipboardData);
      if (!raw.length) {
        return;
      }
      e.preventDefault();
      this.addStatusFilesFromInputList(raw);
    },
    async addStatusFilesFromInputList(rawList) {
      const list = (Array.isArray(rawList) ? rawList : [rawList])
        .map((f) => this.normalizeFileForStatusUpload(f))
        .filter(Boolean);
      if (!list.length) {
        return;
      }
      const reads = await Promise.all(
        list.map(async (file) => {
          const err = this.validateStatusFileCandidate(file);
          if (err) {
            return { ok: false, err, file };
          }
          let mime = (file.type || '').trim().toLowerCase();
          if (!mime || mime === 'application/octet-stream') {
            mime = this.mimeFromFilename(file.name);
          }
          if (!mime) {
            return { ok: false, err: `Could not detect type for ${file.name}`, file };
          }
          try {
            const data = await this.readFileBase64(file);
            return {
              ok: true,
              item: {
                id: `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
                name: file.name,
                mimeType: mime,
                size: file.size,
                dataBase64: data,
              },
            };
          } catch (err) {
            return { ok: false, err: err?.message || 'Failed to read file', file };
          }
        }),
      );
      let hadOk = false;
      reads.forEach((r) => {
        if (r.ok) {
          this.statusFiles.push(r.item);
          hadOk = true;
        } else if (r.err) {
          this.errorMessage = r.err;
        }
      });
      if (hadOk) {
        this.errorMessage = '';
      }
    },
    async onStatusFilesPicked(files) {
      this.statusFilePickerModel = null;
      if (!files) {
        return;
      }
      const list = Array.isArray(files) ? files : [files];
      await this.addStatusFilesFromInputList(list);
    },
    removeStatusFile(index) {
      this.statusFiles.splice(index, 1);
    },
    async generate() {
      const userMessage = this.draftMessage.trim();
      if (!userMessage) {
        return;
      }
      this.errorMessage = '';
      this.isRunning = true;
      this.lastHandledSuccessJobId = null;
      this.resetJobState();
      this.messages.push({ role: 'user', content: userMessage });
      this.draftMessage = '';
      this.persistThread();
      this.scrollChatToBottom();
      try {
        const job = await this.$api.createAICampaignBuilderJob({
          context: {
            ...(this.context || {}),
            editorMode: this.context?.editorMode || this.context?.contentType || '',
          },
          current: this.current,
          instructions: this.buildInstructionsFromMessages(),
          statusAttachments: this.statusFiles.map((f) => ({
            filename: f.name,
            mimeType: f.mimeType,
            data: f.dataBase64,
          })),
        });
        this.job = {
          ...this.job,
          ...job,
        };
        await this.streamJob();
      } catch (e) {
        this.errorMessage = e?.response?.message || e?.message || 'Failed to start generation.';
      } finally {
        this.isRunning = false;
        if (this.streamAbortController) {
          this.streamAbortController = null;
        }
        this.persistThread();
        this.scrollChatToBottom();
      }
    },
    async cancelGeneration(silent = false) {
      if (!this.job.id || this.isCancelling) {
        return;
      }
      this.isCancelling = true;
      try {
        if (this.streamAbortController) {
          this.streamAbortController.abort();
          this.streamAbortController = null;
        }
        const out = await this.$api.cancelAICampaignBuilderJob(this.job.id);
        this.job = out;
        if (!silent && out?.status === 'canceled') {
          this.errorMessage = '';
        }
      } finally {
        this.isCancelling = false;
      }
    },
    applyJobTerminalState(out) {
      if (out.status === 'success') {
        const result = out.result || {};
        this.pendingResult = result;
        const jid = out.id || this.job.id || '';
        if (jid && this.lastHandledSuccessJobId === jid) {
          return true;
        }
        if (jid) {
          this.lastHandledSuccessJobId = jid;
        }
        const aiText = [
          'Draft ready. Review the preview below and click Apply to editor when you want to use it.',
          result.subject ? `Subject: ${result.subject}` : '',
          result.preheader ? `Preheader: ${result.preheader}` : '',
          result.notes ? `Notes: ${result.notes}` : '',
        ].filter(Boolean).join('\n');
        this.messages.push({
          role: 'assistant',
          content: aiText || 'Draft is ready.',
        });
        this.persistThread();
        this.scrollChatToBottom();
        return true;
      }
      if (out.status === 'failed') {
        this.errorMessage = out.error || 'Generation failed.';
        return true;
      }
      if (out.status === 'canceled') {
        return true;
      }
      return false;
    },
    async streamJob() {
      if (!this.job.id) {
        return;
      }
      const controller = new AbortController();
      this.streamAbortController = controller;
      try {
        await this.$api.streamAICampaignBuilderJob(this.job.id, {
          signal: controller.signal,
          onJob: (out) => {
            this.job = out;
            this.applyJobTerminalState(out);
          },
        });
      } catch (e) {
        if (controller.signal.aborted) {
          return;
        }
        await this.pollJob();
      }
    },
    async pollJob() {
      if (!this.job.id) {
        return;
      }
      const maxAttempts = 240;
      for (let i = 0; i < maxAttempts; i += 1) {
        const out = await this.$api.getAICampaignBuilderJob(this.job.id);
        this.job = out;

        if (this.applyJobTerminalState(out)) {
          return;
        }
        // eslint-disable-next-line no-await-in-loop
        await new Promise((resolve) => {
          setTimeout(resolve, 1000);
        });
      }
      this.errorMessage = 'Generation timed out. Please try again.';
    },
  },

  beforeUnmount() {
    clearTimeout(this._persistTimer);
    this.flushToSession(this.effectiveSessionKey);
    if (this.modelValue) {
      this.$events?.$emit('layout.aiPanel', false);
    }
  },
};
</script>

<style scoped>
:deep(.app-right-sidebar__body) {
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.ai-chat-body {
  min-height: 0;
}

.ai-chat-toolbar {
  flex-shrink: 0;
}

.ai-chat-thread-row {
  flex-shrink: 0;
  background: rgba(var(--v-theme-surface-variant), 0.2);
}

.ai-chat-messages {
  min-height: 0;
  overflow-y: auto;
  scroll-behavior: smooth;
}

.chat-welcome {
  border: 1px dashed rgba(var(--v-theme-outline), 0.35);
  border-radius: 12px;
}

.chat-row {
  display: flex;
  margin-bottom: 12px;
}

.chat-row--user {
  justify-content: flex-end;
}

.chat-row--assistant {
  justify-content: flex-start;
}

.chat-bubble {
  max-width: 92%;
  padding: 10px 14px;
  border-radius: 16px;
  line-height: 1.45;
  word-break: break-word;
  overflow-wrap: anywhere;
}

.chat-bubble--user {
  background: rgb(var(--v-theme-primary));
  color: rgb(var(--v-theme-on-primary));
  border-bottom-right-radius: 4px;
}

.chat-bubble--assistant {
  background: rgba(var(--v-theme-surface-variant), 0.55);
  color: rgba(var(--v-theme-on-surface), 0.92);
  border-bottom-left-radius: 4px;
}

.chat-bubble-meta {
  font-size: 0.7rem;
  opacity: 0.85;
  margin-bottom: 4px;
  font-weight: 600;
  letter-spacing: 0.02em;
  text-transform: uppercase;
}

.chat-bubble--assistant .chat-bubble-meta {
  opacity: 0.65;
}

.chat-bubble-text {
  font-size: 0.9rem;
  white-space: pre-wrap;
}

.chat-anchor {
  height: 1px;
  width: 100%;
}

.ai-chat-panels {
  flex-shrink: 0;
}

.ai-chat-composer {
  flex-shrink: 0;
}

:deep(.composer-input textarea) {
  max-height: 220px;
  overflow-y: auto !important;
}

.draft-preview-card {
  border-radius: 12px !important;
}

:deep(.thread-select .v-field) {
  font-size: 0.875rem;
}
</style>
