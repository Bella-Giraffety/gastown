package cmd

import (
	"slices"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/polecat"
)

func TestPolecatSessionSet(t *testing.T) {
	sessions := newPolecatSessionSet([]string{
		"gastown-zed",
		"hq-mayor",
		"gastown-alpha",
		"other-beta",
	})

	if got, ok := sessions.lookup("gastown", "alpha"); !ok || got != "gastown-alpha" {
		t.Fatalf("lookup(gastown, alpha) = (%q, %v), want gastown-alpha, true", got, ok)
	}
	if _, ok := sessions.lookup("gastown", "mayor"); ok {
		t.Fatal("town-level mayor session should not be indexed as a polecat")
	}

	want := []string{"gastown-alpha", "gastown-zed"}
	if got := sessions.namesForRig("gastown"); !slices.Equal(got, want) {
		t.Fatalf("namesForRig(gastown) = %#v, want %#v", got, want)
	}
}

func TestBuildPolecatInventoryItem(t *testing.T) {
	sessions := newPolecatSessionSet([]string{"gastown-dust"})
	tests := []struct {
		name                string
		fields              *beads.AgentFields
		activeWork          *beads.Issue
		sessions            polecatSessionSet
		wantState           polecat.State
		wantIssue           string
		wantVerdict         string
		wantReusable        bool
		wantRecovery        bool
		wantCapacityCounted bool
		wantSessionRunning  bool
	}{
		{
			name:               "clean idle is reusable",
			fields:             &beads.AgentFields{CleanupStatus: "clean"},
			wantState:          polecat.StateIdle,
			wantVerdict:        polecat.WorkstateVerdictSafeToNuke,
			wantReusable:       true,
			wantSessionRunning: false,
		},
		{
			name:                "active work with session is working capacity",
			fields:              &beads.AgentFields{CleanupStatus: "clean"},
			activeWork:          &beads.Issue{ID: "gt-work", Status: string(beads.IssueStatusHooked)},
			sessions:            sessions,
			wantState:           polecat.StateWorking,
			wantIssue:           "gt-work",
			wantVerdict:         polecat.WorkstateVerdictWorking,
			wantCapacityCounted: true,
			wantSessionRunning:  true,
		},
		{
			name:                "active work without session is stalled capacity",
			fields:              &beads.AgentFields{CleanupStatus: "clean"},
			activeWork:          &beads.Issue{ID: "gt-work", Status: string(beads.StatusInProgress)},
			wantState:           polecat.StateStalled,
			wantIssue:           "gt-work",
			wantVerdict:         polecat.WorkstateVerdictNeedsRecovery,
			wantRecovery:        true,
			wantCapacityCounted: true,
		},
		{
			name:                "blocked work protects but does not consume capacity",
			fields:              &beads.AgentFields{CleanupStatus: "clean"},
			activeWork:          &beads.Issue{ID: "gt-blocked", Status: string(beads.StatusBlocked)},
			wantState:           polecat.StateIdle,
			wantIssue:           "gt-blocked",
			wantVerdict:         polecat.WorkstateVerdictNeedsRecovery,
			wantRecovery:        true,
			wantCapacityCounted: false,
		},
		{
			name:                "hook only remains recovery blocked without capacity",
			fields:              &beads.AgentFields{CleanupStatus: "clean", HookBead: "gt-hook"},
			wantState:           polecat.StateIdle,
			wantVerdict:         polecat.WorkstateVerdictNeedsRecovery,
			wantRecovery:        true,
			wantCapacityCounted: false,
		},
		{
			name:        "active mr only is pending mr",
			fields:      &beads.AgentFields{CleanupStatus: "clean", ActiveMR: "gt-mr"},
			wantState:   polecat.StateIdle,
			wantVerdict: polecat.WorkstateVerdictPendingMR,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := buildPolecatInventoryItem("gastown", "dust", tt.fields, tt.activeWork, tt.sessions)
			if item.State != tt.wantState || item.Issue != tt.wantIssue || item.SessionRunning != tt.wantSessionRunning {
				t.Fatalf("item = %+v, want state=%q issue=%q running=%v", item, tt.wantState, tt.wantIssue, tt.wantSessionRunning)
			}
			d := item.Disposition
			if d.Verdict != tt.wantVerdict || d.Reusable != tt.wantReusable || d.NeedsRecovery != tt.wantRecovery || d.CountsTowardCapacity != tt.wantCapacityCounted {
				t.Fatalf("disposition = %+v, want verdict=%q reusable=%v recovery=%v capacity=%v", d, tt.wantVerdict, tt.wantReusable, tt.wantRecovery, tt.wantCapacityCounted)
			}
		})
	}
}

func TestPolecatSummaryIssueRankPrefersActiveWork(t *testing.T) {
	blocked := &beads.Issue{ID: "gt-blocked", Status: string(beads.StatusBlocked)}
	open := &beads.Issue{ID: "gt-open", Status: string(beads.StatusOpen)}
	hooked := &beads.Issue{ID: "gt-hooked", Status: string(beads.IssueStatusHooked)}

	if !(polecatSummaryIssueRank(hooked) < polecatSummaryIssueRank(open) && polecatSummaryIssueRank(open) < polecatSummaryIssueRank(blocked)) {
		t.Fatalf("summary ranks = hooked:%d open:%d blocked:%d, want active statuses before protected", polecatSummaryIssueRank(hooked), polecatSummaryIssueRank(open), polecatSummaryIssueRank(blocked))
	}
}

func TestPolecatNameFromAssignee(t *testing.T) {
	if got, ok := polecatNameFromAssignee("gastown", "gastown/polecats/dust"); !ok || got != "dust" {
		t.Fatalf("polecatNameFromAssignee valid = (%q, %v), want dust, true", got, ok)
	}
	for _, assignee := range []string{"other/polecats/dust", "gastown/crew/dust", "gastown/polecats/", "gastown/polecats/dust/extra"} {
		if got, ok := polecatNameFromAssignee("gastown", assignee); ok || got != "" {
			t.Fatalf("polecatNameFromAssignee(%q) = (%q, %v), want empty, false", assignee, got, ok)
		}
	}
}
