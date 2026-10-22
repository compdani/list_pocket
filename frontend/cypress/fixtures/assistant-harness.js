import { createApp, ref, h } from 'vue';
import { createRouter, createMemoryHistory } from 'vue-router';
import { VApp, VMain } from 'vuetify/components';
import vuetify from '../../src/plugins/vuetify';
import AIAssistant from '../../src/components/AIAssistant.vue';
import { pb } from '../../src/api';
import { assistantContext, registerAssistantEditor } from '../../src/assistant/context';
const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { render: () => h('div') } }] });
await router.push('/campaigns/example');
pb.authStore.save('fixture-token', { id: 'fixture-owner' });
const snapshot = ref({ subject: 'Welcome', preheader: 'Preview', body: '<h1>Welcome</h1><a>Shop today</a><p>Footer</p>', contentType: 'html' });
registerAssistantEditor({ key: 'campaign:example', label: 'Unsaved campaign editor', snapshot: () => ({ ...snapshot.value }), apply: value => { snapshot.value = value; } });
assistantContext.open = true;
window.assistantFixture = { snapshot, navigate: () => router.push('/analytics'), change: () => { snapshot.value.body += 'new edit'; } };
if (new URLSearchParams(location.search).has('mock')) {
  let revision;
  window.fetch = async (url, options) => {
    if (url.endsWith('/turns')) { revision = JSON.parse(options.body).context.editor.revision; return Response.json({ data: { id: 'fixture-turn' } }); }
    if (url.includes('/stream')) {
      const events = [
        { id: 1, type: 'text_delta', data: { text: '**Updated** CTA proposal' } },
        { id: 2, type: 'tool', data: { id: 'tool1', name: 'propose_editor_patch', status: 'completed' } },
        { id: 3, type: 'proposal', data: { id: 'proposal1', tool: 'propose_editor_patch', status: 'pending', arguments: { revision, summary: 'Shorten CTA', operations: [{ op: 'replace_text', before: 'Shop today', after: 'Shop' }] } } },
        { id: 4, type: 'done', data: null },
      ];
      return new Response(events.map(event => `data: ${JSON.stringify(event)}\n\n`).join(''), { headers: { 'Content-Type': 'text/event-stream' } });
    }
    if (url.includes('/decision')) return Response.json({ data: { status: JSON.parse(options.body).approve ? 'approved' : 'discarded' } });
    return Response.json({});
  };
}
createApp({ render: () => h(VApp, {}, () => [h(VMain, {}, () => [h('div', { 'data-cy': 'fixture-body' }, snapshot.value.body), h('button', { onClick: () => router.push('/analytics') }, 'Navigate to analytics'), h('button', { onClick: window.assistantFixture.change }, 'Manual edit')]), h(AIAssistant)]) }).use(router).use(vuetify).mount('#app');
