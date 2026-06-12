-- name: CreateSession :exec
INSERT INTO sessions (user_id, session_hash, created_at, expires_at, last_seen_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetActiveSessionUser :one
SELECT u.id, u.public_id, u.username, u.username_normalized, u.display_name, u.password_hash, u.disabled_at
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.session_hash = ? AND s.revoked_at IS NULL AND s.expires_at > ? AND u.disabled_at IS NULL;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = ? WHERE session_hash = ? AND revoked_at IS NULL;
