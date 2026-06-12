-- +goose Up
CREATE TABLE campaigns (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'paused', 'archived')),
    public_link_enabled INTEGER NOT NULL DEFAULT 0,
    created_by_user_id INTEGER NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    archived_at TEXT NULL,
    disabled_at TEXT NULL,
    UNIQUE(organization_id, slug)
);

CREATE INDEX campaigns_organization_id_idx ON campaigns(organization_id);
CREATE INDEX campaigns_status_idx ON campaigns(status);

CREATE TABLE campaign_settings (
    campaign_id INTEGER PRIMARY KEY REFERENCES campaigns(id) ON DELETE CASCADE,
    collect_install_token INTEGER NOT NULL DEFAULT 1,
    hash_install_token INTEGER NOT NULL DEFAULT 1 CHECK (hash_install_token = 1),
    count_raw_visits INTEGER NOT NULL DEFAULT 1,
    count_unique_token_visits INTEGER NOT NULL DEFAULT 1,
    collect_referrer_domain INTEGER NOT NULL DEFAULT 0,
    collect_coarse_browser INTEGER NOT NULL DEFAULT 0,
    collect_coarse_os INTEGER NOT NULL DEFAULT 0,
    public_language_default TEXT NOT NULL DEFAULT 'en' CHECK (public_language_default IN ('en', 'de', 'es')),
    show_privacy_notice INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL,
    updated_by_user_id INTEGER NULL REFERENCES users(id)
);

CREATE TABLE campaign_members (
    id INTEGER PRIMARY KEY,
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'analyst', 'viewer')),
    created_at TEXT NOT NULL,
    created_by_user_id INTEGER NULL REFERENCES users(id),
    UNIQUE(campaign_id, user_id)
);

CREATE INDEX campaign_members_user_id_idx ON campaign_members(user_id);

-- +goose Down
DROP TABLE campaign_members;
DROP TABLE campaign_settings;
DROP TABLE campaigns;
