package cmd

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/formula"
)

func TestExtractPatrolRole(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		expected string
	}{
		{
			name:     "deacon patrol",
			title:    "Digest: mol-deacon-patrol",
			expected: "deacon",
		},
		{
			name:     "witness patrol",
			title:    "Digest: mol-witness-patrol",
			expected: "witness",
		},
		{
			name:     "refinery patrol",
			title:    "Digest: mol-refinery-patrol",
			expected: "refinery",
		},
		{
			name:     "wisp digest without patrol suffix",
			title:    "Digest: gt-wisp-abc123",
			expected: "patrol",
		},
		{
			name:     "random title",
			title:    "Some other digest",
			expected: "patrol",
		},
		{
			name:     "empty title",
			title:    "",
			expected: "patrol",
		},
		{
			name:     "just digest prefix",
			title:    "Digest: ",
			expected: "patrol",
		},
		{
			name:     "mol prefix but no patrol suffix",
			title:    "Digest: mol-deacon-other",
			expected: "patrol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPatrolRole(tt.title)
			if got != tt.expected {
				t.Errorf("extractPatrolRole(%q) = %q, want %q", tt.title, got, tt.expected)
			}
		})
	}
}

func TestPatrolDigestDateFormat(t *testing.T) {
	// Test that PatrolDigest.Date format is YYYY-MM-DD
	digest := PatrolDigest{
		Date:        "2026-01-17",
		TotalCycles: 5,
		ByRole:      map[string]int{"deacon": 2, "witness": 3},
	}

	if digest.Date != "2026-01-17" {
		t.Errorf("Date format incorrect: got %q", digest.Date)
	}

	if digest.TotalCycles != 5 {
		t.Errorf("TotalCycles: got %d, want 5", digest.TotalCycles)
	}

	if digest.ByRole["deacon"] != 2 {
		t.Errorf("ByRole[deacon]: got %d, want 2", digest.ByRole["deacon"])
	}
}

func TestPatrolCycleEntry(t *testing.T) {
	entry := PatrolCycleEntry{
		ID:          "gt-abc123",
		Role:        "deacon",
		Title:       "Digest: mol-deacon-patrol",
		Description: "Test description",
	}

	if entry.ID != "gt-abc123" {
		t.Errorf("ID: got %q, want %q", entry.ID, "gt-abc123")
	}

	if entry.Role != "deacon" {
		t.Errorf("Role: got %q, want %q", entry.Role, "deacon")
	}
}

func TestParseStepResults(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:  "single step",
			input: "heartbeat:OK",
			expected: map[string]string{
				"heartbeat": "OK",
			},
		},
		{
			name:  "multiple steps",
			input: "heartbeat:OK,inbox-check:OK,orphan-cleanup:SKIP",
			expected: map[string]string{
				"heartbeat":      "OK",
				"inbox-check":    "OK",
				"orphan-cleanup": "SKIP",
			},
		},
		{
			name:  "mixed case normalized to upper",
			input: "heartbeat:ok,inbox-check:Skip",
			expected: map[string]string{
				"heartbeat":   "OK",
				"inbox-check": "SKIP",
			},
		},
		{
			name:  "whitespace trimmed",
			input: " heartbeat : OK , inbox-check : OK ",
			expected: map[string]string{
				"heartbeat":   "OK",
				"inbox-check": "OK",
			},
		},
		{
			name:  "trailing comma ignored",
			input: "heartbeat:OK,",
			expected: map[string]string{
				"heartbeat": "OK",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStepResults(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("parseStepResults(%q) returned %d entries, want %d", tt.input, len(got), len(tt.expected))
				return
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("parseStepResults(%q)[%q] = %q, want %q", tt.input, k, got[k], v)
				}
			}
		})
	}
}

func TestBuildStepAudit(t *testing.T) {
	tests := []struct {
		name        string
		formulaName string
		stepsFlag   string
		wantErr     string
		wantPrefix  string // check prefix of output
		wantSuffix  string // check suffix of output
		wantContain string // check output contains this
	}{
		{
			name:        "deacon patrol with no steps reported",
			formulaName: "mol-deacon-patrol",
			stepsFlag:   "",
			wantErr:     "--steps is required",
		},
		{
			name:        "deacon patrol with all steps OK",
			formulaName: "mol-deacon-patrol",
			stepsFlag:   mustBuildStepFlag(t, "mol-deacon-patrol", "OK", nil),
			wantPrefix:  "Steps:",
			wantSuffix:  "(26/26)",
			wantContain: "heartbeat OK",
		},
		{
			name:        "deacon patrol with some steps skipped",
			formulaName: "mol-deacon-patrol",
			stepsFlag:   mustBuildStepFlag(t, "mol-deacon-patrol", "SKIP", map[string]string{"heartbeat": "OK", "inbox-check": "OK", "loop-or-exit": "OK"}),
			wantPrefix:  "Steps:",
			wantSuffix:  "(3/26)",
			wantContain: "orphan-process-cleanup SKIP",
		},
		{
			name:        "missing steps fail validation",
			formulaName: "mol-deacon-patrol",
			stepsFlag:   "heartbeat:OK",
			wantErr:     "--steps missing required formula step IDs: inbox-check",
		},
		{
			name:        "unknown step fails validation",
			formulaName: "mol-deacon-patrol",
			stepsFlag:   mustBuildStepFlag(t, "mol-deacon-patrol", "OK", map[string]string{"not-a-step": "OK"}),
			wantErr:     "--steps contains unknown step IDs: not-a-step",
		},
		{
			name:        "nonexistent formula fails validation",
			formulaName: "mol-nonexistent",
			stepsFlag:   "heartbeat:OK",
			wantErr:     "loading patrol formula \"mol-nonexistent\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildStepAudit(tt.formulaName, tt.stepsFlag)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("buildStepAudit() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("buildStepAudit() error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildStepAudit() error = %v", err)
			}
			if tt.wantPrefix != "" && !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("buildStepAudit() = %q, want prefix %q", got, tt.wantPrefix)
			}
			if tt.wantSuffix != "" && !strings.HasSuffix(got, tt.wantSuffix) {
				t.Errorf("buildStepAudit() = %q, want suffix %q", got, tt.wantSuffix)
			}
			if tt.wantContain != "" && !strings.Contains(got, tt.wantContain) {
				t.Errorf("buildStepAudit() = %q, want to contain %q", got, tt.wantContain)
			}
		})
	}
}

func mustBuildStepFlag(t *testing.T, formulaName, defaultStatus string, overrides map[string]string) string {
	t.Helper()

	content, err := formula.GetEmbeddedFormulaContent(formulaName)
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent(%q) error = %v", formulaName, err)
	}

	f, err := formula.Parse(content)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", formulaName, err)
	}

	entries := make([]string, 0, len(f.GetAllIDs())+len(overrides))
	for _, stepID := range f.GetAllIDs() {
		status := defaultStatus
		if override, ok := overrides[stepID]; ok {
			status = override
		}
		entries = append(entries, stepID+":"+status)
	}
	for stepID, status := range overrides {
		if !slicesContains(f.GetAllIDs(), stepID) {
			entries = append(entries, stepID+":"+status)
		}
	}

	return strings.Join(entries, ",")
}

func slicesContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
