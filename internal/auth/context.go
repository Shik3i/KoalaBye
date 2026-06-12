package auth

import (
	"context"

	"github.com/koalastuff/koalabye/internal/db"
)

type contextKey string

const userContextKey contextKey = "current-user"

func WithUser(ctx context.Context, user db.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

func UserFromContext(ctx context.Context) (db.User, bool) {
	user, ok := ctx.Value(userContextKey).(db.User)
	return user, ok
}
