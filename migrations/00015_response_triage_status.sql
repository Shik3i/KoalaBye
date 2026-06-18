-- +goose Up
ALTER TABLE campaign_submissions ADD COLUMN triage_status TEXT NOT NULL DEFAULT 'new' CHECK (triage_status IN ('new', 'reviewed', 'actionable', 'closed'));

-- +goose Down
ALTER TABLE campaign_submissions DROP COLUMN triage_status;
