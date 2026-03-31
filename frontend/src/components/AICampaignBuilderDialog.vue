<template>
  <v-navigation-drawer
    :model-value="modelValue"
    location="right"
    temporary
    scrim
    width="520"
    @update:model-value="onDialogToggle"
  >
    <v-card class="ai-sidebar">
      <v-card-title class="text-subtitle-1 d-flex justify-space-between align-center">
        <span>AI Campaign Builder Chat</span>
        <v-btn size="small" variant="text" :disabled="isRunning" @click="clearChatHistory">
          Clear
        </v-btn>
      </v-card-title>
      <v-card-text class="ai-sidebar-content">
        <div class="chat-thread pa-2 mb-3">
          <div
            v-for="(message, index) in messages"
            :key="`${message.role}-${index}`"
            :class="['chat-message', message.role]"
          >
            <div class="chat-role">
              {{ message.role === 'user' ? 'You' : 'AI' }}
            </div>
            <div class="chat-content">
              {{ message.content }}
            </div>
          </div>
          <div v-if="messages.length === 0" class="chat-empty">
            Ask AI to draft or revise this campaign.
          </div>
        </div>

        <v-textarea
          v-model="draftMessage"
          label="Message"
          placeholder="Ask AI to draft or revise this campaign..."
          auto-grow
          rows="3"
          variant="outlined"
          :disabled="isRunning"
          :maxlength="20000"
          counter
        />

        <v-card variant="outlined" class="mt-3">
          <v-card-title class="text-subtitle-2 d-flex justify-space-between align-center">
            <span>Pictures</span>
            <v-btn size="small" variant="outlined" prepend-icon="mdi-image-plus" @click="isMediaVisible = true">
              Add Picture
            </v-btn>
          </v-card-title>
          <v-card-text>
            <div v-if="pictures.length === 0" class="text-body-2 opacity-70">
              No pictures selected.
            </div>
            <div v-for="(picture, index) in pictures" :key="`${picture.url}-${index}`" class="mb-3">
              <div class="text-caption mb-1">URL sent to AI</div>
              <v-text-field :model-value="picture.url" readonly density="compact" variant="outlined" />
              <v-text-field
                v-model="picture.note"
                label="Note (for your reference only)"
                density="compact"
                variant="outlined"
              />
              <div class="d-flex justify-end">
                <v-btn size="x-small" color="error" variant="text" @click="removePicture(index)">
                  Remove
                </v-btn>
              </div>
            </div>
          </v-card-text>
        </v-card>

        <v-alert v-if="errorMessage" type="error" variant="tonal" class="mt-3">
          {{ errorMessage }}
        </v-alert>

        <v-alert v-if="job.status && job.status !== 'success' && job.status !== 'failed' && job.status !== 'canceled'" type="info" variant="tonal" class="mt-3">
          Generation status: {{ job.status }} ({{ job.progress || 0 }}%)
        </v-alert>
        <v-alert v-if="job.status === 'canceled'" type="warning" variant="tonal" class="mt-3">
          Generation cancelled.
        </v-alert>

        <v-alert v-if="job.status === 'success' && job.result.notes" type="success" variant="tonal" class="mt-3">
          {{ job.result.notes }}
        </v-alert>

        <v-card v-if="pendingResult" variant="outlined" class="mt-3">
          <v-card-title class="text-subtitle-2">Draft Preview</v-card-title>
          <v-card-text>
            <div v-if="pendingResult.subject" class="mb-2">
              <strong>Subject:</strong> {{ pendingResult.subject }}
            </div>
            <div v-if="pendingResult.preheader" class="mb-2">
              <strong>Preheader:</strong> {{ pendingResult.preheader }}
            </div>
            <div class="text-caption mb-1">
              Body (before -> after)
            </div>
            <v-row>
              <v-col cols="12" md="6">
                <v-textarea
                  :model-value="(current && current.body) || ''"
                  label="Before"
                  readonly
                  rows="8"
                  variant="outlined"
                />
              </v-col>
              <v-col cols="12" md="6">
                <v-textarea
                  :model-value="pendingResult.body || ''"
                  label="After"
                  readonly
                  rows="8"
                  variant="outlined"
                />
              </v-col>
            </v-row>
          </v-card-text>
          <v-card-actions class="justify-end">
            <v-btn variant="outlined" @click="discardDraft">Discard</v-btn>
            <v-btn color="primary" @click="applyDraft">Apply</v-btn>
          </v-card-actions>
        </v-card>
      </v-card-text>

      <v-divider />
      <v-card-actions class="justify-end">
        <v-btn variant="outlined" @click="closeDialog">
          Close
        </v-btn>
        <v-btn v-if="isRunning && job.id" color="warning" variant="outlined" :loading="isCancelling" @click="cancelGeneration">
          Cancel
        </v-btn>
        <v-btn color="primary" :loading="isRunning" :disabled="!draftMessage.trim()" @click="generate">
          Send
        </v-btn>
      </v-card-actions>
    </v-card>

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
  </v-navigation-drawer>
