describe('Time tracking panel', () => {
  const conversation = '/inboxes/all/conversation/8bde4479-e405-4905-8c64-0e71f250e42d'

  beforeEach(() => {
    cy.viewport(1440, 900)
    cy.login()
    cy.intercept('GET', '**/time-entries').as('entries')
    cy.visit(conversation)
    cy.contains('button', 'Учёт времени').then(($button) => {
      if ($button.attr('aria-expanded') !== 'true') cy.wrap($button).click()
    })
    cy.wait('@entries').its('response.statusCode').should('eq', 200)
  })

  it('shows the entries and the total time', () => {
    cy.contains('Разбор логов').should('be.visible')
    cy.contains('30m').should('be.visible')
  })

  it('adds a manual entry', () => {
    cy.intercept('POST', '**/time-entries', {
      statusCode: 200,
      body: { status: 'success', data: { id: 99 } }
    }).as('addEntry')

    cy.get('[data-testid="time-minutes"]').type('15')
    cy.get('[data-testid="time-note"]').type('Проверка ручного ввода')
    cy.get('[data-testid="time-add"]').click()

    cy.wait('@addEntry').then(({ request }) => {
      expect(request.body).to.deep.equal({
        duration_seconds: 900,
        note: 'Проверка ручного ввода',
        billable: true
      })
    })
  })

  it('starts a timer', () => {
    cy.intercept('POST', '**/timer/start', {
      statusCode: 200,
      body: { status: 'success', data: { id: 100 } }
    }).as('startTimer')

    cy.get('[data-testid="time-start"]').click()
    cy.wait('@startTimer').its('response.statusCode').should('eq', 200)
  })
})
