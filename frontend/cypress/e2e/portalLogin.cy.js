// Customer portal login page. The portal is a separate SPA served at /portal.
describe('Portal UI: login', () => {
  it('requests a magic link for the entered email', () => {
    cy.intercept('POST', '/api/v1/portal/auth/magic-link', {
      statusCode: 200,
      body: { status: 'success', data: { sent: true } }
    }).as('magicLink')

    cy.visit('/portal/login')
    cy.title().should('eq', 'Libredesk')
    cy.get('#portal-email').should('be.visible').type('ivan@example.com')
    cy.get('button[type="submit"]').click()

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
    cy.get('form').should('not.exist')
    cy.get('#portal-email').should('not.exist')
  })

  it('keeps the form available when the request fails', () => {
    cy.intercept('POST', '/api/v1/portal/auth/magic-link', {
      statusCode: 500,
      body: { status: 'error', message: 'Something went wrong' }
    })

    cy.visit('/portal/login')
    cy.get('#portal-email').type('ivan@example.com')
    cy.get('button[type="submit"]').click()
    cy.get('#portal-email').should('be.visible')
  })
})
