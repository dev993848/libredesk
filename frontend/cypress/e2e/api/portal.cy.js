// Portal settings are global, so this spec restores whatever it changes.

describe('API: portal settings', () => {
  let original

  before(() => {
    cy.login()
    cy.api('GET', '/api/v1/settings/portal').then(({ body }) => {
      original = body.data
    })
  })

  beforeEach(() => cy.login())

  after(() => {
    if (original) {
      cy.login()
      cy.api('PUT', '/api/v1/settings/portal', original)
    }
  })

  it('reads the portal settings', () => {
    cy.api('GET', '/api/v1/settings/portal').then(({ status, body }) => {
      expect(status).to.eq(200)
      expect(body.data).to.have.property('portal.enabled')
      expect(body.data).to.have.property('portal.default_inbox_id')
      expect(body.data).to.have.property('portal.root_url')
      expect(body.data).to.have.property('portal.jwt_enabled')
      expect(body.data).to.have.property('portal.session_lifetime')
      expect(body.data).to.have.property('portal.magic_link_ttl')
    })
  })

  it('persists the enabled flag and lifetimes', () => {
    cy.api('PUT', '/api/v1/settings/portal', {
      ...original,
      'portal.enabled': false,
      'portal.session_lifetime': '24h',
      'portal.magic_link_ttl': '10m'
    }).its('status').should('eq', 200)

    cy.api('GET', '/api/v1/settings/portal').then(({ body }) => {
      expect(body.data['portal.enabled']).to.eq(false)
      expect(body.data['portal.session_lifetime']).to.eq('24h')
      expect(body.data['portal.magic_link_ttl']).to.eq('10m')
    })
  })

  it('normalizes the public portal URL', () => {
    cy.api('PUT', '/api/v1/settings/portal', {
      ...original,
      'portal.root_url': 'https://portal.example.com/'
    }).its('status').should('eq', 200)

    cy.api('GET', '/api/v1/settings/portal').then(({ body }) => {
      expect(body.data['portal.root_url']).to.eq('https://portal.example.com')
    })
  })

  it('rejects a public portal URL without a scheme', () => {
    cy.api('PUT', '/api/v1/settings/portal', {
      ...original,
      'portal.root_url': 'portal.example.com'
    }, { failOnStatusCode: false }).its('status').should('be.gte', 400)
  })

  it('rejects a weak JWT secret when JWT sign-in is enabled', () => {
    cy.api('PUT', '/api/v1/settings/portal', {
      ...original,
      'portal.jwt_enabled': true,
      'portal.jwt_secret': 'too-short'
    }, { failOnStatusCode: false }).its('status').should('be.gte', 400)
  })

  it('rejects an unknown default inbox', () => {
    cy.api('PUT', '/api/v1/settings/portal', {
      ...original,
      'portal.default_inbox_id': 999999
    }, { failOnStatusCode: false }).its('status').should('be.gte', 400)
  })
})

describe('API: time spent report', () => {
  beforeEach(() => cy.login())

  it('returns totals and top agents', () => {
    cy.api('GET', '/api/v1/reports/overview/time?days=30').then(({ status, body }) => {
      expect(status).to.eq(200)
      expect(body.data).to.have.property('total_seconds')
      expect(body.data).to.have.property('billable_seconds')
      expect(body.data.agents).to.be.an('array')
      expect(body.data.teams).to.be.an('array')
    })
  })
})
