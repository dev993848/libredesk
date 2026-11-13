// Package portal implements the customer-facing portal: magic-link login and
// read access to a contact's own conversations.
package portal

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/abhinavxd/libredesk/internal/attachment"
	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	"github.com/abhinavxd/libredesk/internal/dbutil"
	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/abhinavxd/libredesk/internal/portal/models"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/zerodha/logf"
)

const (
	defaultMagicLinkTTL = 15 * time.Minute
	defaultPageSize     = 20
	maxPageSize         = 100
)

var (
	//go:embed queries.sql
	efs embed.FS

	// ErrInvalidToken is returned when a magic-link token is unknown, expired or already used.
	ErrInvalidToken = errors.New("invalid or expired portal token")
)

// ConversationStore is the subset of the conversation manager used by the portal.
type ConversationStore interface {
	GetAllConversationMessages(conversationUUID string, private *bool, msgTypes []string, limit int) ([]cmodels.Message, error)
	SignAttachmentURLs(attachments attachment.Attachments)
}

// Config holds the portal runtime configuration.
type Config struct {
	Enabled         bool
	MagicLinkTTL    time.Duration
	DefaultInboxID  int
	JWTEnabled      bool
	JWTSecret       string
	SessionLifetime time.Duration
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB           *sqlx.DB
	Lo           *logf.Logger
	I18n         *i18n.I18n
	Conversation ConversationStore
	Config       Config
}

type queries struct {
	GetContactByEmail        *sqlx.Stmt `query:"get-contact-by-email"`
	GetContactByExternalID   *sqlx.Stmt `query:"get-contact-by-external-id"`
	GetContactByID           *sqlx.Stmt `query:"get-contact-by-id"`
	InsertLoginToken         *sqlx.Stmt `query:"insert-login-token"`
	ConsumeLoginToken        *sqlx.Stmt `query:"consume-login-token"`
	DeleteExpiredLoginTokens *sqlx.Stmt `query:"delete-expired-login-tokens"`
	ListConversations        *sqlx.Stmt `query:"list-conversations"`
	GetConversation          *sqlx.Stmt `query:"get-conversation"`
	MarkConversationSeen     *sqlx.Stmt `query:"mark-conversation-seen"`
}

// Manager handles portal login and conversation reads.
type Manager struct {
	db   *sqlx.DB
	q    queries
	lo   *logf.Logger
	i18n *i18n.I18n
	conv ConversationStore
	cfg  Config
}

// New creates and returns a new portal Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}

	if opts.Config.MagicLinkTTL <= 0 {
		opts.Config.MagicLinkTTL = defaultMagicLinkTTL
	}

	return &Manager{
		db:   opts.DB,
		q:    q,
		lo:   opts.Lo,
		i18n: opts.I18n,
		conv: opts.Conversation,
		cfg:  opts.Config,
	}, nil
}

// Enabled reports whether the portal is enabled.
func (m *Manager) Enabled() bool {
	return m.cfg.Enabled
}

// Config returns the portal configuration.
func (m *Manager) Config() Config {
	return m.cfg
}

// GetContactByEmail returns an active contact by email, case-insensitively.
func (m *Manager) GetContactByEmail(email string) (models.Contact, error) {
	var c models.Contact
	if err := m.q.GetContactByEmail.Get(&c, strings.TrimSpace(email)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundUser"), nil)
		}
		m.lo.Error("error fetching portal contact by email", "error", err)
		return c, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return c, nil
}

// GetContactByExternalID returns an active contact by external user ID.
func (m *Manager) GetContactByExternalID(externalUserID string) (models.Contact, error) {
	var c models.Contact
	if err := m.q.GetContactByExternalID.Get(&c, externalUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundUser"), nil)
		}
		m.lo.Error("error fetching portal contact by external id", "error", err)
		return c, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return c, nil
}

// GetContactByID returns an active contact by ID.
func (m *Manager) GetContactByID(id int) (models.Contact, error) {
	var c models.Contact
	if err := m.q.GetContactByID.Get(&c, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundUser"), nil)
		}
		m.lo.Error("error fetching portal contact by id", "error", err)
		return c, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return c, nil
}

