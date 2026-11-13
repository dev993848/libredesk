package portal

import (
	"errors"
	"testing"
	"time"

	"github.com/abhinavxd/libredesk/internal/attachment"
	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/abhinavxd/libredesk/internal/testutil"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

type fakeConversationStore struct {
	messages []cmodels.Message
}

func (f *fakeConversationStore) GetAllConversationMessages(string, *bool, []string, int) ([]cmodels.Message, error) {
	return f.messages, nil
}

func (f *fakeConversationStore) SignAttachmentURLs(attachment.Attachments) {}

func (f *fakeConversationStore) GetConversation(int, string, string) (cmodels.Conversation, error) {
	return cmodels.Conversation{}, nil
}

func (f *fakeConversationStore) ProcessIncomingMessage(cmodels.IncomingMessage) (cmodels.Message, error) {
	return cmodels.Message{}, nil
}

func (f *fakeConversationStore) ProcessIncomingLiveChatMessage(cmodels.Message) (cmodels.Message, error) {
	return cmodels.Message{}, nil
}

func loPtr(l logf.Logger) *logf.Logger { return &l }

func newTestManager(t *testing.T, db *sqlx.DB, store ConversationStore) *Manager {
	t.Helper()
	m, err := New(Opts{
		DB:           db,
		Lo:           loPtr(logf.New(logf.Opts{Level: logf.FatalLevel})),
		I18n:         testutil.NewI18n(t),
		Conversation: store,
		Config:       Config{Enabled: true, MagicLinkTTL: time.Minute},
	})
	if err != nil {
		t.Fatalf("initializing portal manager: %v", err)
	}
	return m
}

func insertContact(t *testing.T, db *sqlx.DB, name, email string) int {
	t.Helper()
	var id int
	if err := db.Get(&id, `
		INSERT INTO users (type, first_name, email, enabled)
		VALUES ('contact', $1, $2, true) RETURNING id`, name, email); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}
	return id
}

func insertAgent(t *testing.T, db *sqlx.DB) int {
	t.Helper()
	var id int
	if err := db.Get(&id, `
		INSERT INTO users (type, first_name, email, enabled)
		VALUES ('agent', 'System', 'system@test.local', true) RETURNING id`); err != nil {
		t.Fatalf("inserting agent: %v", err)
	}
	return id
}

func insertInbox(t *testing.T, db *sqlx.DB) int {
	t.Helper()
	var id int
	if err := db.Get(&id, `
		INSERT INTO inboxes (name, channel, enabled, config)
		VALUES ('Test inbox', 'livechat', true, '{}'::jsonb) RETURNING id`); err != nil {
		t.Fatalf("inserting inbox: %v", err)
	}
	return id
}

func insertConversation(t *testing.T, db *sqlx.DB, contactID, inboxID int, subject string, lastSeenAgo time.Duration, timeSpent int) (int, string) {
	t.Helper()
	var row struct {
		ID   int    `db:"id"`
		UUID string `db:"uuid"`
	}
	if err := db.Get(&row, `
		INSERT INTO conversations
			(contact_id, inbox_id, status_id, priority_id, subject, last_message,
			 last_message_sender, last_message_at, contact_last_seen_at, time_spent_seconds)
		VALUES ($1, $2, (SELECT id FROM conversation_statuses WHERE name = 'Open'), 2, $3,
			'Last message', 'agent', NOW(), NOW() - ($4 * INTERVAL '1 second'), $5)
		RETURNING id, uuid`, contactID, inboxID, subject, int(lastSeenAgo.Seconds()), timeSpent); err != nil {
		t.Fatalf("inserting conversation: %v", err)
	}
	return row.ID, row.UUID
}

func insertMessage(t *testing.T, db *sqlx.DB, conversationID int, senderID int, senderType, content string, private bool) {
	t.Helper()
	msgType, status := "incoming", "received"
	if senderType == "agent" {
		msgType, status = "outgoing", "sent"
	}
	if _, err := db.Exec(`
		INSERT INTO conversation_messages
			(type, status, conversation_id, content, text_content, sender_id, sender_type, private, content_type)
		VALUES ($1, $2, $3, $4, $4, $5, $6, $7, 'text')`,
		msgType, status, conversationID, content, senderID, senderType, private); err != nil {
		t.Fatalf("inserting message: %v", err)
	}
}

