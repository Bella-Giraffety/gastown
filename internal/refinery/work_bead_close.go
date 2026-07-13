package refinery

import (
	"fmt"
	"io"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/workitem"
)

type mergedWorkBeadCloseRequest struct {
	MRID        string
	Branch      string
	Target      string
	SourceIssue string
	AgentBead   string
	MergeCommit string
}

type mergedWorkBeadCloseResult struct {
	WorkBeadID string
	Closed     bool
	NotFound   bool
}

type workBeadCloser interface {
	Show(id string) (*beads.Issue, error)
	ForceCloseWithReason(reason string, ids ...string) error
}

type issueReader interface {
	Show(id string) (*beads.Issue, error)
}

func closeMergedWorkBead(work workBeadCloser, agent issueReader, out io.Writer, req mergedWorkBeadCloseRequest) mergedWorkBeadCloseResult {
	logf := func(format string, args ...interface{}) {
		if out != nil {
			_, _ = fmt.Fprintf(out, format, args...)
		}
	}

	workBeadID := resolveMergedWorkBead(agent, req)
	result := mergedWorkBeadCloseResult{WorkBeadID: workBeadID}
	if workBeadID == "" {
		logf("[Refinery] Note: merged MR %s has no resolvable work bead to close\n", req.MRID)
		result.NotFound = true
		return result
	}
	if work == nil {
		logf("[Refinery] Warning: no beads client available to close work bead %s\n", workBeadID)
		result.NotFound = true
		return result
	}

	issue, err := work.Show(workBeadID)
	if err != nil || issue == nil {
		logf("[Refinery] Warning: failed to fetch work bead %s: %v\n", workBeadID, err)
		result.NotFound = true
		return result
	}
	if assessment := assessConcreteWorkBead(issue); !assessment.Concrete {
		logf("[Refinery] Warning: refusing to close non-concrete work bead %s (%s)\n", workBeadID, assessment.Reason)
		result.NotFound = true
		return result
	}
	if beads.IssueStatus(issue.Status).IsTerminal() {
		logf("[Refinery] Work bead already closed: %s\n", workBeadID)
		result.Closed = true
		return result
	}

	closeReason := fmt.Sprintf("Merged in %s", req.MRID)
	if req.MergeCommit != "" {
		closeReason = fmt.Sprintf("%s\ntarget_branch: %s\ncommit_sha: %s", closeReason, req.Target, req.MergeCommit)
	}

	if err := work.ForceCloseWithReason(closeReason, workBeadID); err != nil {
		if issue, showErr := work.Show(workBeadID); showErr == nil && issue != nil &&
			assessConcreteWorkBead(issue).Concrete && beads.IssueStatus(issue.Status).IsTerminal() {
			logf("[Refinery] Work bead already closed: %s\n", workBeadID)
			result.Closed = true
			return result
		}
		logf("[Refinery] Warning: failed to close work bead %s: %v\n", workBeadID, err)
		result.NotFound = true
		return result
	}

	logf("[Refinery] Closed work bead: %s\n", workBeadID)
	result.Closed = true
	return result
}

func resolveMergedWorkBead(agent issueReader, req mergedWorkBeadCloseRequest) string {
	if sourceIssue := cleanWorkBeadID(req.SourceIssue); sourceIssue != "" {
		return sourceIssue
	}
	if agent == nil || cleanWorkBeadID(req.AgentBead) == "" || cleanWorkBeadID(req.MRID) == "" {
		return ""
	}

	agentIssue, err := agent.Show(req.AgentBead)
	if err != nil || !beads.IsAgentBead(agentIssue) {
		return ""
	}
	fields := beads.ParseAgentFields(agentIssue.Description)
	if fields == nil {
		return ""
	}
	if fields.ActiveMR != req.MRID && fields.MRID != req.MRID {
		return ""
	}
	if fields.Branch != "" && req.Branch != "" && fields.Branch != req.Branch {
		return ""
	}
	return cleanWorkBeadID(fields.LastSourceIssue)
}

func cleanWorkBeadID(id string) string {
	id = strings.TrimSpace(id)
	if strings.EqualFold(id, "null") {
		return ""
	}
	return id
}

func assessConcreteWorkBead(issue *beads.Issue) workitem.Assessment {
	if issue == nil {
		return workitem.AssessConcrete(workitem.Snapshot{})
	}
	return workitem.AssessConcrete(workitem.Snapshot{
		ID:        issue.ID,
		Title:     issue.Title,
		Type:      issue.Type,
		Labels:    issue.Labels,
		Ephemeral: issue.Ephemeral,
	})
}
