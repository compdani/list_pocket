import test from 'node:test';
import assert from 'node:assert/strict';
import {
  createRegistrar,
  createRegistry,
  mergeNavItems,
  PLUGIN_SLOTS,
  pluginFieldEditor,
  runPluginSetup,
  schemaWidget,
} from '../src/pluginHost/registry.mjs';

function setup(id = 'sample', fieldEditors = []) {
  const registry = createRegistry();
  const routes = [];
  const registrar = createRegistrar(registry, id, {
    addRoute: (r) => routes.push(r),
    wrapComponent: (_, c) => c,
    fieldEditors,
  });
  return { registry, routes, registrar };
}

test('routes are forced into the plugin namespace', () => {
  const { routes, registrar } = setup();
  const name = registrar.routes.add({ name: 'dashboard', path: '/dashboard', component: {} });
  assert.equal(name, 'plugin.sample.dashboard');
  assert.equal(routes[0].name, 'plugin.sample.dashboard');
  assert.equal(routes[0].path, '/plugins/sample/dashboard');
  assert.equal(routes[0].meta.plugin, 'sample');
});

test('nav keys match the route name, which App.vue depends on', () => {
  const { registry, registrar } = setup();
  registrar.nav.add({ route: 'contacts', label: 'Contacts', parent: 'settings' });
  assert.equal(registry.nav[0].key, 'plugin.sample.contacts');
  assert.deepEqual(registry.nav[0].route, { name: 'plugin.sample.contacts' });
});

test('unknown slots and incomplete registrations throw', () => {
  const { registrar } = setup();
  assert.throws(() => registrar.slots.add('nope', {}), /unknown slot/);
  assert.throws(() => registrar.routes.add({ name: 'x' }), /needs/);
  assert.throws(() => registrar.nav.add({}), /needs/);
  assert.throws(() => registrar.settings.addTab({ key: 'x' }), /needs/);
});

test('slots collect components from each plugin', () => {
  const { registry, registrar } = setup();
  const [slot] = PLUGIN_SLOTS;
  registrar.slots.add(slot, { a: 1 });
  registrar.slots.add(slot, { a: 2 });
  assert.equal(registry.slots[slot].length, 2);
  assert.notEqual(registry.slots[slot][0].key, registry.slots[slot][1].key);
});

test('a failing or hanging plugin is recorded and does not throw', async () => {
  const registry = createRegistry();
  assert.equal(await runPluginSetup(registry, 'bad', () => { throw new Error('boom'); }), false);
  assert.equal(await runPluginSetup(registry, 'slow', () => new Promise(() => {}), 5), false);
  assert.equal(await runPluginSetup(registry, 'good', () => {}), true);
  assert.deepEqual(registry.failures.map((f) => f.plugin), ['bad', 'slow']);
  assert.match(registry.failures[0].message, /boom/);
  assert.match(registry.failures[1].message, /timed out/);
});

test('mergeNavItems nests under a parent and honours permissions', () => {
  const items = [{ key: 'settings', label: 'S', children: [{ key: 'a' }] }, { key: 'dash', label: 'D' }];
  const merged = mergeNavItems(items, [
    { key: 'p1', label: 'One', parent: 'settings', route: {} },
    { key: 'p2', label: 'Two', route: {} },
    { key: 'p3', label: 'Three', permission: 'plugin_x:y', route: {} },
    { key: 'p4', label: 'Four', parent: 'dash', route: {} },
  ], () => false);
  assert.deepEqual(merged[0].children.map((c) => c.key), ['a', 'p1']);
  assert.deepEqual(merged.map((i) => i.key), ['settings', 'dash', 'p2', 'p4']);
  assert.equal(items[0].children.length, 1, 'the input must not be mutated');
});

test('field editors are namespaced and must be declared', () => {
  const { registry, registrar } = setup('sample', ['color']);
  const editor = {};
  registrar.fields.add('color', editor);
  assert.equal(registry.fields['sample.color'], editor);
  assert.throws(() => registrar.fields.add('other', {}), /isn't declared/);
  assert.throws(() => registrar.fields.add('color', null), /needs a component/);
});

test('a schema property finds its editor by x-ui, or gets a plain input', () => {
  const { registry, registrar } = setup('sample', ['color']);
  const editor = {};
  registrar.fields.add('color', editor);
  assert.equal(pluginFieldEditor(registry, { 'x-ui': 'sample.color' }), editor);
  assert.equal(pluginFieldEditor(registry, { 'x-ui': { widget: 'sample.color' } }), editor);
  assert.equal(pluginFieldEditor(registry, { 'x-ui': 'sample.gone' }), null);
  assert.equal(pluginFieldEditor(registry, { type: 'string' }), null);
  assert.deepEqual(schemaWidget({ 'x-ui': { widget: 'a.b', size: 2 } }).options, { widget: 'a.b', size: 2 });
  assert.equal(schemaWidget({ 'x-ui': 3 }).name, '');
});
