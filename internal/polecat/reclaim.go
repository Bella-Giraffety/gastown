package polecat

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

func brokenIdleReclaimDispositionBlocker(d WorkstateDisposition) string {
	if d.Reason != "git-check-failed" {
		return fmt.Sprintf("workstate=%s reason=%s", d.Verdict, d.Reason)
	}
	if len(d.Blockers) != 1 || d.Blockers[0] != "git_state=unknown" {
		return fmt.Sprintf("workstate blockers=%s", strings.Join(d.Blockers, ","))
	}
	return ""
}

func brokenIdleReclaimAgentBlocker(fields *beads.AgentFields) string {
	if fields == nil {
		return "agent_fields=<missing>"
	}
	if status := CleanupStatus(fields.CleanupStatus); status != CleanupClean {
		if status == "" {
			return "cleanup_status=<missing>"
		}
		return "cleanup_status=" + string(status)
	}
	if strings.TrimSpace(fields.HookBead) != "" {
		return "hook_bead=" + fields.HookBead
	}
	if strings.TrimSpace(fields.ActiveMR) != "" {
		return "active_mr=" + fields.ActiveMR
	}
	if fields.PushFailed {
		return "push_failed=true"
	}
	if fields.MRFailed {
		return "mr_failed=true"
	}
	if strings.TrimSpace(fields.Branch) == "" {
		return "branch=<missing>"
	}
	if evidence := AssessAgentStateWork(beads.AgentState(fields.AgentState)); evidence.BlocksCleanup {
		return evidence.Blocker
	}
	return ""
}

func brokenIdleReclaimMRBlocker(branch string, mr *beads.Issue, err error) string {
	if err != nil {
		return fmt.Sprintf("checking MR for branch %s: %v", branch, err)
	}
	if mr != nil {
		return fmt.Sprintf("branch %s has open MR %s status=%s", branch, mr.ID, mr.Status)
	}
	return ""
}
