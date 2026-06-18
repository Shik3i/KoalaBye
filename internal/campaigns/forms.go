package campaigns

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/koalastuff/koalabye/internal/auth"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/i18n"
	"github.com/koalastuff/koalabye/internal/ids"
	"github.com/koalastuff/koalabye/internal/web"
	"github.com/koalastuff/koalabye/templates"
)

const maxPublicFormBody = 128 << 10

var optionValuePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (h *Handler) Form(w http.ResponseWriter, r *http.Request) {
	user, campaign, role, ok := h.campaign(r, permissionView)
	if !ok {
		h.forbidden(w, r)
		return
	}
	fields, err := h.q.ListFormFields(r.Context(), campaign.ID, false)
	branding, _ := h.q.GetCampaignBranding(r.Context(), campaign.ID)
	if err != nil {
		http.Error(w, "load form", http.StatusInternalServerError)
		return
	}
	canEdit := (role == "owner" || role == "editor") && campaign.Status != "archived" && !campaign.DisabledAt.Valid
	web.Render(w, r, http.StatusOK, templates.CampaignForm(h.cfg.InstanceName, user, campaign, branding, fields, canEdit, ""))
}

func (h *Handler) FormFieldCreate(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionEdit)
	if !ok {
		h.forbidden(w, r)
		return
	}
	input, errorKey := formFieldInput(r, true)
	if errorKey != "" {
		h.renderFormError(w, r, user, campaign, errorKey)
		return
	}
	input.CampaignID = campaign.ID
	input.PublicID, _ = ids.New("field")
	if err := h.q.CreateFormField(r.Context(), input, user.ID); err != nil {
		h.renderFormError(w, r, user, campaign, "form.error.save")
		return
	}
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/form", http.StatusSeeOther)
}

func (h *Handler) FormFieldLoadPreset(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionEdit)
	if !ok {
		h.forbidden(w, r)
		return
	}
	preset := r.FormValue("preset_id")
	ctx := r.Context()

	var fields []db.SaveFormFieldInput
	var options [][]db.FormOption

	switch preset {
	case "uninstall":
		f1, _ := ids.New("field")
		fields = append(fields, db.SaveFormFieldInput{CampaignID: campaign.ID, PublicID: f1, Label: i18n.T(ctx, "preset.uninstall.reason.label"), FieldType: "radio_group", Required: true})
		o1, _ := ids.New("option")
		o2, _ := ids.New("option")
		o3, _ := ids.New("option")
		o4, _ := ids.New("option")
		options = append(options, []db.FormOption{
			{PublicID: o1, Label: i18n.T(ctx, "preset.uninstall.reason.opt1"), Value: "missing-features"},
			{PublicID: o2, Label: i18n.T(ctx, "preset.uninstall.reason.opt2"), Value: "bugs"},
			{PublicID: o3, Label: i18n.T(ctx, "preset.uninstall.reason.opt3"), Value: "expensive"},
			{PublicID: o4, Label: i18n.T(ctx, "preset.uninstall.reason.opt4"), Value: "other"},
		})
		f2, _ := ids.New("field")
		fields = append(fields, db.SaveFormFieldInput{CampaignID: campaign.ID, PublicID: f2, Label: i18n.T(ctx, "preset.uninstall.details.label"), FieldType: "textarea", Required: false})
		options = append(options, nil)
	case "feedback":
		f1, _ := ids.New("field")
		fields = append(fields, db.SaveFormFieldInput{CampaignID: campaign.ID, PublicID: f1, Label: i18n.T(ctx, "preset.feedback.rating.label"), FieldType: "rating_1_5", Required: true})
		options = append(options, nil)
		f2, _ := ids.New("field")
		fields = append(fields, db.SaveFormFieldInput{CampaignID: campaign.ID, PublicID: f2, Label: i18n.T(ctx, "preset.feedback.details.label"), FieldType: "textarea", Required: false})
		options = append(options, nil)
	case "bug":
		f1, _ := ids.New("field")
		fields = append(fields, db.SaveFormFieldInput{CampaignID: campaign.ID, PublicID: f1, Label: i18n.T(ctx, "preset.bug.details.label"), FieldType: "textarea", Required: true})
		options = append(options, nil)
	}

	for i, input := range fields {
		_ = h.q.CreateFormField(ctx, input, user.ID)
		if len(options[i]) > 0 {
			field, err := h.q.GetFormField(ctx, campaign.ID, input.PublicID)
			if err == nil {
				for _, opt := range options[i] {
					_ = h.q.CreateFormOption(ctx, campaign.ID, field.ID, opt.PublicID, opt.Label, opt.Value, user.ID)
				}
			}
		}
	}
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/form", http.StatusSeeOther)
}

