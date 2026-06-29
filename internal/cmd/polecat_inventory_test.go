package cmd

import (
	"slices"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/session"
)

func TestPolecatSessionSet(t *testing.T) {
	installTestPrefixRegistry(t)
	sessions := newPolecatSessionSet([]string{
		"gt-zed",
		"hq-mayor",
		"gt-alpha",
		"ot-beta",
	})

	if got, ok := sessions.lookup("gastown", "alpha"); !ok || got != "gt-alpha" {
		t.Fatalf("lookup(gastown, alpha) = (%q, %v), want gt-alpha, true", got, ok)
	}
	if _, ok := sessions.lookup("gastown", "mayor"); ok {
		t.Fatal("town-level mayor session should not be indexed as a polecat")
	}

	want := []string{"gt-alpha", "gt-zed"}
	if got := sessions.namesForRig("gastown"); !slices.Equal(got, want) {
		t.Fatalf("namesForRig(gastown) = %#v, want %#v", got, want)
	}
}

func TestBuildPolecatInventoryItem(t *testing.T) {
	installTestPrefixRegistry(t)
	sessions := newPolecatSessionSet([]string{"gt-dust"})
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

func installTestPrefixRegistry(t *testing.T) {
	t.Helper()
	old := session.DefaultRegistry()
	registry := session.NewPrefixRegistry()
	registry.Register("gt", "gastown")
	registry.Register("ot", "other")
	session.SetDefaultRegistry(registry)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

func TestPolecatSummaryIssueRankPrefersActiveWork(t *testing.T) {
	blocked := &beads.Issue{ID: "gt-blocked", Status: string(beads.StatusBlocked)}
	open := &beads.Issue{ID: "gt-open", Status: string(beads.StatusOpen)}
	hooked := &beads.Issue{ID: "gt-hooked", Status: string(beads.IssueStatusHooked)}
	pinned := &beads.Issue{ID: "gt-pinned", Status: string(beads.IssueStatusPinned)}

	if !(polecatSummaryIssueRank(hooked) < polecatSummaryIssueRank(open) && polecatSummaryIssueRank(open) < polecatSummaryIssueRank(blocked) && polecatSummaryIssueRank(blocked) < polecatSummaryIssueRank(pinned)) {
		t.Fatalf("summary ranks = hooked:%d open:%d blocked:%d pinned:%d, want active statuses before protected", polecatSummaryIssueRank(hooked), polecatSummaryIssueRank(open), polecatSummaryIssueRank(blocked), polecatSummaryIssueRank(pinned))
	}
}

func TestBuildPolecatInventoryItemActiveWorkLookupErrorFailsClosed(t *testing.T) {
	item := buildPolecatInventoryItemFromEvidence("gastown", "dust", &beads.AgentFields{CleanupStatus: "clean"}, polecat.ActiveWorkEvidence{
		Protected:     true,
		BlocksCleanup: true,
		Blocker:       "assigned_work status=lookup_error: bd failed",
		HookSafe:      true,
	}, nil)

	if item.Disposition.Reusable || item.Disposition.SafeToNuke || !item.Disposition.NeedsRecovery || item.Disposition.CountsTowardCapacity {
		t.Fatalf("disposition = %+v, want fail-closed recovery without capacity", item.Disposition)
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
