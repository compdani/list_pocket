import { createApp, h } from 'vue';
import { createI18n } from 'vue-i18n';

import App from './App.vue';
import router from './router';
import store from './store';
import * as api from './api';
import Utils from './utils';
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
  legacy: true,
  locale: 'en',
  fallbackLocale: 'en',
  messages: {},
});
const eventBus = createEventBus();

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

function withI18nFallback(methodName) {
  const original = i18n.global[methodName].bind(i18n.global);

  return (key, ...args) => {
    try {
      return original(key, ...args);
    } catch (err) {
      if (!err || !String(err.message || err).includes('Empty placeholder')) {
        throw err;
      }

      const { locale } = i18n.global;
      const localeKey = typeof locale === 'string' ? locale : locale && locale.value;
      const messages = localeKey ? i18n.global.getLocaleMessage(localeKey) : {};
      const rawMessage = getRawLocaleMessage(messages, key);
      return typeof rawMessage === 'string' ? rawMessage : key;
    }
  };
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
  const lang = await api.getLang(cfg.lang);
  i18n.global.locale = cfg.lang;
  i18n.global.setLocaleMessage(cfg.lang, lang);

  proxy.$utils = sharedUtils;
  proxy.$api = api;
  proxy.$events = eventBus;
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
    return listPerms.some((list) => list.id === id && list.permissions.includes(perm));
  };

  if (vueApp) {
    vueApp.config.globalProperties.$utils = sharedUtils;
    vueApp.config.globalProperties.$api = api;
    vueApp.config.globalProperties.$events = eventBus;
    vueApp.config.globalProperties.$can = proxy.$can;
    vueApp.config.globalProperties.$canList = proxy.$canList;
  }

  const to = router.currentRoute.value;
  const title = to.meta.title && i18n.global.te(to.meta.title) ? `${i18n.global.tc(to.meta.title, 0)} /` : '';
  document.title = `${title} listpocket`;

  proxy.isLoaded = true;
}

function redirectToLogin() {
  const adminBase = import.meta.env.BASE_URL.replace(/\/$/, '');
  const next = `${window.location.pathname}${window.location.search}${window.location.hash}`;
  window.location.href = `${adminBase}/login?next=${encodeURIComponent(next || '/admin')}`;
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
app.config.globalProperties.$can = () => false;
app.config.globalProperties.$canList = () => false;

const rootProxy = app.mount('#app');
initConfig(rootProxy).catch((err) => {
  if (err && (err.status === 401 || (err.response && err.response.status === 401))) {
    redirectToLogin();
    return;
  }
  console.error(err);
});
