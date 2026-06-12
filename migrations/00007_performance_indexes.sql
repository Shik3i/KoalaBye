-- +goose Up
CREATE INDEX organization_members_user_id_idx ON organization_members(user_id);

-- +goose Down
DROP INDEX organization_members_user_id_idx;
