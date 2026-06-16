-- +goose Up
ALTER TABLE campaign_branding ADD COLUMN custom_css TEXT NULL;

-- +goose Down
ALTER TABLE campaign_branding DROP COLUMN custom_css;
