-- +goose Up
CREATE TABLE error_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    level TEXT NOT NULL DEFAULT 'error',
    message TEXT NOT NULL,
    context TEXT NULL,
    created_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE error_logs;
