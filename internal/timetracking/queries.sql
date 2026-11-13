-- name: get-conversation-id
SELECT id FROM conversations WHERE uuid = $1;

-- name: start-timer
INSERT INTO conversation_time_entries (conversation_id, user_id, started_at)
VALUES ($1, $2, NOW())
RETURNING
    id, created_at, updated_at, conversation_id, user_id,
    started_at, ended_at, duration_seconds, note, billable;

-- name: stop-timer
UPDATE conversation_time_entries
SET
    ended_at = NOW(),
    duration_seconds = LEAST(86400, GREATEST(0, EXTRACT(EPOCH FROM (NOW() - started_at))::int)),
    updated_at = NOW()
WHERE user_id = $1 AND ended_at IS NULL
RETURNING id, conversation_id;

-- name: active-timer
SELECT
    e.id, e.created_at, e.updated_at, e.conversation_id, e.user_id,
    e.started_at, e.ended_at, e.duration_seconds, e.note, e.billable,
    CONCAT_WS(' ', u.first_name, NULLIF(u.last_name, '')) AS user_name
FROM conversation_time_entries e
LEFT JOIN users u ON u.id = e.user_id
WHERE e.user_id = $1 AND e.ended_at IS NULL;

-- name: insert-manual
INSERT INTO conversation_time_entries
    (conversation_id, user_id, started_at, ended_at, duration_seconds, note, billable)
VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7)
RETURNING
    id, created_at, updated_at, conversation_id, user_id,
    started_at, ended_at, duration_seconds, note, billable;

-- name: list-entries
SELECT
    e.id, e.created_at, e.updated_at, e.conversation_id, e.user_id,
    e.started_at, e.ended_at, e.duration_seconds, e.note, e.billable,
    CONCAT_WS(' ', u.first_name, NULLIF(u.last_name, '')) AS user_name
FROM conversation_time_entries e
LEFT JOIN users u ON u.id = e.user_id
WHERE e.conversation_id = (SELECT id FROM conversations WHERE uuid = $1)
ORDER BY e.started_at DESC, e.id DESC;

-- name: get-entry
SELECT
    e.id, e.created_at, e.updated_at, e.conversation_id, e.user_id,
    e.started_at, e.ended_at, e.duration_seconds, e.note, e.billable,
    CONCAT_WS(' ', u.first_name, NULLIF(u.last_name, '')) AS user_name
FROM conversation_time_entries e
LEFT JOIN users u ON u.id = e.user_id
WHERE e.id = $1;

-- name: update-entry
UPDATE conversation_time_entries
SET
    duration_seconds = $2,
    note = NULLIF($3, ''),
    billable = $4,
    updated_at = NOW()
WHERE id = $1
RETURNING
    id, created_at, updated_at, conversation_id, user_id,
    started_at, ended_at, duration_seconds, note, billable;

-- name: delete-entry
DELETE FROM conversation_time_entries WHERE id = $1 RETURNING conversation_id;

-- name: recompute-totals
UPDATE conversations c
SET
    time_spent_seconds = COALESCE((
        SELECT SUM(duration_seconds) FROM conversation_time_entries
        WHERE conversation_id = c.id
    ), 0),
    billable_time_seconds = COALESCE((
        SELECT SUM(duration_seconds) FROM conversation_time_entries
        WHERE conversation_id = c.id AND billable
    ), 0)
WHERE c.id = $1;

-- name: get-conversation-totals
SELECT
    COALESCE(SUM(duration_seconds), 0)::int AS total_seconds,
    COALESCE(SUM(CASE WHEN billable THEN duration_seconds ELSE 0 END), 0)::int AS billable_seconds
FROM conversation_time_entries
WHERE conversation_id = (SELECT id FROM conversations WHERE uuid = $1);
