const fixture = '/admin/cypress/fixtures/workflow-builder-harness.html';
const workflow = { id: 'workflow-fixture', name: 'Untitled Workflow', status: 'draft', version: 1, triggerType: 'manual' };
const empty = { workflows: [], activeWorkflow: null, contacts: [], companies: [], runLogs: [] };
const created = { ...empty, workflows: [workflow], activeWorkflow: { workflow, nodes: [], edges: [] } };
describe('Workflow builder creation', () => {
  beforeEach(() => {
    cy.intercept('GET', '**/api/control-plane/dashboard*', empty).as('dashboard');
    cy.intercept('POST', '**/api/control-plane/workflows', { body: created, delay: 250 }).as('createWorkflow');
    cy.visit(fixture); cy.wait('@dashboard');
  });
  it('creates the first workflow from the toolbar and enables editing', () => {
    cy.contains('No workflow selected').should('be.visible');
    cy.contains('button', /^Save$/).should('be.disabled');
    cy.contains('button', /^New workflow$/).should('be.enabled').click();
    cy.contains('button', /^New workflow$/).should('be.disabled');
    cy.wait('@createWorkflow');
    cy.get('.workflow-title-input input').should('be.enabled').and('have.value', 'Untitled Workflow');
    cy.contains('button', '+ Add node').should('be.enabled');
    cy.contains('No workflow selected').should('not.exist');
  });
  it('creates from the empty state and allows retry after an API error', () => {
    cy.intercept('POST', '**/api/control-plane/workflows', { statusCode: 500, body: { message: 'Creation failed' } }).as('failedCreate');
    cy.contains('button', /^Create workflow$/).click(); cy.wait('@failedCreate');
    cy.contains('button', /^Create workflow$/).should('be.enabled');
    cy.intercept('POST', '**/api/control-plane/workflows', created).as('retryCreate');
    cy.contains('button', /^Create workflow$/).click(); cy.wait('@retryCreate');
    cy.get('.workflow-title-input input').should('be.enabled');
  });
});

describe('Workflow studio editing', () => {
  beforeEach(() => {
    cy.intercept('GET', '**/api/control-plane/dashboard*', created);
    cy.visit(fixture);
  });
  it('supports a guided trigger, undo, redo and searchable steps', () => {
    cy.contains('button', /^Add trigger$/).click();
    cy.get('.workflow-node-card').should('have.length', 1).and('contain.text', 'Manual Trigger');
    cy.get('[aria-label="Close node settings"]').click();
    cy.contains('button', /^Undo$/).click(); cy.get('.workflow-node-card').should('not.exist');
    cy.contains('button', /^Redo$/).click(); cy.get('.workflow-node-card').should('have.length', 1);
    cy.get('[aria-label="Search steps"]').type('email');
    cy.get('.step-library-item').should('have.length', 1).and('contain.text', 'Transactional Email');
  });
  it('saves the edited graph before running a test and blocks the test after a failed save', () => {
    cy.contains('button', /^Add trigger$/).click();
    cy.get('[aria-label="Close node settings"]').click();
    cy.intercept('POST', '**/workflows/workflow-fixture/save', req => {
      expect(req.body.nodes).to.have.length(1);
      expect(req.body.nodes[0].config.mode).to.eq('manual');
      req.reply({ ...created, activeWorkflow: { workflow, nodes: req.body.nodes, edges: req.body.edges } });
    }).as('saveEdited');
    cy.intercept('POST', '**/workflows/workflow-fixture/run', created).as('runTest');
    cy.contains('button', /^Run Test$/).click(); cy.wait('@saveEdited'); cy.wait('@runTest');
    cy.intercept('POST', '**/workflows/workflow-fixture/save', { statusCode: 500, body: { message: 'Save failed' } });
    let runRequests = 0;
    cy.intercept('POST', '**/workflows/workflow-fixture/run', req => { runRequests += 1; req.reply(created); });
    cy.contains('button', /^Run Test$/).click();
    cy.contains('Save failed').should('be.visible').then(() => expect(runRequests).to.eq(0));
  });
});
