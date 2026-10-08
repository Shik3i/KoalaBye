-- +goose Up
ALTER TABLE campaign_submissions ADD COLUMN has_text INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_submissions ADD COLUMN starred INTEGER NOT NULL DEFAULT 0;

UPDATE campaign_submissions SET has_text = 1
WHERE EXISTS (
    SELECT 1 FROM campaign_submission_answers a
    WHERE a.submission_id = campaign_submissions.id
      AND a.field_type = 'textarea'
      AND TRIM(a.value_json) NOT IN ('""', 'null', '')
);

CREATE TABLE campaign_submission_reads (
    submission_id INTEGER NOT NULL REFERENCES campaign_submissions(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    first_read_at TEXT NOT NULL,
    PRIMARY KEY (submission_id, user_id)
);

CREATE INDEX campaign_submission_reads_user_idx ON campaign_submission_reads(user_id, submission_id);
CREATE INDEX campaign_submissions_campaign_text_idx ON campaign_submissions(campaign_id, has_text, id);
CREATE INDEX campaign_submissions_campaign_starred_idx ON campaign_submissions(campaign_id, starred, id);
CREATE INDEX campaign_submissions_campaign_triage_idx ON campaign_submissions(campaign_id, triage_status, id);

-- +goose Down
DROP INDEX campaign_submissions_campaign_triage_idx;
DROP INDEX campaign_submissions_campaign_starred_idx;
DROP INDEX campaign_submissions_campaign_text_idx;
DROP INDEX campaign_submission_reads_user_idx;
DROP TABLE campaign_submission_reads;
ALTER TABLE campaign_submissions DROP COLUMN starred;
ALTER TABLE campaign_submissions DROP COLUMN has_text;
