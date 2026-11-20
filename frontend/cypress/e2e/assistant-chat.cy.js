// The side panel on the module chat API: AIAssistantHost → AIAssistantChat,
// hosting <lp-agent-panel>, with /api/mcp/chat/v1 stubbed.
const fixture = '/admin/cypress/fixtures/assistant-chat-harness.html';

const conversation = (id, title, updatedAt = '2026-11-11T09:00:00Z') => ({
  id, title, createdAt: '2026-11-10T09:00:00Z', updatedAt,
});

const contextStatus = () => ({
  tokens: 1000, window: 128000, estimated: false, compacting: false, canCompact: false,
});

const statusBody = (over = {}) => ({
  version: '1',
  models: ['gpt-4.1', 'gpt-4.1-mini'],
  choices: [
    { id: 'api::gpt-4.1', provider: 'api', model: 'gpt-4.1', label: 'GPT-4.1', default: true },
    { id: 'api::gpt-4.1-mini', provider: 'api', model: 'gpt-4.1-mini', label: 'GPT-4.1 mini' },
  ],
  defaultChoice: 'api::gpt-4.1',
  features: { proposals: true, uploads: true, context: true, ready: true },
  limits: {
    maxMessageChars: 200000,
    maxUploadBytes: 6 * 1024 * 1024,
    maxAttachmentsPerTurn: 5,
    attachmentTypes: ['text/plain', 'image/png', 'application/pdf'],
  },
  ...over,
});

function userMessage(id, text, conversationId = 'c1', references = []) {
  return {
    id,
    conversationId,
    turnId: 't0',
    role: 'user',
    createdAt: '2026-11-11T09:00:00Z',
    parts: [
      { type: 'text', text },
      ...(references.length ? [{ type: 'context', result: { screen: 'panel', references } }] : []),
    ],
  };
}

function assistantMessage(id, text, conversationId = 'c1') {
  return {
    id,
    conversationId,
    turnId: 't0',
    role: 'assistant',
    createdAt: '2026-11-11T09:00:01Z',
    parts: [{ type: 'text', text }],
  };
}

const pendingProposal = (over = {}) => ({
  id: 'p1',
  conversationId: 'c1',
  turnId: 't0',
  callId: 'call1',
  tool: 'create_campaign_draft',
  location: 'server',
  state: 'pending',
  revision: 1,
  arguments: { summary: 'Spring launch draft', draft: { name: 'Spring', subject: 'Hello', content_type: 'plain', body: 'Hello' } },
  summary: 'Spring launch draft',
  warnings: [],
  createdAt: '2026-11-11T09:00:00Z',
  updatedAt: '2026-11-11T09:00:00Z',
  ...over,
});

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

function editorProposal(context, operations, editorId) {
  return {
    id: 'e1',
    conversationId: 'new1',
    turnId: 't1',
    tool: 'propose_editor_patch',
    location: 'editor',
    state: 'pending',
    revision: 1,
    arguments: {
      editorId: editorId || context.editor.key,
      baseRevision: context.editor.revision,
      summary: 'Edit',
      operations,
    },
    summary: 'Edit',
    warnings: [],
    createdAt: '2026-11-11T09:00:00Z',
    updatedAt: '2026-11-11T09:00:00Z',
  };
}

function stubList(rows = [conversation('c1', 'Spring launch'), conversation('c2', 'Inbox triage', '2026-11-12T09:00:00Z')]) {
  cy.intercept('GET', '**/api/mcp/chat/v1/status', statusBody()).as('status');
  cy.intercept('GET', '**/api/mcp/chat/v1/conversations', { items: rows, nextCursor: '' }).as('list');
}

function visit(query = '') {
  cy.visit(`${fixture}${query}`, { onBeforeLoad: (win) => win.localStorage.clear() });
}

