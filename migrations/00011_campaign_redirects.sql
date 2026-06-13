-- +goose Up
CREATE TABLE campaign_redirects (
    source_campaign_id INTEGER PRIMARY KEY REFERENCES campaigns(id) ON DELETE CASCADE,
    target_campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by_user_id INTEGER NULL REFERENCES users(id),
    CHECK (source_campaign_id != target_campaign_id)
);

CREATE INDEX campaign_redirects_target_idx ON campaign_redirects(target_campaign_id);

-- +goose Down
DROP INDEX campaign_redirects_target_idx;
DROP TABLE campaign_redirects;
