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

func (s *Service) CampaignRole(ctx context.Context, userID, campaignID int64) (string, error) {
	if owner, err := s.IsInstanceOwner(ctx, userID); err != nil {
		return "", err
	} else if owner {
		return "owner", nil
	}
	return s.queries.CampaignRole(ctx, campaignID, userID)
}

func (s *Service) CanViewCampaign(ctx context.Context, userID, campaignID int64) (bool, error) {
	role, err := s.CampaignRole(ctx, userID, campaignID)
	return role == "owner" || role == "editor" || role == "analyst" || role == "viewer", err
}

func (s *Service) CanEditCampaign(ctx context.Context, userID, campaignID int64) (bool, error) {
	role, err := s.CampaignRole(ctx, userID, campaignID)
	return role == "owner" || role == "editor", err
}

func (s *Service) CanManageCampaignAccess(ctx context.Context, userID, campaignID int64) (bool, error) {
	role, err := s.CampaignRole(ctx, userID, campaignID)
	return role == "owner", err
}

func (s *Service) CanChangeCampaignPrivacy(ctx context.Context, userID, campaignID int64) (bool, error) {
	return s.CanManageCampaignAccess(ctx, userID, campaignID)
}

func (s *Service) CanArchiveCampaign(ctx context.Context, userID, campaignID int64) (bool, error) {
	return s.CanManageCampaignAccess(ctx, userID, campaignID)
}
