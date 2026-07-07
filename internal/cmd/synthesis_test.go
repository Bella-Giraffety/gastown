package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/workspace"
)

func TestExpandOutputPath(t *testing.T) {
	tests := []struct {
		name      string
		directory string
		pattern   string
		reviewID  string
		legID     string
		want      string
	}{
		{
			name:      "basic expansion",
			directory: ".reviews/{{review_id}}",
			pattern:   "{{leg.id}}-findings.md",
			reviewID:  "abc123",
			legID:     "security",
			want:      ".reviews/abc123/security-findings.md",
		},
		{
			name:      "no templates",
			directory: ".output",
			pattern:   "results.md",
			reviewID:  "xyz",
			legID:     "test",
			want:      ".output/results.md",
		},
		{
			name:      "complex path",
			directory: "reviews/{{review_id}}/findings",
			pattern:   "leg-{{leg.id}}-analysis.md",
			reviewID:  "pr-123",
			legID:     "performance",
			want:      "reviews/pr-123/findings/leg-performance-analysis.md",
		},
		{
			name:      "go template expansion",
			directory: ".designs/{{.review_id}}",
			pattern:   "{{.leg.id}}.md",
			reviewID:  "abc123",
			legID:     "api",
			want:      ".designs/abc123/api.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandOutputPath(tt.directory, tt.pattern, tt.reviewID, tt.legID)
			if filepath.ToSlash(got) != tt.want {
				t.Errorf("expandOutputPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLegOutput(t *testing.T) {
	// Test LegOutput struct
	output := LegOutput{
		LegID:    "correctness",
		Title:    "Correctness Review",
		Status:   "closed",
		FilePath: "/tmp/findings.md",
		Content:  "## Findings\n\nNo issues found.",
		HasFile:  true,
	}

	if output.LegID != "correctness" {
		t.Errorf("LegID = %q, want %q", output.LegID, "correctness")
	}

	if output.Status != "closed" {
		t.Errorf("Status = %q, want %q", output.Status, "closed")
	}

	if !output.HasFile {
		t.Error("HasFile should be true")
	}
}

func TestConvoyMeta(t *testing.T) {
	// Test ConvoyMeta struct
	meta := ConvoyMeta{
		ID:        "hq-cv-abc",
		Title:     "Code Review: PR #123",
		Status:    "open",
		Formula:   "code-review",
		ReviewID:  "pr123",
		LegIssues: []string{"gt-leg1", "gt-leg2", "gt-leg3"},
	}

	if meta.ID != "hq-cv-abc" {
		t.Errorf("ID = %q, want %q", meta.ID, "hq-cv-abc")
	}

	if len(meta.LegIssues) != 3 {
		t.Errorf("len(LegIssues) = %d, want 3", len(meta.LegIssues))
	}
}

func TestParseConvoyMetaDescription_LegacyFormulaConvoyAndRig(t *testing.T) {
	meta := &ConvoyMeta{}
	parseConvoyMetaDescription(meta, "Formula convoy: code-review\n\nLegs: 3\nRig: gastown\nReview ID: abc123")

	if meta.Formula != "code-review" {
		t.Fatalf("Formula = %q, want code-review", meta.Formula)
	}
	if meta.Rig != "gastown" {
		t.Fatalf("Rig = %q, want gastown", meta.Rig)
	}
	if meta.ReviewID != "abc123" {
		t.Fatalf("ReviewID = %q, want abc123", meta.ReviewID)
	}
}

func TestLoadSynthesisFormula_FormulaPathPrecedence(t *testing.T) {
	formulaPath := filepath.Join(t.TempDir(), "explicit.formula.toml")
	if err := os.WriteFile(formulaPath, []byte(testWorkflowFormula("explicit-formula", 7)), 0644); err != nil {
		t.Fatalf("write explicit formula: %v", err)
	}

	f, err := loadSynthesisFormula(&ConvoyMeta{
		Formula:     "mol-polecat-work",
		FormulaPath: formulaPath,
	}, "", "")
	if err != nil {
		t.Fatalf("loadSynthesisFormula() error = %v", err)
	}
	if f.Name != "explicit-formula" || f.Version != 7 {
		t.Fatalf("loaded formula = %s v%d, want explicit-formula v7", f.Name, f.Version)
	}
}

func TestLoadFormulaByName_UsesRoutedRigBeadsDir(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir town beads: %v", err)
	}
	if err := beads.AppendRoute(townRoot, beads.Route{Prefix: "gt-", Path: "gastown/mayor/rig"}); err != nil {
		t.Fatalf("append route: %v", err)
	}

	decoyDir := filepath.Join(townRoot, "gastown", ".beads", "formulas")
	if err := os.MkdirAll(decoyDir, 0755); err != nil {
		t.Fatalf("mkdir decoy formulas: %v", err)
	}
	if err := os.WriteFile(filepath.Join(decoyDir, "routed-only.formula.toml"), []byte(testWorkflowFormula("routed-only", 1)), 0644); err != nil {
		t.Fatalf("write decoy formula: %v", err)
	}

	routedDir := filepath.Join(townRoot, "gastown", "mayor", "rig", ".beads", "formulas")
	if err := os.MkdirAll(routedDir, 0755); err != nil {
		t.Fatalf("mkdir routed formulas: %v", err)
	}
	if err := os.WriteFile(filepath.Join(routedDir, "routed-only.formula.toml"), []byte(testWorkflowFormula("routed-only", 2)), 0644); err != nil {
		t.Fatalf("write routed formula: %v", err)
	}

	f, err := loadFormulaByName("routed-only", townRoot, "gastown")
	if err != nil {
		t.Fatalf("loadFormulaByName() error = %v", err)
	}
	if f.Version != 2 {
		t.Fatalf("loaded version = %d, want routed version 2", f.Version)
	}
}

func TestRenderFormulaStepsFull_UsesRoutedRigFormula(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir town beads: %v", err)
	}
	if err := beads.AppendRoute(townRoot, beads.Route{Prefix: "gt-", Path: "gastown/mayor/rig"}); err != nil {
		t.Fatalf("append route: %v", err)
	}

	decoyDir := filepath.Join(townRoot, "gastown", ".beads", "formulas")
	if err := os.MkdirAll(decoyDir, 0755); err != nil {
		t.Fatalf("mkdir decoy formulas: %v", err)
	}
	if err := os.WriteFile(filepath.Join(decoyDir, "prime-routed.formula.toml"), []byte(testWorkflowFormulaWithStep("prime-routed", 1, "Decoy Step")), 0644); err != nil {
		t.Fatalf("write decoy formula: %v", err)
	}

	routedDir := filepath.Join(townRoot, "gastown", "mayor", "rig", ".beads", "formulas")
	if err := os.MkdirAll(routedDir, 0755); err != nil {
		t.Fatalf("mkdir routed formulas: %v", err)
	}
	if err := os.WriteFile(filepath.Join(routedDir, "prime-routed.formula.toml"), []byte(testWorkflowFormulaWithStep("prime-routed", 2, "Routed Step")), 0644); err != nil {
		t.Fatalf("write routed formula: %v", err)
	}

	rendered, err := renderFormulaStepsFull("prime-routed", townRoot, "gastown")
	if err != nil {
		t.Fatalf("renderFormulaStepsFull() error = %v", err)
	}
	if !strings.Contains(rendered, "Routed Step") {
		t.Fatalf("rendered steps did not use routed formula: %s", rendered)
	}
	if strings.Contains(rendered, "Decoy Step") {
		t.Fatalf("rendered steps used decoy formula: %s", rendered)
	}
}

func TestRenderFormulaStepsFull_IgnoresLegacyHomeFormula(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	homeFormulas := filepath.Join(home, ".beads", "formulas")
	if err := os.MkdirAll(homeFormulas, 0755); err != nil {
		t.Fatalf("mkdir home formulas: %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeFormulas, "mol-polecat-work.formula.toml"), []byte(testWorkflowFormulaWithStep("mol-polecat-work", 99, "Home Step")), 0644); err != nil {
		t.Fatalf("write home formula: %v", err)
	}

	rendered, err := renderFormulaStepsFull("mol-polecat-work", "", "")
	if err != nil {
		t.Fatalf("renderFormulaStepsFull() error = %v", err)
	}
	if strings.Contains(rendered, "Home Step") {
		t.Fatalf("prime rendering used legacy home formula: %s", rendered)
	}
}

func TestLoadSynthesisFormula_UsesGTTownRootOnly(t *testing.T) {
	townRoot := setupFormulaResolverTown(t)
	formulasDir := filepath.Join(townRoot, ".beads", "formulas")
	if err := os.MkdirAll(formulasDir, 0755); err != nil {
		t.Fatalf("mkdir formulas: %v", err)
	}
	if err := os.WriteFile(filepath.Join(formulasDir, "town-only.formula.toml"), []byte(testWorkflowFormula("town-only", 3)), 0644); err != nil {
		t.Fatalf("write town formula: %v", err)
	}
	chdirForTest(t, t.TempDir())
	t.Setenv("GT_ROOT", "")
	t.Setenv("GT_TOWN_ROOT", townRoot)
	t.Setenv("HOME", t.TempDir())

	resolvedTownRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		t.Fatalf("FindFromCwdOrError() error = %v", err)
	}
	f, err := loadSynthesisFormula(&ConvoyMeta{Formula: "town-only"}, resolvedTownRoot, "")
	if err != nil {
		t.Fatalf("loadSynthesisFormula() error = %v", err)
	}
	if f.Version != 3 {
		t.Fatalf("loaded version = %d, want 3", f.Version)
	}
}

func TestLoadSynthesisFormula_EmbeddedFallback(t *testing.T) {
	f, err := loadSynthesisFormula(&ConvoyMeta{Formula: "mol-polecat-work"}, "", "")
	if err != nil {
		t.Fatalf("loadSynthesisFormula() error = %v", err)
	}
	if f == nil || f.Name != "mol-polecat-work" {
		t.Fatalf("loaded formula = %#v, want mol-polecat-work", f)
	}
}

func TestRunFormulaRunDryRun_UsesGTTownRootOnlyEmbeddedFormula(t *testing.T) {
	townRoot := setupFormulaResolverTown(t)
	chdirForTest(t, t.TempDir())
	t.Setenv("GT_ROOT", "")
	t.Setenv("GT_TOWN_ROOT", townRoot)
	t.Setenv("HOME", t.TempDir())

	oldRig := formulaRunRig
	oldDryRun := formulaRunDryRun
	oldPR := formulaRunPR
	oldAgent := formulaRunAgent
	oldFiles := formulaRunFiles
	oldSet := formulaRunSet
	t.Cleanup(func() {
		formulaRunRig = oldRig
		formulaRunDryRun = oldDryRun
		formulaRunPR = oldPR
		formulaRunAgent = oldAgent
		formulaRunFiles = oldFiles
		formulaRunSet = oldSet
	})
	formulaRunRig = "gastown"
	formulaRunDryRun = true
	formulaRunPR = 0
	formulaRunAgent = ""
	formulaRunFiles = nil
	formulaRunSet = nil

	if err := runFormulaRun(nil, []string{"mol-polecat-work"}); err != nil {
		t.Fatalf("runFormulaRun() error = %v", err)
	}
}

func setupFormulaResolverTown(t *testing.T) string {
	t.Helper()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"type":"town","version":1,"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	return townRoot
}

func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}

func testWorkflowFormula(name string, version int) string {
	return testWorkflowFormulaWithStep(name, version, "Step")
}

func testWorkflowFormulaWithStep(name string, version int, title string) string {
	return fmt.Sprintf(`formula = %q
type = "workflow"
version = %d

[[steps]]
id = "step"
title = %q
description = "Do it"
`, name, version, title)
}