</template>

<script>
import Media from '../views/Media.vue';

export default {
  name: 'AICampaignBuilderDialog',
  components: {
    Media,
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
      isMediaVisible: false,
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
    };
  },
  methods: {
    onDialogToggle(next) {
      this.$emit('update:modelValue', next);
    },
    async closeDialog() {
      if (this.isRunning && this.job.id) {
        await this.cancelGeneration(true);
      }
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
    clearChatHistory() {
      this.messages = [];
      this.draftMessage = '';
      this.pendingResult = null;
      this.pictures = [];
      this.resetJobState();
    },
    applyDraft() {
      if (!this.pendingResult) {
        return;
      }
      this.$emit('apply', this.pendingResult);
      this.messages.push({
        role: 'assistant',
        content: 'Draft applied to editor.',
      });
      this.pendingResult = null;
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
    async generate() {
      const userMessage = this.draftMessage.trim();
      if (!userMessage) {
        return;
      }
      this.errorMessage = '';
      this.isRunning = true;
      this.resetJobState();
      this.messages.push({ role: 'user', content: userMessage });
      this.draftMessage = '';
      try {
        const job = await this.$api.createAICampaignBuilderJob({
          context: {
            ...(this.context || {}),
            editorMode: this.context?.editorMode || this.context?.contentType || '',
          },
          current: this.current,
          instructions: this.buildInstructionsFromMessages(),
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
        const aiText = [
          'Draft ready. Review and click Apply.',
          result.subject ? `Subject: ${result.subject}` : '',
          result.preheader ? `Preheader: ${result.preheader}` : '',
          result.notes ? `Notes: ${result.notes}` : '',
        ].filter(Boolean).join('\n');
        this.messages.push({
          role: 'assistant',
          content: aiText || 'Applied updated content.',
        });
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
        // Fallback when SSE isn't available in current runtime/proxy.
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
        // Poll every second until done or timeout.
        // eslint-disable-next-line no-await-in-loop
        await new Promise((resolve) => {
          setTimeout(resolve, 1000);
        });
      }
      this.errorMessage = 'Generation timed out. Please try again.';
    },
  },
  watch: {
    async modelValue(next, prev) {
      if (prev && !next && this.isRunning && this.job.id) {
        await this.cancelGeneration(true);
      }
      if (!next) {
        this.isMediaVisible = false;
      }
    },
    sessionKey(next, prev) {
      if (next !== prev) {
        this.clearChatHistory();
      }
    },
  },
};
</script>

<style scoped>
.chat-thread {
  max-height: 260px;
  overflow-y: auto;
  background: rgba(var(--v-theme-surface-variant), 0.25);
  border-radius: 10px;
}

.ai-sidebar {
  border-radius: 0;
  display: flex;
  flex-direction: column;
  height: 100%;
}

.ai-sidebar-content {
  flex: 1;
  overflow-y: auto;
}

.chat-message {
  margin-bottom: 12px;
  padding: 10px;
  border-radius: 8px;
}

.chat-message.user {
  background: rgba(var(--v-theme-primary), 0.12);
}

.chat-message.assistant {
  background: rgba(var(--v-theme-secondary), 0.12);
}

.chat-role {
  font-size: 0.75rem;
  opacity: 0.7;
  margin-bottom: 4px;
}

.chat-empty {
  font-size: 0.9rem;
  opacity: 0.7;
  padding: 8px;
}
</style>
