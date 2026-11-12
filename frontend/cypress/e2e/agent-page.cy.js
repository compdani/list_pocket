// The Agent page: views/Agent.vue hosting <lp-agent-page> (the agent-panel package's
// <agent-page>). The page draws inside its shadow root.
const fixture = '/admin/cypress/fixtures/agent-page-harness.html';
const png = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==';

const conversations = [
  { id: 'c1', title: 'Spring launch', updated: '2026-11-10T09:00:00Z' },
  { id: 'c2', title: 'Inbox triage', updated: '2026-11-11T09:00:00Z' },
];
const spring = {
  id: 'c1',
  title: 'Spring launch',
  messages: [
    { id: 'm1', role: 'user', content: 'Draft the launch email', context: [{ type: 'campaign', id: '7', label: 'Spring sale' }] },
    {
      id: 'm2',
      role: 'assistant',
      content: 'Here is a **draft** for the launch.',
      parts: [
        { type: 'tool', name: 'campaigns' },
        { type: 'tool', name: 'lists', error: 'list not found' },
        { type: 'proposal', tool: 'create_campaign_draft', status: 'pending', arguments: { summary: 'Spring launch draft' } },
      ],
    },
  ],
};

function stubApi() {
  cy.intercept('GET', '**/mailapi/agent/conversations', { data: conversations }).as('list');
  cy.intercept('GET', '**/mailapi/agent/conversations/c1', { data: spring }).as('c1');
  cy.intercept('GET', '**/mailapi/agent/models*', { data: { providers: [] } });
  cy.intercept('GET', '**/mailapi/agent/mentions*', { data: [{ type: 'campaign', id: '7', label: 'Spring sale' }, { type: 'list', id: '3', label: 'Customers' }] }).as('mentions');
}

