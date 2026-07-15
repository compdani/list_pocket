const apiUrl = Cypress.env('apiUrl');

describe('Subscribers', () => {
  it('Opens subscribers page', () => {
    cy.resetDB();
    cy.loginAndVisit('/admin/subscribers');
  });

  it('Counts subscribers', () => {
    cy.get('tbody td[data-label=E-mail]').its('length').should('eq', 2);
  });

  it('Searches subscribers', () => {
    const cases = [
      { value: 'john{enter}', count: 1, contains: 'john@example.com' },
      { value: 'anon{enter}', count: 1, contains: 'anon@example.com' },
      { value: '{enter}', count: 2, contains: null },
    ];

    cases.forEach((c) => {
      cy.get('[data-cy=search]').clear().type(c.value);
      cy.get('tbody td[data-label=E-mail]').its('length').should('eq', c.count);
      if (c.contains) {
        cy.get('tbody td[data-label=E-mail]').contains(c.contains);
      }
    });
  });

  it('Exports subscribers', () => {
    const cases = [
      {
        listIDs: [], ids: [], query: '', length: 3,
      },
      {
        listIDs: [],
        ids: [],
        query: "subscribers.name LIKE '%anon%'",
        length: 2,
      },
      {
        listIDs: [],
        ids: [],
        query: "subscribers.name LIKE 'nope'",
        length: 1,
      },
    ];

    // listIDs[] and ids[] are unused for now as Cypress doesn't support encoding of arrays in `qs`.
    cases.forEach((c) => {
      cy.request({ url: `${apiUrl}/mailapi/subscribers/export`, qs: { query: c.query, list_id: c.listIDs, id: c.ids } }).then((resp) => {
        cy.expect(resp.body.trim().split('\n')).to.have.lengthOf(c.length);
      });
    });
  });

  it('Filters subscribers with the visual builder', () => {
    cy.get('[data-cy=btn-advanced-search]').click();
    cy.get('[data-cy=filter-builder]').should('be.visible');

    cy.get('[data-cy=filter-rule]').first().within(() => {
      cy.get('[data-cy=filter-field]').click();
      cy.get('.v-list-item').contains('Attribute').click();
      cy.get('[data-cy=filter-attrib-key]').clear().type('city');
      cy.get('[data-cy=filter-value]').clear().type('Bengaluru');
    });
    cy.get('[data-cy=btn-query]').click();
    cy.get('[data-cy=active-filter-chips]').should('exist');
    cy.get('tbody td[data-label=E-mail]').its('length').should('eq', 2);

    cy.get('[data-cy=btn-advanced-search]').click();
    if (Cypress.$('[data-cy=btn-mode-sql]').length) {
      cy.get('[data-cy=btn-mode-sql]').click();
      cy.get('[data-cy=query]').clear().type("json_extract(subscribers.attribs, '$.city') = 'Bengaluru' AND subscribers.name LIKE 'Anon%'");
      cy.get('[data-cy=btn-query]').click();
      cy.get('tbody td[data-label=E-mail]').its('length').should('eq', 1);
    }

    cy.get('[data-cy=btn-advanced-search]').click();
    cy.get('[data-cy=btn-query-reset]').click();
    cy.get('[data-cy=active-filter-chips]').should('not.exist');
    cy.wait(1000);
    cy.get('tbody td[data-label=E-mail]').its('length').should('eq', 2);
  });

  it('Does bulk subscriber list add and remove', () => {
    const cases = [
      // radio: action to perform, rows: table rows to select and perform on: [expected statuses of those rows after thea action]
      { radio: 'check-list-add', lists: [0, 1], rows: { 0: ['confirmed', 'confirmed'] } },
      { radio: 'check-list-unsubscribe', lists: [0, 1], rows: { 0: ['unsubscribed', 'unsubscribed'], 1: ['unsubscribed'] } },
      { radio: 'check-list-remove', lists: [0, 1], rows: { 1: [] } },
      { radio: 'check-list-add', lists: [0, 1], rows: { 0: ['unsubscribed', 'unsubscribed'], 1: ['unconfirmed', 'unconfirmed'] } },
      { radio: 'check-list-remove', lists: [0], rows: { 0: ['unsubscribed'] } },
      { radio: 'check-list-add', lists: [0], rows: { 0: ['unconfirmed', 'unsubscribed'] } },
    ];

    cases.forEach((c, n) => {
      // Select one of the 2 subscribers in the table.
      Object.keys(c.rows).forEach((r) => {
        cy.get('tbody td.checkbox-cell .checkbox').eq(r).click();
      });

      // Open the 'manage lists' modal.
      cy.get('[data-cy=btn-manage-lists]').click();

      // Check both lists in the modal.
      c.lists.forEach((l) => {
        cy.get('.list-selector input').click();
        cy.get('.list-selector .autocomplete a').first().click();
      });

      // Select the radio option in the modal.
      cy.get(`[data-cy=${c.radio}] .check`).click();

      // For the first test, check the optin preconfirm box.
      if (n === 0) {
        cy.get('[data-cy=preconfirm]').click();
      }

      // Save.
      cy.get('.modal button.is-primary').click();

      // Check the status of the lists on the subscriber.
      Object.keys(c.rows).forEach((r) => {
        cy.get('tbody td[data-label=E-mail]').eq(r).find('.tags').then(($el) => {
          cy.wrap($el).find('.tag').should('have.length', c.rows[r].length);
          c.rows[r].forEach((status, n) => {
            // eg: .tag(n).unconfirmed
            cy.wrap($el).find('.tag').eq(n).should('have.class', status);
          });
        });
      });
    });
  });

  it('Resets subscribers page', () => {
    cy.resetDB();
    cy.loginAndVisit('/admin/subscribers');
  });

  it('Edits subscribers', () => {
    const status = ['enabled', 'blocklisted'];
    const json = '{"string": "hello", "ints": [1,2,3], "null": null, "sub": {"bool": true}}';

    // Collect values being edited on each sub to confirm the changes in the next step
    // index by their ID shown in the modal.
    const rows = {};

    // Open the edit popup and edit the default lists.
    cy.get('[data-cy=btn-edit]').each(($el, n) => {
      const email = `email-${n}@EMAIL.com`;
      const phone = `+15555550${n}${n}`;
      const firstName = `name-${n}`;
      const lastName = 'subscriber';

      // Open the edit modal.
      cy.wrap($el).click();

      // Get the ID from the header and proceed to fill the form.
      let id = 0;
      cy.get('[data-cy=id]').then(($el) => {
        id = parseInt($el.text());

        cy.get('input[name=email]').clear().type(email);
        cy.get('input[name=phone]').clear().type(phone);
        cy.get('input[name=first_name]').clear().type(firstName);
        cy.get('input[name=last_name]').clear().type(lastName);

        if (status[n] === 'blocklisted') {
          cy.get('select[name=status]').select(status[n]);
        }
        cy.get('.list-selector input').click();
        cy.get('.list-selector .autocomplete a').first().click();
        cy.get('textarea[name=attribs]').clear().type(json, { parseSpecialCharSequences: false, delay: 0 });
        cy.get('.modal-card-foot button[type=submit]').click();

        rows[id] = { email, phone, name: `${firstName} ${lastName}`, status: status[n] };
      });
    });

    // Confirm the edits on the table.
    cy.wait(500);
    cy.log(rows);
    cy.get('tbody tr').each(($el) => {
      cy.wrap($el).find('td[data-id]').invoke('attr', 'data-id').then((idStr) => {
        const id = parseInt(idStr);
        cy.wrap($el).find('td[data-label=E-mail]').contains(rows[id].email.toLowerCase());
        cy.wrap($el).find('td[data-label=E-mail]').contains(rows[id].phone);
        cy.wrap($el).find('td[data-label=Name]').contains(rows[id].name);

        if (rows[id].status === 'blocklisted') {
          cy.wrap($el).find('[data-cy=blocklisted]');
        }

        // Both lists on the enabled sub should be 'unconfirmed' and the blocklisted one, 'unsubscribed.'
        cy.wrap($el).find(`.tags .${rows[id].status === 'enabled' ? 'unconfirmed' : 'unsubscribed'}`)
          .its('length').should('eq', 2);
        cy.wrap($el).find('td[data-label=Lists]').then((l) => {
          cy.expect(parseInt(l.text().trim())).to.equal(rows[id].status === 'blocklisted' ? 0 : 2);
        });
      });
    });
  });

  it('Deletes subscribers', () => {
    // Delete all visible lists.
    cy.get('tbody tr').each(() => {
      cy.get('tbody a[data-cy=btn-delete]').first().click();
      cy.get('.modal button.is-primary').click();
    });

    // Confirm deletion.
    cy.get('table tr.is-empty');
  });

  it('Creates new subscribers', () => {
    const statuses = ['enabled', 'blocklisted'];
    const lists = [[1], [2], [1, 2]];
    const json = '{"string": "hello", "ints": [1,2,3], "null": null, "sub": {"bool": true}}';

    // Cycle through each status and each list ID combination and create subscribers.
    const n = 0;
    for (let n = 0; n < 6; n++) {
      const email = `email-${n}@EMAIL.com`;
      const phone = `+15555551${n}${n}`;
      const firstName = `name-${n}`;
      const lastName = 'subscriber';
      const status = statuses[(n + 1) % statuses.length];
      const list = lists[(n + 1) % lists.length];

      cy.get('[data-cy=btn-new]').click();
      cy.get('input[name=email]').type(email);
      cy.get('input[name=phone]').type(phone);
      cy.get('input[name=first_name]').type(firstName);
      cy.get('input[name=last_name]').type(lastName);
      cy.get('select[name=status]').select(status);

      list.forEach((l) => {
        cy.get('.list-selector input').click();
        cy.get('.list-selector .autocomplete a').first().click();
      });
      cy.get('textarea[name=attribs]').clear().type(json, { parseSpecialCharSequences: false, delay: 0 });
      cy.get('.modal-card-foot button[type=submit]').click();

      // Confirm the addition by inspecting the newly created list row,
      // which is always the first row in the table.
      cy.wait(250);
      const tr = cy.get('tbody tr:nth-child(1)').then(($el) => {
        cy.wrap($el).find('td[data-label=E-mail]').contains(email.toLowerCase());
        cy.wrap($el).find('td[data-label=E-mail]').contains(phone);
        cy.wrap($el).find('td[data-label=Name]').contains(`${firstName} ${lastName}`);

        if (status === 'blocklisted') {
          cy.wrap($el).find('[data-cy=blocklisted]');
        }
        cy.wrap($el).find(`.tags .${status === 'enabled' ? 'unconfirmed' : 'unsubscribed'}`)
          .its('length').should('eq', list.length);
        cy.wrap($el).find('td[data-label=Lists]').then((l) => {
          cy.expect(parseInt(l.text().trim())).to.equal(status === 'blocklisted' ? 0 : list.length);
        });
      });
    }
  });

  it('Sorts subscribers', () => {
    const asc = [3, 4, 5, 6, 7, 8];
    const desc = [8, 7, 6, 5, 4, 3];
    const cases = ['cy-email', 'cy-name', 'cy-created_at', 'cy-updated_at'];

    cases.forEach((c) => {
      cy.sortTable(`thead th.${c}`, asc);
      cy.wait(250);
      cy.sortTable(`thead th.${c}`, desc);
      cy.wait(250);
    });
  });
});

