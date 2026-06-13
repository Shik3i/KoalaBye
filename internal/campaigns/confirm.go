package campaigns

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/koalastuff/koalabye/internal/web"
	"github.com/koalastuff/koalabye/templates"
)

func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	action := chi.URLParam(r, "action")
	permission := permissionEdit
	if action == "archive-campaign" || action == "delete-responses" || action == "delete-visits" || action == "delete-old" || action == "remove-member" {
		permission = permissionArchive
	}
	if action == "redirect" {
		permission = permissionRedirect
	}
	user, campaign, role, ok := h.campaign(r, permission)
	if !ok || ((action == "delete-responses" || action == "delete-visits" || action == "delete-old" || action == "remove-member") && role != "owner") {
		h.forbidden(w, r)
		return
	}

	titleKey, messageKey, subject, actionURL := "", "", "", campaignURL(campaign.OrganizationPublicID, campaign.PublicID)
	cancelURL := actionURL
	hidden := map[string]string{"confirmed": "yes"}
	switch action {
	case "archive-campaign":
		titleKey, messageKey, subject = "confirm.archive.title", "confirm.archive.message", campaign.Name
		actionURL += "/status"
		hidden["status"] = "archived"
	case "redirect":
		titleKey, messageKey = "confirm.redirect.title", "confirm.redirect.message"
		actionURL += "/redirect"
		cancelURL += "/settings"
		targetID := r.URL.Query().Get("target_campaign_public_id")
		hidden["target_campaign_public_id"] = targetID
		if targetID == "" {
			subject = campaign.Name
			messageKey = "confirm.redirect.remove_message"
		} else {
			targets, _ := h.q.ListCampaignRedirectTargets(r.Context(), campaign)
			for _, target := range targets {
				if target.PublicID == targetID {
					subject = target.Name
					break
				}
			}
			if subject == "" {
				h.forbidden(w, r)
				return
			}
		}
	case "archive-field":
		field, err := h.q.GetFormField(r.Context(), campaign.ID, r.URL.Query().Get("field"))
		if err != nil || field.ArchivedAt.Valid {
			h.forbidden(w, r)
			return
		}
		titleKey, messageKey, subject = "confirm.field.title", "confirm.field.message", field.Label
		actionURL += "/form/fields/" + field.PublicID + "/archive"
		cancelURL += "/form"
	case "archive-option":
		option, err := h.q.GetFormOption(r.Context(), campaign.ID, r.URL.Query().Get("option"))
		if err != nil || option.ArchivedAt.Valid {
			h.forbidden(w, r)
			return
		}
		titleKey, messageKey, subject = "confirm.option.title", "confirm.option.message", option.Label
		actionURL += "/form/options/" + option.PublicID + "/archive"
		cancelURL += "/form"
	case "delete-responses":
		titleKey, messageKey, subject = "confirm.responses.title", "confirm.responses.message", campaign.Name
		actionURL += "/responses/delete-all"
		cancelURL += "/analytics"
		hidden["confirmation"] = campaign.Slug
	case "delete-visits":
		titleKey, messageKey, subject = "confirm.visits.title", "confirm.visits.message", campaign.Name
		actionURL += "/visits/delete-all"
		cancelURL += "/analytics"
		hidden["confirmation"] = campaign.Slug
	case "delete-old":
		settings, err := h.q.GetCampaignSettings(r.Context(), campaign.ID)
		if err != nil || !settings.RetentionEnabled || !settings.RetentionDays.Valid {
			h.forbidden(w, r)
			return
		}
		titleKey, messageKey, subject = "confirm.retention.title", "confirm.retention.message", campaign.Name
		actionURL += "/retention/delete-old"
		cancelURL += "/analytics"
	case "remove-member":
		memberID := r.URL.Query().Get("member")
		members, _ := h.q.ListCampaignAccess(r.Context(), campaign.ID, campaign.OrganizationID)
		for _, member := range members {
			if member.PublicID == memberID && member.Role.Valid {
				subject = member.DisplayName
				break
			}
		}
		if subject == "" {
			h.forbidden(w, r)
			return
		}
		titleKey, messageKey = "confirm.member.title", "confirm.member.message"
		actionURL += "/access/" + memberID + "/remove"
		cancelURL += "/access"
	default:
		h.forbidden(w, r)
		return
	}
	web.Render(w, r, http.StatusOK, templates.ConfirmationPage(h.cfg.InstanceName, user, campaign, titleKey, messageKey, subject, actionURL, cancelURL, hidden))
}

func confirmed(r *http.Request) bool {
	return r.FormValue("confirmed") == "yes"
}

func requireConfirmation(w http.ResponseWriter, r *http.Request) bool {
	if confirmed(r) {
		return true
	}
	http.Error(w, "confirmation required", http.StatusUnprocessableEntity)
	return false
}
