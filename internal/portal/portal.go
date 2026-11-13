// Package portal implements the customer-facing portal: magic-link login and
// read access to a contact's own conversations.
package portal

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/abhinavxd/libredesk/internal/attachment"
	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	"github.com/abhinavxd/libredesk/internal/dbutil"
	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/abhinavxd/libredesk/internal/portal/models"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/volatiletech/null/v9"
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
	GetConversation(id int, uuid, refNum string) (cmodels.Conversation, error)
	GetAllConversationMessages(conversationUUID string, private *bool, msgTypes []string, limit int) ([]cmodels.Message, error)
	SignAttachmentURLs(attachments attachment.Attachments)
	ProcessIncomingMessage(in cmodels.IncomingMessage) (cmodels.Message, error)
	ProcessIncomingLiveChatMessage(msg cmodels.Message) (cmodels.Message, error)
	ProcessIncomingWhatsAppMessage(msg cmodels.Message, isNewConversation bool, inboundAt time.Time) (cmodels.Message, error)
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
	ValidateLoginToken       *sqlx.Stmt `query:"validate-login-token"`
	ConsumeLoginToken        *sqlx.Stmt `query:"consume-login-token"`
	DeleteExpiredLoginTokens *sqlx.Stmt `query:"delete-expired-login-tokens"`
	ListConversations        *sqlx.Stmt `query:"list-conversations"`
	GetConversation          *sqlx.Stmt `query:"get-conversation"`
	MarkConversationSeen     *sqlx.Stmt `query:"mark-conversation-seen"`
	GetInbox                 *sqlx.Stmt `query:"get-inbox"`
}

// Manager handles portal login and conversation reads.
type Manager struct {
	db   *sqlx.DB
	q    queries
	lo   *logf.Logger
	i18n *i18n.I18n
	conv ConversationStore
	mu   sync.RWMutex
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
	return m.config().Enabled
}

// Config returns a copy of the portal configuration.
func (m *Manager) Config() Config {
	return m.config()
}

