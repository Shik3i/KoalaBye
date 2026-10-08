package campaigns

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/koalastuff/koalabye/internal/db"
)

var autoCloseChoices = map[int64]bool{7: true, 14: true, 30: true, 60: true, 90: true, 180: true}

func (h *Handler) responseDetailURL(campaign db.Campaign, submissionPublicID, fragment string) string {
	target := campaignURL(campaign.OrganizationPublicID, campaign.PublicID) + "/responses/" + url.PathEscape(submissionPublicID)
	if fragment != "" {
		target += "#" + fragment
	}
	return target
}

// ResponseNoteAdd stores an internal team note on a response.
func (h *Handler) ResponseNoteAdd(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.responseCampaign(r)
	if !ok {
		h.forbidden(w, r)
		return
	}
	submissionPublicID := chi.URLParam(r, "submissionPublicID")
	err := h.q.AddSubmissionNote(r.Context(), campaign, submissionPublicID, user.ID, r.FormValue("body"))
	if err != nil && !errors.Is(err, db.ErrInvalidInput) && !errors.Is(err, db.ErrLimitReached) {
		if errors.Is(err, sql.ErrNoRows) {
			h.forbidden(w, r)
			return
		}
		h.logError(r.Context(), "add response note", err)
		http.Error(w, "add note", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, h.responseDetailURL(campaign, submissionPublicID, "notes"), http.StatusSeeOther)
}

func (h *Handler) ResponseNoteDelete(w http.ResponseWriter, r *http.Request) {
	user, campaign, role, ok := h.responseCampaign(r)
	if !ok {
		h.forbidden(w, r)
		return
	}
	submissionPublicID := chi.URLParam(r, "submissionPublicID")
	if err := h.q.DeleteSubmissionNote(r.Context(), campaign, chi.URLParam(r, "notePublicID"), user.ID, canEditResponses(role)); err != nil && !errors.Is(err, sql.ErrNoRows) {
		h.logError(r.Context(), "delete response note", err)
		http.Error(w, "delete note", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, h.responseDetailURL(campaign, submissionPublicID, "notes"), http.StatusSeeOther)
}

// ResponseViewCreate saves the posted filter as a personal or shared view.
func (h *Handler) ResponseViewCreate(w http.ResponseWriter, r *http.Request) {
	user, campaign, role, ok := h.responseCampaign(r)
	if !ok {
		h.forbidden(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	filter := db.ResponseFilterFromValues(r.PostForm)
	shared := r.PostForm.Get("shared") == "on" && canEditResponses(role)
	err := h.q.CreateResponseView(r.Context(), campaign.ID, user.ID, r.PostForm.Get("name"), filter, shared)
	if err != nil && !errors.Is(err, db.ErrInvalidInput) && !errors.Is(err, db.ErrLimitReached) {
		h.logError(r.Context(), "create response view", err)
		http.Error(w, "save view", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, responsesListURL(campaign, filter), http.StatusSeeOther)
}

func (h *Handler) ResponseViewDelete(w http.ResponseWriter, r *http.Request) {
	user, campaign, role, ok := h.responseCampaign(r)
	if !ok {
		h.forbidden(w, r)
		return
	}
	if err := h.q.DeleteResponseView(r.Context(), campaign.ID, chi.URLParam(r, "viewPublicID"), user.ID, role == "owner"); err != nil && !errors.Is(err, sql.ErrNoRows) {
		h.logError(r.Context(), "delete response view", err)
		http.Error(w, "delete view", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, responsesListURL(campaign, db.ResponseFilter{}), http.StatusSeeOther)
}

func (h *Handler) ResponseTagDelete(w http.ResponseWriter, r *http.Request) {
	user, campaign, role, ok := h.responseCampaign(r)
	if !ok || !canEditResponses(role) {
		h.forbidden(w, r)
		return
	}
	if err := h.q.DeleteResponseTag(r.Context(), campaign, chi.URLParam(r, "tagPublicID"), user.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		h.logError(r.Context(), "delete response tag", err)
		http.Error(w, "delete tag", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, responsesListURL(campaign, db.ResponseFilter{}), http.StatusSeeOther)
}

// ResponseAutoClose configures closing of read, unassigned, unstarred responses after N days.
func (h *Handler) ResponseAutoClose(w http.ResponseWriter, r *http.Request) {
	user, campaign, role, ok := h.responseCampaign(r)
	if !ok || !canEditResponses(role) {
		h.forbidden(w, r)
		return
	}
	var days sql.NullInt64
	if raw := r.FormValue("days"); raw != "" && raw != "0" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || !autoCloseChoices[value] {
			http.Error(w, "invalid auto-close value", http.StatusUnprocessableEntity)
			return
		}
		days = sql.NullInt64{Int64: value, Valid: true}
	}
	if err := h.q.SetAutoCloseDays(r.Context(), campaign, days, user.ID); err != nil {
		h.logError(r.Context(), "set auto close", err)
		http.Error(w, "update auto-close", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, responsesListURL(campaign, db.ResponseFilter{}), http.StatusSeeOther)
}
