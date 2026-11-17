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
      { id: 1, type: 'tool', data: { id: 'tool1', name: 'campaigns', status: 'running' } },
      { id: 2, type: 'tool', data: { id: 'tool1', name: 'campaigns', status: 'completed' } },
      { id: 3, type: 'text_delta', data: { text: '## Campaign performance\n\n| Campaign | Sent |\n|---|---:|\n| Newsletter | **6,211** |\n\n**Updated** CTA proposal' } },
      { id: 4, type: 'proposal', data: { id: 'proposal1', tool: 'propose_editor_patch', status: 'pending', arguments: { revision, summary: 'Shorten CTA', operations: [{ op: 'replace_text', before: 'Shop today', after: 'Shop' }] } } },
      { id: 5, type: 'done', data: null },
    ];
    const after = Number(new URL(req.url).searchParams.get('after'));
    req.reply({ headers: { 'Content-Type': 'text/event-stream' }, body: events.filter(event => event.id > after).map(event => `data: ${JSON.stringify(event)}\n\n`).join('') });
  });
  cy.intercept('POST', '**/mailapi/ai/assistant/turns/test-turn/decision', req => req.reply({ data: { status: req.body.approve ? 'approved' : 'discarded' } })).as('decision');
}
// The chat is drawn by <lp-agent-panel> inside its shadow root.
describe('Context-aware assistant', { includeShadowDom: true }, () => {
  beforeEach(() => { cy.visit(fixture, { onBeforeLoad: win => win.localStorage.clear() }); });
  it('streams replies, reviews targeted changes and supports undo across navigation', () => {
    setupTurn();
    cy.get('.input--rich').type('Shorten the CTA{enter}');
    cy.contains('strong', 'Updated').should('be.visible');
    cy.contains('h2', 'Campaign performance').should('be.visible');
    cy.get('.message table').should('be.visible');
    cy.get('.scroll').then($scroll => {
      const tool = $scroll[0].querySelector('[data-tool-id="tool1"]');
      const answer = $scroll[0].querySelector('.message');
      expect(Boolean(tool.compareDocumentPosition(answer) & 4)).to.eq(true);
    });
    cy.contains('Shorten CTA').should('be.visible');
    cy.window().then(win => win.assistantFixture.navigate());
    cy.contains('/analytics').should('be.visible');
    cy.contains('button', 'Apply').click(); cy.wait('@decision');
    cy.get('[data-cy=fixture-body]').should('contain.text', '<a>Shop</a><p>Footer</p>');
    cy.contains('button', 'Undo last edit').click();
    cy.get('[data-cy=fixture-body]').should('contain.text', '<a>Shop today</a>');
  });
  it('rejects proposals after an unsaved manual edit', () => {
    setupTurn(); cy.get('.input--rich').type('Shorten the CTA{enter}');
    cy.contains('Shorten CTA').should('be.visible');
    cy.window().then(win => win.assistantFixture.change());
    cy.contains('button', 'Apply').click();
    cy.contains('The editor changed').should('be.visible');
    cy.get('[data-cy=fixture-body]').should('contain.text', 'Shop today');
  });
  it('keeps replies at full height when the chat outgrows the panel', () => {
    const reply = Array.from({ length: 6 }, (_, i) => `Paragraph ${i + 1} of a reply long enough to wrap across the panel.`).join('\n\n');
    const messages = Array.from({ length: 6 }, (_, i) => [{ role: 'user', content: `Question ${i}` }, { role: 'assistant', content: reply, parts: [{ type: 'text', content: reply }] }]).flat();
    cy.window().then(win => win.localStorage.setItem('listpocket_assistant_v1:fixture-owner', JSON.stringify({ active: 'long', threads: [{ id: 'long', title: 'Long chat', messages, tools: [], proposals: [], after: 0, turn: null }] })));
    cy.reload();
    cy.get('.message').should('have.length', 6).each($reply => {
      expect($reply[0].clientHeight).to.eq($reply[0].scrollHeight);
    });
    cy.get('.scroll').then($scroll => {
      expect($scroll[0].scrollHeight).to.be.greaterThan($scroll[0].clientHeight);
    });
  });

  it('attaches pasted and dropped images, and keeps them for follow-up questions', () => {
    const bodies = [];
    cy.intercept('POST', '**/mailapi/ai/assistant/turns', req => { bodies.push(req.body); req.reply({ data: { id: 'paste-turn' } }); });
    cy.intercept('GET', '**/mailapi/ai/assistant/turns/paste-turn/stream*', { headers: { 'Content-Type': 'text/event-stream' }, body: `data: ${JSON.stringify({ id: 1, type: 'done', data: null })}\n\n` });
    const png = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==';
    const withFiles = (win, files) => {
      const transfer = new win.DataTransfer();
      files.forEach(([name, type, data]) => transfer.items.add(new win.File([Uint8Array.from(atob(data), c => c.charCodeAt(0))], name, { type })));
      return transfer;
    };
    // A screenshot: an image with no useful name.
    cy.get('.input--rich').then($input => {
      const win = $input[0].ownerDocument.defaultView;
      $input[0].dispatchEvent(new win.ClipboardEvent('paste', { clipboardData: withFiles(win, [['image.png', 'image/png', png]]), bubbles: true, composed: true, cancelable: true }));
    });
    cy.get('.chip').should('have.length', 1).and('contain.text', 'pasted-image-').find('img.thumb').should('be.visible');
    // Text copied from a spreadsheet comes with a picture of the cells; it stays text.
    cy.get('.input--rich').then($input => {
      const win = $input[0].ownerDocument.defaultView;
      const transfer = withFiles(win, [['image.png', 'image/png', png]]);
      transfer.setData('text/plain', 'Name\tEmail');
      $input[0].dispatchEvent(new win.ClipboardEvent('paste', { clipboardData: transfer, bubbles: true, composed: true, cancelable: true }));
    });
    cy.get('.chip').should('have.length', 1);
    // Dropped anywhere on the panel.
    cy.get('.panel').then($panel => {
      const win = $panel[0].ownerDocument.defaultView;
      const dataTransfer = withFiles(win, [['notes.txt', 'text/plain', btoa('hello')]]);
      ['dragenter', 'dragover', 'drop'].forEach(type => $panel[0].dispatchEvent(new win.DragEvent(type, { dataTransfer, bubbles: true, composed: true, cancelable: true })));
    });
    cy.get('.chip').should('have.length', 2).last().should('contain.text', 'notes.txt');
    cy.get('.drop').should('not.be.visible');
    // Types the server rejects are turned away before sending.
    cy.get('.panel').then($panel => {
      const win = $panel[0].ownerDocument.defaultView;
      const dataTransfer = withFiles(win, [['leads.csv', 'text/csv', btoa('a,b')]]);
      $panel[0].dispatchEvent(new win.DragEvent('drop', { dataTransfer, bubbles: true, composed: true, cancelable: true }));
    });
    cy.get('.attach-error').should('contain.text', "leads.csv isn't a type that can be attached");
    cy.get('.chip').should('have.length', 2);

    cy.get('.input--rich').type('What is in this screenshot?{enter}');
    cy.get('[data-role=user]').should('contain.text', 'What is in this screenshot?');
    cy.get('[data-cy=assistant-panel]').should(($panel) => expect($panel[0].draft).to.eq(''));
    cy.get('.chip').should('have.length', 2).then(() => {
      const [first] = bodies;
      expect(first.attachments.map(a => a.mimeType)).to.deep.eq(['image/png', 'text/plain']);
      expect(first.attachments[0].filename).to.match(/^pasted-image-\d{8}-\d{6}\.png$/);
      expect(first.attachments[0].data).to.eq(png);
      expect(first.attachments[1].data).to.eq(btoa('hello'));
    });
    cy.get('.working').should('not.be.visible');
    cy.get('.input--rich').type('And the notes?{enter}');
    cy.get('[data-role=user]').should('have.length', 2).then(() => {
      expect(bodies).to.have.length(2);
      expect(bodies[1].attachments).to.have.length(2);
    });
    cy.get('.chip [aria-label^="Remove pasted-image"]').click();
    cy.get('.chip').should('have.length', 1);
  });
});