func (m *Manager) config() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// Reload replaces the runtime configuration, e.g. after settings are saved.
func (m *Manager) Reload(cfg Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg.MagicLinkTTL <= 0 {
		cfg.MagicLinkTTL = defaultMagicLinkTTL
	}
	m.cfg = cfg
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

	if _, err := m.q.InsertLoginToken.Exec(contactID, hash[:], time.Now().Add(m.config().MagicLinkTTL), requestedIP); err != nil {
		m.lo.Error("error inserting portal login token", "contact_id", contactID, "error", err)
		return "", envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return token, nil
}

// ValidateLoginToken checks a login token without burning it. It is used by the
// confirmation page so mail scanners that prefetch links do not consume the token.
func (m *Manager) ValidateLoginToken(token string) (models.Contact, error) {
	hash := sha256.Sum256([]byte(strings.TrimSpace(token)))

	var contactID int
	if err := m.q.ValidateLoginToken.Get(&contactID, hash[:]); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Contact{}, ErrInvalidToken
		}
		m.lo.Error("error validating portal login token", "error", err)
		return models.Contact{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	contact, err := m.GetContactByID(contactID)
	if err != nil {
		return models.Contact{}, ErrInvalidToken
	}
	return contact, nil
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

// inbox is the subset of an inbox needed to start a portal conversation.
type inbox struct {
	ID                int             `db:"id"`
	Name              string          `db:"name"`
	Channel           string          `db:"channel"`
	Config            json.RawMessage `db:"config"`
	ReopenWindowHours int             `db:"reopen_window_hours"`
}

// GetInbox returns an enabled inbox by ID.
func (m *Manager) GetInbox(id int) (inbox, error) {
	var inb inbox
	if err := m.q.GetInbox.Get(&inb, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return inb, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundInbox"), nil)
		}
		m.lo.Error("error fetching portal inbox", "inbox_id", id, "error", err)
		return inb, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return inb, nil
}

// CreateConversation starts a new conversation for the contact in the configured inbox.
func (m *Manager) CreateConversation(contact models.Contact, subject, content string) (string, error) {
	if m.config().DefaultInboxID <= 0 {
		return "", envelope.NewError(envelope.InputError, m.i18n.T("portal.noInbox"), nil)
	}

	inb, err := m.GetInbox(m.config().DefaultInboxID)
	if err != nil {
		return "", err
	}

	source := make([]byte, 16)
	if _, err := rand.Read(source); err != nil {
		return "", envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}

	msg, err := m.conv.ProcessIncomingMessage(cmodels.IncomingMessage{
		Channel: inb.Channel,
		InboxID: inb.ID,
		Contact: cmodels.IncomingContact{
			ID:        contact.ID,
			FirstName: contact.FirstName,
			LastName:  contact.LastName,
			Email:     null.StringFrom(contact.Email),
		},
		Subject:     strings.TrimSpace(subject),
		SourceID:    null.StringFrom("portal:" + hex.EncodeToString(source)),
		Content:     content,
		ContentType: cmodels.ContentTypeText,
	})
	if err != nil {
		m.lo.Error("error creating conversation from portal", "contact_id", contact.ID, "error", err)
		return "", envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return msg.ConversationUUID, nil
}

// Reply adds a public contact reply to one of the contact's conversations.
func (m *Manager) Reply(contactID int, uuid, content string) (string, error) {
	conv, err := m.conv.GetConversation(0, uuid, "")
	if err != nil {
		return "", envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundConversation"), nil)
	}
	if conv.ContactID != contactID {
		return "", envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundConversation"), nil)
	}
	if err := m.canReply(conv); err != nil {
		return "", err
	}

	msg, err := m.ProcessContactMessage(conv, cmodels.Message{
		ConversationUUID: conv.UUID,
		ConversationID:   conv.ID,
		SenderID:         contactID,
		Type:             cmodels.MessageIncoming,
		SenderType:       cmodels.SenderTypeContact,
		Status:           cmodels.MessageStatusReceived,
		Content:          content,
		ContentType:      cmodels.ContentTypeText,
		Private:          false,
	})
	if err != nil {
		m.lo.Error("error inserting portal reply", "contact_id", contactID, "uuid", uuid, "error", err)
		return "", envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.errorSendingMessage"), nil)
	}
	return msg.UUID, nil
}

// ProcessContactMessage stores an inbound contact message using the conversation's channel rules.
func (m *Manager) ProcessContactMessage(conv cmodels.Conversation, msg cmodels.Message) (cmodels.Message, error) {
	if conv.InboxChannel == "whatsapp" {
		return m.conv.ProcessIncomingWhatsAppMessage(msg, false, time.Now())
	}
	return m.conv.ProcessIncomingLiveChatMessage(msg)
}

// CanReply enforces channel-specific reply rules for customer messages.
func (m *Manager) CanReply(conv cmodels.Conversation) error {
	return m.canReply(conv)
}

// canReply enforces channel-specific reply rules for customer replies.
func (m *Manager) canReply(conv cmodels.Conversation) error {
	switch conv.InboxChannel {
	case "whatsapp":
		if !conv.LastInboundAt.Valid || time.Since(conv.LastInboundAt.Time) > 24*time.Hour {
			return envelope.NewError(envelope.ConflictError, m.i18n.T("conversation.whatsapp.error.windowClosed"), nil)
		}
	case "livechat":
		if conv.Status.String != "Closed" {
			return nil
		}
		inb, err := m.GetInbox(conv.InboxID)
		if err != nil {
			return err
		}
		var cfg struct {
			PreventReplyToClosedConversation bool `json:"prevent_reply_to_closed_conversation"`
		}
		if err := json.Unmarshal(inb.Config, &cfg); err != nil {
			m.lo.Warn("error parsing livechat inbox config for portal reply", "inbox_id", inb.ID, "error", err)
			return nil
		}
		if cfg.PreventReplyToClosedConversation {
			return envelope.NewError(envelope.ConflictError, m.i18n.T("portal.replyClosed"), nil)
		}
	}
	return nil
}
