package permissions

import (
	"context"

	"github.com/koalastuff/koalabye/internal/db"
)

type Service struct {
	queries *db.Querier
}

func New(queries *db.Querier) *Service {
	return &Service{queries: queries}
}

func (s *Service) IsInstanceOwner(ctx context.Context, userID int64) (bool, error) {
	return s.queries.UserHasInstanceRole(ctx, userID, "instance_owner")
}

func (s *Service) CanAccessInstanceAdmin(ctx context.Context, userID int64) (bool, error) {
	return s.IsInstanceOwner(ctx, userID)
}

func (s *Service) CanAccessOrganization(ctx context.Context, userID, organizationID int64) (bool, error) {
	return s.queries.UserCanAccessOrganization(ctx, userID, organizationID)
}
