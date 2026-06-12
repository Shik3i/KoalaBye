-- +goose Up
CREATE TABLE campaign_branding (
    campaign_id INTEGER PRIMARY KEY REFERENCES campaigns(id) ON DELETE CASCADE,
    brand_name TEXT NULL,
    brand_url TEXT NULL,
    privacy_policy_url TEXT NULL,
    legal_notice_url TEXT NULL,
    support_url TEXT NULL,
    contact_url TEXT NULL,
    accent_preset TEXT NOT NULL DEFAULT 'default' CHECK (accent_preset IN ('default', 'purple', 'blue', 'green', 'orange', 'gray')),
    background_style TEXT NOT NULL DEFAULT 'theme-default' CHECK (background_style IN ('theme-default', 'theme-light', 'theme-dark', 'theme-soft')),
    show_koalabye_branding INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL,
    updated_by_user_id INTEGER NULL REFERENCES users(id)
);

-- +goose Down
DROP TABLE campaign_branding;
