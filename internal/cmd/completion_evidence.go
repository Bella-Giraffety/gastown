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
	if doneHasTerminalNoBranchEvidence(issue) {
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

func doneHasTerminalNoBranchEvidence(issue *beads.Issue) bool {
	if issue == nil || !beads.IssueStatus(strings.TrimSpace(issue.Status)).IsTerminal() {
		return false
	}
	return doneTextHasNoBranchEvidence(issue.CloseReason) ||
		doneTextHasNoBranchEvidence(issue.Notes) ||
		doneTextHasNoBranchEvidence(issue.Design)
}

func doneTextHasNoBranchEvidence(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	if text == "merged" {
		return true
	}
	for _, phrase := range []string{
		"no-changes",
		"no changes",
		"already fixed",
		"already landed",
		"already merged",
		"merged in ",
		"not applicable",
		"cannot reproduce",
		"can't reproduce",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func doneZeroDeliverableError(issueID, target string) error {
	if strings.TrimSpace(target) == "" {
		target = "the target branch"
	}
	return fmt.Errorf("cannot complete %s: no deliverable evidence for completed work\n"+
		"Observed: submitted ref has no patch work requiring submission to %s.\n"+
		"Finish the work with a commit, use DEFERRED or ESCALATED for incomplete work, or record explicit closure evidence before completing.", issueID, target)
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
	return uniqueStrings([]string{baseRef, target})
}
