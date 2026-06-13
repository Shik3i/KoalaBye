package campaigns

import (
	"context"

	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/ids"
)

type presetFieldOption struct {
	label map[string]string
	value string
}

type presetField struct {
	fieldType string
	label     map[string]string
	required  bool
	options   []presetFieldOption
}

var presets = map[string][]presetField{
	"quick_feedback": {
		{
			fieldType: "rating_1_5",
			label: map[string]string{
				"en": "How was your experience overall?",
				"de": "Wie war Ihre Erfahrung insgesamt?",
				"es": "¿Cómo fue tu experiencia en general?",
			},
			required: true,
		},
		{
			fieldType: "textarea",
			label: map[string]string{
				"en": "What is the one thing we should improve?",
				"de": "Was sollten wir als Erstes verbessern?",
				"es": "¿Qué es lo primero que deberíamos mejorar?",
			},
		},
	},
	"uninstall": {
		{
			fieldType: "radio_group",
			label: map[string]string{
				"en": "Why are you uninstalling?",
				"de": "Warum deinstallieren Sie die Erweiterung?",
				"es": "¿Por qué estás desinstalando la extensión?",
			},
			required: true,
			options: []presetFieldOption{
				{label: map[string]string{"en": "It doesn't work", "de": "Sie funktioniert nicht", "es": "No funciona"}, value: "not_working"},
				{label: map[string]string{"en": "I don't need it anymore", "de": "Ich brauche sie nicht mehr", "es": "Ya no la necesito"}, value: "no_longer_needed"},
				{label: map[string]string{"en": "Found a better alternative", "de": "Eine bessere Alternative gefunden", "es": "Encontré una alternativa mejor"}, value: "better_alternative"},
				{label: map[string]string{"en": "Too many ads / spam", "de": "Zu viel Werbung / Spam", "es": "Demasiados anuncios / spam"}, value: "too_many_ads"},
				{label: map[string]string{"en": "Other", "de": "Anderer Grund", "es": "Otro motivo"}, value: "other"},
			},
		},
		{
			fieldType: "textarea",
			label: map[string]string{
				"en": "What could we have done better?",
				"de": "Was hätten wir besser machen können?",
				"es": "¿Qué podríamos haber hecho mejor?",
			},
			required: false,
		},
		{
			fieldType: "radio_group",
			label: map[string]string{
				"en": "Would you consider using it again in the future?",
				"de": "Würden Sie die Erweiterung in Zukunft wieder nutzen?",
				"es": "¿Considerarías usarla de nuevo en el futuro?",
			},
			required: false,
			options: []presetFieldOption{
				{label: map[string]string{"en": "Yes", "de": "Ja", "es": "Sí"}, value: "yes"},
				{label: map[string]string{"en": "No", "de": "Nein", "es": "No"}, value: "no"},
				{label: map[string]string{"en": "Maybe", "de": "Vielleicht", "es": "Tal vez"}, value: "maybe"},
			},
		},
	},
	"bug_report": {
		{
			fieldType: "radio_group",
			label: map[string]string{
				"en": "What issue did you experience?",
				"de": "Welches Problem ist aufgetreten?",
				"es": "¿Qué problema experimentaste?",
			},
			required: true,
			options: []presetFieldOption{
				{label: map[string]string{"en": "Extension crashed", "de": "Erweiterung ist abgestürzt", "es": "La extensión se cerró"}, value: "crashed"},
				{label: map[string]string{"en": "Slow loading / performance", "de": "Langsame Ladezeit / Performance", "es": "Lentitud / rendimiento"}, value: "slow_loading"},
				{label: map[string]string{"en": "Broken features", "de": "Defekte Funktionen", "es": "Funciones no operativas"}, value: "broken_features"},
				{label: map[string]string{"en": "Confusing interface", "de": "Verwirrende Benutzeroberfläche", "es": "Interfaz confusa"}, value: "confusing_ui"},
				{label: map[string]string{"en": "Other", "de": "Anderes Problem", "es": "Otro"}, value: "other"},
			},
		},
		{
			fieldType: "textarea",
			label: map[string]string{
				"en": "Please describe the issue in detail.",
				"de": "Bitte beschreiben Sie das Problem im Detail.",
				"es": "Por favor, describe el problema en detalle.",
			},
			required: true,
		},
		{
			fieldType: "textarea",
			label: map[string]string{
				"en": "Which browser and version were you using?",
				"de": "Welchen Browser und welche Version haben Sie genutzt?",
				"es": "¿Qué navegador y versión estabas usando?",
			},
			required: false,
		},
	},
	"feature_feedback": {
		{
			fieldType: "textarea",
			label: map[string]string{
				"en": "What is your favorite feature?",
				"de": "Was ist Ihre Lieblingsfunktion?",
				"es": "¿Cuál es tu función favorita?",
			},
			required: false,
		},
		{
			fieldType: "textarea",
			label: map[string]string{
				"en": "What feature is missing or needs improvement?",
				"de": "Welche Funktion fehlt oder muss verbessert werden?",
				"es": "¿Qué función falta o necesita mejorar?",
			},
			required: true,
		},
		{
			fieldType: "rating_1_5",
			label: map[string]string{
				"en": "How would you rate the overall usability?",
				"de": "Wie bewerten Sie die allgemeine Benutzerfreundlichkeit?",
				"es": "¿Cómo calificarías la usabilidad general?",
			},
			required: true,
		},
	},
	"satisfaction": {
		{
			fieldType: "rating_1_5",
			label: map[string]string{
				"en": "How satisfied are you with this extension?",
				"de": "Wie zufrieden sind Sie mit dieser Erweiterung?",
				"es": "¿Qué tan satisfecho estás con esta extensión?",
			},
			required: true,
		},
		{
			fieldType: "textarea",
			label: map[string]string{
				"en": "What is the main reason for your score?",
				"de": "Was ist der Hauptgrund für Ihre Bewertung?",
				"es": "¿Cuál is el motivo principal de tu puntuación?",
			},
			required: false,
		},
	},
	"onboarding": {
		{
			fieldType: "rating_1_5",
			label: map[string]string{
				"en": "How easy was it to get started?",
				"de": "Wie einfach war der Einstieg?",
				"es": "¿Qué tan fácil fue comenzar?",
			},
			required: true,
		},
		{
			fieldType: "radio_group",
			label: map[string]string{
				"en": "Did you achieve what you came to do?",
				"de": "Konnten Sie Ihr Ziel erreichen?",
				"es": "¿Lograste lo que querías hacer?",
			},
			required: true,
			options: []presetFieldOption{
				{label: map[string]string{"en": "Yes, completely", "de": "Ja, vollständig", "es": "Sí, por completo"}, value: "yes"},
				{label: map[string]string{"en": "Partly", "de": "Teilweise", "es": "Parcialmente"}, value: "partly"},
				{label: map[string]string{"en": "No", "de": "Nein", "es": "No"}, value: "no"},
			},
		},
		{
			fieldType: "textarea",
			label: map[string]string{
				"en": "What was unclear or difficult?",
				"de": "Was war unklar oder schwierig?",
				"es": "¿Qué fue confuso o difícil?",
			},
		},
	},
	"performance": {
		{
			fieldType: "radio_group",
			label: map[string]string{
				"en": "Where did the product feel slow?",
				"de": "Wo fühlte sich das Produkt langsam an?",
				"es": "¿Dónde sentiste lentitud?",
			},
			required: true,
			options: []presetFieldOption{
				{label: map[string]string{"en": "Startup", "de": "Start", "es": "Inicio"}, value: "startup"},
				{label: map[string]string{"en": "Loading content", "de": "Inhalte laden", "es": "Carga de contenido"}, value: "loading"},
				{label: map[string]string{"en": "Using a feature", "de": "Nutzung einer Funktion", "es": "Uso de una función"}, value: "feature"},
				{label: map[string]string{"en": "Everywhere", "de": "Überall", "es": "En todas partes"}, value: "everywhere"},
			},
		},
		{
			fieldType: "textarea",
			label: map[string]string{
				"en": "What were you doing when it became slow?",
				"de": "Was haben Sie getan, als es langsam wurde?",
				"es": "¿Qué estabas haciendo cuando se volvió lento?",
			},
			required: true,
		},
	},
	"support_followup": {
		{
			fieldType: "rating_1_5",
			label: map[string]string{
				"en": "How satisfied are you with the support you received?",
				"de": "Wie zufrieden sind Sie mit dem erhaltenen Support?",
				"es": "¿Qué tan satisfecho estás con el soporte recibido?",
			},
			required: true,
		},
		{
			fieldType: "radio_group",
			label: map[string]string{
				"en": "Was your issue resolved?",
				"de": "Wurde Ihr Problem gelöst?",
				"es": "¿Se resolvió tu problema?",
			},
			required: true,
			options: []presetFieldOption{
				{label: map[string]string{"en": "Yes", "de": "Ja", "es": "Sí"}, value: "yes"},
				{label: map[string]string{"en": "Partly", "de": "Teilweise", "es": "Parcialmente"}, value: "partly"},
				{label: map[string]string{"en": "No", "de": "Nein", "es": "No"}, value: "no"},
			},
		},
		{
			fieldType: "textarea",
			label: map[string]string{
				"en": "What could we do better next time?",
				"de": "Was können wir beim nächsten Mal besser machen?",
				"es": "¿Qué podríamos hacer mejor la próxima vez?",
			},
		},
	},
}