func (h *Handler) FormFieldEdit(w http.ResponseWriter, r *http.Request) {
	user, campaign, role, ok := h.campaign(r, permissionView)
	if !ok {
		h.forbidden(w, r)
		return
	}
	field, err := h.q.GetFormField(r.Context(), campaign.ID, chi.URLParam(r, "fieldPublicID"))
	if err != nil || field.ArchivedAt.Valid {
		h.forbidden(w, r)
		return
	}
	canEdit := (role == "owner" || role == "editor") && campaign.Status != "archived" && !campaign.DisabledAt.Valid
	web.Render(w, r, http.StatusOK, templates.CampaignFormFieldEdit(h.cfg.InstanceName, user, campaign, field, canEdit, ""))
}

func (h *Handler) FormFieldUpdate(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionEdit)
	if !ok {
		h.forbidden(w, r)
		return
	}
	field, err := h.q.GetFormField(r.Context(), campaign.ID, chi.URLParam(r, "fieldPublicID"))
	if err != nil || field.ArchivedAt.Valid {
		h.forbidden(w, r)
		return
	}
	input, errorKey := formFieldInput(r, false)
	input.FieldType = field.FieldType
	if errorKey != "" {
		web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignFormFieldEdit(h.cfg.InstanceName, user, campaign, field, true, errorKey))
		return
	}
	if err := h.q.UpdateFormField(r.Context(), campaign.ID, field.PublicID, input, user.ID); err != nil {
		web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignFormFieldEdit(h.cfg.InstanceName, user, campaign, field, true, "form.error.save"))
		return
	}
	for _, option := range field.Options {
		if option.ArchivedAt.Valid {
			continue
		}
		label := strings.TrimSpace(r.FormValue("option_" + option.PublicID))
		if label != "" && len(label) <= 200 && label != option.Label {
			_ = h.q.UpdateFormOption(r.Context(), campaign.ID, field.ID, option.PublicID, label, user.ID)
		}
	}
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/form/fields/"+field.PublicID+"/edit", http.StatusSeeOther)
}

func (h *Handler) FormFieldArchive(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionEdit)
	if !ok {
		h.forbidden(w, r)
		return
	}
	_ = h.q.ArchiveFormField(r.Context(), campaign.ID, chi.URLParam(r, "fieldPublicID"), user.ID)
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/form", http.StatusSeeOther)
}

func (h *Handler) FormFieldMove(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionEdit)
	if !ok {
		h.forbidden(w, r)
		return
	}
	direction := r.FormValue("direction")
	if direction != "up" && direction != "down" {
		http.Error(w, "invalid direction", http.StatusUnprocessableEntity)
		return
	}
	_ = h.q.MoveFormField(r.Context(), campaign.ID, chi.URLParam(r, "fieldPublicID"), direction, user.ID)
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/form", http.StatusSeeOther)
}

func (h *Handler) FormOptionCreate(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionEdit)
	if !ok {
		h.forbidden(w, r)
		return
	}
	field, err := h.q.GetFormField(r.Context(), campaign.ID, chi.URLParam(r, "fieldPublicID"))
	if err != nil || (field.FieldType != "checkbox_group" && field.FieldType != "radio_group") {
		h.forbidden(w, r)
		return
	}
	label := strings.TrimSpace(r.FormValue("label"))
	value := strings.TrimSpace(strings.ToLower(r.FormValue("value")))
	if label == "" || len(label) > 200 || !optionValuePattern.MatchString(value) || len(value) > 100 {
		web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignFormFieldEdit(h.cfg.InstanceName, user, campaign, field, true, "form.option.error"))
		return
	}
	publicID, _ := ids.New("option")
	if err := h.q.CreateFormOption(r.Context(), campaign.ID, field.ID, publicID, label, value, user.ID); err != nil {
		web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignFormFieldEdit(h.cfg.InstanceName, user, campaign, field, true, "form.option.error"))
		return
	}
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/form/fields/"+field.PublicID+"/edit", http.StatusSeeOther)
}

func (h *Handler) FormOptionArchive(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionEdit)
	if !ok {
		h.forbidden(w, r)
		return
	}
	_ = h.q.ArchiveFormOption(r.Context(), campaign.ID, chi.URLParam(r, "optionPublicID"), user.ID)
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/form", http.StatusSeeOther)
}

