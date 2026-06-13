package campaigns

import (
	"net/url"
	"reflect"
	"testing"
)

func TestSafeURLContext(t *testing.T) {
	values := url.Values{
		"app_version":       {"1.2.3"},
		"platform":          {"chrome"},
		"utm_campaign":      {"summer_launch"},
		"email":             {"person@example.com"},
		"unknown":           {"ignored"},
		"source":            {"javascript:alert"},
		"channel":           {"https://example.com"},
		"extension_version": {"bad value"},
		"utm_term":          {"data:text"},
		"t":                 {"raw-token"},
	}
	got := safeURLContext(values, true)
	want := map[string]string{
		"app_version":  "1.2.3",
		"platform":     "chrome",
		"utm_campaign": "summer_launch",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("safe context=%v want=%v", got, want)
	}
	if disabled := safeURLContext(values, false); disabled != nil {
		t.Fatalf("disabled context=%v", disabled)
	}
}

func TestCoarseUserAgent(t *testing.T) {
	tests := []struct {
		name, raw, browser, os string
	}{
		{"chrome windows", "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/124.0 Safari/537.36", "Chrome", "Windows"},
		{"firefox linux", "Mozilla/5.0 (X11; Linux x86_64; rv:126.0) Gecko/20100101 Firefox/126.0", "Firefox", "Linux"},
		{"safari mac", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 Version/17.5 Safari/605.1.15", "Safari", "macOS"},
		{"edge windows", "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/124.0 Safari/537.36 Edg/124.0", "Edge", "Windows"},
		{"unknown", "", "Unknown", "Unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			browser, os := coarseUserAgent(test.raw)
			if browser != test.browser || os != test.os {
				t.Fatalf("coarseUserAgent()=(%q,%q) want=(%q,%q)", browser, os, test.browser, test.os)
			}
		})
	}
}
