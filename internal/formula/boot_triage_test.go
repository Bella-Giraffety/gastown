package formula

import (
	"strings"
	"testing"
)

func TestBootTriageFormulaUsesNudgeForWake(t *testing.T) {
	content, err := formulasFS.ReadFile("formulas/mol-boot-triage.formula.toml")
	if err != nil {
		t.Fatalf("reading mol-boot-triage.formula.toml: %v", err)
	}

	text := string(content)
	lower := strings.ToLower(text)
	for _, forbidden := range []string{"tmux send-keys", "Send escape", "Escape +"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("Boot triage formula must not contain %q", forbidden)
		}
	}
	if !strings.Contains(text, "gt nudge --mode=immediate deacon") {
		t.Fatal("Boot triage formula missing immediate gt nudge wake path")
	}
}