func formFieldInput(r *http.Request, requireType bool) (db.SaveFormFieldInput, string) {
	fieldType := r.FormValue("field_type")
	if !requireType {
		fieldType = ""
	}
	label := strings.TrimSpace(r.FormValue("label"))
	help := strings.TrimSpace(r.FormValue("help_text"))
	if label == "" || len(label) > 300 || len(help) > 1000 {
		return db.SaveFormFieldInput{}, "form.error.invalid"
	}
	if requireType && fieldType != "text_block" && fieldType != "checkbox_group" && fieldType != "radio_group" && fieldType != "rating_1_5" && fieldType != "textarea" {
		return db.SaveFormFieldInput{}, "form.error.invalid"
	}
	config := db.FieldConfig{}
	if fieldType == "text_block" || (!requireType && r.FormValue("body") != "") {
		config.Body = strings.TrimSpace(r.FormValue("body"))
		if len(config.Body) > 5000 {
			return db.SaveFormFieldInput{}, "form.error.invalid"
		}
	}
	if fieldType == "textarea" || (!requireType && r.FormValue("max_length") != "") {
		config.MaxLength, _ = strconv.Atoi(r.FormValue("max_length"))
		if config.MaxLength == 0 {
			config.MaxLength = 1000
		}
		if config.MaxLength < 1 || config.MaxLength > 5000 {
			return db.SaveFormFieldInput{}, "form.error.invalid"
		}
	}
	configJSON := ""
	if encoded, _ := json.Marshal(config); string(encoded) != "{}" {
		configJSON = string(encoded)
	}
	return db.SaveFormFieldInput{FieldType: fieldType, Label: label, HelpText: help, Required: r.FormValue("required") == "on", ConfigJSON: configJSON}, ""
}

func (h *Handler) renderFormError(w http.ResponseWriter, r *http.Request, user db.User, campaign db.Campaign, key string) {
	fields, _ := h.q.ListFormFields(r.Context(), campaign.ID, false)
	branding, _ := h.q.GetCampaignBranding(r.Context(), campaign.ID)
	web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignForm(h.cfg.InstanceName, user, campaign, branding, fields, true, key))
}

func (h *Handler) PublicSubmitByID(w http.ResponseWriter, r *http.Request) {
	h.publicSubmit(w, r, func() (db.PublicCampaign, error) {
		return h.q.GetPublicCampaignByID(r.Context(), chi.URLParam(r, "campaignPublicID"))
	})
}

func (h *Handler) PublicSubmitBySlug(w http.ResponseWriter, r *http.Request) {
	h.publicSubmit(w, r, func() (db.PublicCampaign, error) {
		return h.q.GetPublicCampaignBySlug(r.Context(), chi.URLParam(r, "orgSlug"), chi.URLParam(r, "campaignSlug"))
	})
}

func (h *Handler) publicSubmit(w http.ResponseWriter, r *http.Request, resolve func() (db.PublicCampaign, error)) {
	if !h.submitLimiter.Allow(web.ClientIP(r, h.cfg.TrustedProxies)) {
		h.publicUnavailable(w, r, http.StatusTooManyRequests, false, "en")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPublicFormBody)
	if err := r.ParseForm(); err != nil {
		h.publicUnavailable(w, r, http.StatusRequestEntityTooLarge, false, "en")
		return
	}
	publicCampaign, err := resolve()
	if err != nil || !publicCampaign.Available() {
		h.publicUnavailable(w, r, http.StatusNotFound, false, "en")
		return
	}
	r = r.WithContext(i18n.PublicCampaignContext(r.Context(), r, publicCampaign.Settings.PublicLanguageDefault))
	if r.FormValue("website") != "" {
		web.Render(w, r, http.StatusOK, templates.PublicCampaignThankYou(h.cfg.InstanceName, &publicCampaign.Branding))
		return
	}
	fields, err := h.q.ListFormFields(r.Context(), publicCampaign.Campaign.ID, false)
	if err != nil {
		h.publicUnavailable(w, r, http.StatusServiceUnavailable, false, publicCampaign.Settings.PublicLanguageDefault)
		return
	}
	answers, validationKey := validateSubmission(fields, r)
	if validationKey != "" {
		web.Render(w, r, http.StatusUnprocessableEntity, templates.PublicCampaignPage(h.cfg.InstanceName, publicCampaign.Campaign, publicCampaign.Settings, publicCampaign.Branding, fields, r.FormValue("visit_public_id"), validationKey))
		return
	}
	publicID, _ := ids.New("submission")
	err = h.q.CreateSubmission(r.Context(), db.CreateSubmissionInput{
		PublicID: publicID, VisitPublicID: r.FormValue("visit_public_id"),
		CampaignID: publicCampaign.Campaign.ID, OrgID: publicCampaign.Campaign.OrganizationID,
		SubmittedAt: time.Now().UTC(), Answers: answers,
	})
	if errors.Is(err, db.ErrSubmissionLimitReached) {
		web.Render(w, r, http.StatusServiceUnavailable, templates.PublicSubmissionLimit(h.cfg.InstanceName, &publicCampaign.Branding))
		return
	}
	if err != nil {
		h.publicUnavailable(w, r, http.StatusServiceUnavailable, false, publicCampaign.Settings.PublicLanguageDefault)
		return
	}
	web.Render(w, r, http.StatusOK, templates.PublicCampaignThankYou(h.cfg.InstanceName, &publicCampaign.Branding))
}

