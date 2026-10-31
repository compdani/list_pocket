<template>
  <div class="settings-section" data-cy="ai-settings">
    <v-progress-linear v-if="!state" indeterminate color="primary" class="mb-4" />
    <template v-else>
      <section class="mb-6">
        <h3 class="text-subtitle-1 mb-1">{{ $t('settings.ai.defaultTitle') }}</h3>
        <p class="text-body-2 text-medium-emphasis">{{ $t('settings.ai.defaultHelp') }}</p>
        <v-row dense class="mt-2">
          <v-col cols="12" md="4">
            <v-select v-model="choice.provider" :items="providerItems" :label="$t('settings.ai.provider')" :disabled="!canManage" density="compact" variant="outlined" hide-details data-cy="ai-provider" @update:model-value="choice.model = ''" />
          </v-col>
          <v-col cols="12" md="5">
            <v-combobox v-model="choice.model" :items="modelItems" :label="$t('settings.ai.model')" :disabled="!canManage" :loading="loadingModels" density="compact" variant="outlined" hide-details data-cy="ai-model" />
          </v-col>
          <v-col cols="12" md="3" class="d-flex align-center">
            <v-btn v-if="canManage" color="primary" variant="flat" class="text-none" :loading="saving" :disabled="!canSaveChoice" data-cy="ai-save-default" @click="saveDefault">{{ $t('globals.buttons.save') }}</v-btn>
          </v-col>
        </v-row>
        <p v-if="!state.api_configured" class="text-caption text-medium-emphasis mt-2 mb-0">{{ $t('settings.ai.noApiKey') }}</p>
      </section>

      <v-divider class="mb-6" />

      <section class="mb-6">
        <h3 class="text-subtitle-1 mb-1">{{ $t('settings.ai.planTitle') }}</h3>
        <p class="text-body-2 mb-1">{{ $t('settings.ai.planHelp') }}</p>
        <p class="text-caption text-medium-emphasis">{{ state.note }}</p>

        <div v-if="status.signed_in" class="d-flex align-center flex-wrap ga-2 mb-2" data-cy="ai-signed-in">
          <v-chip size="small" label :color="status.plan ? 'success' : 'error'">{{ $t(status.plan ? 'settings.ai.planOn' : 'settings.ai.planOff') }}</v-chip>
          <strong>{{ status.email }}</strong>
          <span class="text-caption text-medium-emphasis">{{ $t('settings.ai.tokenHelp') }}</span>
          <v-spacer />
          <template v-if="canManage">
            <v-btn size="small" variant="outlined" class="text-none" :disabled="busy" @click="signIn">{{ $t('settings.ai.signInAgain') }}</v-btn>
            <v-btn size="small" variant="text" color="error" class="text-none" data-cy="ai-sign-out" @click="signOut">{{ $t('settings.ai.signOut') }}</v-btn>
          </template>
        </div>
        <div v-else class="d-flex align-center flex-wrap ga-2 mb-2">
          <v-chip size="small" label>{{ $t('settings.ai.notSignedIn') }}</v-chip>
          <v-spacer />
          <v-btn v-if="canManage" color="primary" variant="flat" class="text-none" :loading="busy" data-cy="ai-sign-in" @click="signIn">{{ $t('settings.ai.signIn') }}</v-btn>
        </div>

        <template v-if="busy">
          <p class="text-body-2 mt-3 mb-1">{{ $t('settings.ai.pasteHelp') }}</p>
          <p v-if="authURL" class="text-caption mb-2"><a :href="authURL" target="_blank" rel="noopener noreferrer" data-cy="ai-auth-link">{{ $t('settings.ai.openSignIn') }}</a></p>
          <div class="d-flex ga-2">
            <v-text-field v-model="callbackURL" placeholder="http://127.0.0.1:1455/auth/callback?code=…&state=…" density="compact" variant="outlined" hide-details data-cy="ai-callback" />
            <v-btn color="primary" variant="flat" class="text-none" :disabled="!callbackURL.trim()" @click="pasteCallback">{{ $t('settings.ai.submit') }}</v-btn>
            <v-btn variant="text" class="text-none" data-cy="ai-cancel" @click="cancelSignIn">{{ $t('globals.buttons.cancel') }}</v-btn>
          </div>
        </template>
      </section>

      <v-alert v-if="message" :type="message.kind" variant="tonal" density="compact" class="mb-4" closable @click:close="message = null">{{ message.text }}</v-alert>

      <details v-if="canManage">
        <summary class="text-subtitle-2 mb-2">{{ $t('settings.ai.importTitle') }}</summary>
        <p class="text-body-2 text-medium-emphasis">{{ $t('settings.ai.importHelp') }}</p>
        <v-textarea v-model="importText" rows="4" density="compact" variant="outlined" hide-details placeholder='{"client_id":"oaiapp_…","refresh_token":"…"}' class="mb-2" />
        <v-btn size="small" variant="outlined" class="text-none" :disabled="!importText.trim()" @click="importCreds">{{ $t('settings.ai.import') }}</v-btn>
      </details>
    </template>
  </div>
