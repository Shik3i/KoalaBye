-- +goose Up
CREATE TABLE organization_limits (
    organization_id INTEGER PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    max_campaigns INTEGER NOT NULL,
    max_members INTEGER NOT NULL,
    max_active_invites INTEGER NOT NULL,
    max_monthly_visits INTEGER NOT NULL,
    max_monthly_submissions INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by_user_id INTEGER NULL REFERENCES users(id)
);

CREATE TABLE invites (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    code_hash TEXT NOT NULL UNIQUE,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('admin', 'member', 'viewer')),
    max_uses INTEGER NOT NULL CHECK (max_uses IN (1, 5, 10)),
    used_count INTEGER NOT NULL DEFAULT 0,
    expires_at TEXT NOT NULL,
    created_by_user_id INTEGER NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    revoked_at TEXT NULL
);

CREATE INDEX invites_organization_id_idx ON invites(organization_id);
CREATE INDEX invites_expires_at_idx ON invites(expires_at);

-- +goose Down
DROP TABLE invites;
DROP TABLE organization_limits;
