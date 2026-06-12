package campaigns

import (
	"database/sql"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/koalastuff/koalabye/internal/db"
)

func TestValidateSubmission(t *testing.T) {
	fields := []db.FormField{
		{ID: 1, PublicID: "field_text", FieldType: "textarea", Label: "Why?", Required: true, ConfigJSON: sql.NullString{String: `{"max_length":10}`, Valid: true}},
		{ID: 2, PublicID: "field_rating", FieldType: "rating_1_5", Label: "Rating"},
		{ID: 3, PublicID: "field_radio", FieldType: "radio_group", Label: "Reason", Options: []db.FormOption{{Value: "bugs"}}},
	}
	request := func(values url.Values) (int, string) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		_ = r.ParseForm()
		answers, key := validateSubmission(fields, r)
		return len(answers), key
	}
	count, key := request(url.Values{"field_field_text": {"plain text"}, "field_field_rating": {"5"}, "field_field_radio": {"bugs"}, "unknown": {"ignored"}})
	if key != "" || count != 3 {
		t.Fatalf("valid submission rejected: key=%q answers=%d", key, count)
	}
	for name, values := range map[string]url.Values{
		"required":  {"field_field_rating": {"3"}},
		"rating":    {"field_field_text": {"ok"}, "field_field_rating": {"6"}},
		"radio":     {"field_field_text": {"ok"}, "field_field_radio": {"unknown"}},
		"textarea":  {"field_field_text": {"this is much too long"}},
		"malformed": {"field_field_text": {"ok"}, "field_field_rating": {"1", "2"}},
	} {
		if _, got := request(values); got == "" {
			t.Fatalf("%s validation accepted invalid input", name)
		}
	}
}
