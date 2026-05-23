package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestResolveDoneMRTargetFailsClosedWhenSourceLookupFailed(t *testing.T) {
	_, _, err := resolveDoneMRTarget("main", "", nil, "gt-src", nil)
	if err == nil {
		t.Fatal("resolveDoneMRTarget returned nil error for missing source issue")
	}
	if !strings.Contains(err.Error(), "source issue gt-src could not be loaded") {
		t.Fatalf("error = %q, want source lookup guidance", err.Error())
	}
}

func TestResolveDoneMRTargetAllowsExplicitMainWithoutSourceLookup(t *testing.T) {
	target, explicit, err := resolveDoneMRTarget("main", "main", nil, "gt-src", nil)
	if err != nil {
		t.Fatalf("resolveDoneMRTarget returned error for explicit main target: %v", err)
	}
	if !explicit || target != "main" {
		t.Fatalf("target=%q explicit=%v, want explicit main", target, explicit)
	}
}

func TestResolveDoneMRTargetUsesFormulaBaseBranchIncludingMain(t *testing.T) {
	source := &beads.Issue{Description: "formula_vars: issue=gt-src base_branch=main"}
	target, explicit, err := resolveDoneMRTarget("main", "", source, "gt-src", func() (string, error) {
		return "integration/should-not-run", nil
	})
	if err != nil {
		t.Fatalf("resolveDoneMRTarget returned error: %v", err)
	}
	if explicit || target != "main" {
		t.Fatalf("target=%q explicit=%v, want formula main", target, explicit)
	}
}

func TestResolveDoneMRTargetFailsClosedOnIntegrationDetectionError(t *testing.T) {
	source := &beads.Issue{Description: "plain source issue"}
	_, _, err := resolveDoneMRTarget("main", "", source, "gt-src", func() (string, error) {
		return "", errors.New("lookup parent: dolt unavailable")
	})
	if err == nil {
		t.Fatal("resolveDoneMRTarget returned nil error for integration detection failure")
	}
	if !strings.Contains(err.Error(), "rerun with --target <branch>") {
		t.Fatalf("error = %q, want explicit recovery guidance", err.Error())
	}
}

func TestResolveDoneMRTargetDefaultsOnlyAfterSourceLoaded(t *testing.T) {
	target, explicit, err := resolveDoneMRTarget("main", "", &beads.Issue{Description: "plain source issue"}, "gt-src", nil)
	if err != nil {
		t.Fatalf("resolveDoneMRTarget returned error: %v", err)
	}
	if explicit || target != "main" {
		t.Fatalf("target=%q explicit=%v, want default main after source load", target, explicit)
	}
}
