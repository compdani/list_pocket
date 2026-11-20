// The Agent page on the module chat API: views/AgentRoute.vue → views/AgentChat.vue
// hosting <lp-agent-page>, with /api/mcp/chat/v1 stubbed. The page draws inside the
// element's shadow root.
const fixture = '/admin/cypress/fixtures/agent-page-chat-harness.html';

const conversation = (id, title, updatedAt = '2026-11-11T09:00:00Z') => ({
  id, title, createdAt: '2026-11-10T09:00:00Z', updatedAt,
});

const conversations = [
  conversation('c1', 'Spring launch'),
  conversation('c2', 'Inbox triage', '2026-11-12T09:00:00Z'),
];

const contextStatus = () => ({
  tokens: 1000, window: 128000, estimated: false, compacting: false, canCompact: false,
});

const statusBody = (over = {}) => ({
  version: '1',
  models: ['gpt-4.1'],
  choices: [{
    id: 'api::gpt-4.1', provider: 'api', model: 'gpt-4.1', label: 'GPT-4.1', default: true,
  }],
  defaultChoice: 'api::gpt-4.1',
  features: { proposals: true, uploads: true, context: true },
  limits: {
    maxMessageChars: 200000,
    maxUploadBytes: 6 * 1024 * 1024,
    maxAttachmentsPerTurn: 5,
    attachmentTypes: ['text/plain', 'image/png', 'application/pdf'],
  },
  ...over,
});

const userMessage = (id, text, references = []) => ({
  id,
  conversationId: 'c1',
  turnId: 't0',
  role: 'user',
  createdAt: '2026-11-11T09:00:00Z',
  parts: [
    { type: 'text', text },
    ...(references.length > 0
      ? [{ type: 'context', result: { screen: 'agent', path: '/agent/c1', references } }]
      : []),
  ],
});

const assistantMessage = (id, text, parts = []) => ({
  id,
  conversationId: 'c1',
  turnId: 't0',
  role: 'assistant',
  createdAt: '2026-11-11T09:00:01Z',
  parts: [{ type: 'text', text }, ...parts],
});

const pendingProposal = (over = {}) => ({
  id: 'p1',
  conversationId: 'c1',
  turnId: 't0',
  callId: 'call1',
  tool: 'create_campaign_draft',
  location: 'server',
  state: 'pending',
  revision: 1,
  arguments: {
    summary: 'Spring launch draft',
    draft: {
      name: 'Spring', subject: 'Hello', content_type: 'plain', body: 'Hello',
    },
  },
  summary: 'Spring launch draft',
  warnings: ['This creates a campaign draft.'],
  createdAt: '2026-11-11T09:00:00Z',
  updatedAt: '2026-11-11T09:00:00Z',
  ...over,
});

// A static SSE body with the frames the reducer expects (sequence from 1).
function sse(events) {
  return {
    statusCode: 200,
    headers: { 'Content-Type': 'text/event-stream' },
    body: events.map((event) => `data: ${JSON.stringify(event)}\n\n`).join(''),
  };
}

function frame(seq, type, data, turnId = 't1') {
  return { v: '1', seq, turnId, type, ...(data === undefined ? {} : { data }) };
}

function stubPage({
  status = statusBody(),
  conversations: rows = conversations,
  detail,
} = {}) {
  cy.intercept('GET', '**/api/mcp/chat/v1/status', status).as('status');
  cy.intercept('GET', '**/api/mcp/chat/v1/conversations', { items: rows, nextCursor: '' }).as('list');
  if (detail) {
    cy.intercept('GET', `**/api/mcp/chat/v1/conversations/${detail.id}`, {
      conversation: conversation(detail.id, detail.title),
      messages: detail.messages || [],
      proposals: detail.proposals || [],
      activeTurn: null,
      context: contextStatus(),
    }).as('conversation');
  }
}