func validateSubmission(fields []db.FormField, r *http.Request) ([]db.SubmissionAnswerInput, string) {
	var answers []db.SubmissionAnswerInput
	for _, field := range fields {
		if field.FieldType == "text_block" {
			continue
		}
		name := "field_" + field.PublicID
		values, present := r.PostForm[name]
		if field.Required && (!present || len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			return nil, "public.form.error.required"
		}
		if !present || len(values) == 0 || (len(values) == 1 && strings.TrimSpace(values[0]) == "") {
			continue
		}
		var value any
		switch field.FieldType {
		case "checkbox_group":
			valid := optionSet(field.Options)
			selected := make([]string, 0, len(values))
			for _, candidate := range values {
				if _, ok := valid[candidate]; !ok {
					return nil, "public.form.error.invalid"
				}
				selected = append(selected, candidate)
			}
			if field.Required && len(selected) == 0 {
				return nil, "public.form.error.required"
			}
			value = selected
		case "radio_group":
			if len(values) != 1 {
				return nil, "public.form.error.invalid"
			}
			if _, ok := optionSet(field.Options)[values[0]]; !ok {
				return nil, "public.form.error.invalid"
			}
			value = values[0]
		case "rating_1_5":
			if len(values) != 1 {
				return nil, "public.form.error.invalid"
			}
			rating, err := strconv.Atoi(values[0])
			if err != nil || rating < 1 || rating > 5 {
				return nil, "public.form.error.invalid"
			}
			value = rating
		case "textarea":
			if len(values) != 1 {
				return nil, "public.form.error.invalid"
			}
			text := strings.TrimSpace(values[0])
			maxLength := field.Config().MaxLength
			if len([]rune(text)) > maxLength || len([]rune(text)) > 5000 {
				return nil, "public.form.error.too_long"
			}
			value = text
		default:
			return nil, "public.form.error.invalid"
		}
		encoded, _ := json.Marshal(value)
		answers = append(answers, db.SubmissionAnswerInput{
			FieldID: field.ID, FieldPublicID: field.PublicID, FieldType: field.FieldType,
			FieldLabelSnapshot: field.Label, ValueJSON: string(encoded),
		})
	}
	return answers, ""
}

func optionSet(options []db.FormOption) map[string]struct{} {
	set := make(map[string]struct{}, len(options))
	for _, option := range options {
		if !option.ArchivedAt.Valid {
			set[option.Value] = struct{}{}
		}
	}
	return set
}

func (h *Handler) Responses(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.responseCampaign(r)
	if !ok {
		h.forbidden(w, r)
		return
	}
	submissions, err := h.q.ListSubmissions(r.Context(), campaign.ID)
	if err != nil {
		http.Error(w, "load responses", http.StatusInternalServerError)
		return
	}
	web.Render(w, r, http.StatusOK, templates.CampaignResponses(h.cfg.InstanceName, user, campaign, submissions))
}

func (h *Handler) ResponseDetail(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.responseCampaign(r)
	if !ok {
		h.forbidden(w, r)
		return
	}
	submission, err := h.q.GetSubmission(r.Context(), campaign.ID, chi.URLParam(r, "submissionPublicID"))
	if err != nil {
		h.forbidden(w, r)
		return
	}
	web.Render(w, r, http.StatusOK, templates.CampaignResponseDetail(h.cfg.InstanceName, user, campaign, submission))
}

func (h *Handler) responseCampaign(r *http.Request) (db.User, db.Campaign, string, bool) {
	user, _ := auth.UserFromContext(r.Context())
	campaign, err := h.q.GetCampaignByPublicID(r.Context(), chi.URLParam(r, "orgPublicID"), chi.URLParam(r, "campaignPublicID"))
	if err != nil {
		return user, campaign, "", false
	}
	if _, err = h.q.OrganizationRole(r.Context(), user.ID, campaign.OrganizationID); err != nil {
		return user, campaign, "", false
	}
	role, err := h.q.CampaignRole(r.Context(), campaign.ID, user.ID)
	if err != nil {
		return user, campaign, "", false
	}
	return user, campaign, role, role == "owner" || role == "editor" || role == "analyst"
}