describe('Domain blocklist', () => {
  it('Opens settings page', () => {
    cy.resetDB();
  });

  it('Add domains to blocklist', () => {
    cy.loginAndVisit('/admin/settings');
    cy.get('.b-tabs nav a').eq(2).click();
    cy.get('textarea[name="privacy.domain_blocklist"]').clear().type('ban.net\n\nBaN.OrG\n\nban.com\n\n');
    cy.get('[data-cy=btn-save]').click();
  });

  it('Try subscribing via public page', () => {
    cy.wait(1000);
    cy.visit(`${apiUrl}/subscription/form`);
    cy.get('input[name=email]').clear().type('test@noban.net');
    cy.get('button[type=submit]').click();
    cy.get('h2').contains('Subscribe');

    cy.visit(`${apiUrl}/subscription/form`);
    cy.get('input[name=email]').clear().type('test@ban.net');
    cy.get('button[type=submit]').click();
    cy.get('h2').contains('Error');
  });

  // Post to the admin API.
  it('Try via admin API', () => {
    cy.wait(1000);

    // Add non-banned domain.
    cy.request({
      method: 'POST',
      url: `${apiUrl}/mailapi/subscribers`,
      failOnStatusCode: true,
      body: {
        email: 'test1@noban.net', name: 'test', lists: [1], status: 'enabled',
      },
    }).then((createResp) => {
      expect(createResp.status).to.equal(200);
      const subscriberId = createResp.body.data.id;

      // Add banned domain.
      cy.request({
        method: 'POST',
        url: `${apiUrl}/mailapi/subscribers`,
        failOnStatusCode: false,
        body: {
          email: 'test1@ban.com', name: 'test', lists: [1], status: 'enabled',
        },
      }).should((response) => {
        expect(response.status).to.equal(400);
      });

      // Modify an existing subscriber to a banned domain.
      cy.request({
        method: 'PUT',
        url: `${apiUrl}/mailapi/subscribers/${subscriberId}`,
        failOnStatusCode: false,
        body: {
          email: 'test3@ban.org', name: 'test', lists: [1], status: 'enabled',
        },
      }).should((response) => {
        expect(response.status).to.equal(400);
      });
    });
  });

  it('Try via import', () => {
    cy.loginAndVisit('/admin/subscribers/import');
    cy.get('.list-selector input').click();
    cy.get('.list-selector .autocomplete a').first().click();

    cy.fixture('subs-domain-blocklist.csv').then((data) => {
      cy.get('input[type="file"]').attachFile({
        fileContent: data.toString(),
        fileName: 'subs.csv',
        mimeType: 'text/csv',
      });
    });

    cy.get('button.is-primary').click();
    cy.get('section.wrap .has-text-success');
    // cy.get('button.is-primary').click();
    cy.get('.log-view').should('contain', 'ban1-import@BAN.net').and('contain', 'ban2-import@ban.ORG');
    cy.wait(100);
  });

  it('Clear blocklist and try', () => {
    cy.loginAndVisit('/admin/settings');
    cy.get('.b-tabs nav a').eq(2).click();
    cy.get('textarea[name="privacy.domain_blocklist"]').clear();
    cy.get('[data-cy=btn-save]').click();
    cy.wait(3000);

    // Add banned domain.
    cy.request({
      method: 'POST',
      url: `${apiUrl}/mailapi/subscribers`,
      failOnStatusCode: true,
      body: {
        email: 'test4@BAN.com', name: 'test', lists: [1], status: 'enabled',
      },
    }).then((createResp) => {
      expect(createResp.status).to.equal(200);
      const subscriberId = createResp.body.data.id;

      // Modify an existing subscriber (still allowed while domain not on blocklist).
      cy.request({
        method: 'PUT',
        url: `${apiUrl}/mailapi/subscribers/${subscriberId}`,
        failOnStatusCode: true,
        body: {
          email: 'test4@BAN.org', name: 'test', lists: [1], status: 'enabled',
        },
      }).should((response) => {
        expect(response.status).to.equal(200);
      });
    });
  });
});
