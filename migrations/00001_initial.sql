-- +goose Up
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL UNIQUE,
    username_normalized TEXT NOT NULL UNIQUE,
    email TEXT NULL,
    email_normalized TEXT NULL,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    disabled_at TEXT NULL
);

CREATE UNIQUE INDEX users_email_normalized_unique
    ON users(email_normalized) WHERE email_normalized IS NOT NULL;

CREATE TABLE instance_roles (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('instance_owner', 'instance_admin', 'instance_moderator', 'instance_support')),
    created_at TEXT NOT NULL,
    created_by_user_id INTEGER NULL REFERENCES users(id),
    revoked_at TEXT NULL
);

CREATE UNIQUE INDEX active_instance_role_unique
    ON instance_roles(user_id, role) WHERE revoked_at IS NULL;

CREATE TABLE organizations (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_by_user_id INTEGER NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    disabled_at TEXT NULL
);

CREATE TABLE organization_members (
    id INTEGER PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    created_at TEXT NOT NULL,
    created_by_user_id INTEGER NULL REFERENCES users(id),
    UNIQUE(organization_id, user_id)
);

CREATE TABLE sessions (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    revoked_at TEXT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);

CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY,
    actor_user_id INTEGER NULL REFERENCES users(id),
    organization_id INTEGER NULL REFERENCES organizations(id),
    action TEXT NOT NULL,
    target_type TEXT NULL,
    target_id TEXT NULL,
    reason TEXT NULL,
    metadata_json TEXT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX audit_log_created_at_idx ON audit_log(created_at);
CREATE INDEX audit_log_actor_user_id_idx ON audit_log(actor_user_id);

CREATE TABLE instance_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by_user_id INTEGER NULL REFERENCES users(id)
);

-- +goose Down
DROP TABLE instance_settings;
DROP TABLE audit_log;
DROP TABLE sessions;
DROP TABLE organization_members;
DROP TABLE organizations;
DROP TABLE instance_roles;
DROP TABLE users;
