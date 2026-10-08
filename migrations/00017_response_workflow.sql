-- +goose Up
ALTER TABLE campaign_submissions ADD COLUMN assignee_user_id INTEGER NULL;
CREATE INDEX campaign_submissions_campaign_assignee_idx ON campaign_submissions(campaign_id, assignee_user_id, id);

CREATE TABLE campaign_submission_notes (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    submission_id INTEGER NOT NULL REFERENCES campaign_submissions(id) ON DELETE CASCADE,
    author_user_id INTEGER NULL REFERENCES users(id) ON DELETE SET NULL,
    body TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX campaign_submission_notes_submission_idx ON campaign_submission_notes(submission_id, id);

CREATE TABLE campaign_response_tags (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    name_normalized TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (campaign_id, name_normalized)
);

CREATE TABLE campaign_submission_tags (
    submission_id INTEGER NOT NULL REFERENCES campaign_submissions(id) ON DELETE CASCADE,
    tag_id INTEGER NOT NULL REFERENCES campaign_response_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (submission_id, tag_id)
);
CREATE INDEX campaign_submission_tags_tag_idx ON campaign_submission_tags(tag_id, submission_id);

CREATE TABLE campaign_response_views (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    query TEXT NOT NULL,
    shared INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE INDEX campaign_response_views_campaign_idx ON campaign_response_views(campaign_id, shared, user_id);

ALTER TABLE campaign_settings ADD COLUMN auto_close_days INTEGER NULL CHECK (auto_close_days IS NULL OR auto_close_days BETWEEN 1 AND 3650);

-- +goose Down
ALTER TABLE campaign_settings DROP COLUMN auto_close_days;
DROP INDEX campaign_response_views_campaign_idx;
DROP TABLE campaign_response_views;
DROP INDEX campaign_submission_tags_tag_idx;
DROP TABLE campaign_submission_tags;
DROP TABLE campaign_response_tags;
DROP INDEX campaign_submission_notes_submission_idx;
DROP TABLE campaign_submission_notes;
DROP INDEX campaign_submissions_campaign_assignee_idx;
ALTER TABLE campaign_submissions DROP COLUMN assignee_user_id;
