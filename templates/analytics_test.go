package templates

import "testing"

func TestSubmissionRate(t *testing.T) {
	if got := submissionRate(3, 0); got != "—" {
		t.Fatalf("zero-visit rate=%q", got)
	}
	if got := submissionRate(1, 4); got != "25.0%" {
		t.Fatalf("submission rate=%q", got)
	}
}
