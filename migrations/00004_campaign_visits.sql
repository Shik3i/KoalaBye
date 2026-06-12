-- +goose Up
CREATE TABLE campaign_visits (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    install_token_hash TEXT NULL,
    visit_kind TEXT NOT NULL CHECK (visit_kind IN ('public_page')),
    counted_as_unique_token_visit INTEGER NOT NULL DEFAULT 0,
    counted_as_raw_visit INTEGER NOT NULL DEFAULT 1,
    referrer_domain TEXT NULL,
    coarse_browser TEXT NULL,
    coarse_os TEXT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_campaign_visits_campaign_created_at
    ON campaign_visits(campaign_id, created_at);
CREATE INDEX idx_campaign_visits_campaign_token_hash
    ON campaign_visits(campaign_id, install_token_hash);

-- Only one stored visit can be the unique count for a campaign/token pair.
CREATE UNIQUE INDEX campaign_visits_unique_counted_token
    ON campaign_visits(campaign_id, install_token_hash)
    WHERE counted_as_unique_token_visit = 1 AND install_token_hash IS NOT NULL;

-- +goose Down
DROP INDEX campaign_visits_unique_counted_token;
DROP INDEX idx_campaign_visits_campaign_token_hash;
DROP INDEX idx_campaign_visits_campaign_created_at;
DROP TABLE campaign_visits;
