-- +goose Up
ALTER TABLE campaign_settings ADD COLUMN collect_url_context INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_visits ADD COLUMN context_json TEXT NULL;

-- +goose Down
ALTER TABLE campaign_visits DROP COLUMN context_json;
ALTER TABLE campaign_settings DROP COLUMN collect_url_context;
