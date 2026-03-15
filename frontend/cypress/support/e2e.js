import './commands';

beforeEach(() => {
  cy.intercept('GET', '/sockjs-node/**', (req) => {
    req.destroy();
  });

  cy.intercept('GET', '/mailapi/health', (req) => {
    req.reply({});
  });
});
