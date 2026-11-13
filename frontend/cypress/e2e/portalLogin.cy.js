const contact = {
  id: 2,
  first_name: 'Ivan',
  last_name: 'Petrov',
  email: 'ivan@example.com',
  avatar_url: ''
}

describe('Portal UI: login', () => {
  it('signs in with email and password', () => {
    cy.intercept('GET', '/api/v1/portal/me', {
      statusCode: 200,
      body: { status: 'success', data: contact }
    })
    cy.intercept(
      { method: 'GET', pathname: '/api/v1/portal/conversations' },
      { statusCode: 200, body: { status: 'success', data: { results: [], total: 0 } } }
    )
    cy.intercept('POST', '/api/v1/portal/auth/login', {
      statusCode: 200,
      body: { status: 'success', data: contact }
    }).as('login')

    cy.visit('/portal/login')
    cy.title().should('eq', 'Libredesk')
    cy.get('#portal-email').type('ivan@example.com')
    cy.get('#portal-password').type('Client2026!Pass')
    cy.get('[data-testid="portal-login-submit"]').click()

    cy.wait('@login').then(({ request }) => {
      expect(request.body).to.deep.equal({
        email: 'ivan@example.com',
        password: 'Client2026!Pass'
      })
    })
    cy.url().should('match', /\/portal\/?$/)
  })

  it('requests a magic link when the link mode is selected', () => {
    cy.intercept('POST', '/api/v1/portal/auth/magic-link', {
      statusCode: 200,
      body: { status: 'success', data: { sent: true } }
    }).as('magicLink')

    cy.visit('/portal/login')
    cy.get('[data-testid="portal-magic-toggle"]').click()
    cy.get('#portal-email').should('be.visible').type('ivan@example.com')
    cy.get('[data-testid="portal-login-submit"]').click()

    cy.wait('@magicLink').then(({ request }) => {
      expect(request.body).to.deep.equal({ email: 'ivan@example.com' })
    })
    cy.get('#portal-email').should('not.exist')
  })

  it('shows a disabled message when the portal is turned off', () => {
    cy.intercept('GET', '/api/v1/config', {
      statusCode: 200,
      body: {
        status: 'success',
        data: {
          'app.lang': 'en-US',
          'app.site_name': 'Libredesk',
          'portal.enabled': false
        }
      }
    })

    cy.visit('/portal/login')
    cy.get('[data-testid="portal-login-form"]').should('not.exist')
    cy.get('#portal-email').should('not.exist')
  })

  it('keeps the form available when the request fails', () => {
    cy.intercept('POST', '/api/v1/portal/auth/login', {
      statusCode: 500,
      body: { status: 'error', message: 'Something went wrong' }
    })

    cy.visit('/portal/login')
    cy.get('#portal-email').type('ivan@example.com')
    cy.get('#portal-password').type('wrong-password')
    cy.get('[data-testid="portal-login-submit"]').click()
    cy.get('#portal-email').should('be.visible')
    cy.get('#portal-password').should('be.visible')
  })
})