describe('Agent page on the chat module', { includeShadowDom: true }, () => {
  beforeEach(() => cy.viewport(1280, 800));

  it('loads the list and opens a conversation from the URL', () => {
    stubPage({
      detail: {
        id: 'c1',
        title: 'Spring launch',
        messages: [
          userMessage('m1', 'Draft the launch email', [{ type: 'campaign', id: '7', label: 'Spring sale' }]),
          assistantMessage('m2', 'Here is a **draft** for the launch.', [
            { type: 'tool_call', callId: 'tool1', tool: 'campaigns' },
          ]),
          {
            id: 'm3',
            conversationId: 'c1',
            turnId: 't0',
            role: 'tool',
            createdAt: '2026-11-11T09:00:02Z',
            parts: [{ type: 'tool_result', callId: 'tool1', result: { ok: true } }],
          },
          {
            id: 'n1',
            conversationId: 'c1',
            turnId: 't0',
            role: 'notice',
            createdAt: '2026-11-11T09:00:03Z',
            parts: [{
              type: 'proposal_outcome', proposalId: 'p0', state: 'rejected', tool: 'propose_reply', text: 'The reply was not sent.',
            }],
          },
        ],
        proposals: [pendingProposal()],
      },
    });
    cy.intercept('POST', '**/mailapi/agent/preview', { data: { title: 'Spring sale', data: { id: '7' } } }).as('preview');
    cy.intercept('GET', '**/mailapi/agent/mentions*', { data: [{ type: 'list', id: '3', label: 'Customers' }] }).as('mentions');

    cy.visit(`${fixture}?path=${encodeURIComponent('/agent/c1?ref=list&id=3&label=Customers')}`);
    cy.wait(['@status', '@list', '@conversation']);

    // ?ref= pre-attaches a composer chip on mount.
    cy.get('.chips [data-reference="list:3"]').should('be.visible');
    // The Context button opens the @ picker without typing.
    cy.get('[data-cy=agent-context]').click();
    cy.wait('@mentions');
    cy.get('[data-cy=agent-context-picker]').should('be.visible').and('contain.text', 'Customers');
    cy.get('[data-cy=agent-context]').click();

    cy.get('.conv').should('have.length', 2).first().should('contain.text', 'Spring launch');
    cy.get('.conv--active').should('contain.text', 'Spring launch');
    cy.get('.title').should('have.text', 'Spring launch');
    cy.get('.context-bar').should('be.visible').and('contain.text', 'In context:').and('contain.text', 'Spring sale');
    cy.get('.user-turn').within(() => {
      cy.get('[data-reference="campaign:7"]').should('be.visible');
      cy.get('[data-role=user]').should('have.text', 'Draft the launch email');
    });
    cy.get('.message strong').should('have.text', 'draft');
    cy.get('.tools').should('have.attr', 'data-status', 'completed');
    cy.get('.notice').should('contain.text', 'The reply was not sent.');
    cy.get('[data-cy=assistant-proposal]').should('contain.text', 'Spring launch draft').within(() => {
      cy.contains('button', 'Review').should('be.visible');
      cy.contains('button', 'Reject').should('be.visible');
    });

    // A reference chip opens the preview modal; an attachment chip would download.
    cy.get('.context-bar [data-reference="campaign:7"] .chip-open').click();
    cy.wait('@preview');
    cy.get('[data-cy=agent-context-preview]').should('be.visible');
  });

  it('sends this message\'s @ references as the page context and shows them after reload', () => {
    const turns = [];
    let messages = [];
    stubPage();
    cy.intercept('GET', '**/mailapi/agent/mentions*', { data: [{ type: 'campaign', id: '7', label: 'Spring sale' }] }).as('mentions');
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations', conversation('new1', '')).as('create');
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations/new1/turns', (req) => {
      turns.push(req.body);
      messages = [
        userMessage('u1', req.body.text, [{ type: 'campaign', id: '7', label: 'Spring sale' }]),
        assistantMessage('a1', 'Comparing the two.'),
      ];
      req.reply({ turn: { id: 't1', conversationId: 'new1', state: 'completed' }, created: true });
    }).as('turn');
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/new1', (req) => req.reply({
      conversation: conversation('new1', 'Compare these'),
      messages,
      proposals: [],
      activeTurn: null,
      context: contextStatus(),
    })).as('newConversation');

    cy.visit(fixture);
    cy.wait(['@status', '@list']);

    cy.get('textarea').type('Compare @Spr');
    cy.wait('@mentions');
    cy.get('[data-cy=agent-context-picker]').should('be.visible');
    cy.get('textarea').type('{enter}');
    cy.get('.chips [data-reference="campaign:7"]').should('be.visible');

    cy.get('textarea').type('these{enter}');
    cy.wait(['@create', '@turn', '@newConversation']);
    cy.then(() => {
      expect(turns[0].text).to.eq('Compare these');
      expect(turns[0].context.screen).to.eq('agent');
      expect(turns[0].context.references).to.deep.eq([{ type: 'campaign', id: '7', label: 'Spring sale' }]);
    });
    cy.get('.user-turn [data-reference="campaign:7"]').should('be.visible');
    cy.get('.user-turn [data-role=user]').should('have.text', 'Compare these');
    // The composer's chips are cleared once the turn is accepted.
    cy.get('[part=attachments] [data-reference="campaign:7"]').should('not.exist');
    cy.window().its('agentFixture.router.currentRoute.value.fullPath').should('eq', '/agent/new1');
  });

  it('streams a reply and replaces it with the stored transcript', () => {
    stubPage();
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations', conversation('new1', '')).as('create');
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations/new1/turns', {
      turn: { id: 't1', conversationId: 'new1', state: 'running' }, created: true,
    }).as('turn');
    cy.intercept('GET', '**/api/mcp/chat/v1/turns/t1/events', sse([
      frame(1, 'turn.started'),
      frame(2, 'text.delta', { text: 'Looking' }),
      frame(3, 'text.delta', { text: ' into it' }),
      frame(4, 'tool.started', { callId: 'call1', tool: 'campaigns' }),
      frame(5, 'tool.completed', { callId: 'call1', tool: 'campaigns', result: { ok: true } }),
      frame(6, 'turn.completed'),
    ])).as('events');
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/new1', {
      conversation: conversation('new1', 'Hello'),
      messages: [userMessage('u1', 'Hello'), assistantMessage('a1', 'Looking into it')],
      proposals: [],
      activeTurn: null,
      context: contextStatus(),
    }).as('newConversation');

    cy.visit(fixture);
    cy.wait(['@status', '@list']);
    cy.get('textarea').type('Hello{enter}');
    cy.wait(['@create', '@turn', '@events', '@newConversation']);
    cy.get('[data-role=assistant]').should('contain.text', 'Looking into it');
    cy.get('[data-cy=assistant-proposal]').should('not.exist');
  });

  it('approves a pending proposal at its revision and shows the applied card with Open', () => {
    stubPage({
      detail: {
        id: 'c1',
        title: 'Spring launch',
        messages: [assistantMessage('m1', 'I can draft that.')],
        proposals: [pendingProposal()],
      },
    });
    cy.intercept('POST', '**/api/mcp/chat/v1/proposals/p1/decision', (req) => {
      req.reply({
        proposal: pendingProposal({
          state: 'succeeded',
          revision: 2,
          result: { source: { href: '/admin/campaigns/9' } },
        }),
      });
    }).as('decision');

    cy.visit(`${fixture}?path=/agent/c1`);
    cy.wait(['@status', '@list', '@conversation']);
    cy.get('[data-cy=assistant-apply]').click();
    cy.wait('@decision').its('request.body').should('deep.eq', { decision: 'approve', revision: 1 });
    cy.get('[data-cy=assistant-proposal-status]').should('have.text', 'Applied');
    cy.get('[data-cy=assistant-open-result]').should('have.attr', 'href', '/admin/campaigns/9');
    cy.get('[data-cy=assistant-apply]').should('not.exist');
  });

  it('reviews a proposal and sends the edited arguments with the approval', () => {
    stubPage({
      detail: {
        id: 'c1',
        title: 'Spring launch',
        messages: [assistantMessage('m1', 'I can draft that.')],
        proposals: [pendingProposal()],
      },
    });
    cy.intercept('POST', '**/api/mcp/chat/v1/proposals/p1/decision', (req) => req.reply({
      proposal: pendingProposal({ state: 'succeeded', revision: 2, arguments: req.body.arguments, result: { source: { href: '/admin/campaigns/9' } } }),
    })).as('decision');

    cy.visit(`${fixture}?path=/agent/c1`);
    cy.wait(['@status', '@list', '@conversation']);
    cy.get('[data-cy=assistant-review]').click();
    cy.get('[data-cy=agent-review]').should('be.visible');
    cy.get('[data-cy=agent-review] input').first().clear().type('Spring v2');
    cy.get('[data-cy=agent-review-apply]').click();
    cy.wait('@decision').then(({ request }) => {
      expect(request.body.decision).to.eq('approve');
      expect(request.body.revision).to.eq(1);
      expect(request.body.arguments.draft.name).to.eq('Spring v2');
      expect(request.body.arguments.summary).to.eq('Spring launch draft');
    });
    cy.get('[data-cy=agent-review]').should('not.exist');
    cy.get('[data-cy=assistant-open-result]').should('have.attr', 'href', '/admin/campaigns/9');
  });

  it('keeps a refused decision pending with its message', () => {
    stubPage({
      detail: {
        id: 'c1',
        title: 'Spring launch',
        messages: [assistantMessage('m1', 'I can draft that.')],
        proposals: [pendingProposal()],
      },
    });
    cy.intercept('POST', '**/api/mcp/chat/v1/proposals/p1/decision', {
      statusCode: 422,
      body: {
        error: { code: 'refused', message: 'The list was deleted.' },
        proposal: pendingProposal({ revision: 2 }),
      },
    }).as('decision');

    cy.visit(`${fixture}?path=/agent/c1`);
    cy.wait(['@status', '@list', '@conversation']);
    cy.get('[data-cy=assistant-apply]').click();
    cy.wait('@decision');
    cy.get('[data-cy=assistant-apply]').should('be.visible');
    cy.get('[data-cy=assistant-proposal] .proposal-error')
      .should('contain.text', 'The list was deleted.')
      .and('contain.text', 'waiting for your decision again');
    cy.get('[data-cy=assistant-proposal-status]').should('not.exist');
  });

  it('explains that AI is not set up instead of a generic error and disables sending', () => {
    stubPage({ status: statusBody({ models: [], choices: [], defaultChoice: '' }) });
    cy.visit(fixture);
    cy.wait(['@status', '@list']);
    cy.get('[data-cy=agent-no-provider]').should('be.visible').and('contain.text', "AI isn't set up");
    cy.get('[part=send]').should('be.disabled');
    cy.get('.empty-title').should('be.visible');
  });

  it('keeps the URL in step with rename and delete', () => {
    stubPage({
      detail: {
        id: 'c1', title: 'Spring launch', messages: [], proposals: [],
      },
    });
    cy.intercept('PATCH', '**/api/mcp/chat/v1/conversations/c1', (req) => req.reply(conversation('c1', req.body.title))).as('rename');
    cy.intercept('DELETE', '**/api/mcp/chat/v1/conversations/c2', {}).as('deleteOther');
    cy.intercept('DELETE', '**/api/mcp/chat/v1/conversations/c1', {}).as('deleteOpen');

    cy.visit(`${fixture}?path=/agent/c1`);
    cy.wait(['@status', '@list', '@conversation']);
    cy.get('.title').click();
    cy.get('.title-input').should('be.focused').clear().type('Spring launch v2{enter}');
    cy.wait('@rename').its('request.body').should('deep.eq', { title: 'Spring launch v2' });
    cy.get('.title').should('have.text', 'Spring launch v2');
    cy.window().its('agentFixture.router.currentRoute.value.fullPath').should('eq', '/agent/c1');

    cy.get('[data-conversation-id=c2] .conv-del').click({ force: true });
    cy.get('[data-conversation-id=c2] [data-action=confirm-delete]').click();
    cy.wait('@deleteOther');
    cy.get('[data-conversation-id=c2]').should('not.exist');
    cy.window().its('agentFixture.router.currentRoute.value.fullPath').should('eq', '/agent/c1');

    cy.get('[data-conversation-id=c1] .conv-del').click({ force: true });
    cy.get('[data-conversation-id=c1] [data-action=confirm-delete]').click();
    cy.wait('@deleteOpen');
    cy.window().its('agentFixture.router.currentRoute.value.fullPath').should('eq', '/agent');
  });

  it('adds pasted data as a text file and uploads it with the turn', () => {
    stubPage();
    const turns = [];
    let messages = [];
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/c1', (req) => req.reply({
      conversation: conversation('c1', 'Spring launch'),
      messages,
      proposals: [],
      activeTurn: null,
      context: contextStatus(),
    })).as('conversation');
    cy.intercept('POST', '**/api/mcp/chat/v1/uploads', (req) => {
      expect(req.headers['content-type']).to.match(/multipart\/form-data/);
      req.reply({
        upload: {
          id: 'u1', conversationId: 'c1', name: 'leads.csv', mime: 'text/plain', kind: 'text', size: 10,
        },
      });
    }).as('upload');
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations/c1/turns', (req) => {
      turns.push(req.body);
      messages = [{
        id: 'u1',
        conversationId: 'c1',
        turnId: 't1',
        role: 'user',
        createdAt: '2026-11-11T09:00:00Z',
        parts: [
          { type: 'text', text: 'these' },
          { type: 'attachment', uploadId: 'u1', name: 'leads.csv', mime: 'text/plain' },
        ],
      }];
      req.reply({ turn: { id: 't1', conversationId: 'c1', state: 'completed' }, created: true });
    }).as('turn');

    cy.visit(`${fixture}?path=/agent/c1`);
    cy.wait(['@status', '@list', '@conversation']);
    cy.get('[data-cy=agent-paste]').click();
    cy.get('[data-cy=agent-paste-name] input').type('leads.csv');
    cy.get('[data-cy=agent-paste-content] textarea').type('name,email');
    cy.get('[data-cy=agent-paste-attach]').click();
    cy.get('.chip[data-attachment-id]').should('have.length', 1);

    cy.get('textarea').type('these{enter}');
    cy.wait(['@upload', '@turn']);
    cy.then(() => {
      expect(turns[0].text).to.eq('these');
      expect(turns[0].attachments).to.deep.eq(['u1']);
    });
    // The stored attachment chip stays on the user turn and still downloads.
    cy.get('.user-turn [data-reference="attachment:u1"]').should('contain.text', 'leads.csv');
    cy.get('[part=attachments] .chip[data-attachment-id]').should('not.exist');
  });
});
