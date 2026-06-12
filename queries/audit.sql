-- name: CreateAuditEvent :exec
INSERT INTO audit_log (actor_user_id, organization_id, action, target_type, target_id, reason, metadata_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListRecentAuditEvents :many
SELECT id, actor_user_id, organization_id, action, target_type, target_id, reason, metadata_json, created_at
FROM audit_log ORDER BY id DESC LIMIT ?;