var presetOrder = []string{
	"quick_feedback",
	"uninstall",
	"bug_report",
	"feature_feedback",
	"satisfaction",
	"onboarding",
	"performance",
	"support_followup",
}

func PresetNames() []string {
	return append([]string(nil), presetOrder...)
}

func ApplyFormPreset(ctx context.Context, q *db.Querier, campaignID int64, presetName string, lang string, actorID int64) error {
	fields, ok := presets[presetName]
	if !ok {
		return nil // None preset or invalid preset name is a no-op
	}

	for _, pf := range fields {
		fieldPublicID, err := ids.New("field")
		if err != nil {
			return err
		}

		label := pf.label[lang]
		if label == "" {
			label = pf.label["en"]
		}

		err = q.CreateFormField(ctx, db.SaveFormFieldInput{
			PublicID:   fieldPublicID,
			CampaignID: campaignID,
			FieldType:  pf.fieldType,
			Label:      label,
			Required:   pf.required,
		}, actorID)
		if err != nil {
			return err
		}

		if len(pf.options) > 0 {
			// Get the created field from db to get its internal ID
			dbField, err := q.GetFormField(ctx, campaignID, fieldPublicID)
			if err != nil {
				return err
			}

			for _, po := range pf.options {
				optPublicID, err := ids.New("option")
				if err != nil {
					return err
				}

				optLabel := po.label[lang]
				if optLabel == "" {
					optLabel = po.label["en"]
				}

				err = q.CreateFormOption(ctx, campaignID, dbField.ID, optPublicID, optLabel, po.value, actorID)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}
