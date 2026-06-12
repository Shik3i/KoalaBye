-- +goose Up
CREATE TABLE campaign_form_fields (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    field_type TEXT NOT NULL CHECK (field_type IN ('text_block', 'checkbox_group', 'radio_group', 'rating_1_5', 'textarea')),
    label TEXT NOT NULL,
    help_text TEXT NULL,
    required INTEGER NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL,
    config_json TEXT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    archived_at TEXT NULL
);

CREATE INDEX campaign_form_fields_campaign_order_idx
    ON campaign_form_fields(campaign_id, archived_at, sort_order);

CREATE TABLE campaign_form_options (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    field_id INTEGER NOT NULL REFERENCES campaign_form_fields(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    value TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    archived_at TEXT NULL,
    UNIQUE(field_id, value)
);

CREATE INDEX campaign_form_options_field_order_idx
    ON campaign_form_options(field_id, archived_at, sort_order);

CREATE TABLE campaign_submissions (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    visit_id INTEGER NULL REFERENCES campaign_visits(id) ON DELETE SET NULL,
    install_token_hash TEXT NULL,
    submitted_at TEXT NOT NULL
);

CREATE INDEX campaign_submissions_campaign_submitted_idx
    ON campaign_submissions(campaign_id, submitted_at);

CREATE TABLE campaign_submission_answers (
    id INTEGER PRIMARY KEY,
    submission_id INTEGER NOT NULL REFERENCES campaign_submissions(id) ON DELETE CASCADE,
    field_id INTEGER NULL REFERENCES campaign_form_fields(id) ON DELETE SET NULL,
    field_public_id TEXT NOT NULL,
    field_type TEXT NOT NULL,
    field_label_snapshot TEXT NOT NULL,
    value_json TEXT NOT NULL
);

CREATE INDEX campaign_submission_answers_submission_idx
    ON campaign_submission_answers(submission_id, id);

-- +goose Down
DROP INDEX campaign_submission_answers_submission_idx;
DROP TABLE campaign_submission_answers;
DROP INDEX campaign_submissions_campaign_submitted_idx;
DROP TABLE campaign_submissions;
DROP INDEX campaign_form_options_field_order_idx;
DROP TABLE campaign_form_options;
DROP INDEX campaign_form_fields_campaign_order_idx;
DROP TABLE campaign_form_fields;
