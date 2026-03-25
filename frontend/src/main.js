import { createApp, h } from 'vue';
import { createI18n } from 'vue-i18n';

import App from './App.vue';
import router from './router';
import store from './store';
import * as api from './api';
import Utils from './utils';
import { docsUrl } from './utils/docs';
import vuetify from './plugins/vuetify';
import { installLegacyUIStyles, registerLegacyUI } from './legacy-ui';

function createEventBus() {
  const listeners = new Map();

  return {
    $on(event, handler) {
      const handlers = listeners.get(event) || new Set();
      handlers.add(handler);
      listeners.set(event, handlers);
    },

    $off(event, handler) {
      const handlers = listeners.get(event);
      if (!handlers) {
        return;
      }

      if (!handler) {
        listeners.delete(event);
        return;
      }

      handlers.delete(handler);
      if (handlers.size === 0) {
        listeners.delete(event);
      }
    },

    $emit(event, ...args) {
      const handlers = listeners.get(event);
      if (!handlers) {
        return;
      }

      handlers.forEach((handler) => {
        handler(...args);
      });
    },
  };
}

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  fallbackLocale: 'en',
  messages: {},
});
const eventBus = createEventBus();

const NAMED_PLACEHOLDER_RE = /\{([A-Za-z_][A-Za-z0-9_]*)\}/g;
const SIMPLE_PLACEHOLDER_RE = /\{([^{}]+)\}/g;
const PLACEHOLDER_TOKEN_PREFIX = '__LISTPOCKET_I18N_PLACEHOLDER_';

function isNamedPlaceholderName(value) {
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(value);
}

