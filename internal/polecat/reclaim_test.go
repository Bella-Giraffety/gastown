package polecat

import (
	"errors"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestBrokenIdleReclaimDispositionBlocker(t *testing.T) {
	tests := []struct {
		name string
		d    WorkstateDisposition
		want string
	}{
		{
			name: "only git unknown passes",
			d:    WorkstateDisposition{Verdict: WorkstateVerdictNeedsRecovery, Reason: "git-check-failed", Blockers: []string{"git_state=unknown"}},
			want: "",
		},
		{
			name: "terminal active mr git unsafe passes with git unknown",
			d:    WorkstateDisposition{Verdict: WorkstateVerdictNeedsRecovery, Reason: "git-check-failed", Blockers: []string{"git_state=unknown", "active_mr=gt-mr status=closed source_issue=gt-work git_state=unsafe"}},
			want: "",
		},
		{
			name: "terminal active mr without git unknown blocks",
			d:    WorkstateDisposition{Verdict: WorkstateVerdictNeedsRecovery, Reason: "git-check-failed", Blockers: []string{"active_mr=gt-mr status=closed source_issue=gt-work git_state=unsafe"}},
			want: "workstate blockers=active_mr=gt-mr status=closed source_issue=gt-work git_state=unsafe",
		},
		{
			name: "wrong reason blocks",
			d:    WorkstateDisposition{Verdict: WorkstateVerdictNeedsRecovery, Reason: "cleanup-unknown", Blockers: []string{"cleanup_status=<missing>"}},
			want: "workstate=NEEDS_RECOVERY reason=cleanup-unknown",
		},
		{
			name: "extra blocker blocks",
			d:    WorkstateDisposition{Verdict: WorkstateVerdictNeedsRecovery, Reason: "git-check-failed", Blockers: []string{"cleanup_status=<missing>", "git_state=unknown"}},
			want: "workstate blockers=cleanup_status=<missing>,git_state=unknown",
		},
		{
			name: "missing blocker blocks",
			d:    WorkstateDisposition{Verdict: WorkstateVerdictNeedsRecovery, Reason: "git-check-failed"},
			want: "workstate blockers=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := brokenIdleReclaimDispositionBlocker(tt.d, "gt-mr"); got != tt.want {
				t.Fatalf("brokenIdleReclaimDispositionBlocker() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBrokenIdleReclaimMRBlocker(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		mr     *beads.Issue
		err    error
		want   string
	}{
		{name: "no open mr passes", branch: "polecat/toast/gt-work@abc123", want: ""},
		{name: "lookup error blocks", branch: "polecat/toast/gt-work@abc123", err: errors.New("dolt locked"), want: "checking MR for branch polecat/toast/gt-work@abc123: dolt locked"},
		{name: "open mr blocks", branch: "polecat/toast/gt-work@abc123", mr: &beads.Issue{ID: "gt-mr", Status: "open"}, want: "branch polecat/toast/gt-work@abc123 has open MR gt-mr status=open"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := brokenIdleReclaimMRBlocker(tt.branch, tt.mr, tt.err); got != tt.want {
				t.Fatalf("brokenIdleReclaimMRBlocker() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBrokenIdleReclaimAgentBlocker(t *testing.T) {
	base := func() *beads.AgentFields {
		return &beads.AgentFields{
			AgentState:    string(beads.AgentStateIdle),
			CleanupStatus: string(CleanupClean),
			Branch:        "polecat/toast/gt-work@abc123",
		}
	}

	tests := []struct {
		name             string
		mutate           func(*beads.AgentFields) *beads.AgentFields
		terminalActiveMR bool
		want             string
	}{
		{name: "clean idle passes", want: ""},
		{name: "nil fields block", mutate: func(*beads.AgentFields) *beads.AgentFields { return nil }, want: "agent_fields=<missing>"},
		{name: "missing cleanup blocks", mutate: func(f *beads.AgentFields) *beads.AgentFields { f.CleanupStatus = ""; return f }, want: "cleanup_status=<missing>"},
		{name: "dirty cleanup blocks", mutate: func(f *beads.AgentFields) *beads.AgentFields { f.CleanupStatus = string(CleanupUncommitted); return f }, want: "cleanup_status=has_uncommitted"},
		{name: "hook blocks", mutate: func(f *beads.AgentFields) *beads.AgentFields { f.HookBead = "gt-work"; return f }, want: "hook_bead=gt-work"},
		{name: "active mr blocks without terminal proof", mutate: func(f *beads.AgentFields) *beads.AgentFields { f.ActiveMR = "gt-mr"; return f }, want: "active_mr=gt-mr"},
		{name: "terminal active mr passes", mutate: func(f *beads.AgentFields) *beads.AgentFields { f.ActiveMR = "gt-mr"; return f }, terminalActiveMR: true, want: ""},
		{name: "push failed blocks", mutate: func(f *beads.AgentFields) *beads.AgentFields { f.PushFailed = true; return f }, want: "push_failed=true"},
		{name: "mr failed blocks", mutate: func(f *beads.AgentFields) *beads.AgentFields { f.MRFailed = true; return f }, want: "mr_failed=true"},
		{name: "missing branch blocks", mutate: func(f *beads.AgentFields) *beads.AgentFields { f.Branch = ""; return f }, want: "branch=<missing>"},
		{name: "spawning blocks", mutate: func(f *beads.AgentFields) *beads.AgentFields {
			f.AgentState = string(beads.AgentStateSpawning)
			return f
		}, want: "agent_state=spawning"},
		{name: "working blocks", mutate: func(f *beads.AgentFields) *beads.AgentFields {
			f.AgentState = string(beads.AgentStateWorking)
			return f
		}, want: "agent_state=working"},
		{name: "stuck blocks", mutate: func(f *beads.AgentFields) *beads.AgentFields { f.AgentState = string(beads.AgentStateStuck); return f }, want: "agent_state=stuck"},
		{name: "done passes", mutate: func(f *beads.AgentFields) *beads.AgentFields { f.AgentState = string(beads.AgentStateDone); return f }, want: ""},
		{name: "nuked passes", mutate: func(f *beads.AgentFields) *beads.AgentFields { f.AgentState = string(beads.AgentStateNuked); return f }, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := base()
			if tt.mutate != nil {
				fields = tt.mutate(fields)
			}
			if got := brokenIdleReclaimAgentBlocker(fields, tt.terminalActiveMR); got != tt.want {
				t.Fatalf("brokenIdleReclaimAgentBlocker() = %q, want %q", got, tt.want)
			}
		})
	}
}