func TestLoginTokenIsSingleUse(t *testing.T) {
	db := testutil.NewDB(t, "portal_token")
	m := newTestManager(t, db, &fakeConversationStore{})
	contactID := insertContact(t, db, "Ivan", "ivan@example.com")

	token, err := m.CreateLoginToken(contactID, "127.0.0.1")
	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	contact, err := m.ConsumeLoginToken(token)
	if err != nil {
		t.Fatalf("consuming token: %v", err)
	}
	if contact.ID != contactID || contact.Email != "ivan@example.com" {
		t.Fatalf("unexpected contact: %+v", contact)
	}

	if _, err := m.ConsumeLoginToken(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken on reuse, got %v", err)
	}
}

func TestConsumeLoginTokenRejectsUnknownToken(t *testing.T) {
	db := testutil.NewDB(t, "portal_bad_token")
	m := newTestManager(t, db, &fakeConversationStore{})

	if _, err := m.ConsumeLoginToken("not-a-real-token"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestListConversationsIsScopedToContact(t *testing.T) {
	db := testutil.NewDB(t, "portal_scope")
	m := newTestManager(t, db, &fakeConversationStore{})
	inboxID := insertInbox(t, db)
	agentID := insertAgent(t, db)

	ivan := insertContact(t, db, "Ivan", "ivan@example.com")
	anna := insertContact(t, db, "Anna", "anna@example.com")

	ivanConvID, ivanUUID := insertConversation(t, db, ivan, inboxID, "Ivan's ticket", 24*time.Hour, 5400)
	insertMessage(t, db, ivanConvID, ivan, "contact", "Help me", false)
	insertMessage(t, db, ivanConvID, agentID, "agent", "Sure", false)
	_, annaUUID := insertConversation(t, db, anna, inboxID, "Anna's ticket", 24*time.Hour, 60)

	items, total, err := m.ListConversations(ivan, "", "", 1, 20)
	if err != nil {
		t.Fatalf("listing conversations: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected one conversation, got total=%d len=%d", total, len(items))
	}
	if items[0].UUID != ivanUUID {
		t.Fatalf("expected contact's conversation, got %s", items[0].UUID)
	}
	if items[0].TimeSpentSeconds != 5400 {
		t.Fatalf("expected time spent 5400, got %d", items[0].TimeSpentSeconds)
	}
	if items[0].UnreadCount != 1 {
		t.Fatalf("expected one unread agent message, got %d", items[0].UnreadCount)
	}

	// Another contact's conversation must not be readable.
	if _, err := m.GetConversation(ivan, annaUUID); err == nil {
		t.Fatal("expected access to another contact's conversation to fail")
	} else if e, ok := err.(envelope.Error); !ok || e.ErrorType != envelope.NotFoundError {
		t.Fatalf("expected NotFound error, got %v", err)
	}
}

func TestGetConversationMapsPublicMessages(t *testing.T) {
	db := testutil.NewDB(t, "portal_detail")
	inboxID := insertInbox(t, db)
	ivan := insertContact(t, db, "Ivan", "ivan@example.com")
	convID, uuid := insertConversation(t, db, ivan, inboxID, "Ticket", time.Hour, 60)
	insertMessage(t, db, convID, ivan, "contact", "Hello", false)

	store := &fakeConversationStore{messages: []cmodels.Message{
		{
			UUID:        "11111111-1111-1111-1111-111111111111",
			CreatedAt:   time.Now(),
			Content:     "Hello",
			ContentType: "text",
			SenderType:  "contact",
			Author:      cmodels.MessageAuthor{FirstName: "Ivan", LastName: "Petrov"},
		},
		{
			UUID:        "22222222-2222-2222-2222-222222222222",
			CreatedAt:   time.Now(),
			Content:     "Hi",
			ContentType: "text",
			SenderType:  "agent",
			Author:      cmodels.MessageAuthor{FirstName: "System"},
		},
	}}
	m := newTestManager(t, db, store)

	conv, err := m.GetConversation(ivan, uuid)
	if err != nil {
		t.Fatalf("getting conversation: %v", err)
	}
	if conv.Subject != "Ticket" || conv.TimeSpentSeconds != 60 {
		t.Fatalf("unexpected conversation: %+v", conv)
	}
	if len(conv.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(conv.Messages))
	}
	if conv.Messages[0].AuthorName != "Ivan Petrov" || conv.Messages[1].AuthorName != "System" {
		t.Fatalf("unexpected authors: %+v", conv.Messages)
	}
}
