// Mounts views/AgentChat.vue directly (the module chat API at /api/mcp/chat/v1).
// The spec stubs the module API with cy.intercept.
import { createApp, h } from 'vue';
import { createRouter, createMemoryHistory, RouterView } from 'vue-router';
import { VApp, VMain } from 'vuetify/components';
import vuetify from '../../src/plugins/vuetify';
import store from '../../src/store';
import AgentChat from '../../src/views/AgentChat.vue';
import { pb } from '../../src/api';

// Enough English to read the page; other keys show as themselves.
const EN = {
  'agent.newChat': 'New chat', 'agent.conversations': 'Conversations', 'agent.noConversations': 'No conversations yet.',
  'agent.inContext': 'In context:', 'agent.welcomeTitle': 'What should we work on?', 'agent.context': 'Context',
  'agent.welcomeHelp': 'Ask freely: anything I change comes back as a proposal. Type {at} to attach something.',
  'agent.placeholder': 'Ask anything… ({at} to attach context)', 'agent.pasteData': 'Paste data', 'agent.pasteName': 'Name',
  'agent.pasteContent': 'Content', 'agent.attach': 'Attach', 'agent.attachFile': 'Attach file',
  'agent.searchModels': 'Search models', 'agent.defaultNamed': 'Default · {model}',
  'agent.usedTools': 'Used {n} tool(s)', 'agent.review.title': 'Review', 'agent.reject': 'Reject', 'agent.view': 'View',
  'agent.suggest.overview': 'Give me an overview', 'agent.suggest.replies': 'Which replies are waiting?', 'agent.suggest.workflow': 'Build a workflow',
  'agent.loadMore': 'Load more',
  'agent.noProvider': "AI isn't set up. Add an API key or sign in with ChatGPT in Settings → AI, then reload this page.",
  'globals.buttons.delete': 'Delete', 'globals.buttons.cancel': 'Cancel', 'globals.buttons.close': 'Close', 'agent.defaultModel': 'Default',
};
const t = (key, params = {}) => (EN[key] || key).replace(/\{(\w+)\}/g, (_, k) => params[k] ?? '');

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/agent/:id?', name: 'agent', component: AgentChat }, { path: '/:rest(.*)*', component: { render: () => h('div') } }],
});
await router.push(new URLSearchParams(location.search).get('path') || '/agent');
pb.authStore.save('fixture-token', { id: 'fixture-owner', collectionId: 'users' });
window.agentFixture = { router, store, pb };
const app = createApp({ render: () => h(VApp, {}, () => [h(VMain, {}, () => [h('div', { style: 'padding: 16px' }, [h(RouterView)])])]) });
app.config.globalProperties.$t = t;
app.use(store).use(router).use(vuetify).mount('#app');
