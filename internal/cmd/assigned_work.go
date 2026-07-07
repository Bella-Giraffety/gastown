package cmd

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/beads"
)

func queryAssignedWork(b *beads.Beads, agentID string) ([]*beads.Issue, error) {
	hooked, err := b.ListAssignedWork(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		return nil, fmt.Errorf("querying hooked beads: %w", err)
	}
	if len(hooked) > 0 {
		return hooked, nil
	}

	inProgress, err := b.ListAssignedWork(beads.ListOptions{
		Status:   "in_progress",
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		return nil, fmt.Errorf("querying in-progress beads: %w", err)
	}
	return inProgress, nil
}
