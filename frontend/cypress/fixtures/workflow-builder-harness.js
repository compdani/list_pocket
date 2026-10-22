import { createApp, h } from 'vue';
import { createRouter, createMemoryHistory, RouterView } from 'vue-router';
import { createI18n } from 'vue-i18n';
import { VApp, VMain } from 'vuetify/components';
import store from '../../src/store';
import vuetify from '../../src/plugins/vuetify';
import WorkflowBuilder from '../../src/views/WorkflowBuilder.vue';
import ConfirmDialog from '../../src/components/ConfirmDialog.vue';
import { pb } from '../../src/api';
const router = createRouter({ history: createMemoryHistory(), routes: [{ name: 'workflowBuilder', path: '/workflows/builder/:workflowId?', component: WorkflowBuilder }] });
// Mock only authentication and network boundaries; mount the actual route view.
pb.authStore.save(`fixture.${btoa(JSON.stringify({ exp: 4102444800 }))}.signature`, { id: 'workflow-fixture-owner' });
for (const name of ['workflows', 'workflow_nodes', 'workflow_edges', 'workflow_runs', 'node_runs', 'subscribers']) {
  pb.collection(name).subscribe = async () => () => {};
  pb.collection(name).unsubscribe = async () => {};
}
if (new URLSearchParams(location.search).has('mock')) {
  let created = false; let graph = { nodes: [], edges: [] }; let status = 'draft';
  if (new URLSearchParams(location.search).has('demo')) {
    created = true; graph = { name: 'Event follow-up', nodes: [
      { id: 'start', type: 'trigger', label: 'Event signup', description: 'Start when a new event signup arrives.', config: { mode: 'manual' }, schema: [], positionX: 180, positionY: 80, contactMode: 'manual' },
      { id: 'wait', type: 'wait_until', label: 'Wait until the reminder', description: 'Pause until the day before the event.', config: { offsetDays: -1 }, schema: [], positionX: 180, positionY: 280, contactMode: 'manual' },
      { id: 'email', type: 'send_transactional_email', label: 'Send a friendly reminder', description: 'Share the event details with your contact.', config: {}, schema: [], positionX: 180, positionY: 480, contactMode: 'manual' },
    ], edges: [ { id: 'first', sourceNodeId: 'start', targetNodeId: 'wait', sourceHandle: '', targetHandle: '', condition: { branch: 'next' } }, { id: 'second', sourceNodeId: 'wait', targetNodeId: 'email', sourceHandle: '', targetHandle: '', condition: { branch: 'next' } } ] };
  }
  window.fetch = async (url, options = {}) => {
    const path = new URL(url, location.origin).pathname;
    if (path === '/api/control-plane/workflows' && options.method === 'POST') {
      await new Promise(resolve => setTimeout(resolve, 250)); created = true; graph = { nodes: [], edges: [] };
    }
    if (path.endsWith('/save')) { graph = JSON.parse(options.body); document.documentElement.dataset.fixtureGraph = JSON.stringify(graph); }
    if (path.endsWith('/validate')) return Response.json({ valid: true, errors: [], findings: [] });
    if (path.endsWith('/publish')) status = 'published';
    const workflow = { id: 'workflow-fixture', name: graph.name || 'Untitled Workflow', status, version: 1, triggerType: 'manual' };
    return Response.json({ workflows: created ? [workflow] : [], activeWorkflow: created ? { workflow, nodes: graph.nodes, edges: graph.edges } : null, contacts: [], companies: [], runLogs: [] });
  };
}
await router.push('/workflows/builder');
const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: {} } });
createApp({ render: () => h(VApp, {}, () => h(VMain, {}, () => [h(RouterView), h(ConfirmDialog)])) }).use(router).use(store).use(i18n).use(vuetify).mount('#app');
