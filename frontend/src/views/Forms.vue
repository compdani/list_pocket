<template>
  <section class="forms">
    <header class="page-header mb-4">
      <h1 class="text-h4">{{ $t('forms.title') }}</h1>
      <v-divider class="mt-2" />
    </header>

    <v-progress-linear
      v-if="loading.lists"
      indeterminate
      color="primary"
      rounded
      class="mb-4"
    />

    <v-alert
      v-else-if="publicLists.length === 0"
      type="info"
      variant="tonal"
      class="mb-4"
    >
      {{ $t('forms.noPublicLists') }}
    </v-alert>

    <v-row v-else class="forms-grid">
      <v-col cols="12" md="4">
        <v-card variant="outlined" class="pa-4">
          <h2 class="text-subtitle-1 font-weight-bold mb-2">{{ $t('forms.publicLists') }}</h2>
          <p class="text-body-2 text-medium-emphasis mb-4">{{ $t('forms.selectHelp') }}</p>

          <v-selection-control-group v-model="checked" multiple class="list-checks" data-cy="lists">
            <v-checkbox
              v-for="l in publicLists"
              :key="l.id"
              :value="String(l.uuid)"
              :label="l.name"
              color="primary"
              density="compact"
              hide-details
              class="mb-1"
            />
          </v-selection-control-group>

          <template v-if="publicSubscriptionVisible">
            <v-divider class="my-4" />
            <h3 class="text-subtitle-2 font-weight-bold mb-2">{{ $t('forms.publicSubPage') }}</h3>
            <a
              :href="publicSubscriptionURL"
              target="_blank"
              rel="noopener noreferer"
              data-cy="url"
              class="form-link"
            >
              {{ publicSubscriptionURL }}
            </a>
          </template>
        </v-card>
      </v-col>

      <v-col cols="12" md="8" data-cy="form">
        <v-card variant="outlined" class="pa-4">
          <h2 class="text-subtitle-1 font-weight-bold mb-2">{{ $t('forms.formHTML') }}</h2>
          <p class="text-body-2 text-medium-emphasis mb-4">{{ $t('forms.formHTMLHelp') }}</p>

          <code-editor v-if="checkedListUUIDs.length > 0" :value="formHTML" lang="html" disabled />
          <v-alert v-else type="info" variant="tonal">
            {{ $t('forms.selectHelp') }}
          </v-alert>
        </v-card>
      </v-col>
    </v-row>
  </section>
</template>

<script setup>
import {
  computed,
  getCurrentInstance,
  ref,
} from 'vue';
import { useStore } from 'vuex';
import CodeEditor from '../components/CodeEditor.vue';

const store = useStore();
const { proxy } = getCurrentInstance();

const checked = ref([]);

const loading = computed(() => store.state.loading || {});
const lists = computed(() => store.state.lists || {});
const serverConfig = computed(() => store.state.serverConfig || {});
const settings = computed(() => store.state.settings || {});

const publicLists = computed(() => {
  const results = Array.isArray(lists.value.results) ? lists.value.results : [];
  return results.filter((l) => l.type === 'public');
});

const checkedListUUIDs = computed(() => {
  if (Array.isArray(checked.value)) {
    return checked.value.map((v) => String(v));
  }
  if (checked.value === null || checked.value === undefined || checked.value === '') {
    return [];
  }
  return [String(checked.value)];
});

const selectedPublicLists = computed(() => {
  const selected = new Set(checkedListUUIDs.value);
  return publicLists.value.filter((l) => selected.has(String(l.uuid)));
});

function toBoolean(value) {
  if (typeof value === 'boolean') {
    return value;
  }
  if (typeof value === 'number') {
    return value === 1;
  }
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    return ['true', '1', 'yes', 'on'].includes(normalized);
  }
  return Boolean(value);
}

const hasPublicSubscriptionFlag = computed(() => (
  serverConfig.value
  && serverConfig.value.public_subscription
  && Object.prototype.hasOwnProperty.call(serverConfig.value.public_subscription, 'enabled')
));

const publicSubscriptionEnabled = computed(() => {
  const cfgEnabled = serverConfig.value.public_subscription?.enabled;
  if (cfgEnabled !== undefined && cfgEnabled !== null) {
    return toBoolean(cfgEnabled);
  }

  const fallback = settings.value['app.enable_public_subscription_page'];
  if (fallback !== undefined && fallback !== null) {
    return toBoolean(fallback);
  }

  return true;
});

const baseRootURL = computed(() => {
  const configuredRootURL = serverConfig.value.root_url || settings.value['app.root_url'];
  if (configuredRootURL) {
    return String(configuredRootURL).replace(/\/$/, '');
  }

  if (typeof window !== 'undefined' && window.location && window.location.origin) {
    return window.location.origin.replace(/\/$/, '');
  }

  return '';
});

const publicSubscriptionVisible = computed(() => {
  if (hasPublicSubscriptionFlag.value) {
    return publicSubscriptionEnabled.value;
  }

  return Boolean(baseRootURL.value);
});

const publicSubscriptionURL = computed(() => `${baseRootURL.value}/subscription/form`);

const formHTML = computed(() => {
  const rootURL = baseRootURL.value;
  const subConfig = serverConfig.value.public_subscription || {};

  let output = `<form method="post" action="${rootURL}/subscription/form" class="listmonk-form">\n`
    + '  <div>\n'
    + `    <h3>${proxy.$t('public.sub')}</h3>\n`
    + '    <input type="hidden" name="nonce" />\n\n'
    + `    <p><input type="email" name="email" required placeholder="${proxy.$t('subscribers.email')}" /></p>\n`
    + '    <p><input type="text" name="first_name" placeholder="First name" /></p>\n'
    + '    <p><input type="text" name="last_name" placeholder="Last name" /></p>\n\n';

  selectedPublicLists.value.forEach((l, index) => {
    const id = `${String(l.uuid).slice(0, 5)}-${index}`;

    output += '    <p>\n'
      + `      <input id="${id}" type="checkbox" name="l" checked value="${l.uuid}" />\n`
      + `      <label for="${id}">${l.name}</label>\n`;

    if (l.description) {
      output += '      <br />\n'
        + `      <span>${l.description}</span>\n`;
    }

    output += '    </p>\n';
  });

  if (subConfig.captcha_enabled) {
    if (subConfig.captcha_provider === 'altcha') {
      output += '\n'
        + `    <altcha-widget challengeurl="${rootURL}/mailapi/public/captcha/altcha"></altcha-widget>\n`
        + `    <${'script'} type="module" src="${rootURL}/public/static/altcha.umd.js" async defer></${'script'}>\n`;
    } else if (subConfig.captcha_provider === 'hcaptcha') {
      output += '\n'
        + `    <div class="h-captcha" data-sitekey="${subConfig.captcha_key}"></div>\n`
        + `    <${'script'} src="https://js.hcaptcha.com/1/api.js" async defer></${'script'}>\n`;
    }
  }

  output += '\n'
    + `    <input type="submit" value="${proxy.$t('public.sub')} " />\n`
    + '  </div>\n'
    + '</form>';

  return output;
});
</script>

<style scoped>
.forms {
  min-height: 100%;
}

.list-checks {
  max-height: 340px;
  overflow-y: auto;
  padding-right: 4px;
}

.form-link {
  color: #0f5bd8;
  text-decoration: none;
}

.form-link:hover {
  color: #0a47a7;
  text-decoration: underline;
}
</style>
