import * as Vue from 'vue';
import * as Vuetify from 'vuetify';
import {
  defineAsyncComponent, h, markRaw, reactive,
} from 'vue';
import { getPluginsUI, pb } from '../api';
import { createRegistrar, createRegistry, runPluginSetup } from './registry.mjs';
import PluginBoundary from './PluginBoundary.vue';

// The shims in static/plugin-shims re-export these, which is how a plugin's bare
// `import { h } from 'vue'` reaches the host's single copy of Vue.
globalThis.__LP_HOST__ = { Vue, Vuetify };

const SHIM_BASE = '/admin/static/plugin-shims';

export const pluginRegistry = createRegistry(reactive);

let ctx = null;
let loading = null;
const messagesByPlugin = {};

// initPluginHost must run before app.use(router), which starts the first navigation.
export function initPluginHost(hostContext) {
  ctx = hostContext;
}

function wrapComponent(pluginId, component, permission) {
  const inner = typeof component === 'function' && !component.setup && !component.render
    ? defineAsyncComponent(component)
    : component;
  return markRaw({
    name: `Plugin_${pluginId}`,
    inheritAttrs: false,
    render() {
      return h(PluginBoundary, { pluginId, permission }, () => h(inner, this.$attrs));
    },
  });
}

async function pluginFetch(pluginId, path, options = {}) {
  const { json, ...init } = options;
  const headers = { ...(init.headers || {}), Authorization: `Bearer ${pb.authStore.token}` };
  if (json !== undefined) {
    headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(json);
    init.method = init.method || 'POST';
  }
  const res = await fetch(`/mailapi/plugins/${pluginId}/api/${String(path).replace(/^\/+/, '')}`, { ...init, headers });
  const type = res.headers.get('content-type') || '';
  const data = type.includes('application/json') ? await res.json() : await res.text();
  if (!res.ok) {
    const err = new Error((data && data.message) || `request failed with ${res.status}`);
    err.status = res.status;
    throw err;
  }
  return data;
}

function createHost(pluginId, fieldEditors) {
  const registrar = createRegistrar(pluginRegistry, pluginId, {
    addRoute: (record) => ctx.router.addRoute(record),
    wrapComponent,
    fieldEditors,
  });
  return {
    id: pluginId,
    ...registrar,
    fetch: (path, options) => pluginFetch(pluginId, path, options),
    t: (...args) => ctx.i18n.global.t(...args),
    can: (...args) => ctx.app.config.globalProperties.$can(...args),
    router: ctx.router,
    toast: (...args) => ctx.app.config.globalProperties.$utils.toast(...args),
  };
}

function loadStylesheet(href, { first = false } = {}) {
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = href;
  if (first) {
    // Before the app's own CSS, so the host's Vuetify styles win any tie.
    document.head.prepend(link);
  } else {
    document.head.append(link);
  }
}

async function loadComponentShim() {
  loadStylesheet(`${SHIM_BASE}/vuetify-components.css`, { first: true });
  const { components, directives } = await import(/* @vite-ignore */ `${SHIM_BASE}/vuetify-components.js`);
  Object.entries(components).forEach(([name, component]) => {
    if (!ctx.app.component(name)) {
      ctx.app.component(name, component);
    }
  });
  Object.entries(directives).forEach(([name, directive]) => {
    if (!ctx.app.directive(name)) {
      ctx.app.directive(name, directive);
    }
  });
}

// mergePluginMessages re-applies plugin text. Locale loads replace every message,
// so this has to run after each one.
export function mergePluginMessages(locale) {
  if (!ctx) {
    return;
  }
  Object.values(messagesByPlugin).forEach((byLocale) => {
    [locale, 'en'].forEach((lang) => {
      if (byLocale[lang]) {
        ctx.i18n.global.mergeLocaleMessage(lang, ctx.normalizeMessages(byLocale[lang]));
      }
    });
  });
}

async function loadAll() {
  if (new URLSearchParams(window.location.search).get('plugins') === 'off') {
    return;
  }

  let manifests;
  try {
    manifests = await getPluginsUI();
  } catch {
    return;
  }
  if (!Array.isArray(manifests) || manifests.length === 0) {
    return;
  }
  pluginRegistry.manifests = manifests;

  manifests.forEach((m) => {
    if (m.messages) {
      messagesByPlugin[m.id] = m.messages;
    }
  });
  mergePluginMessages(ctx.i18n.global.locale.value);

  const withUI = manifests.filter((m) => m.entry);
  if (withUI.length > 0) {
    try {
      await loadComponentShim();
    } catch (err) {
      pluginRegistry.failures.push({ plugin: 'host', message: `Vuetify shim: ${err.message}` });
    }
  }

  await Promise.allSettled(withUI.map((m) => runPluginSetup(pluginRegistry, m.id, async () => {
    (m.css || []).forEach((href) => loadStylesheet(href));
    const mod = await import(/* @vite-ignore */ m.entry);
    await mod.default(createHost(m.id, m.fieldEditors));
  })));

  pluginRegistry.failures.forEach(({ plugin, message }) => {
    // eslint-disable-next-line no-console
    console.error(`plugin ${plugin} failed to load: ${message}`);
  });
}

// ensurePluginsLoaded returns true the first time only. It never rejects.
export async function ensurePluginsLoaded() {
  if (loading) {
    await loading;
    return false;
  }
  loading = loadAll();
  await loading;
  return true;
}
