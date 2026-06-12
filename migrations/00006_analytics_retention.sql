-- +goose Up
ALTER TABLE campaign_settings ADD COLUMN retention_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_settings ADD COLUMN retention_days INTEGER NULL CHECK (retention_days IN (30, 90, 180, 365));

-- +goose Down
ALTER TABLE campaign_settings DROP COLUMN retention_days;
ALTER TABLE campaign_settings DROP COLUMN retention_enabled;
