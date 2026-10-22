const fixture = '/admin/cypress/fixtures/assistant-harness.html';
function setupTurn() {
  let revision;
  cy.intercept('POST', '**/mailapi/ai/assistant/turns', req => {
    revision = req.body.context.editor.revision;
    expect(req.body.context.path).to.eq('/campaigns/example');
    req.reply({ data: { id: 'test-turn' } });
  });
  cy.intercept('GET', '**/mailapi/ai/assistant/turns/test-turn/stream*', req => {
    const events = [
      { id: 1, type: 'text_delta', data: { text: '**Updated** CTA proposal' } },
      { id: 2, type: 'tool', data: { id: 'tool1', name: 'propose_editor_patch', status: 'completed' } },
      { id: 3, type: 'proposal', data: { id: 'proposal1', tool: 'propose_editor_patch', status: 'pending', arguments: { revision, summary: 'Shorten CTA', operations: [{ op: 'replace_text', before: 'Shop today', after: 'Shop' }] } } },
      { id: 4, type: 'done', data: null },
    ];
    const after = Number(new URL(req.url).searchParams.get('after'));
    req.reply({ headers: { 'Content-Type': 'text/event-stream' }, body: events.filter(event => event.id > after).map(event => `data: ${JSON.stringify(event)}\n\n`).join('') });
  });
  cy.intercept('POST', '**/mailapi/ai/assistant/turns/test-turn/decision', req => req.reply({ data: { status: req.body.approve ? 'approved' : 'discarded' } })).as('decision');
}
describe('Context-aware assistant', () => {
  beforeEach(() => { cy.visit(fixture, { onBeforeLoad: win => win.localStorage.clear() }); });
  it('streams replies, reviews targeted changes and supports undo across navigation', () => {
    setupTurn();
    cy.get('textarea').type('Shorten the CTA{enter}');
    cy.contains('strong', 'Updated').should('be.visible');
    cy.contains('Shorten CTA').should('be.visible');
    cy.window().then(win => win.assistantFixture.navigate());
    cy.contains('/analytics').should('be.visible');
    cy.contains('button', 'Apply').click(); cy.wait('@decision');
    cy.get('[data-cy=fixture-body]').should('contain.text', '<a>Shop</a><p>Footer</p>');
    cy.contains('button', 'Undo last edit').click();
    cy.get('[data-cy=fixture-body]').should('contain.text', '<a>Shop today</a>');
  });
  it('rejects proposals after an unsaved manual edit', () => {
    setupTurn(); cy.get('textarea').type('Shorten the CTA{enter}');
    cy.contains('Shorten CTA').should('be.visible');
    cy.window().then(win => win.assistantFixture.change());
    cy.contains('button', 'Apply').click();
    cy.contains('The editor changed').should('be.visible');
    cy.get('[data-cy=fixture-body]').should('contain.text', 'Shop today');
  });
});
