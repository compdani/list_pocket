// What plugins have registered in the browser. This file has no browser or Vue
// imports, so it can be unit tested directly.

export const PLUGIN_SLOTS = [
  'campaign.form.after',
  'template.form.after',
  'subscriber.form.after',
  'subscriber.activity.item',
  'dashboard.widgets',
];

export function createRegistry(wrap = (x) => x) {
  return wrap({
    nav: [],
    settingsTabs: [],
    slots: {},
    fields: {},
    failures: [],
    manifests: [],
  });
}

export function pluginRouteName(pluginId, name) {
  return `plugin.${pluginId}.${name}`;
}

export function pluginRoutePath(pluginId, path) {
  return `/plugins/${pluginId}/${String(path || '').replace(/^\/+/, '')}`;
}

// Builds the registration API for one plugin. Names and paths are forced into the
// plugin's namespace, so a plugin can't replace a host route or another plugin's.
export function createRegistrar(registry, pluginId, deps) {
  const { addRoute, wrapComponent, fieldEditors = [] } = deps;

  return {
    routes: {
      add(route) {
        if (!route || !route.name || !route.path || !route.component) {
          throw new Error('routes.add needs a name, path and component');
        }
        const name = pluginRouteName(pluginId, route.name);
        addRoute({
          path: pluginRoutePath(pluginId, route.path),
          name,
          meta: {
            title: route.title,
            group: route.group,
            plugin: pluginId,
            permission: route.permission,
          },
          component: wrapComponent(pluginId, route.component, route.permission),
        });
        return name;
      },
    },
    nav: {
      add(item) {
        if (!item || !item.route || !item.label) {
          throw new Error('nav.add needs a route and label');
        }
        const name = pluginRouteName(pluginId, item.route);
        // App.vue finds the active item by route name, so the key must match it.
        registry.nav.push({
          key: name,
          label: item.label,
          icon: item.icon || 'mdi-puzzle-outline',
          route: { name },
          parent: item.parent || '',
          permission: item.permission || '',
          plugin: pluginId,
        });
      },
    },
    settings: {
      addTab(tab) {
        if (!tab || !tab.key || !tab.label || !tab.component) {
          throw new Error('settings.addTab needs a key, label and component');
        }
        registry.settingsTabs.push({
          key: `plugin:${pluginId}:${tab.key}`,
          label: tab.label,
          component: wrapComponent(pluginId, tab.component),
          plugin: pluginId,
        });
      },
    },
    slots: {
      add(slot, component) {
        if (!PLUGIN_SLOTS.includes(slot)) {
          throw new Error(`unknown slot "${slot}"; available: ${PLUGIN_SLOTS.join(', ')}`);
        }
        if (!component) {
          throw new Error('slots.add needs a component');
        }
        if (!registry.slots[slot]) {
          registry.slots[slot] = [];
        }
        registry.slots[slot].push({
          key: `${pluginId}:${registry.slots[slot].length}`,
          component: wrapComponent(pluginId, component),
          plugin: pluginId,
        });
      },
    },
    fields: {
      add(name, component) {
        if (!fieldEditors.includes(name)) {
          throw new Error(`field editor "${name}" isn't declared in plugin.json`);
        }
        if (!component) {
          throw new Error('fields.add needs a component');
        }
        registry.fields[`${pluginId}.${name}`] = wrapComponent(pluginId, component);
      },
    },
  };
}

// The name and options a schema property's x-ui asks for.
export function schemaWidget(prop) {
  const ui = prop && prop['x-ui'];
  if (typeof ui === 'string') {
    return { name: ui, options: {} };
  }
  if (ui && typeof ui === 'object' && typeof ui.widget === 'string') {
    return { name: ui.widget, options: ui };
  }
  return { name: '', options: {} };
}

// The plugin component that edits a property, or null if there's none running, in
// which case the property gets a plain input.
export function pluginFieldEditor(registry, prop) {
  return registry.fields[schemaWidget(prop).name] || null;
}

// Runs one plugin's setup under a time limit, and records any failure instead of
// throwing, so one bad plugin can't stop the others or the admin UI.
export async function runPluginSetup(registry, pluginId, load, timeoutMs = 5000) {
  let timer;
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error(`timed out after ${timeoutMs}ms`)), timeoutMs);
  });
  try {
    await Promise.race([Promise.resolve().then(load), timeout]);
    return true;
  } catch (err) {
    registry.failures.push({ plugin: pluginId, message: String((err && err.message) || err) });
    return false;
  } finally {
    clearTimeout(timer);
  }
}

// Fits a plugin nav item into the host's nav tree.
export function mergeNavItems(items, pluginItems, can) {
  const out = items.map((item) => (item.children ? { ...item, children: [...item.children] } : item));
  pluginItems.forEach((item) => {
    if (item.permission && !can(item.permission)) {
      return;
    }
    const entry = { key: item.key, label: item.label, icon: item.icon, route: item.route };
    const parent = item.parent && out.find((i) => i.key === item.parent && i.children);
    if (parent) {
      parent.children.push(entry);
    } else {
      out.push(entry);
    }
  });
  return out;
}
