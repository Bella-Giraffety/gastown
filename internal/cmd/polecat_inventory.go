package cmd

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/polecat"
)

const polecatSessionKeySep = "\x00"

type polecatSessionSet map[string]string

type polecatInventoryItem struct {
	Rig            string
	Name           string
	State          polecat.State
	Issue          string
	CleanupStatus  string
	ActiveMR       string
	Branch         string
	SessionRunning bool
	SessionName    string
	Disposition    polecat.WorkstateDisposition
}

func newPolecatSessionSet(sessionNames []string) polecatSessionSet {
	sessions := make(polecatSessionSet, len(sessionNames))
	for _, sessionName := range sessionNames {
		rigName, polecatName, ok := parsePolecatSessionName(sessionName)
		if !ok {
			continue
		}
		sessions[polecatSessionKey(rigName, polecatName)] = sessionName
	}
	return sessions
}

func (s polecatSessionSet) lookup(rigName, polecatName string) (string, bool) {
	if s == nil {
		return "", false
	}
	sessionName, ok := s[polecatSessionKey(rigName, polecatName)]
	return sessionName, ok
}

func (s polecatSessionSet) namesForRig(rigName string) []string {
	if len(s) == 0 {
		return nil
	}
	var names []string
	for _, sessionName := range s {
		sessionRig, _, ok := parsePolecatSessionName(sessionName)
		if ok && sessionRig == rigName {
			names = append(names, sessionName)
		}
	}
	return names
}

func polecatSessionKey(rigName, polecatName string) string {
	return rigName + polecatSessionKeySep + polecatName
}

func buildPolecatInventoryItem(rigName, polecatName string, fields *beads.AgentFields, activeWork *beads.Issue, sessions polecatSessionSet) polecatInventoryItem {
	sessionName, running := sessions.lookup(rigName, polecatName)
	item := polecatInventoryItem{
		Rig:            rigName,
		Name:           polecatName,
		State:          polecat.StateIdle,
		SessionRunning: running,
		SessionName:    sessionName,
	}

	input := polecat.WorkstateInput{State: polecat.StateIdle}
	if fields != nil {
		item.CleanupStatus = strings.TrimSpace(fields.CleanupStatus)
		item.ActiveMR = strings.TrimSpace(fields.ActiveMR)
		item.Branch = strings.TrimSpace(fields.Branch)
		input.CleanupStatus = polecat.CleanupStatus(item.CleanupStatus)
		input.PushFailed = fields.PushFailed
		input.MRFailed = fields.MRFailed
		input.Branch = item.Branch
		input.ActiveMR = item.ActiveMR
	}

	if activeWork != nil && activeWorkBlocksSummary(activeWork) {
		item.Issue = activeWork.ID
		if running {
			item.State = polecat.StateWorking
		} else {
			item.State = polecat.StateStalled
		}
		input.ApplyActiveWork(activeWorkEvidenceForSummary(activeWork))
	} else if running && !polecat.CleanupStatus(item.CleanupStatus).IsSafe() {
		item.State = polecat.StateReviewNeeded
	} else {
		item.State = polecat.StateIdle
	}

	if fields != nil && activeWork == nil && strings.TrimSpace(fields.HookBead) != "" {
		input.HookBead = strings.TrimSpace(fields.HookBead)
		input.ActiveWorkBlocker = fmt.Sprintf("hook_bead=%s status=unverified", input.HookBead)
	}
	if item.ActiveMR != "" {
		input.ActiveMRBlocker = "active_mr=" + item.ActiveMR + " status=unknown"
	}

	input.State = item.State
	item.Disposition = polecat.DecideWorkstate(input)
	return item
}

func listActivePolecatWorkByName(bd *beads.Beads, rigName string) (map[string]*beads.Issue, error) {
	byName := make(map[string]*beads.Issue)
	for _, status := range []string{beads.StatusHooked, string(beads.StatusInProgress), string(beads.StatusOpen)} {
		issues, err := bd.List(beads.ListOptions{Status: status, Priority: -1})
		if err != nil {
			return nil, err
		}
		for _, issue := range issues {
			if !activeWorkBlocksSummary(issue) {
				continue
			}
			name, ok := polecatNameFromAssignee(rigName, issue.Assignee)
			if !ok {
				continue
			}
			if _, exists := byName[name]; !exists {
				byName[name] = issue
			}
		}
	}
	return byName, nil
}

func polecatNameFromAssignee(rigName, assignee string) (string, bool) {
	prefix := rigName + "/polecats/"
	if !strings.HasPrefix(assignee, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(assignee, prefix)
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return name, true
}

func activeWorkBlocksSummary(issue *beads.Issue) bool {
	if issue == nil || beads.IsAgentBead(issue) || beads.IsProtectedBead(issue) {
		return false
	}
	return !beads.IssueStatus(issue.Status).IsTerminal()
}

func activeWorkEvidenceForSummary(issue *beads.Issue) polecat.ActiveWorkEvidence {
	active := issueRequiresRestartForSummary(beads.IssueStatus(issue.Status))
	return polecat.ActiveWorkEvidence{
		Active:               active,
		Protected:            !active,
		BlocksCleanup:        true,
		RequiresRestart:      active,
		CountsTowardCapacity: active,
		Blocker:              fmt.Sprintf("assigned_work=%s status=%s", issue.ID, issue.Status),
		AssignedIssue:        issue.ID,
		HookSafe:             true,
	}
}

func issueRequiresRestartForSummary(status beads.IssueStatus) bool {
	switch status {
	case beads.StatusOpen, beads.StatusInProgress, beads.IssueStatusHooked:
		return true
	default:
		return false
	}
}
