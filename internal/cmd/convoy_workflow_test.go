package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

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

func TestIssueToDetailsPreservesWorkflowStepDescription(t *testing.T) {
	t.Parallel()

	details := issueToDetails(&beads.Issue{
		ID:          "gt-step",
		Status:      "open",
		Description: "workflow_interactive: true\n\nHuman gate",
	})
	if details == nil || details.Description == "" {
		t.Fatalf("issueToDetails() did not preserve description: %#v", details)
	}

	dep := trackedDependency{ID: details.ID}
	applyFreshIssueDetails(&dep, details)
	info := trackedIssueInfo{ID: dep.ID, Status: dep.Status, Description: dep.Description}
	if isReadyIssue(info, map[string]bool{}) {
		t.Fatal("interactive workflow metadata was lost before ready check")
	}
}
