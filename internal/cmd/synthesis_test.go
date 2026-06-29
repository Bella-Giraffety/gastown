package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

func TestRunSynthesisClose_ExportsJSONLAfterClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"type":"town","name":"test"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	logPath := filepath.Join(binDir, "bd.log")
	bdScript := `#!/bin/sh
LOG="` + logPath + `"
case "$1" in
  show)
    printf 'show|%s|%s|%s|%s|%s|%s|%s|%s\n' "$*" "$(pwd)" "${BEADS_DIR:-}" "${BEADS_DOLT_SERVER_DATABASE:-}" "${BEADS_DB:-}" "${BD_DB:-}" "${BD_DOLT_AUTO_COMMIT:-}" "${BD_READONLY:-}" >> "$LOG"
    printf '%s\n' '[{"status":"open"}]'
    exit 0
    ;;
  close)
    printf 'close|%s|%s|%s|%s|%s|%s\n' "$*" "$(pwd)" "${BEADS_DIR:-}" "${BD_DOLT_AUTO_COMMIT:-}" "${BD_READONLY:-}" "${BD_NO_GIT_OPS:-}" >> "$LOG"
    exit 0
    ;;
  export)
    printf 'export|%s|%s|%s|%s|%s|%s\n' "$*" "$(pwd)" "${BEADS_DIR:-}" "${BD_DOLT_AUTO_COMMIT:-}" "${BD_READONLY:-}" "${BD_NO_GIT_OPS:-}" >> "$LOG"
    exit 0
    ;;
esac
exit 1
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GT_AGENT", "")
	t.Setenv("GT_SESSION_ID_ENV", "")
	t.Setenv("CLAUDE_SESSION_ID", "")
	t.Setenv("BD_DOLT_AUTO_COMMIT", "off")
	t.Setenv("BD_READONLY", "true")
	t.Setenv("BD_NO_GIT_OPS", "false")
	t.Setenv("BEADS_DIR", filepath.Join(townRoot, "wrong", ".beads"))
	t.Setenv("BEADS_DOLT_SERVER_DATABASE", "wrong-db")
	t.Setenv("BEADS_DB", "wrong-beads-db")
	t.Setenv("BD_DB", "wrong-bd-db")
	t.Setenv("GT_DOLT_DATA", "")
	t.Setenv("GT_DOLT_HOST", "")
	t.Setenv("GT_DOLT_PORT", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := runSynthesisClose(nil, []string{"hq-cv-synth"}); err != nil {
		t.Fatalf("runSynthesisClose returned error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("log lines = %v, want show, close, and export", lines)
	}
	showFields := strings.Split(lines[0], "|")
	if len(showFields) != 9 {
		t.Fatalf("show fields = %v, want 9", showFields)
	}
	if showFields[1] != "show hq-cv-synth --json" {
		t.Fatalf("show args = %q, want synthesis show", showFields[1])
	}
	if showFields[2] != townRoot || showFields[3] != filepath.Join(townRoot, ".beads") {
		t.Fatalf("show target = cwd %q BEADS_DIR %q, want town root and town .beads", showFields[2], showFields[3])
	}
	if showFields[4] != "" || showFields[5] != "" || showFields[6] != "" {
		t.Fatalf("show stale target env leaked: DB=%q BEADS_DB=%q BD_DB=%q", showFields[4], showFields[5], showFields[6])
	}
	if showFields[7] != "off" || showFields[8] != "true" {
		t.Fatalf("show env = BD_DOLT_AUTO_COMMIT=%q BD_READONLY=%q, want read-only", showFields[7], showFields[8])
	}

	closeFields := strings.Split(lines[1], "|")
	if len(closeFields) != 7 {
		t.Fatalf("close fields = %v, want 7", closeFields)
	}
	if !strings.Contains(closeFields[1], "close hq-cv-synth --reason=synthesis complete") {
		t.Fatalf("close args = %q, want synthesis close", closeFields[1])
	}
	if closeFields[2] != townRoot || closeFields[3] != filepath.Join(townRoot, ".beads") {
		t.Fatalf("close target = cwd %q BEADS_DIR %q, want town root and town .beads", closeFields[2], closeFields[3])
	}
	if closeFields[4] != "on" || closeFields[5] != "" || closeFields[6] != "true" {
		t.Fatalf("close env = BD_DOLT_AUTO_COMMIT=%q BD_READONLY=%q BD_NO_GIT_OPS=%q, want mutation/suppressed", closeFields[4], closeFields[5], closeFields[6])
	}

	exportFields := strings.Split(lines[2], "|")
	if len(exportFields) != 7 {
		t.Fatalf("export fields = %v, want 7", exportFields)
	}
	if want := "export -o " + filepath.Join(townRoot, ".beads", "issues.jsonl"); exportFields[1] != want {
		t.Fatalf("export args = %q, want %q", exportFields[1], want)
	}
	if exportFields[2] != townRoot || exportFields[3] != filepath.Join(townRoot, ".beads") {
		t.Fatalf("export target = cwd %q BEADS_DIR %q, want town root and town .beads", exportFields[2], exportFields[3])
	}
	if exportFields[4] != "off" || exportFields[5] != "true" || exportFields[6] != "true" {
		t.Fatalf("export env = BD_DOLT_AUTO_COMMIT=%q BD_READONLY=%q BD_NO_GIT_OPS=%q, want read-only/suppressed", exportFields[4], exportFields[5], exportFields[6])
	}
}

func TestRunSynthesisClose_SkipsAlreadyClosedWithoutExport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"type":"town","name":"test"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	logPath := filepath.Join(binDir, "bd.log")
	bdScript := `#!/bin/sh
case "$1" in
  show)
    echo show >> "` + logPath + `"
    printf '%s\n' '[{"status":"closed"}]'
    exit 0
    ;;
  close|export)
    echo "$1" >> "` + logPath + `"
    exit 0
    ;;
esac
exit 1
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := runSynthesisClose(nil, []string{"hq-cv-synth"}); err != nil {
		t.Fatalf("runSynthesisClose returned error: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "show" {
		t.Fatalf("bd calls = %q, want only show", got)
	}
}
