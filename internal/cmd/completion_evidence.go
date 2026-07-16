package cmd

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/git"
)

type completionEvidenceMode int

const (
	completionEvidenceDone completionEvidenceMode = iota
	completionEvidenceMQSubmit
)

type completionEvidenceResult struct {
	HasSubmittableWork bool
	AllowsNoBranchWork bool
	NoBranchWorkReason string
	Status             git.BranchPreservationStatus
}

func assessSourceCompletionEvidence(g *git.Git, submittedRef string, targetRefs []string, issueID string, issue *beads.Issue, attachment *beads.AttachmentFields, mode completionEvidenceMode) (completionEvidenceResult, error) {
	status, err := g.RefTargetStatus(submittedRef, "origin", targetRefs)
	return assessSourceCompletionEvidenceWithStatus(g, submittedRef, targetRefs, issueID, issue, attachment, mode, status, err)
}

func assessSourceCompletionEvidenceWithStatus(g *git.Git, submittedRef string, targetRefs []string, issueID string, issue *beads.Issue, attachment *beads.AttachmentFields, mode completionEvidenceMode, status git.BranchPreservationStatus, statusErr error) (completionEvidenceResult, error) {
	var result completionEvidenceResult
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return result, fmt.Errorf("cannot verify completion evidence: missing source issue")
	}
	if issue == nil {
		return result, fmt.Errorf("cannot verify completion evidence for %s: source issue unavailable", issueID)
	}
	if attachment == nil {
		attachment = beads.ParseAttachmentFields(issue)
	}

	if mode == completionEvidenceMQSubmit {
		if reason := mqSubmitSourceIneligibleReason(issue, attachment); reason != "" {
			return result, fmt.Errorf("cannot submit %s to merge queue: %s", issueID, reason)
		}
	}

	if statusErr != nil {
		return result, fmt.Errorf("cannot verify deliverable evidence for %s: %w", issueID, statusErr)
	}
	result.Status = status
	result.HasSubmittableWork = status.UnpreservedPatchCount > 0
	if result.HasSubmittableWork {
		hasNonRuntimeDiff, diffErr := g.DiffHasNonRuntimeChanges(status.ComparisonBase, submittedRef)
		if diffErr != nil {
			return result, fmt.Errorf("cannot verify deliverable content for %s: %w", issueID, diffErr)
		}
		if !hasNonRuntimeDiff {
			if mode == completionEvidenceMQSubmit {
				return result, fmt.Errorf("cannot submit %s to merge queue: submitted ref changes only runtime artifacts, not deliverable work", issueID)
			}
			return result, fmt.Errorf("cannot complete %s: submitted ref changes only runtime artifacts, not deliverable work", issueID)
		}
	}
	if mode == completionEvidenceDone && beads.IssueStatus(strings.TrimSpace(issue.Status)).IsTerminal() && result.HasSubmittableWork {
		return result, fmt.Errorf("cannot complete %s: source issue is already terminal", issueID)
	}
	if result.HasSubmittableWork {
		return result, nil
	}

	if mode == completionEvidenceMQSubmit {
		return result, fmt.Errorf("cannot submit %s to merge queue: submitted ref has no patch work requiring merge", issueID)
	}

	if attachment != nil && attachment.ReviewOnly {
		result.AllowsNoBranchWork = true
		result.NoBranchWorkReason = "review-only"
		return result, nil
	}
	terminalNoBranch, terminalErr := doneHasTerminalNoBranchEvidence(g, issue, targetRefs)
	if terminalErr != nil {
		return result, fmt.Errorf("cannot verify terminal completion evidence for %s: %w", issueID, terminalErr)
	}
	if terminalNoBranch {
		result.AllowsNoBranchWork = true
		result.NoBranchWorkReason = "source-terminal"
		return result, nil
	}

	return result, doneZeroDeliverableError(issueID, firstCompletionTargetRef(targetRefs...))
}

func mqSubmitSourceIneligibleReason(issue *beads.Issue, attachment *beads.AttachmentFields) string {
	if issue == nil {
		return "source issue unavailable"
	}
	if beads.IssueStatus(strings.TrimSpace(issue.Status)).IsTerminal() {
		return "source issue is already terminal"
	}
	if attachment == nil {
		return ""
	}
	switch {
	case attachment.NoMerge:
		return "source is marked outside the merge queue"
	case attachment.ReviewOnly:
		return "source is review-only"
	case strings.EqualFold(strings.TrimSpace(attachment.MergeStrategy), "local"):
		return "source keeps work outside the merge queue"
	default:
		return ""
	}
}

func doneHasTerminalNoBranchEvidence(g *git.Git, issue *beads.Issue, targetRefs []string) (bool, error) {
	if issue == nil || !beads.IssueStatus(strings.TrimSpace(issue.Status)).IsTerminal() {
		return false, nil
	}
	if doneIssueTypeAllowsTerminalNoBranch(issue.Type) {
		return true, nil
	}
	commit, ok := doneStructuredLandingCommit(issue.CloseReason)
	if !ok {
		return false, nil
	}
	reachable, err := doneLandingCommitReachableFromTarget(g, commit, targetRefs)
	if err != nil || !reachable {
		return false, err
	}
	return true, nil
}

func doneIssueTypeAllowsTerminalNoBranch(issueType string) bool {
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "decision", "spike", "milestone":
		return true
	default:
		return false
	}
}

func doneStructuredLandingCommit(closeReason string) (string, bool) {
	fields := make(map[string]string)
	for _, line := range strings.Split(closeReason, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[strings.ToLower(strings.TrimSpace(key))] = firstField(value)
	}
	if fields["target_branch"] == "" {
		return "", false
	}
	for _, key := range []string{"commit_sha", "merge_commit"} {
		if commit := fields[key]; isGitHexSHA(commit) {
			return commit, true
		}
	}
	return "", false
}

func firstField(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func isGitHexSHA(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 7 || len(value) > 40 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func doneLandingCommitReachableFromTarget(g *git.Git, commit string, targetRefs []string) (bool, error) {
	if g == nil {
		return false, fmt.Errorf("git unavailable")
	}
	if _, err := g.Rev(commit + "^{commit}"); err != nil {
		return false, fmt.Errorf("landing commit %s is unavailable: %w", commit, err)
	}
	var checked bool
	var lastErr error
	for _, target := range uniqueStrings(targetRefs) {
		ok, err := g.IsAncestor(commit, target)
		if err != nil {
			lastErr = err
			continue
		}
		checked = true
		if ok {
			return true, nil
		}
	}
	if checked {
		return false, nil
	}
	if lastErr != nil {
		return false, lastErr
	}
	return false, fmt.Errorf("no target refs available")
}

func doneZeroDeliverableError(issueID, target string) error {
	if strings.TrimSpace(target) == "" {
		target = "the target branch"
	}
	return fmt.Errorf("cannot complete %s: no deliverable evidence for completed work\n"+
		"Observed: submitted ref has no patch work requiring submission to %s.\n"+
		"Finish the work with a commit, use DEFERRED or ESCALATED for incomplete work, or use the review-only/non-deliverable workflow.", issueID, target)
}

func firstCompletionTargetRef(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func completionTargetRefs(target, baseRef string) []string {
	if strings.TrimSpace(baseRef) != "" {
		return []string{strings.TrimSpace(baseRef)}
	}
	return uniqueStrings([]string{target})
}
