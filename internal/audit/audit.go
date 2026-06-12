package audit

import (
	"context"
	"log/slog"

	"github.com/koalastuff/koalabye/internal/db"
)

type Logger struct {
	queries *db.Querier
}

func New(queries *db.Querier) *Logger {
	return &Logger{queries: queries}
}

func (l *Logger) Record(ctx context.Context, actorUserID any, action string, targetType string, targetID any) {
	if err := l.queries.CreateAuditEvent(ctx, actorUserID, nil, action, targetType, targetID, nil, nil); err != nil {
		slog.Error("write audit event", "action", action, "error", err)
	}
}
