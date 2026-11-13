package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_11_0 adds the customer portal (login tokens) and conversation time tracking.
func V2_11_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS portal_login_tokens (
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			contact_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash BYTEA NOT NULL UNIQUE,
			expires_at TIMESTAMPTZ NOT NULL,
			used_at TIMESTAMPTZ NULL,
			requested_ip TEXT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS index_portal_login_tokens_on_contact_id
			ON portal_login_tokens (contact_id);`,
		`CREATE INDEX IF NOT EXISTS index_portal_login_tokens_on_expires_at
			ON portal_login_tokens (expires_at);`,

		`CREATE TABLE IF NOT EXISTS conversation_time_entries (
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
			user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
			started_at TIMESTAMPTZ NOT NULL,
			ended_at TIMESTAMPTZ NULL,
			duration_seconds INT NOT NULL DEFAULT 0,
			note TEXT NULL,
			billable BOOL NOT NULL DEFAULT TRUE,
			CONSTRAINT constraint_time_entries_duration
				CHECK (duration_seconds >= 0 AND duration_seconds <= 86400)
		);`,
		`CREATE INDEX IF NOT EXISTS index_time_entries_on_conversation_id
			ON conversation_time_entries (conversation_id);`,
		`CREATE INDEX IF NOT EXISTS index_time_entries_on_user_id
			ON conversation_time_entries (user_id);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS index_time_entries_one_running_per_user
			ON conversation_time_entries (user_id) WHERE ended_at IS NULL;`,

		`ALTER TABLE conversations
			ADD COLUMN IF NOT EXISTS time_spent_seconds INT NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS billable_time_seconds INT NOT NULL DEFAULT 0;`,

		`UPDATE roles
			SET permissions = array_append(permissions, 'time_tracking:manage')
			WHERE name = 'Admin' AND NOT ('time_tracking:manage' = ANY(permissions));`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
