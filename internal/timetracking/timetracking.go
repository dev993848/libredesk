// Package timetracking manages time entries recorded against conversations.
package timetracking

import (
	"database/sql"
	"embed"
	"errors"
	"time"

	"github.com/abhinavxd/libredesk/internal/dbutil"
	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

// Opts contains options for initializing the Manager.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18n.I18n
}

type queries struct {
	GetConversationID     *sqlx.Stmt `query:"get-conversation-id"`
	StartTimer            *sqlx.Stmt `query:"start-timer"`
	StopTimer             *sqlx.Stmt `query:"stop-timer"`
	ActiveTimer           *sqlx.Stmt `query:"active-timer"`
	InsertManual          *sqlx.Stmt `query:"insert-manual"`
	ListEntries           *sqlx.Stmt `query:"list-entries"`
	GetEntry              *sqlx.Stmt `query:"get-entry"`
	UpdateEntry           *sqlx.Stmt `query:"update-entry"`
	DeleteEntry           *sqlx.Stmt `query:"delete-entry"`
	RecomputeTotals       *sqlx.Stmt `query:"recompute-totals"`
	GetConversationTotals *sqlx.Stmt `query:"get-conversation-totals"`
}

// Manager handles conversation time entries.
type Manager struct {
	db   *sqlx.DB
	q    queries
	lo   *logf.Logger
	i18n *i18n.I18n
}

// Entry is a single time entry on a conversation.
type Entry struct {
	ID              int         `db:"id" json:"id"`
	CreatedAt       time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time   `db:"updated_at" json:"updated_at"`
	ConversationID  int         `db:"conversation_id" json:"conversation_id"`
	UserID          null.Int    `db:"user_id" json:"user_id"`
	UserName        string      `db:"user_name" json:"user_name"`
	StartedAt       time.Time   `db:"started_at" json:"started_at"`
	EndedAt         null.Time   `db:"ended_at" json:"ended_at"`
	DurationSeconds int         `db:"duration_seconds" json:"duration_seconds"`
	Note            null.String `db:"note" json:"note"`
	Billable        bool        `db:"billable" json:"billable"`
}

// Totals is the aggregated time for a conversation.
type Totals struct {
	TotalSeconds    int `db:"total_seconds" json:"total_seconds"`
	BillableSeconds int `db:"billable_seconds" json:"billable_seconds"`
}

// New creates and returns a new time tracking Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{db: opts.DB, q: q, lo: opts.Lo, i18n: opts.I18n}, nil
}

// Start starts a timer for the current agent on a conversation.
func (m *Manager) Start(conversationUUID string, userID int) (Entry, error) {
	var convID int
	if err := m.q.GetConversationID.Get(&convID, conversationUUID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundConversation"), nil)
		}
		return Entry{}, m.generalError(err)
	}

	var entry Entry
	if err := m.q.StartTimer.Get(&entry, convID, userID); err != nil {
		if dbutil.IsUniqueViolationError(err) {
			return Entry{}, envelope.NewError(envelope.ConflictError, m.i18n.T("timeTracking.timerAlreadyRunning"), nil)
		}
		return Entry{}, m.generalError(err)
	}
	return entry, nil
}

// Stop stops the agent's running timer and returns the finished entry.
func (m *Manager) Stop(userID int) (Entry, error) {
	var stopped struct {
		ID             int `db:"id"`
		ConversationID int `db:"conversation_id"`
	}
	if err := m.q.StopTimer.Get(&stopped, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, envelope.NewError(envelope.NotFoundError, m.i18n.T("timeTracking.noRunningTimer"), nil)
		}
		return Entry{}, m.generalError(err)
	}
	if err := m.Recompute(stopped.ConversationID); err != nil {
		return Entry{}, err
	}
	return m.Get(stopped.ID)
}

// AddManual records a completed time entry.
func (m *Manager) AddManual(conversationUUID string, userID, durationSeconds int, note string, billable bool, startedAt time.Time) (Entry, error) {
	if durationSeconds < 1 || durationSeconds > 86400 {
		return Entry{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.invalidDuration"), nil)
	}
	var convID int
	if err := m.q.GetConversationID.Get(&convID, conversationUUID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundConversation"), nil)
		}
		return Entry{}, m.generalError(err)
	}
	if startedAt.IsZero() {
		startedAt = time.Now().Add(-time.Duration(durationSeconds) * time.Second)
	}
	endedAt := startedAt.Add(time.Duration(durationSeconds) * time.Second)

	var entry Entry
	if err := m.q.InsertManual.Get(&entry, convID, userID, startedAt, endedAt, durationSeconds, note, billable); err != nil {
		return Entry{}, m.generalError(err)
	}
	if err := m.Recompute(convID); err != nil {
		return Entry{}, err
	}
	return m.Get(entry.ID)
}

// List returns all time entries of a conversation, newest first.
func (m *Manager) List(conversationUUID string) ([]Entry, error) {
	entries := make([]Entry, 0)
	if err := m.q.ListEntries.Select(&entries, conversationUUID); err != nil {
		return nil, m.generalError(err)
	}
	return entries, nil
}

// Totals returns the aggregated time of a conversation.
func (m *Manager) Totals(conversationUUID string) (Totals, error) {
	var t Totals
	if err := m.q.GetConversationTotals.Get(&t, conversationUUID); err != nil {
		return t, m.generalError(err)
	}
	return t, nil
}

// Get returns a single entry.
func (m *Manager) Get(id int) (Entry, error) {
	var e Entry
	if err := m.q.GetEntry.Get(&e, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return e, envelope.NewError(envelope.NotFoundError, m.i18n.T("timeTracking.notFound"), nil)
		}
		return e, m.generalError(err)
	}
	return e, nil
}

// Update changes the duration, note and billable flag of an entry.
func (m *Manager) Update(id, durationSeconds int, note string, billable bool) (Entry, error) {
	if durationSeconds < 1 || durationSeconds > 86400 {
		return Entry{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.invalidDuration"), nil)
	}
	existing, err := m.Get(id)
	if err != nil {
		return Entry{}, err
	}
	if _, err := m.q.UpdateEntry.Exec(id, durationSeconds, note, billable); err != nil {
		return Entry{}, m.generalError(err)
	}
	if err := m.Recompute(existing.ConversationID); err != nil {
		return Entry{}, err
	}
	return m.Get(id)
}

// Delete removes an entry and returns its conversation ID.
func (m *Manager) Delete(id int) (int, error) {
	var convID int
	if err := m.q.DeleteEntry.Get(&convID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, envelope.NewError(envelope.NotFoundError, m.i18n.T("timeTracking.notFound"), nil)
		}
		return 0, m.generalError(err)
	}
	if err := m.Recompute(convID); err != nil {
		return 0, err
	}
	return convID, nil
}

// Active returns the agent's running timer, if any.
func (m *Manager) Active(userID int) (Entry, bool, error) {
	var e Entry
	if err := m.q.ActiveTimer.Get(&e, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, false, nil
		}
		return Entry{}, false, m.generalError(err)
	}
	return e, true, nil
}

// Recompute refreshes the denormalized totals on a conversation.
func (m *Manager) Recompute(conversationID int) error {
	if _, err := m.q.RecomputeTotals.Exec(conversationID); err != nil {
		return m.generalError(err)
	}
	return nil
}

func (m *Manager) generalError(err error) error {
	m.lo.Error("time tracking error", "error", err)
	return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
}
