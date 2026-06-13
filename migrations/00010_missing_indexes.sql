-- +goose Up
CREATE INDEX IF NOT EXISTS idx_organizations_created_by ON organizations(created_by_user_id);
CREATE INDEX IF NOT EXISTS idx_campaign_submissions_visit_id ON campaign_submissions(visit_id);

-- +goose Down
DROP INDEX IF EXISTS idx_campaign_submissions_visit_id;
DROP INDEX IF EXISTS idx_organizations_created_by;
