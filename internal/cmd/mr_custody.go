package cmd

import (
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/git"
)

type mergeRequestFieldsOptions struct {
	Branch        string
	Target        string
	SourceIssue   string
	Rig           string
	Worker        string
	CommitSHA     string
	AgentBead     string
	SkipVerify    bool
	PreVerified   bool
	CleanupPolicy string
}

func newMergeRequestFields(g *git.Git, opts mergeRequestFieldsOptions) *beads.MRFields {
	cleanupOwner := ""
	if opts.Worker != "" {
		cleanupOwner = opts.Worker
	}

	fields := beads.NewMRFieldsFromCustody(beads.BranchCustody{
		SourceRef:       opts.Branch,
		SourceCommitSHA: opts.CommitSHA,
		SourceRemote:    "origin",
		SourceFetchRef:  gitString(g, func() (string, error) { return g.RemoteURL("origin") }),
		SourcePushRef:   gitString(g, func() (string, error) { return g.GetPushURL("origin") }),
		TargetRef:       opts.Target,
		TargetBase:      gitString(g, func() (string, error) { return g.Rev("origin/" + opts.Target) }),
		SourceIssue:     opts.SourceIssue,
		Worker:          opts.Worker,
		AgentBead:       opts.AgentBead,
		Phase:           "submitted",
		CleanupOwner:    cleanupOwner,
		CleanupPolicy:   opts.CleanupPolicy,
		Rig:             opts.Rig,
	})
	fields.SkipVerify = opts.SkipVerify
	fields.LastConflictSHA = "null"
	fields.ConflictTaskID = "null"

	if opts.PreVerified {
		fields.PreVerified = true
		fields.PreVerifiedAt = time.Now().UTC().Format(time.RFC3339)
		fields.PreVerifiedBase = fields.TargetBase
	}

	return fields
}

func custodyCleanupPolicy(worker string, noCleanup bool) string {
	if worker == "" {
		return "none"
	}
	if noCleanup {
		return "manual"
	}
	return "witness-after-merge"
}

func updateMergeRequestFields(bd *beads.Beads, issue *beads.Issue, fields *beads.MRFields) error {
	if issue == nil || fields == nil {
		return nil
	}
	newDesc := beads.SetMRFields(issue, fields)
	if newDesc == issue.Description {
		return nil
	}
	return bd.Update(issue.ID, beads.UpdateOptions{Description: &newDesc})
}

func gitString(g *git.Git, fn func() (string, error)) string {
	if g == nil {
		return ""
	}
	value, err := fn()
	if err != nil {
		return ""
	}
	return value
}