function isPlaceholderCandidate(value) {
  return /^[^"'`:[\].,{}\s]+$/.test(value);
}

function extractNamedPlaceholders(message) {
  const placeholders = [];
  if (typeof message !== 'string') {
    return placeholders;
  }

  message.replace(NAMED_PLACEHOLDER_RE, (_, name) => {
    placeholders.push(name);
    return _;
  });

  return placeholders;
}

function normalizeLocaleMessage(message, referenceMessage) {
  if (typeof message !== 'string' || message.length === 0) {
    return message;
  }

  const referencePlaceholders = extractNamedPlaceholders(referenceMessage);
  let placeholderIndex = 0;

  let normalized = message.replace(SIMPLE_PLACEHOLDER_RE, (match, inner) => {
    const token = inner.trim();
    const expected = referencePlaceholders[placeholderIndex];

    if (isNamedPlaceholderName(token)) {
      placeholderIndex += 1;
      if (expected && !referencePlaceholders.includes(token)) {
        return `{${expected}}`;
      }
      return match;
    }

    if (!expected || !isPlaceholderCandidate(token)) {
      return match;
    }

    placeholderIndex += 1;
    return `{${expected}}`;
  });

  const protectedPlaceholders = [];
  normalized = normalized.replace(NAMED_PLACEHOLDER_RE, (match) => {
    const token = `${PLACEHOLDER_TOKEN_PREFIX}${protectedPlaceholders.length}__`;
    protectedPlaceholders.push(match);
    return token;
  });

  normalized = normalized
    .replace(/\{/g, "{'{'}")
    .replace(/\}/g, "{'}'}")
    .replace(/@/g, "{'@'}");

  protectedPlaceholders.forEach((placeholder, index) => {
    normalized = normalized.replace(`${PLACEHOLDER_TOKEN_PREFIX}${index}__`, placeholder);
  });

  return normalized;
}

function normalizeLocaleMessages(messages, referenceMessages = {}) {
  if (Array.isArray(messages)) {
    return messages.map((value, index) => normalizeLocaleMessages(
      value,
      Array.isArray(referenceMessages) ? referenceMessages[index] : undefined,
    ));
  }

  if (!messages || typeof messages !== 'object') {
    return messages;
  }

  return Object.fromEntries(Object.entries(messages).map(([key, value]) => {
    const referenceValue = referenceMessages && typeof referenceMessages === 'object'
      ? referenceMessages[key]
      : undefined;

    if (typeof value === 'string') {
      return [
        key,
        normalizeLocaleMessage(value, typeof referenceValue === 'string' ? referenceValue : ''),
      ];
    }

    if (value && typeof value === 'object') {
      return [key, normalizeLocaleMessages(value, referenceValue)];
    }

    return [key, value];
  }));
}

async function loadLocaleMessages(locale) {
  const [localeResult, defaultLocaleResult] = await Promise.allSettled([
    api.getLang(locale),
    locale === 'en' ? Promise.resolve(null) : api.getLang('en'),
  ]);

  if (localeResult.status !== 'fulfilled') {
    throw localeResult.reason;
  }

  const rawLocaleMessages = localeResult.value;
  const rawDefaultMessages = locale === 'en' || defaultLocaleResult.status !== 'fulfilled'
    ? rawLocaleMessages
    : defaultLocaleResult.value;

  const defaultMessages = normalizeLocaleMessages(rawDefaultMessages, rawDefaultMessages);
  const localeMessages = locale === 'en'
    ? defaultMessages
    : normalizeLocaleMessages(rawLocaleMessages, defaultMessages);

  return { defaultMessages, localeMessages };
}

function getRawLocaleMessage(messages, key) {
  if (!messages || !key) {
    return null;
  }

  if (Object.prototype.hasOwnProperty.call(messages, key)) {
    return messages[key];
  }

  return key.split('.').reduce((value, segment) => {
    if (!value || typeof value !== 'object') {
      return null;
    }

    return value[segment];
  }, messages);
}

function renderRawLocaleMessage(rawMessage, methodName, args) {
  if (typeof rawMessage !== 'string') {
    return rawMessage;
  }

  let namedValues = {};
  if (methodName === 'tc') {
    namedValues = args.find((arg, index) => index > 0 && arg && typeof arg === 'object' && !Array.isArray(arg)) || {};
  } else {
    namedValues = args.find((arg) => arg && typeof arg === 'object' && !Array.isArray(arg)) || {};
  }

  return rawMessage
    .replace(NAMED_PLACEHOLDER_RE, (match, name) => (
      Object.prototype.hasOwnProperty.call(namedValues, name) ? String(namedValues[name]) : match
    ))
    .replace(/\{'@'\}/g, '@')
    .replace(/\{'\{'\}/g, '{')
    .replace(/\{'\}'\}/g, '}');
}

function isI18nSyntaxError(err) {
  if (!err) {
    return false;
  }

  const message = String(err.message || err);
  return err.name === 'SyntaxError'
    || message.includes('placeholder')
    || message.includes('linked format')
    || message.includes('lexical')
    || message.includes('Unexpected');
}

function withI18nFallback(methodName) {
  const translate = methodName === 'tc'
    ? (key, choice, named) => i18n.global.t(key, choice, named)
    : i18n.global[methodName];
  const fallbackTarget = methodName === 'tc' ? i18n.global.t : null;
  let original = (key) => key;
  if (typeof translate === 'function') {
    original = translate.bind(i18n.global);
  } else if (typeof fallbackTarget === 'function') {
    original = fallbackTarget.bind(i18n.global);
  }

  return (key, ...args) => {
    try {
      if (methodName === 'tc' && typeof translate !== 'function') {
        return original(key);
      }
      return original(key, ...args);
    } catch (err) {
      if (!isI18nSyntaxError(err)) {
        throw err;
      }

      const { locale } = i18n.global;
      const localeKey = typeof locale === 'string' ? locale : locale && locale.value;
      const messages = localeKey ? i18n.global.getLocaleMessage(localeKey) : {};
      const rawMessage = getRawLocaleMessage(messages, key);
      return typeof rawMessage === 'string'
        ? renderRawLocaleMessage(rawMessage, methodName, args)
        : key;
    }
  };
}

function setI18nLocale(locale) {
  if (typeof i18n.global.locale === 'string') {
    i18n.global.locale = locale;
    return;
  }

  i18n.global.locale.value = locale;
}

i18n.global.t = withI18nFallback('t');
i18n.global.tc = withI18nFallback('tc');

const sharedUtils = new Utils(i18n.global);
let vueApp = null;

function getRoleId(profile) {
  if (!profile) {
    return 0;
  }

  if (profile.userRole && Number(profile.userRole.id) > 0) {
    return Number(profile.userRole.id);
  }

  if (Number(profile.userRoleId) > 0) {
    return Number(profile.userRoleId);
  }

  const authRecord = api.getAuthRecord();
  if (authRecord && Number(authRecord.role) > 0) {
    return Number(authRecord.role);
  }

  return 0;
}

function isSuperAdmin(profile) {
  return getRoleId(profile) === 1;
}

async function initConfig(rootProxy) {
  const proxy = rootProxy;
  const profile = api.getStoredUserProfile();
  if (!api.isAuthenticated() || !profile) {
    const unauthorized = new Error('missing auth profile');
    unauthorized.status = 401;
    throw unauthorized;
  }

  const cfg = await api.getServerConfig();
  const { defaultMessages, localeMessages } = await loadLocaleMessages(cfg.lang);
  i18n.global.setLocaleMessage('en', defaultMessages);
  setI18nLocale(cfg.lang);
  i18n.global.setLocaleMessage(cfg.lang, localeMessages);
  sharedUtils.updateRelativeTimeLocale();

  proxy.$utils = sharedUtils;
  proxy.$api = api;
  proxy.$events = eventBus;
  proxy.$t = i18n.global.t;
  proxy.$tc = i18n.global.tc;
  proxy.$can = (...perms) => {
    if (isSuperAdmin(profile)) {
      return true;
    }

    const userPerms = Array.isArray(profile.userRole && profile.userRole.permissions)
      ? profile.userRole.permissions
      : [];

    return perms.some((perm) => {
      if (perm.endsWith('*')) {
        const group = `${perm.split(':')[0]}:`;
        return userPerms.some((p) => p.startsWith(group));
      }
      return userPerms.includes(perm);
    });
  };

  proxy.$canList = (id, perm) => {
    if (isSuperAdmin(profile)) {
      return true;
    }

    const can = proxy.$can('lists:get_all', 'lists:manage_all');
    if (can) {
      return true;
    }
    const listPerms = Array.isArray(profile.listRole && profile.listRole.lists)
      ? profile.listRole.lists
      : [];
    const targetID = typeof id === 'string' ? id : String(id);
    return listPerms.some((list) => String(list.id) === targetID && list.permissions.includes(perm));
  };

  if (vueApp) {
    vueApp.config.globalProperties.$utils = sharedUtils;
    vueApp.config.globalProperties.$api = api;
    vueApp.config.globalProperties.$events = eventBus;
    vueApp.config.globalProperties.$t = i18n.global.t;
    vueApp.config.globalProperties.$tc = i18n.global.tc;
    vueApp.config.globalProperties.$can = proxy.$can;
    vueApp.config.globalProperties.$canList = proxy.$canList;
    vueApp.config.globalProperties.$docsUrl = docsUrl;
  }

  const to = router.currentRoute.value;
  const title = to.meta.title && i18n.global.te(to.meta.title) ? `${i18n.global.tc(to.meta.title, 0)} /` : '';
  document.title = `${title} listpocket`;

  proxy.isLoaded = true;
}

function getAdminBasePath() {
  const baseURL = import.meta.env.BASE_URL || '/';
  if (baseURL === '/') {
    return '';
  }
  return baseURL.replace(/\/$/, '');
}

function getLoginRedirectTarget() {
  const adminBase = getAdminBasePath();
  const { pathname = '/', search = '', hash = '' } = window.location;
  let nextPath = pathname;

  if (adminBase && nextPath.startsWith(`${adminBase}/`)) {
    nextPath = nextPath.slice(adminBase.length);
  } else if (adminBase && nextPath === adminBase) {
    nextPath = '/';
  }

  const next = `${nextPath || '/'}${search}${hash}`;
  return next === '/' ? '/admin' : next;
}

function redirectToLogin() {
  const adminBase = getAdminBasePath();
  const next = getLoginRedirectTarget();
  window.location.href = `${adminBase}/login?next=${encodeURIComponent(next)}`;
}

router.afterEach((to) => {
  const title = to.meta.title && i18n.global.te(to.meta.title) ? `${i18n.global.tc(to.meta.title, 0)} /` : '';
  document.title = `${title} listpocket`;
});

const Root = {
  data() {
    return {
      isLoaded: false,
    };
  },
  methods: {
    async loadConfig() {
      await initConfig(this);
    },
    awaitRestart(response) {
      return new Promise((resolve) => {
        if (response && typeof response === 'object' && response.needsRestart) {
          this.loadConfig();
          resolve({ needsRestart: true });
          return;
        }

        this.$utils.toast(i18n.global.t('settings.messengers.messageSaved'));
        const pollId = setInterval(() => {
          api.getHealth().then(() => {
            clearInterval(pollId);
            this.loadConfig();
            resolve({ needsRestart: false });
          });
        }, 1000);
      });
    },
  },
  render() {
    return h(App);
  },
};

const app = createApp(Root);
vueApp = app;
app.use(router);
app.use(store);
app.use(i18n);
app.use(vuetify);
registerLegacyUI(app);
installLegacyUIStyles();

app.config.globalProperties.$api = api;
app.config.globalProperties.$utils = sharedUtils;
app.config.globalProperties.$events = null;
app.config.globalProperties.$t = i18n.global.t;
app.config.globalProperties.$tc = i18n.global.tc;
app.config.globalProperties.$can = () => false;
app.config.globalProperties.$canList = () => false;
app.config.globalProperties.$docsUrl = docsUrl;

const rootProxy = app.mount('#app');
initConfig(rootProxy).catch((err) => {
  if (err && (err.status === 401 || (err.response && err.response.status === 401))) {
    redirectToLogin();
    return;
  }
  const msg = (err && err.response && err.response.message) || (err && err.message)
    || 'Failed to initialize the app';
  sharedUtils.toast(msg, 'is-danger', 5000, false);
});
