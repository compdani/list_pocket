// Mounts components/AIAssistantHost.vue with /api/config saying the chat module
// is on, so the side panel is components/AIAssistantChat.vue. The spec stubs
// /api/mcp/chat/v1. ?path= sets the route the turn's context describes.
// ?editor=campaign|template|workflow|visual registers that editor.
import { createApp, h, ref } from 'vue';
import { createRouter, createMemoryHistory } from 'vue-router';
import { VApp, VMain } from 'vuetify/components';
import vuetify from '../../src/plugins/vuetify';
import store from '../../src/store';
import AIAssistantHost from '../../src/components/AIAssistantHost.vue';
import { pb } from '../../src/api';
import { assistantContext, registerAssistantEditor } from '../../src/assistant/context';
import { events } from '../../src/utils/events';

const EN = {
  'agent.noProvider': "AI isn't set up. Add an API key or sign in with ChatGPT in Settings → AI, then reload this page.",
  Campaign: 'Campaign',
};
const t = (key, params = {}) => (EN[key] || key).replace(/\{(\w+)\}/g, (_, name) => params[name] ?? '');

store.commit('setModelResponse', { model: 'serverConfig', data: { chat_backend: 'module' } });

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/campaigns/:id', name: 'campaign', meta: { title: 'Campaign' }, component: { render: () => h('div') } },
    { path: '/agent/:id?', name: 'agent', component: { render: () => h('div') } },
    { path: '/:rest(.*)*', component: { render: () => h('div') } },
  ],
});

const params = new URLSearchParams(location.search);
await router.push(params.get('path') || '/campaigns/example?status=live');
pb.authStore.save('fixture-token', { id: 'fixture-owner', collectionId: 'users' });
assistantContext.open = true;

function campaignBlank() {
  return { subject: 'Welcome', preheader: 'Preview', body: '<h1>Welcome</h1><a>Shop today</a><p>Footer</p>', contentType: 'html' };
}
function templateBlank() {
  return { subject: 'Hello', preheader: '', body: 'Hello there', contentType: 'plain' };
}
function workflowBlank() {
  return {
    kind: 'workflow',
    nodes: [{ id: 'trigger', type: 'trigger' }, { id: 'wait', type: 'wait', config: { days: 1 } }],
    edges: [{ id: 'e1', source: 'trigger', target: 'wait' }],
  };
}
function visualBlank() {
  const document = { root: { type: 'EmailLayout' }, cta: { type: 'Button', text: 'Shop today' } };
  return { subject: 'Welcome', preheader: '', body: JSON.stringify(document), contentType: 'visual' };
}

const EDITORS = {
  campaign: { key: 'campaign:example', label: 'Unsaved campaign editor', blank: campaignBlank },
  template: { key: 'template:example:campaign:html', label: 'Unsaved template editor', blank: templateBlank },
  workflow: { key: 'workflow:example', label: 'Unsaved workflow editor', blank: workflowBlank },
  visual: { key: 'campaign:visual', label: 'Unsaved campaign editor', blank: visualBlank },
};

const snapshot = ref(null);
function useEditor(name) {
  const spec = EDITORS[name] || EDITORS.campaign;
  snapshot.value = spec.blank();
  registerAssistantEditor({
    key: spec.key,
    label: spec.label,
    snapshot: () => snapshot.value,
    selection: () => '',
    apply: (next) => { snapshot.value = next; },
  });
}
function change() {
  const value = snapshot.value;
  if (value && typeof value.body === 'string') snapshot.value = { ...value, body: `${value.body} new edit` };
  else if (value && Array.isArray(value.nodes)) snapshot.value = { ...value, nodes: [...value.nodes, { id: 'extra', type: 'note' }] };
}

const editorName = params.get('editor') || 'campaign';
if (editorName !== 'none') useEditor(editorName);
window.assistantFixture = { snapshot, change, useEditor, router, store };

const app = createApp({
  render: () => h(VApp, {}, () => [h(VMain, {}, () => [
    h('pre', { 'data-cy': 'fixture-body' }, JSON.stringify(snapshot.value)),
    h('button', { 'data-cy': 'fixture-edit', onClick: change }, 'Manual edit'),
    h(AIAssistantHost),
  ])]),
});
app.config.globalProperties.$t = t;
app.config.globalProperties.$tc = t;
app.config.globalProperties.$can = () => false;
app.config.globalProperties.$utils = {
  getPref: () => false,
  setPref: () => {},
  niceDate: () => '1 Jan',
  confirm: (_message, action) => action && action(),
};
app.config.globalProperties.$events = events;
app.config.globalProperties.$api = {
  listPictures: async () => ({
    items: [{ id: 'pic1', original_name: 'hero.png', file: 'hero.png', created: '2026-01-01T00:00:00Z' }],
    totalItems: 1,
    perPage: 20,
  }),
  pb: { files: { getURL: () => 'https://cdn.example/hero.png' } },
};
app.use(store).use(router).use(vuetify).mount('#app');