</template>

<script setup>
import {
  computed, getCurrentInstance, onBeforeUnmount, onMounted, ref,
} from 'vue';
import { agentGet, agentPost, agentPut } from '../../agent/client';

const POLL_MS = 2000;

const { proxy } = getCurrentInstance();
const state = ref(null);
const models = ref({ providers: [] });
const loadingModels = ref(false);
const choice = ref({ provider: 'api', model: '' });
const saving = ref(false);
const busy = ref(false);
const authURL = ref('');
const callbackURL = ref('');
const importText = ref('');
const message = ref(null);
let timer = null;

const canManage = computed(() => proxy.$can('settings:manage'));
const status = computed(() => state.value?.status || {});
const providerItems = computed(() => [
  { value: 'api', title: proxy.$t('settings.ai.providerApi') },
  { value: 'chatgpt', title: proxy.$t('settings.ai.providerPlan'), props: { disabled: !status.value.signed_in } },
]);
const modelItems = computed(() => {
  const provider = (models.value.providers || []).find((p) => p.name === choice.value.provider);
  return (provider?.models || []).map((m) => m.id);
});
// The API key has a model of its own to fall back on, and the plan doesn't.
const canSaveChoice = computed(() => choice.value.provider === 'api' || Boolean(String(choice.value.model || '').trim()));

async function loadModels(refresh = false) {
  loadingModels.value = true;
  try {
    models.value = await agentGet(`agent/models${refresh ? '?refresh=1' : ''}`);
  } catch {
    // Models can still be typed by hand.
  } finally {
    loadingModels.value = false;
  }
}

async function load() {
  state.value = await agentGet('ai/settings');
  const saved = state.value.default || {};
  choice.value = { provider: saved.provider || 'api', model: saved.model || '' };
}

async function saveDefault() {
  saving.value = true;
  try {
    const model = String(choice.value.model || '').trim();
    // An API key with no model chosen goes back to the app's own default.
    await agentPut('ai/settings/default', choice.value.provider === 'api' && !model ? {} : { provider: choice.value.provider, model });
    message.value = { kind: 'success', text: proxy.$t('settings.ai.saved') };
  } catch (err) {
    message.value = { kind: 'error', text: err.message };
  } finally {
    saving.value = false;
  }
}

function watchSignIn() {
  clearInterval(timer);
  timer = setInterval(async () => {
    try {
      const next = await agentGet('ai/settings');
      state.value = next;
      if (next.pending) return;
      clearInterval(timer);
      busy.value = false;
      authURL.value = '';
      if (next.error) {
        message.value = { kind: 'error', text: next.error };
      } else {
        message.value = { kind: 'success', text: proxy.$t('settings.ai.signedInAs', { email: next.status.email }) };
        loadModels(true);
      }
    } catch (err) {
      message.value = { kind: 'error', text: err.message };
    }
  }, POLL_MS);
}

async function signIn() {
  busy.value = true;
  message.value = null;
  try {
    const out = await agentPost('ai/chatgpt/login');
    authURL.value = out.auth_url;
    window.open(out.auth_url, '_blank', 'noopener');
    watchSignIn();
  } catch (err) {
    busy.value = false;
    message.value = { kind: 'error', text: err.message };
  }
}

async function pasteCallback() {
  try {
    await agentPost('ai/chatgpt/callback', { url: callbackURL.value });
    callbackURL.value = '';
  } catch (err) {
    message.value = { kind: 'error', text: err.message };
  }
}

async function cancelSignIn() {
  try {
    await agentPost('ai/chatgpt/cancel');
  } catch (err) {
    message.value = { kind: 'error', text: err.message };
  }
}

function signOut() {
  proxy.$utils.confirm(proxy.$t('settings.ai.signOutConfirm'), async () => {
    try {
      const out = await agentPost('ai/chatgpt/logout');
      message.value = out.warning ? { kind: 'warning', text: out.warning } : { kind: 'success', text: proxy.$t('settings.ai.signedOut') };
      await load();
      loadModels(true);
    } catch (err) {
      message.value = { kind: 'error', text: err.message };
    }
  });
}

async function importCreds() {
  try {
    await agentPost('ai/chatgpt/import', JSON.parse(importText.value));
    importText.value = '';
    message.value = { kind: 'success', text: proxy.$t('settings.ai.imported') };
    await load();
    loadModels(true);
  } catch (err) {
    message.value = { kind: 'error', text: err instanceof SyntaxError ? proxy.$t('settings.ai.importInvalid') : err.message };
  }
}

onMounted(async () => {
  try {
    await load();
    if (state.value.pending) {
      busy.value = true;
      watchSignIn();
    }
    loadModels();
  } catch (err) {
    state.value = { status: {} };
    message.value = { kind: 'error', text: err.message };
  }
});

onBeforeUnmount(() => clearInterval(timer));
</script>
