package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type ErrorLogEntry struct {
	ID        int64
	Level     string
	Message   string
	Context   sql.NullString
	CreatedAt string
}

func (q *Querier) CreateErrorLog(ctx context.Context, level, message string, contextData any) error {
	var contextJSON sql.NullString
	if contextData != nil {
		if b, err := json.Marshal(contextData); err == nil {
			contextJSON = sql.NullString{String: string(b), Valid: true}
		}
	}
	_, err := q.db.ExecContext(ctx, `
		INSERT INTO error_logs (level, message, context, created_at)
		VALUES (?, ?, ?, ?)`, level, message, contextJSON, Now())
	return err
}

func (q *Querier) ListRecentErrorLogs(ctx context.Context, limit int64) ([]ErrorLogEntry, error) {
	q.cleanupErrorLogs(ctx)
	rows, err := q.db.QueryContext(ctx, `
		SELECT id, level, message, context, created_at
		FROM error_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []ErrorLogEntry
	for rows.Next() {
		var entry ErrorLogEntry
		if err := rows.Scan(&entry.ID, &entry.Level, &entry.Message, &entry.Context, &entry.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, entry)
	}
	return logs, rows.Err()
}

func (q *Querier) cleanupErrorLogs(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	q.db.ExecContext(ctx, `DELETE FROM error_logs WHERE created_at < ?`, cutoff)
}
