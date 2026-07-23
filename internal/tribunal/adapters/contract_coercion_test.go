package adapters

import (
	"strings"
	"testing"
)

// L-01 regression: a review whose schema_version fields are numeric strings
// ("1", "1.0", "2") must decode via the fail-soft coercion candidate and be
// marked repaired; the same payload with correct integers is not repaired.
func TestDecodeReviewCoercesNumericStringSchemaVersions(t *testing.T) {
	stringVersions := `{
	  "schema_version": "1.0",
	  "reviewer_id": "R-001",
	  "findings": [{
	    "schema_version": "2",
	    "id": "F-1", "reviewer": "R-001", "origin": "panel",
	    "severity": "major", "category": "correctness",
	    "anchor": {"kind": "quote", "packet_item": "artifact:a.md", "quote": "q", "item_sha256": "s"},
	    "issue": "i", "recommendation": "r",
	    "evidence_status": "anchored", "confidence": "high"
	  }]
	}`
	review, repaired, err := DecodeReview([]byte(stringVersions), "R-001")
	if err != nil {
		t.Fatalf("string schema_version payload rejected: %v", err)
	}
	if !repaired {
		t.Fatal("coerced payload must be marked repaired")
	}
	if review.SchemaVersion != 1 || len(review.Findings) != 1 || review.Findings[0].SchemaVersion != 2 {
		t.Fatalf("coerced review = %#v", review)
	}

	intVersions := strings.ReplaceAll(strings.ReplaceAll(stringVersions, `"1.0"`, `1`), `"schema_version": "2"`, `"schema_version": 2`)
	if _, repaired, err := DecodeReview([]byte(intVersions), "R-001"); err != nil || repaired {
		t.Fatalf("integer payload: err=%v repaired=%v, want clean accept", err, repaired)
	}
}

// Non-integral and non-numeric schema_version strings stay rejected —
// coercion must not widen the contract beyond the one known deviation.
func TestDecodeReviewDoesNotCoerceNonIntegralVersions(t *testing.T) {
	for _, version := range []string{`"1.5"`, `"one"`, `"v1"`, `""`} {
		payload := `{"schema_version": ` + version + `, "reviewer_id": "R-001", "findings": []}`
		if _, _, err := DecodeReview([]byte(payload), "R-001"); err == nil {
			t.Fatalf("schema_version %s was accepted", version)
		}
	}
}
