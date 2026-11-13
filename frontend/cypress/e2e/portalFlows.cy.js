const contact = {
  id: 2,
  first_name: 'Ivan',
  last_name: 'Petrov',
  email: 'ivan@example.com',
  avatar_url: ''
}

const conversation = {
  uuid: 'c1',
  reference_number: '100',
  subject: 'Cannot pay',
  status: { name: 'Open', category: 'open' },
  inbox_name: 'Support',
  inbox_channel: 'email',
  created_at: '2026-11-01T10:00:00Z',
  last_message_at: '2026-11-02T12:30:00Z',
  last_message_preview: 'Please check the card',
  unread_count: 0,
  time_spent_seconds: 5400
}

const detail = {
  ...conversation,
  messages: [
    {
      uuid: 'm1',
      created_at: '2026-11-01T10:00:00Z',
      content: 'Hello, my card is declined',
      content_type: 'text',
      sender_type: 'contact',
      author_name: 'Ivan Petrov',
      attachments: []
    },
    {
      uuid: 'm2',
      created_at: '2026-11-02T12:30:00Z',
      content: 'Please check the card expiry date',
      content_type: 'text',
      sender_type: 'agent',
      author_name: 'Support',
      attachments: []
    }
  ]
}

describe('Portal UI: conversations', () => {
  beforeEach(() => {
    cy.intercept('GET', '/api/v1/config', {
      statusCode: 200,
      body: {
        status: 'success',
        data: {
          'app.lang': 'en-US',
          'app.site_name': 'Libredesk',
          'portal.enabled': true
        }
      }
    })
    cy.intercept('GET', '/api/v1/lang/en-US', {
      statusCode: 200,
      body: {
        status: 'success',
        data: {
          'portal.time.spent': 'Time spent: {duration}',
          'portal.time.hour': 'h',
          'portal.time.minute': 'm',
          'portal.time.lessThanMinute': 'less than a minute',
          'portal.status.open': 'Open',
          'portal.status.waiting': 'Waiting',
          'portal.status.resolved': 'Resolved',
          'portal.status.closed': 'Closed'
        }
      }
    })
    cy.intercept('GET', '/api/v1/portal/me', {
      statusCode: 200,
      body: { status: 'success', data: contact }
    }).as('me')
    cy.intercept(
      { method: 'GET', pathname: '/api/v1/portal/conversations' },
      { statusCode: 200, body: { status: 'success', data: { results: [conversation], total: 1 } } }
    ).as('list')
    cy.intercept(
      { method: 'GET', pathname: '/api/v1/portal/conversations/c1' },
      { statusCode: 200, body: { status: 'success', data: detail } }
    ).as('detail')
    cy.intercept(
      { method: 'POST', pathname: '/api/v1/portal/conversations/c1/seen' },
      { statusCode: 200, body: { status: 'success', data: { seen: true } } }
    )
    cy.intercept(
      { method: 'POST', pathname: '/api/v1/portal/conversations/c1/messages' },
      { statusCode: 200, body: { status: 'success', data: { uuid: 'm3' } } }
    ).as('reply')
    cy.intercept(
      { method: 'POST', pathname: '/api/v1/portal/conversations/c1/attachments' },
      { statusCode: 200, body: { status: 'success', data: { uuid: 'm4' } } }
    ).as('upload')
    cy.intercept(
      { method: 'POST', pathname: '/api/v1/portal/auth/password' },
      { statusCode: 200, body: { status: 'success', data: { updated: true } } }
    ).as('password')
    cy.intercept(
      { method: 'POST', pathname: '/api/v1/portal/conversations' },
      { statusCode: 200, body: { status: 'success', data: { uuid: 'new1' } } }
    ).as('create')
    cy.intercept(
      { method: 'GET', pathname: '/api/v1/portal/conversations/new1' },
      {
        statusCode: 200,
        body: {
          status: 'success',
          data: {
            ...conversation,
            uuid: 'new1',
            reference_number: '101',
            subject: 'New ticket',
            time_spent_seconds: 0,
            messages: []
          }
        }
      }
    )
  })

  it('navigates between tickets and profile from the sidebar', () => {
    cy.visit('/portal/')
    cy.get('[data-testid="portal-nav-profile"]').click()
    cy.url().should('include', '/portal/profile')

    cy.get('[data-testid="portal-nav-conversations"]').click()
    cy.url().should('match', /\/portal\/?$/)
  })

  it('lists the contact tickets with the total time spent', () => {
    cy.visit('/portal/')

    cy.wait('@list')
    cy.get('[data-testid="portal-conversation"]').should('have.length', 1)
    cy.get('[data-testid="portal-conversation"]').should('contain', 'Cannot pay')
    cy.get('[data-testid="portal-conversation"]').should('contain', 'Please check the card')
    cy.get('[data-testid="portal-conversation-time"]').should('contain', '1')
  })

  it('loads more tickets', () => {
    const all = Array.from({ length: 25 }, (_, index) => ({
      ...conversation,
      uuid: `c${index + 1}`,
      reference_number: String(100 + index + 1),
      subject: `Ticket ${index + 1}`
    }))
    cy.intercept(
      { method: 'GET', pathname: '/api/v1/portal/conversations' },
      (req) => {
        const page = Number(req.query.page || 1)
        req.reply({
          statusCode: 200,
          body: {
            status: 'success',
            data: { results: all.slice((page - 1) * 20, page * 20), total: all.length }
          }
        })
      }
    ).as('pages')

    cy.visit('/portal/')
    cy.wait('@pages')
    cy.get('[data-testid="portal-conversation"]').should('have.length', 20)

    cy.get('[data-testid="portal-load-more"]').click()
    cy.wait('@pages')
    cy.get('[data-testid="portal-conversation"]').should('have.length', 25)
    cy.get('[data-testid="portal-load-more"]').should('not.exist')
  })

  it('opens a ticket and shows its public messages', () => {
    cy.visit('/portal/')
    cy.get('[data-testid="portal-conversation"]').click()

    cy.url().should('include', '/portal/c/c1')
    cy.wait('@detail')
    cy.get('[data-testid="portal-message"]').should('have.length', 2)
    cy.get('[data-testid="portal-message"]').eq(0).should('contain', 'Hello, my card is declined')
    cy.get('[data-testid="portal-message"]').eq(1).should('contain', 'Please check the card expiry date')
  })

  it('sends a reply from the ticket page', () => {
    cy.visit('/portal/c/c1')
    cy.get('[data-testid="portal-reply"]').type('I checked the date, it is valid')
    cy.get('[data-testid="portal-reply-submit"]').click()

    cy.wait('@reply').then(({ request }) => {
      expect(request.body).to.deep.equal({ message: 'I checked the date, it is valid' })
    })
    cy.get('[data-testid="portal-reply"]').should('have.value', '')
  })

  it('uploads an attachment from the ticket page', () => {
    cy.visit('/portal/c/c1')
    cy.get('input[type="file"]').selectFile(
      {
        contents: Cypress.Buffer.from('attachment body'),
        fileName: 'note.txt',
        mimeType: 'text/plain'
      },
      { force: true }
    )

    cy.wait('@upload').then(({ request }) => {
      expect(request.headers['content-type']).to.contain('multipart/form-data')
    })
  })

  it('shows the contact profile', () => {
    cy.visit('/portal/profile')
    cy.wait('@me')
    cy.get('[data-testid="portal-profile-email"]').should('contain', 'ivan@example.com')
    cy.get('main').contains('Ivan Petrov').should('be.visible')
  })

  it('sets a password from the profile', () => {
    cy.visit('/portal/profile')
    cy.get('[data-testid="portal-new-password"]').type('New-Password-2026!')
    cy.get('[data-testid="portal-confirm-password"]').type('New-Password-2026!')
    cy.get('[data-testid="portal-save-password"]').click()

    cy.wait('@password').then(({ request }) => {
      expect(request.body).to.deep.equal({ password: 'New-Password-2026!' })
    })
    cy.get('[data-testid="portal-password-saved"]').should('be.visible')
  })

  it('downloads the ticket transcript', () => {
    cy.visit('/portal/c/c1')
    cy.window().then((win) => {
      cy.stub(win, 'open').as('open')
    })
    cy.get('[data-testid="portal-transcript"]').click()
    cy.get('@open').should(
      'have.been.calledWith',
      '/api/v1/portal/conversations/c1/transcript',
      '_blank',
      'noopener'
    )
  })

  it('creates a new ticket', () => {
    cy.visit('/portal/new')
    cy.get('[data-testid="portal-subject"]').type('New ticket')
    cy.get('[data-testid="portal-message"]').type('Please help with my order')
    cy.get('[data-testid="portal-new-submit"]').click()

    cy.wait('@create').then(({ request }) => {
      expect(request.body).to.deep.equal({
        subject: 'New ticket',
        message: 'Please help with my order'
      })
    })
    cy.url().should('include', '/portal/c/new1')
  })
})