// CreateLoginToken creates a single-use login token and returns the raw value.
// Only the SHA-256 hash is stored.
func (m *Manager) CreateLoginToken(contactID int, requestedIP string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))

	if _, err := m.q.InsertLoginToken.Exec(contactID, hash[:], time.Now().Add(m.cfg.MagicLinkTTL), requestedIP); err != nil {
		m.lo.Error("error inserting portal login token", "contact_id", contactID, "error", err)
		return "", envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return token, nil
}

// ConsumeLoginToken validates and burns a login token, returning the contact.
func (m *Manager) ConsumeLoginToken(token string) (models.Contact, error) {
	hash := sha256.Sum256([]byte(strings.TrimSpace(token)))

	var contactID int
	if err := m.q.ConsumeLoginToken.Get(&contactID, hash[:]); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Contact{}, ErrInvalidToken
		}
		m.lo.Error("error consuming portal login token", "error", err)
		return models.Contact{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}

	return m.GetContactByID(contactID)
}

// CleanupExpiredTokens removes tokens older than the retention window.
func (m *Manager) CleanupExpiredTokens() {
	if _, err := m.q.DeleteExpiredLoginTokens.Exec(); err != nil {
		m.lo.Warn("error deleting expired portal login tokens", "error", err)
	}
}

// ListConversations returns the contact's conversations with pagination.
func (m *Manager) ListConversations(contactID int, status, query string, page, pageSize int) ([]models.ConversationListItem, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	status = strings.TrimSpace(strings.ToLower(status))
	query = strings.TrimSpace(query)
	pattern := dbutil.ContainsPattern(query)

	var rows []models.ConversationListItem
	if err := m.q.ListConversations.Select(&rows, contactID, status, query, pattern, pageSize, dbutil.PageOffset(page, pageSize)); err != nil {
		m.lo.Error("error listing portal conversations", "contact_id", contactID, "error", err)
		return nil, 0, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}

	total := 0
	if len(rows) > 0 {
		total = rows[0].Total
	}
	return rows, total, nil
}

// GetConversation returns one of the contact's conversations with public messages.
func (m *Manager) GetConversation(contactID int, uuid string) (models.Conversation, error) {
	var conv models.Conversation
	if err := m.q.GetConversation.Get(&conv, uuid, contactID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conv, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundConversation"), nil)
		}
		m.lo.Error("error fetching portal conversation", "contact_id", contactID, "uuid", uuid, "error", err)
		return conv, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}

	private := false
	msgs, err := m.conv.GetAllConversationMessages(uuid, &private, []string{cmodels.MessageIncoming, cmodels.MessageOutgoing}, 0)
	if err != nil {
		m.lo.Error("error fetching portal conversation messages", "uuid", uuid, "error", err)
		return conv, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}

	conv.Messages = make([]models.Message, 0, len(msgs))
	for _, msg := range msgs {
		m.conv.SignAttachmentURLs(msg.Attachments)
		pm := models.Message{
			UUID:         msg.UUID,
			CreatedAt:    msg.CreatedAt,
			Content:      msg.Content,
			ContentType:  msg.ContentType,
			SenderType:   msg.SenderType,
			AuthorName:   msg.Author.FullName(),
			AuthorAvatar: msg.Author.AvatarURL.String,
			Attachments:  make([]models.Attachment, 0, len(msg.Attachments)),
		}
		for _, a := range msg.Attachments {
			pm.Attachments = append(pm.Attachments, models.Attachment{
				UUID:        a.UUID,
				Name:        a.Name,
				Size:        a.Size,
				ContentType: a.ContentType,
				URL:         a.URL,
			})
		}
		conv.Messages = append(conv.Messages, pm)
	}
	return conv, nil
}

// MarkSeen updates the contact's last-seen timestamp for a conversation.
func (m *Manager) MarkSeen(contactID int, uuid string) error {
	if _, err := m.q.MarkConversationSeen.Exec(uuid, contactID); err != nil {
		m.lo.Error("error marking portal conversation seen", "contact_id", contactID, "uuid", uuid, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}
