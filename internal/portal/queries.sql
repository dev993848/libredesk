-- name: get-contact-by-email
SELECT
    id,
    first_name,
    COALESCE(last_name, '') AS last_name,
    COALESCE(email, '') AS email,
    COALESCE(avatar_url, '') AS avatar_url
FROM
    users
WHERE
    type = 'contact'
    AND deleted_at IS NULL
    AND enabled = true
    AND LOWER(email) = LOWER($1)
ORDER BY
    id DESC
LIMIT 1;

-- name: get-contact-by-external-id
SELECT
    id,
    first_name,
    COALESCE(last_name, '') AS last_name,
    COALESCE(email, '') AS email,
    COALESCE(avatar_url, '') AS avatar_url
FROM
    users
WHERE
    type = 'contact'
    AND deleted_at IS NULL
    AND enabled = true
    AND external_user_id = $1
ORDER BY
    id DESC
LIMIT 1;

-- name: get-contact-by-id
SELECT
    id,
    first_name,
    COALESCE(last_name, '') AS last_name,
    COALESCE(email, '') AS email,
    COALESCE(avatar_url, '') AS avatar_url
FROM
    users
WHERE
    id = $1
    AND type = 'contact'
    AND deleted_at IS NULL
    AND enabled = true;

-- name: insert-login-token
INSERT INTO portal_login_tokens (contact_id, token_hash, expires_at, requested_ip)
VALUES ($1, $2, $3, NULLIF($4, ''));

-- name: consume-login-token
UPDATE portal_login_tokens
SET used_at = NOW()
WHERE token_hash = $1
    AND used_at IS NULL
    AND expires_at > NOW()
RETURNING contact_id;

-- name: delete-expired-login-tokens
DELETE FROM portal_login_tokens WHERE expires_at < NOW() - INTERVAL '7 days';

-- name: list-conversations
SELECT
    c.uuid,
    c.reference_number,
    COALESCE(c.subject, '') AS subject,
    s.name AS "status.name",
    s.category AS "status.category",
    i.name AS inbox_name,
    i.channel AS inbox_channel,
    c.created_at,
    c.last_message_at,
    COALESCE(c.last_message, '') AS last_message_preview,
    c.time_spent_seconds,
    (
        SELECT COUNT(*)
        FROM conversation_messages m
        WHERE m.conversation_id = c.id
            AND m.sender_type = 'agent'
            AND m.private = false
            AND m.type IN ('incoming', 'outgoing')
            AND m.created_at > COALESCE(c.contact_last_seen_at, c.created_at)
    ) AS unread_count,
    COUNT(*) OVER() AS total
FROM
    conversations c
    JOIN conversation_statuses s ON s.id = c.status_id
    JOIN inboxes i ON i.id = c.inbox_id
WHERE
    c.contact_id = $1
    AND ($2 = '' OR s.category::text = $2)
    AND ($3 = '' OR c.subject ILIKE $4 ESCAPE '\' OR c.reference_number ILIKE $4 ESCAPE '\')
ORDER BY
    COALESCE(c.last_message_at, c.created_at) DESC
LIMIT NULLIF($5, 0) OFFSET $6;

-- name: get-conversation
SELECT
    c.uuid,
    c.reference_number,
    COALESCE(c.subject, '') AS subject,
    s.name AS "status.name",
    s.category AS "status.category",
    i.name AS inbox_name,
    i.channel AS inbox_channel,
    c.created_at,
    c.last_message_at,
    c.resolved_at,
    c.closed_at,
    c.time_spent_seconds
FROM
    conversations c
    JOIN conversation_statuses s ON s.id = c.status_id
    JOIN inboxes i ON i.id = c.inbox_id
WHERE
    c.uuid = $1
    AND c.contact_id = $2;

-- name: mark-conversation-seen
UPDATE conversations
SET contact_last_seen_at = NOW()
WHERE uuid = $1 AND contact_id = $2;

-- name: get-inbox
SELECT
    id,
    "name",
    channel,
    config,
    reopen_window_hours
FROM
    inboxes
WHERE
    id = $1
    AND deleted_at IS NULL
    AND enabled = true;
