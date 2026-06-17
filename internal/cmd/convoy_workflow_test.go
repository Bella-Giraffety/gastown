package cmd

import "testing"

func TestIsReadyIssueSkipsInteractiveWorkflowStep(t *testing.T) {
	t.Parallel()

	scheduled := map[string]bool{}
	interactive := trackedIssueInfo{
		ID:          "gt-step",
		Status:      "open",
		Description: "workflow_interactive: true\n\nHuman gate",
	}
	if isReadyIssue(interactive, scheduled) {
		t.Fatal("interactive workflow step was ready for auto-dispatch")
	}

	normal := trackedIssueInfo{ID: "gt-normal", Status: "open"}
	if !isReadyIssue(normal, scheduled) {
		t.Fatal("normal open unassigned issue was not ready")
	}
}
