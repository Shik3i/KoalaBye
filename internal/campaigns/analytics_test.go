package campaigns

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func TestAnalyticsRangeAndSubmissionRate(t *testing.T) {
	now := time.Date(2026, 6, 12, 18, 0, 0, 0, time.UTC)
	key, start, end := analyticsRange("7", now)
	if key != "7" || start == nil || start.Format(time.RFC3339) != "2026-06-06T00:00:00Z" {
		t.Fatalf("unexpected seven-day range: %q %v", key, start)
	}
	if end == nil || end.Format(time.RFC3339) != "2026-06-13T00:00:00Z" {
		t.Fatalf("unexpected seven-day end: %v", end)
	}
	key, start, end = analyticsRange("all", now)
	if key != "all" || start != nil || end != nil {
		t.Fatalf("unexpected all-time range: %q %v %v", key, start, end)
	}
}

func TestCSVAnswerEscapingAndSanitization(t *testing.T) {
	var output strings.Builder
	writer := csv.NewWriter(&output)
	if err := writer.Write([]string{exportAnswerValue(`"line one,\nline two"`)}); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	records, err := csv.NewReader(strings.NewReader(output.String())).ReadAll()
	if err != nil || len(records) != 1 || records[0][0] != "line one,\nline two" {
		t.Fatalf("CSV round trip failed: %#v %v", records, err)
	}
	if got := sanitizeExportLabel("Why, exactly?"); got != "why_exactly" {
		t.Fatalf("sanitized label=%q", got)
	}
	for _, value := range []string{"=1+1", "+SUM(A1:A2)", "-2", "@cmd"} {
		if got := safeCSVCell(value); !strings.HasPrefix(got, "'") {
			t.Fatalf("unsafe CSV value %q was not neutralized: %q", value, got)
		}
	}
}
