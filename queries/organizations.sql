-- name: ListOrganizationsForUser :many
SELECT o.id, o.public_id, o.slug, o.name, om.role,
  (SELECT COUNT(*) FROM organization_members count_members WHERE count_members.organization_id = o.id) AS member_count
FROM organizations o JOIN organization_members om ON om.organization_id = o.id
WHERE om.user_id = ? AND o.disabled_at IS NULL ORDER BY o.name;

-- name: UserCanAccessOrganization :one
SELECT EXISTS(SELECT 1 FROM organization_members om JOIN organizations o ON o.id = om.organization_id
WHERE om.user_id = ? AND om.organization_id = ? AND o.disabled_at IS NULL);
