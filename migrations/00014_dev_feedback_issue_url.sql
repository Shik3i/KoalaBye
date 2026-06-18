-- +goose Up
ALTER TABLE campaign_settings ADD COLUMN dev_feedback_issue_url TEXT NULL;

-- +goose Down
ALTER TABLE campaign_settings DROP COLUMN dev_feedback_issue_url;