describe('Side panel on the chat module', { includeShadowDom: true }, () => {
  beforeEach(() => cy.viewport(1280, 800));

  it('lists, opens, renames and deletes conversations, and starts a new chat', () => {
    const rows = [conversation('c1', 'Spring launch'), conversation('c2', 'Inbox triage', '2026-11-12T09:00:00Z')];
    stubList(rows);
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/c1', {
      conversation: conversation('c1', 'Spring launch'),
      messages: [userMessage('m1', 'Draft the launch'), assistantMessage('m2', 'Here is a draft.')],
      proposals: [],
      decidedProposals: [],
      activeTurn: null,
      context: contextStatus(),
    }).as('open1');
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/c2', {
      conversation: conversation('c2', 'Inbox triage'),
      messages: [userMessage('m3', 'Who is waiting?', 'c2')],
      proposals: [],
      activeTurn: null,
      context: contextStatus(),
    }).as('open2');
    cy.intercept('PATCH', '**/api/mcp/chat/v1/conversations/c1', (req) => {
      req.reply(conversation('c1', req.body.title));
    }).as('rename');
    const gone = new Set();
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations', (req) => {
      req.reply({ items: rows.filter((row) => !gone.has(row.id)), nextCursor: '' });
    }).as('list');
    cy.intercept('DELETE', '**/api/mcp/chat/v1/conversations/c2', (req) => {
      gone.add('c2');
      req.reply({});
    }).as('delete');

    visit('?editor=none');
    cy.wait(['@status', '@list']);
    cy.contains('Import previous').should('not.exist');
    cy.get('[part=switcher-button]').click();
    cy.get('[data-conversation-id=c1]').should('contain.text', 'Spring launch');
    cy.get('[data-conversation-id=c2]').should('contain.text', 'Inbox triage');
    cy.get('[data-conversation-id=c1] .conv-link').click();
    cy.wait('@open1');
    cy.contains('[data-role=user]', 'Draft the launch').should('be.visible');

    cy.get('[part=switcher-button]').click();
    cy.get('[data-cy=assistant-rename]').click();
    cy.get('[data-cy=assistant-rename-title]').clear().type('Spring launch v2');
    cy.get('[data-cy=assistant-rename-save]').click();
    cy.wait('@rename').its('request.body').should('deep.eq', { title: 'Spring launch v2' });
    cy.get('[part=switcher-button]').should('contain.text', 'Spring launch v2');

    cy.get('[part=switcher-button]').click();
    cy.get('[data-conversation-id=c2] .conv-del').click();
    cy.get('[data-conversation-id=c2] [data-action=confirm-delete]').click();
    cy.wait('@delete');
    cy.get('[part=switcher-button]').click();
    cy.get('[data-conversation-id=c2]').should('not.exist');

    cy.get('[part=switcher-new]').click({ force: true });
    cy.get('[part=switcher-button]').should('contain.text', 'New chat');
    cy.get('[data-role=user]').should('not.exist');
  });

  it('sends the panel context for the page at that moment and streams the reply', () => {
    const turns = [];
    let messages = [];
    stubList([conversation('c2', 'Other')]);
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations', (req) => {
      req.reply(conversation('new1', ''));
    }).as('create');
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations/new1/turns', (req) => {
      turns.push(req.body);
      messages = [
        userMessage('u1', req.body.text, 'new1', req.body.context?.references || []),
        assistantMessage('a1', 'Looking into it', 'new1'),
      ];
      req.reply({ turn: { id: 't1', conversationId: 'new1', state: 'running' }, created: true });
    }).as('turn');
    cy.intercept('GET', '**/api/mcp/chat/v1/turns/t1/events', sse([
      frame(1, 'turn.started'),
      frame(2, 'text.delta', { text: 'Looking' }),
      frame(3, 'text.delta', { text: ' into it' }),
      frame(4, 'tool.started', { callId: 'call1', tool: 'propose_editor_patch' }),
      frame(5, 'tool.completed', { callId: 'call1', tool: 'propose_editor_patch' }),
      frame(6, 'turn.completed'),
    ])).as('events');
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/new1', () => ({
      conversation: conversation('new1', 'Shorten'),
      messages,
      proposals: [],
      activeTurn: null,
      context: contextStatus(),
    })).as('opened');

    visit('?path=/campaigns/example?status=live');
    cy.wait(['@status', '@list']);
    cy.get('[data-cy=assistant-context]').should('contain.text', 'Unsaved campaign editor');
    cy.get('textarea').type('Shorten the CTA{enter}');
    cy.wait(['@create', '@turn', '@events']);
    cy.get('[data-role=assistant]').should('contain.text', 'Looking into it');
    cy.get('[data-tool-id=call1]').should('contain.text', 'Editor patch');
    cy.then(() => {
      expect(turns).to.have.length(1);
      expect(turns[0].context.screen).to.eq('panel');
      expect(turns[0].context.path).to.eq('/campaigns/example');
      expect(turns[0].context.page).to.eq('campaign');
      expect(turns[0].context.params).to.deep.eq({ id: 'example' });
      expect(turns[0].context.filters).to.deep.eq({ status: 'live' });
      expect(turns[0].context.editor.key).to.eq('campaign:example');
      expect(turns[0].context.editor.revision).to.be.a('string').and.not.eq('');
      expect(turns[0].context.editor.current.body).to.contain('Shop today');
      expect(turns[0].context.pictureReferences).to.deep.eq([]);
      expect(turns[0].context.references).to.deep.eq([]);
    });
  });

  it('remembers the model picked last', () => {
    stubList([]);
    visit('?editor=none');
    cy.wait('@status');
    cy.window().then((win) => {
      win.localStorage.setItem('listpocket_agent_model', JSON.stringify({ provider: 'api', model: 'gpt-4.1-mini' }));
    });
    cy.reload();
    cy.wait('@status');
    cy.get('.model-trigger').should('contain.text', 'GPT-4.1 mini');
  });

  it('approves a server proposal, shows the Open link and fires assistant-applied', () => {
    stubList();
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/c1', {
      conversation: conversation('c1', 'Spring launch'),
      messages: [assistantMessage('m1', 'I can draft that.')],
      proposals: [pendingProposal()],
      activeTurn: null,
      context: contextStatus(),
    }).as('conversation');
    cy.intercept('POST', '**/api/mcp/chat/v1/proposals/p1/decision', {
      proposal: pendingProposal({
        state: 'succeeded',
        revision: 2,
        result: { source: { href: '/admin/campaigns/9' } },
      }),
    }).as('decision');

    visit('?editor=none');
    cy.window().then((win) => {
      win.__applied = [];
      win.addEventListener('assistant-applied', (event) => win.__applied.push(event.detail));
    });
    cy.get('[part=switcher-button]').click();
    cy.get('[data-conversation-id=c1] .conv-link').click();
    cy.wait('@conversation');
    cy.get('[data-cy=assistant-apply]').click();
    cy.wait('@decision');
    cy.get('[data-cy=assistant-open-result]').should('have.attr', 'href', '/admin/campaigns/9');
    cy.window().its('__applied').should('deep.eq', [{ tool: 'create_campaign_draft', result: { source: { href: '/admin/campaigns/9' } } }]);
  });

  function stubEditorTurn(operations, editorId) {
    const turns = [];
    let proposal = null;
    stubList([conversation('c2', 'Other')]);
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations', conversation('new1', '')).as('create');
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations/new1/turns', (req) => {
      turns.push(req.body);
      proposal = editorProposal(req.body.context, operations, editorId);
      req.reply({ turn: { id: 't1', conversationId: 'new1', state: 'running' }, created: true });
    }).as('turn');
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/new1*', (req) => {
      req.reply({
        conversation: conversation('new1', 'Edit'),
        messages: proposal ? [userMessage('u1', 'Edit this', 'new1'), assistantMessage('a1', 'Proposed.', 'new1')] : [],
        proposals: proposal ? [proposal] : [],
        activeTurn: null,
        context: contextStatus(),
      });
    });
    cy.intercept('GET', '**/api/mcp/chat/v1/turns/t1/events', (req) => {
      req.reply(sse([
        frame(1, 'turn.started'),
        frame(2, 'text.delta', { text: 'Proposed.' }),
        frame(3, 'proposal.created', { proposal }),
        frame(4, 'turn.completed'),
      ]));
    }).as('events');
    cy.intercept('POST', '**/api/mcp/chat/v1/proposals/e1/decision', (req) => {
      req.reply({ proposal: { ...proposal, state: 'approved', revision: 2 } });
    }).as('decision');
    cy.intercept('POST', '**/api/mcp/chat/v1/proposals/e1/outcome', (req) => {
      req.reply({
        proposal: {
          ...proposal,
          state: req.body.status,
          revision: 3,
          failure: req.body.message || '',
          editorRevision: req.body.editorRevision,
        },
      });
    }).as('outcome');
    return turns;
  }

  it('applies a campaign replace_text, then undoes it', () => {
    stubEditorTurn([{ op: 'replace_text', before: 'Shop today', after: 'Shop' }]);
    visit();
    cy.wait(['@status', '@list']);
    cy.get('textarea').type('Edit this{enter}');
    cy.wait(['@create', '@turn', '@events']);
    cy.get('[data-cy=assistant-apply]').click();
    cy.wait(['@decision', '@outcome']);
    cy.get('[data-cy=fixture-body]').should('contain.text', '<a>Shop</a>');
    cy.get('[data-cy=assistant-undo]').click();
    cy.get('[data-cy=fixture-body]').should('contain.text', 'Shop today');
    cy.get('[data-cy=assistant-undo]').should('not.exist');
  });

  it('applies a template set and a visual block', () => {
    stubEditorTurn([{ op: 'set', field: 'subject', before: 'Hello', after: 'Hi' }]);
    visit('?editor=template');
    cy.wait(['@status', '@list']);
    cy.get('textarea').type('Edit this{enter}');
    cy.wait(['@events']);
    cy.get('[data-cy=assistant-apply]').click();
    cy.wait('@outcome');
    cy.get('[data-cy=fixture-body]').should('contain.text', '"subject":"Hi"');
  });

  it('applies a visual block on a campaign editor', () => {
    const before = JSON.stringify({ type: 'Button', text: 'Shop today' });
    const after = JSON.stringify({ type: 'Button', text: 'Shop' });
    stubEditorTurn([{ op: 'block', blockId: 'cta', before, after }]);
    visit('?editor=visual');
    cy.wait(['@status', '@list']);
    cy.get('textarea').type('Edit this{enter}');
    cy.wait('@events');
    cy.get('[data-cy=assistant-apply]').click();
    cy.wait('@outcome');
    cy.get('[data-cy=fixture-body]').should('contain.text', 'Shop');
    cy.get('[data-cy=fixture-body]').should('not.contain.text', 'Shop today');
  });

  it('applies a workflow node and edge edit', () => {
    stubEditorTurn([
      { op: 'node_upsert', id: 'wait', value: { id: 'wait', type: 'wait', config: { days: 2 } } },
      { op: 'edge_remove', id: 'e1' },
    ]);
    visit('?editor=workflow');
    cy.wait(['@status', '@list']);
    cy.get('[data-cy=assistant-context]').should('contain.text', 'Unsaved workflow editor');
    cy.get('textarea').type('Edit this{enter}');
    cy.wait('@events');
    cy.get('[data-cy=assistant-apply]').click();
    cy.wait('@outcome');
    cy.get('[data-cy=fixture-body]').should('contain.text', '"days":2').and('contain.text', '"edges":[]');
  });

  it('reports a revision mismatch as stale and refuses the wrong editor', () => {
    stubEditorTurn([{ op: 'replace_text', before: 'Shop today', after: 'Shop' }]);
    visit();
    cy.wait(['@status', '@list']);
    cy.get('textarea').type('Edit this{enter}');
    cy.wait('@events');
    cy.window().then((win) => win.assistantFixture.change());
    cy.get('[data-cy=assistant-apply]').click();
    cy.wait('@outcome').its('request.body.status').should('eq', 'stale');
    cy.contains('Not applied: the editor changed').should('be.visible');
    cy.get('[data-cy=fixture-body]').should('contain.text', 'Shop today');
  });

  it('refuses a patch for a different editor without closing it, and applies once that editor is open again', () => {
    stubEditorTurn([{ op: 'replace_text', before: 'Shop today', after: 'Shop' }]);
    visit();
    cy.wait(['@status', '@list']);
    cy.get('textarea').type('Edit this{enter}');
    cy.wait('@events');
    cy.window().then((win) => win.assistantFixture.useEditor('template'));
    cy.get('[data-cy=assistant-apply]').click();
    cy.get('[data-cy=assistant-action-error]').should('contain.text', 'Open the original editor before applying this proposal.');
    cy.get('[data-cy=fixture-body]').should('contain.text', '"subject":"Hello"');
    cy.get('[data-cy=assistant-apply]').should('be.visible');

    cy.window().then((win) => win.assistantFixture.useEditor('campaign'));
    cy.get('[data-cy=assistant-apply]').click();
    cy.wait(['@decision', '@outcome']).then(([, outcome]) => {
      expect(outcome.request.body.status).to.eq('applied');
    });
    cy.get('[data-cy=fixture-body]').should('contain.text', '<a>Shop</a>');
  });

  it('refuses undo after a further edit', () => {
    stubEditorTurn([{ op: 'replace_text', before: 'Shop today', after: 'Shop' }]);
    visit();
    cy.wait(['@status', '@list']);
    cy.get('textarea').type('Edit this{enter}');
    cy.wait('@events');
    cy.get('[data-cy=assistant-apply]').click();
    cy.wait('@outcome');
    cy.get('[data-cy=assistant-undo]').should('be.visible');
    cy.window().then((win) => win.assistantFixture.change());
    cy.get('[data-cy=assistant-undo]').click();
    cy.get('[data-cy=assistant-action-error]').should('contain.text', 'The editor changed after this edit; use its undo controls.');
    cy.get('[data-cy=assistant-undo]').should('not.exist');
    cy.get('[data-cy=fixture-body]').should('contain.text', 'new edit');
  });

  it('keeps a media picture on the next turn and after the conversation is reopened', () => {
    const turns = [];
    let rows = [conversation('c2', 'Other')];
    stubList(rows);
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations', (req) => {
      req.reply({ items: rows, nextCursor: '' });
    }).as('list');
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/c2', {
      conversation: conversation('c2', 'Other'),
      messages: [],
      proposals: [],
      activeTurn: null,
      context: contextStatus(),
    }).as('other');
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations', (req) => {
      rows = [conversation('new1', 'Pictures'), ...rows];
      req.reply(conversation('new1', ''));
    }).as('create');
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations/new1/turns', (req) => {
      turns.push(req.body);
      req.reply({ turn: { id: `t${turns.length}`, conversationId: 'new1', state: 'completed' }, created: true });
    }).as('turn');
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/new1', {
      conversation: conversation('new1', 'Pictures'),
      messages: [],
      proposals: [],
      activeTurn: null,
      context: contextStatus(),
    }).as('opened');

    visit('?editor=none');
    cy.wait(['@status', '@list']);
    cy.get('[data-cy=assistant-add-picture]').click();
    cy.get('.thumb-link').first().click();
    cy.get('[data-cy=assistant-picture]').should('contain.text', 'hero.png');
    cy.get('textarea').type('Use this picture{enter}');
    cy.wait('@turn');
    cy.get('[data-cy=assistant-picture]').should('contain.text', 'hero.png');
    cy.then(() => {
      expect(turns[0].context.pictureReferences).to.deep.eq([{ name: 'hero.png', url: 'https://cdn.example/hero.png' }]);
    });
    cy.get('[part=switcher-button]').click();
    cy.get('[data-conversation-id=c2] .conv-link').click();
    cy.wait('@other');
    cy.get('[data-cy=assistant-picture]').should('not.exist');
    cy.get('[part=switcher-button]').click();
    cy.get('[data-conversation-id=new1] .conv-link').click();
    cy.wait('@opened');
    cy.get('[data-cy=assistant-picture]').should('contain.text', 'hero.png');
    cy.get('textarea').type('And again{enter}');
    cy.wait('@turn');
    cy.then(() => {
      expect(turns[1].context.pictureReferences).to.deep.eq([{ name: 'hero.png', url: 'https://cdn.example/hero.png' }]);
    });
  });

  it('explains that AI is not set up and disables sending', () => {
    cy.intercept('GET', '**/api/mcp/chat/v1/status', statusBody({
      models: [],
      choices: [],
      defaultChoice: '',
      features: { proposals: true, uploads: true, context: true, ready: false },
    })).as('status');
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations*', { items: [], nextCursor: '' }).as('list');
    visit('?editor=none');
    cy.wait('@status');
    cy.get('[part=error]').should('be.visible').and('contain.text', "AI isn't set up");
    cy.get('[part=send]').should('be.disabled');
  });

  it('blocks a second send while the first has not been accepted', () => {
    let creates = 0;
    let releaseCreate = null;
    const turns = [];
    stubList([]);
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations', (req) => {
      creates += 1;
      if (creates > 1) {
        req.reply({ statusCode: 500, body: { error: { code: 'internal', message: 'second create' } } });
        return undefined;
      }
      return new Promise((resolve) => {
        releaseCreate = () => {
          req.reply(conversation('new1', ''));
          resolve();
        };
      });
    }).as('create');
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations/new1/turns', (req) => {
      turns.push(req.body);
      req.reply({ turn: { id: 't1', conversationId: 'new1', state: 'completed' }, created: true });
    }).as('turn');
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/new1', {
      conversation: conversation('new1', 'One'),
      messages: [userMessage('u1', 'One', 'new1')],
      proposals: [],
      activeTurn: null,
      context: contextStatus(),
    });

    visit('?editor=none');
    cy.wait('@status');
    cy.get('textarea').type('One{enter}');
    cy.get('textarea').should('have.value', '');
    cy.get('textarea').type('Two{enter}');
    cy.get('textarea').should('have.value', 'Two');
    cy.wrap(null).should(() => {
      expect(creates).to.eq(1);
      expect(turns).to.have.length(0);
    });
    cy.then(() => releaseCreate());
    cy.wait('@turn');
    cy.then(() => {
      expect(turns).to.have.length(1);
      expect(turns[0].text).to.eq('One');
      expect(turns[0].context.screen).to.eq('panel');
    });
    cy.get('textarea').should('have.value', 'Two');
  });

  it('restores the draft when a turn is refused', () => {
    let fail = true;
    stubList();
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/c1', {
      conversation: conversation('c1', 'Spring launch'),
      messages: [],
      proposals: [],
      activeTurn: null,
      context: contextStatus(),
    }).as('conversation');
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations/c1/turns', (req) => {
      if (fail) {
        req.reply({ statusCode: 400, body: { error: { code: 'turn_refused', message: 'Too much context.' } } });
        return;
      }
      req.reply({ turn: { id: 't1', conversationId: 'c1', state: 'completed' }, created: true });
    }).as('turn');

    visit('?editor=none');
    cy.get('[part=switcher-button]').click();
    cy.get('[data-conversation-id=c1] .conv-link').click();
    cy.wait('@conversation');
    cy.get('textarea').type('Keep this{enter}');
    cy.wait('@turn');
    cy.get('textarea').should('have.value', 'Keep this');
    cy.then(() => { fail = false; });
    cy.get('textarea').type('{enter}');
    cy.wait('@turn');
    cy.get('textarea').should('have.value', '');
  });

  it('keeps the composer disabled until status answers, then offers retry after a failed turn', () => {
    let releaseStatus = null;
    cy.intercept('GET', '**/api/mcp/chat/v1/status', (req) => new Promise((resolve) => {
      releaseStatus = () => {
        req.reply(statusBody());
        resolve();
      };
    })).as('status');
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations', { items: [], nextCursor: '' }).as('list');
    visit('?editor=none');
    cy.get('lp-agent-panel').should('have.attr', 'disabled');
    cy.get('textarea').should('be.disabled');
    cy.get('[part=error]').should('not.be.visible');
    cy.wrap(null).should(() => {
      expect(releaseStatus).to.be.a('function');
    });
    cy.then(() => releaseStatus());
    cy.wait('@status');
    cy.get('textarea').should('not.be.disabled');

    cy.intercept('POST', '**/api/mcp/chat/v1/conversations', conversation('new1', '')).as('create');
    cy.intercept('GET', '**/api/mcp/chat/v1/turns/t1/events', sse([
      frame(1, 'turn.started'),
      frame(2, 'turn.failed', { code: 'provider_error', message: 'The provider refused the key.' }),
    ])).as('events');
    cy.intercept('GET', '**/api/mcp/chat/v1/conversations/new1', {
      conversation: conversation('new1', 'Hello'),
      messages: [userMessage('u1', 'Hello there', 'new1')],
      proposals: [],
      activeTurn: null,
      context: contextStatus(),
    });
    const retried = [];
    cy.intercept('POST', '**/api/mcp/chat/v1/conversations/new1/turns', (req) => {
      retried.push(req.body);
      if (retried.length === 1) {
        req.reply({ turn: { id: 't1', conversationId: 'new1', state: 'running' }, created: true });
        return;
      }
      req.reply({ turn: { id: 't2', conversationId: 'new1', state: 'completed' }, created: true });
    });
    cy.get('lp-agent-panel').then(($el) => { $el[0].model = 'api::gpt-4.1-mini'; });
    cy.get('textarea').type('Hello there{enter}');
    cy.wait('@events');
    cy.get('[data-cy=assistant-retry]').click();
    cy.wrap(null).should(() => {
      expect(retried.length).to.be.greaterThan(1);
      expect(retried[1].text).to.eq('Hello there');
      expect(retried[1].model).to.eq('gpt-4.1-mini');
    });
  });
});
