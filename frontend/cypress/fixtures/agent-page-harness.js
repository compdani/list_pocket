// Mounts the Agent page (views/Agent.vue → <lp-agent-page>) with an in-memory router.
// The specs stub the API with cy.intercept.
import { createApp, h } from 'vue';
import { createRouter, createMemoryHistory, RouterView } from 'vue-router';
import { VApp, VMain } from 'vuetify/components';
import vuetify from '../../src/plugins/vuetify';
import Agent from '../../src/views/Agent.vue';
import { pb } from '../../src/api';

// Enough English to read the page; other keys show as themselves.
const EN = {
  'agent.newChat': 'New chat', 'agent.conversations': 'Conversations', 'agent.noConversations': 'No conversations yet.',
  'agent.inContext': 'In context:', 'agent.welcomeTitle': 'What should we work on?', 'agent.context': 'Context',
  'agent.welcomeHelp': 'Ask freely: anything I change comes back as a proposal. Type {at} to attach something.',
  'agent.placeholder': 'Ask anything… ({at} to attach context)', 'agent.pasteData': 'Paste data', 'agent.attach': 'Attach',
  'agent.usedTools': 'Used {n} tool(s)', 'agent.review.title': 'Review', 'agent.reject': 'Reject', 'agent.view': 'View',
  'agent.suggest.overview': 'Give me an overview', 'agent.suggest.replies': 'Which replies are waiting?', 'agent.suggest.workflow': 'Build a workflow',
  'globals.buttons.delete': 'Delete', 'globals.buttons.cancel': 'Cancel', 'agent.defaultModel': 'Default',
};
const t = (key, params = {}) => (EN[key] || key).replace(/\{(\w+)\}/g, (_, k) => params[k] ?? '');

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/agent/:id?', name: 'agent', component: Agent }, { path: '/:rest(.*)*', component: { render: () => h('div') } }],
});
await router.push(new URLSearchParams(location.search).get('path') || '/agent');
pb.authStore.save('fixture-token', { id: 'fixture-owner' });
window.agentFixture = { router };
const app = createApp({ render: () => h(VApp, {}, () => [h(VMain, {}, () => [h('div', { style: 'padding: 16px' }, [h(RouterView)])])]) });
app.config.globalProperties.$t = t;
app.use(router).use(vuetify).mount('#app');
