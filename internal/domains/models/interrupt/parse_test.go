package interrupt

import (
	"errors"
	"testing"
)

func TestParseRiskFromArgs_Safe(t *testing.T) {
	lv, r, err := ParseRiskFromArgs(`{"command":"ls","risk_level":"safe","risk_reason":"read-only"}`)
	if err != nil || lv != "safe" || r != "read-only" {
		t.Fatalf("want safe/read-only/nil, got %q/%q/%v", lv, r, err)
	}
}

func TestParseRiskFromArgs_Dangerous(t *testing.T) {
	lv, _, err := ParseRiskFromArgs(`{"command":"rm -rf /","risk_level":"dangerous","risk_reason":"destroys fs"}`)
	if err != nil || lv != "dangerous" {
		t.Fatalf("want dangerous/nil, got %q/%v", lv, err)
	}
}

func TestParseRiskFromArgs_BadJSON(t *testing.T) {
	_, _, err := ParseRiskFromArgs(`not json`)
	if !errors.Is(err, ErrParseJSON) {
		t.Fatalf("want ErrParseJSON, got %v", err)
	}
}

func TestParseRiskFromArgs_MissingRisk(t *testing.T) {
	_, _, err := ParseRiskFromArgs(`{"command":"ls"}`)
	if !errors.Is(err, ErrMissingRisk) {
		t.Fatalf("want ErrMissingRisk, got %v", err)
	}
}

func TestParseRiskFromArgs_InvalidRisk(t *testing.T) {
	_, _, err := ParseRiskFromArgs(`{"command":"ls","risk_level":"highrisk"}`)
	if !errors.Is(err, ErrInvalidRisk) {
		t.Fatalf("want ErrInvalidRisk, got %v", err)
	}
}

func TestParseRiskFromArgs_EmptyReasonOK(t *testing.T) {
	lv, r, err := ParseRiskFromArgs(`{"command":"ls","risk_level":"safe"}`)
	if err != nil || lv != "safe" || r != "" {
		t.Fatalf("want safe/empty/nil, got %q/%q/%v", lv, r, err)
	}
}