describe('Agent page', { includeShadowDom: true }, () => {
  beforeEach(() => {
    stubApi();
    cy.viewport(1280, 800);
  });

  it('lists conversations and opens one with its context, tools and proposals', () => {
    cy.visit(fixture);
    cy.get('.conv').should('have.length', 2).first().should('contain.text', 'Spring launch');
    cy.contains('.empty-title', 'What should we work on?').should('be.visible');
    cy.contains('.conv-link', 'Spring launch').click();
    cy.wait('@c1');
    cy.window().its('agentFixture.router.currentRoute.value.fullPath').should('eq', '/agent/c1');
    cy.get('.conv--active').should('contain.text', 'Spring launch');
    cy.get('.title').should('have.text', 'Spring launch');
    cy.get('.context-bar').should('be.visible').and('contain.text', 'In context:').and('contain.text', 'Spring sale');
    cy.get('.user-turn').within(() => {
      cy.get('[data-reference="campaign:7"]').should('be.visible');
      cy.get('[data-role=user]').should('have.text', 'Draft the launch email');
    });
    cy.get('.message strong').should('have.text', 'draft');
    cy.get('.tools').should('have.attr', 'data-status', 'failed').find('summary').first().should('contain.text', 'Used 2 tool(s)');
    cy.get('[data-cy=assistant-proposal]').should('contain.text', 'Spring launch draft').within(() => {
      cy.contains('button', 'Review').should('be.visible');
      cy.contains('button', 'Reject').should('be.visible');
    });

    cy.intercept('POST', '**/mailapi/agent/messages/m2/parts/2/apply', { data: { type: 'proposal', tool: 'create_campaign_draft', status: 'applied', arguments: { summary: 'Spring launch draft' }, result: { source: { href: '/admin/campaigns/9' } } } }).as('apply');
    cy.get('[data-cy=assistant-apply]').click();
    cy.wait('@apply');
    cy.get('[data-cy=assistant-proposal-status]').should('have.text', 'applied');
    cy.get('[data-cy=assistant-open-result]').should('have.attr', 'href', '/admin/campaigns/9');

    // Clicking a context chip previews it.
    cy.intercept('POST', '**/mailapi/agent/preview', { data: { type: 'campaign', id: '7', label: 'Spring sale', text: 'Campaign preview' } }).as('preview');
    cy.get('.context-bar [data-reference="campaign:7"] .chip-open').click();
    cy.wait('@preview');
  });

  it('attaches context with @ and pasted files, and uploads them when sending', () => {
    const turns = [];
    const uploads = [];
    cy.intercept('POST', '**/mailapi/agent/conversations', { data: { id: 'new1', title: 'New chat', updated: '2026-11-12T10:00:00Z' } }).as('create');
    cy.intercept('POST', '**/mailapi/agent/attachments', (req) => { uploads.push(req.body); req.reply({ data: { id: `f${uploads.length}`, name: req.body.name } }); }).as('upload');
    cy.intercept('POST', '**/mailapi/agent/conversations/new1/turns', (req) => { turns.push(req.body); req.reply({ data: { turn: 't1', title: 'Launch questions' } }); }).as('turn');
    cy.intercept('GET', '**/mailapi/ai/assistant/turns/t1/stream*', { headers: { 'Content-Type': 'text/event-stream' }, body: ['data: {"id":1,"type":"text_delta","data":{"text":"Looking"}}', 'data: {"id":2,"type":"done","data":null}'].join('\n\n') + '\n\n' });
    cy.intercept('GET', '**/mailapi/agent/conversations/new1', { data: { id: 'new1', title: 'Launch questions', messages: [{ id: 'u1', role: 'user', content: 'Compare these', context: [] }, { id: 'a1', role: 'assistant', content: 'Looking' }] } });
    cy.visit(fixture);

    cy.get('textarea').type('Compare @Spr');
    cy.wait('@mentions');
    cy.get('[data-cy=agent-context-picker]').should('be.visible');
    cy.get('textarea').type('{enter}');
    cy.get('.chips [data-reference="campaign:7"]').should('be.visible');
    cy.get('textarea').should('have.value', 'Compare ');
    cy.get('[data-cy=agent-context-picker]').should('not.exist');

    // The Context button opens the picker without typing @.
    cy.get('[data-cy=agent-context]').click();
    cy.get('[data-cy=agent-context-option]').eq(1).click();
    cy.get('.chips [data-reference="list:3"]').should('be.visible');

    cy.get('textarea').then(($input) => {
      const win = $input[0].ownerDocument.defaultView;
      const transfer = new win.DataTransfer();
      transfer.items.add(new win.File([Uint8Array.from(atob(png), (c) => c.charCodeAt(0))], 'image.png', { type: 'image/png' }));
      $input[0].dispatchEvent(new win.ClipboardEvent('paste', { clipboardData: transfer, bubbles: true, composed: true, cancelable: true }));
    });
    cy.get('[data-cy=agent-paste]').click();
    cy.get('[data-cy=agent-paste-name] input').type('leads.csv');
    cy.get('[data-cy=agent-paste-content] textarea').type('name,email');
    cy.get('[data-cy=agent-paste-attach]').click();
    cy.get('.chip[data-attachment-id]').should('have.length', 2);

    cy.get('textarea').type('these{enter}');
    cy.wait(['@create', '@upload', '@upload', '@turn']);
    cy.get('[data-role=user]').should('contain.text', 'Compare these');
    cy.get('.chips').should('not.be.visible');
    cy.then(() => {
      expect(uploads.map((u) => [u.mime, u.binary])).to.deep.eq([['image/png', true], ['text/plain', false]]);
      expect(uploads[0].content).to.eq(png);
      expect(uploads[0].name).to.match(/^pasted-image-\d{8}-\d{6}\.png$/);
      expect(uploads[1]).to.include({ name: 'leads.csv', content: 'name,email', conversation: 'new1' });
      expect(turns[0].message).to.eq('Compare these');
      expect(turns[0].context).to.deep.eq([
        { type: 'campaign', id: '7', label: 'Spring sale' },
        { type: 'list', id: '3', label: 'Customers' },
        { type: 'file', id: 'f1', label: uploads[0].name },
        { type: 'file', id: 'f2', label: 'leads.csv' },
      ]);
    });
    cy.window().its('agentFixture.router.currentRoute.value.fullPath').should('eq', '/agent/new1');
    cy.get('.title').should('have.text', 'Launch questions');
  });

  it('renames and deletes conversations', () => {
    cy.intercept('PATCH', '**/mailapi/agent/conversations/c1', (req) => req.reply({ data: { ...spring, title: req.body.title } })).as('rename');
    cy.intercept('DELETE', '**/mailapi/agent/conversations/c2', { data: true }).as('delete');
    cy.visit(`${fixture}?path=/agent/c1`);
    cy.wait('@c1');
    cy.get('.title').click();
    cy.get('.title-input').should('be.focused').clear().type('Spring launch v2{enter}');
    cy.wait('@rename').its('request.body').should('deep.eq', { title: 'Spring launch v2' });
    cy.get('.title').should('have.text', 'Spring launch v2');

    cy.get('[data-conversation-id=c2] .conv-del').click({ force: true });
    cy.get('[data-conversation-id=c2]').should('have.class', 'conv--confirm');
    cy.get('[data-conversation-id=c2] [data-action=confirm-delete]').click();
    cy.wait('@delete');
    cy.get('[data-conversation-id=c2]').should('not.exist');
  });

  it('puts the conversation list in a drawer on narrow screens', () => {
    cy.viewport(500, 800);
    cy.visit(fixture);
    cy.get('.page').then(($page) => {
      const page = $page[0].getBoundingClientRect();
      cy.get('.convs').should(($el) => expect($el[0].getBoundingClientRect().right).to.be.at.most(page.left + 1));
      // The chat takes the whole width once the list moves into a drawer.
      cy.get('.panel').should(($el) => expect($el[0].getBoundingClientRect().width).to.be.closeTo(page.width, 2));
    });
    cy.get('.history').should('be.visible').click();
    cy.get('.page').should('have.class', 'page--drawer');
    cy.contains('.conv-link', 'Inbox triage').should('be.visible');
    cy.intercept('GET', '**/mailapi/agent/conversations/c2', { data: { id: 'c2', title: 'Inbox triage', messages: [] } });
    cy.contains('.conv-link', 'Inbox triage').click();
    cy.get('.page').should('not.have.class', 'page--drawer');
    cy.get('.title').should('have.text', 'Inbox triage');
  });
});
