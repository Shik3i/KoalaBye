-- +goose Up
CREATE TABLE campaign_form_starts (
    id INTEGER PRIMARY KEY,
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    visit_id INTEGER NOT NULL REFERENCES campaign_visits(id) ON DELETE CASCADE,
    started_at TEXT NOT NULL,
    UNIQUE(campaign_id, visit_id)
);

CREATE INDEX campaign_form_starts_campaign_started_idx
    ON campaign_form_starts(campaign_id, started_at);

-- +goose Down
DROP INDEX campaign_form_starts_campaign_started_idx;
DROP TABLE campaign_form_starts;
