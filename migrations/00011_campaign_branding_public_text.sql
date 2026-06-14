-- +goose Up
ALTER TABLE campaign_branding ADD COLUMN public_heading TEXT NULL;
ALTER TABLE campaign_branding ADD COLUMN public_intro TEXT NULL;

-- +goose Down
ALTER TABLE campaign_branding DROP COLUMN public_heading;
ALTER TABLE campaign_branding DROP COLUMN public_intro;
