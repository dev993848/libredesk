package timetracking

import (
	"testing"
	"time"

	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/abhinavxd/libredesk/internal/testutil"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

func newTestManager(t *testing.T, db *sqlx.DB) *Manager {
	t.Helper()
	lo := logf.New(logf.Opts{Level: logf.FatalLevel})
	m, err := New(Opts{DB: db, Lo: &lo, I18n: testutil.NewI18n(t)})
	if err != nil {
		t.Fatalf("initializing time tracking: %v", err)
	}
	return m
}

func seedConversation(t *testing.T, db *sqlx.DB) (conversationID int, conversationUUID string, agentID int) {
	t.Helper()

	var inboxID int
	if err := db.Get(&inboxID, `
		INSERT INTO inboxes (name, channel, enabled, config)
		VALUES ('Test inbox', 'livechat', true, '{}'::jsonb) RETURNING id`); err != nil {
		t.Fatalf("inserting inbox: %v", err)
	}

	var contactID int
	if err := db.Get(&contactID, `
		INSERT INTO users (type, first_name, email, enabled)
		VALUES ('contact', 'Ivan', 'ivan@example.com', true) RETURNING id`); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}

	if err := db.Get(&agentID, `
		INSERT INTO users (type, first_name, email, enabled)
		VALUES ('agent', 'System', 'system@test.local', true) RETURNING id`); err != nil {
		t.Fatalf("inserting agent: %v", err)
	}

	if err := db.Get(&conversationID, `
		INSERT INTO conversations (contact_id, inbox_id, status_id, priority_id, subject)
		VALUES ($1, $2, (SELECT id FROM conversation_statuses WHERE name = 'Open'), 2, 'Ticket')
		RETURNING id`, contactID, inboxID); err != nil {
		t.Fatalf("inserting conversation: %v", err)
	}
	if err := db.Get(&conversationUUID, `SELECT uuid FROM conversations WHERE id = $1`, conversationID); err != nil {
		t.Fatalf("fetching conversation uuid: %v", err)
	}
	return conversationID, conversationUUID, agentID
}

func TestTimerStopAndManualEntryUpdateTotals(t *testing.T) {
	db := testutil.NewDB(t, "time_tracking")
	m := newTestManager(t, db)
	conversationID, uuid, agentID := seedConversation(t, db)

	if _, err := m.Start(uuid, agentID); err != nil {
		t.Fatalf("starting timer: %v", err)
	}

	// A second timer for the same agent must be rejected.
	if _, err := m.Start(uuid, agentID); err == nil {
		t.Fatal("expected a conflict for a second running timer")
	} else if e, ok := err.(envelope.Error); !ok || e.ErrorType != envelope.ConflictError {
		t.Fatalf("expected conflict error, got %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	stopped, err := m.Stop(agentID)
	if err != nil {
		t.Fatalf("stopping timer: %v", err)
	}
	if stopped.DurationSeconds < 1 {
		t.Fatalf("expected a positive duration, got %d", stopped.DurationSeconds)
	}

	if _, err := m.AddManual(uuid, agentID, 1800, "log review", true, time.Time{}); err != nil {
		t.Fatalf("adding manual entry: %v", err)
	}

	totals, err := m.Totals(uuid)
	if err != nil {
		t.Fatalf("fetching totals: %v", err)
	}
	if totals.TotalSeconds != 1800+stopped.DurationSeconds {
		t.Fatalf("unexpected total: %d", totals.TotalSeconds)
	}

	var spent int
	if err := db.Get(&spent, `SELECT time_spent_seconds FROM conversations WHERE id = $1`, conversationID); err != nil {
		t.Fatalf("fetching conversation total: %v", err)
	}
	if spent != totals.TotalSeconds {
		t.Fatalf("conversation total = %d, want %d", spent, totals.TotalSeconds)
	}
}

func TestDeleteEntryRecomputesTotals(t *testing.T) {
	db := testutil.NewDB(t, "time_tracking_delete")
	m := newTestManager(t, db)
	_, uuid, agentID := seedConversation(t, db)

	entry, err := m.AddManual(uuid, agentID, 600, "", true, time.Time{})
	if err != nil {
		t.Fatalf("adding entry: %v", err)
	}
	if _, err := m.Delete(entry.ID); err != nil {
		t.Fatalf("deleting entry: %v", err)
	}
	totals, err := m.Totals(uuid)
	if err != nil {
		t.Fatalf("fetching totals: %v", err)
	}
	if totals.TotalSeconds != 0 {
		t.Fatalf("expected zero total after delete, got %d", totals.TotalSeconds)
	}
}
