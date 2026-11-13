describe('API: portal password auth', () => {
  it('signs a contact in with email and password', () => {
    cy.request('POST', '/api/v1/portal/auth/login', {
      email: 'ivan@example.com',
      password: 'Client2026!Pass'
    }).then(({ status, body }) => {
      expect(status).to.eq(200)
      expect(body.data.email).to.eq('ivan@example.com')
    })
  })

  it('rejects a wrong password', () => {
    cy.request({
      method: 'POST',
      url: '/api/v1/portal/auth/login',
      body: { email: 'ivan@example.com', password: 'wrong-password' },
      failOnStatusCode: false
    }).then(({ status }) => {
      expect(status).to.eq(401)
    })
  })
})
