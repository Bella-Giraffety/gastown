package refinery

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestValidateMRFieldsForProcessingRejectsMissingTarget(t *testing.T) {
	err := validateMRFieldsForProcessing("gt-mr", &beads.MRFields{SourceIssue: "gt-src"})
	if err == nil {
		t.Fatal("validateMRFieldsForProcessing returned nil error")
	}
	if !strings.Contains(err.Error(), "target") || !strings.Contains(err.Error(), "instead of defaulting to main") {
		t.Fatalf("error = %q, want missing target recovery guidance", err.Error())
	}
}

func TestValidateMRFieldsForProcessingRejectsMissingSourceIssue(t *testing.T) {
	err := validateMRFieldsForProcessing("gt-mr", &beads.MRFields{Target: "integration/gt-epic"})
	if err == nil {
		t.Fatal("validateMRFieldsForProcessing returned nil error")
	}
	if !strings.Contains(err.Error(), "source_issue") {
		t.Fatalf("error = %q, want missing source_issue", err.Error())
	}
}

func TestValidateMRFieldsForProcessingAcceptsExplicitMainTarget(t *testing.T) {
	err := validateMRFieldsForProcessing("gt-mr", &beads.MRFields{Target: "main", SourceIssue: "gt-src"})
	if err != nil {
		t.Fatalf("validateMRFieldsForProcessing returned error for explicit main target: %v", err)
	}
}
