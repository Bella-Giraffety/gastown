package cmd

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/beads"
)

var defaultActiveWorkStatuses = []string{beads.StatusHooked, "in_progress"}

// listAssignedWork returns active work assigned to assignee from both durable
// issues and ephemeral wisps. Work assignment is authoritative on the work row:
// status + assignee, not the legacy agent hook_bead slot.
func listAssignedWork(b *beads.Beads, assignee string, statuses ...string) ([]*beads.Issue, error) {
	if b == nil {
		return nil, fmt.Errorf("nil beads handle")
	}
	if len(statuses) == 0 {
		statuses = defaultActiveWorkStatuses
	}

	for _, status := range statuses {
		assigned, err := listAssignedWorkForStatus(b, assignee, status)
		if err != nil {
			return nil, fmt.Errorf("listing %s assigned work: %w", status, err)
		}
		if len(assigned) > 0 {
			return assigned, nil
		}
	}
	return nil, nil
}

func listAssignedWorkAllStatuses(b *beads.Beads, assignee string, statuses ...string) ([]*beads.Issue, error) {
	if b == nil {
		return nil, fmt.Errorf("nil beads handle")
	}
	if len(statuses) == 0 {
		statuses = defaultActiveWorkStatuses
	}

	var work []*beads.Issue
	for _, status := range statuses {
		assigned, err := listAssignedWorkForStatus(b, assignee, status)
		if err != nil {
			return nil, fmt.Errorf("listing %s assigned work: %w", status, err)
		}
		work = append(work, assigned...)
	}
	return dedupeAssignedWork(work), nil
}

func listAssignedWorkForStatus(b *beads.Beads, assignee, status string) ([]*beads.Issue, error) {
	opts := beads.ListOptions{
		Status:   status,
		Assignee: assignee,
		Priority: -1,
	}

	issues, err := b.List(opts)
	if err != nil {
		return nil, err
	}

	opts.Ephemeral = true
	wisps, err := b.List(opts)
	if err != nil {
		return nil, err
	}

	return dedupeAssignedWork(append(issues, wisps...)), nil
}

func dedupeAssignedWork(work []*beads.Issue) []*beads.Issue {
	if len(work) < 2 {
		return work
	}
	seen := make(map[string]bool, len(work))
	deduped := make([]*beads.Issue, 0, len(work))
	for _, issue := range work {
		if issue == nil || issue.ID == "" {
			continue
		}
		if seen[issue.ID] {
			continue
		}
		seen[issue.ID] = true
		deduped = append(deduped, issue)
	}
	return deduped
}
