-- name: CountInstanceOwners :one
SELECT COUNT(*) FROM instance_roles WHERE role = 'instance_owner' AND revoked_at IS NULL;

-- name: GetUserByNormalizedUsername :one
SELECT id, public_id, username, username_normalized, display_name, password_hash, disabled_at
FROM users WHERE username_normalized = ? LIMIT 1;

-- name: GetUserByID :one
SELECT id, public_id, username, username_normalized, display_name, password_hash, disabled_at
FROM users WHERE id = ? LIMIT 1;

-- name: UserHasInstanceRole :one
SELECT EXISTS(SELECT 1 FROM instance_roles WHERE user_id = ? AND role = ? AND revoked_at IS NULL);
