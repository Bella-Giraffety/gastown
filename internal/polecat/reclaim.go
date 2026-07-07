package polecat

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

func brokenIdleReclaimDispositionBlocker(d WorkstateDisposition, terminalActiveMR string) string {
	if d.Reason != "git-check-failed" {
		return fmt.Sprintf("workstate=%s reason=%s", d.Verdict, d.Reason)
	}
	allowed := 0
	for _, blocker := range d.Blockers {
		switch {
		case blocker == "git_state=unknown":
			allowed++
		case terminalActiveMR != "" && strings.HasPrefix(blocker, "active_mr="+terminalActiveMR+" ") && strings.Contains(blocker, "git_state=unsafe"):
			allowed++
		}
	}
	if len(d.Blockers) == 0 || allowed != len(d.Blockers) || !containsString(d.Blockers, "git_state=unknown") {
		return fmt.Sprintf("workstate blockers=%s", strings.Join(d.Blockers, ","))
	}
	return ""
}

func brokenIdleReclaimAgentBlocker(fields *beads.AgentFields, terminalActiveMR bool) string {
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
	if strings.TrimSpace(fields.ActiveMR) != "" && !terminalActiveMR {
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

func brokenIdleReclaimTerminalActiveMRBlocker(assessment ActiveMRAssessment) string {
	if assessment.ActiveMR == "" || !assessment.Pending {
		return ""
	}
	if assessment.Reason != "" {
		return assessment.Reason
	}
	return "active_mr=" + assessment.ActiveMR
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
