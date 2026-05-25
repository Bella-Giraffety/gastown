package cmd

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestValidateMoleculePrereqs(t *testing.T) {
	tests := []struct {
		name      string
		children  []*beads.Issue
		wantErr   bool
		wantInErr []string // Substrings expected in error message
	}{
		{
			name:     "nil children",
			children: nil,
			wantErr:  false,
		},
		{
			name:     "empty children",
			children: []*beads.Issue{},
			wantErr:  false,
		},
		{
			name: "all prereqs closed",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Load context", Status: "closed"},
				{ID: "gt-mol.2", Title: "Set up branch", Status: "closed"},
				{ID: "gt-mol.3", Title: "Implement", Status: "closed"},
				{ID: "gt-mol.4", Title: "Self-review", Status: "closed"},
				{ID: "gt-mol.5", Title: "Build check", Status: "closed"},
				{ID: "gt-mol.6", Title: "Commit changes", Status: "closed"},
				{ID: "gt-mol.7", Title: "Rebase verify", Status: "closed"},
				{ID: "gt-mol.8", Title: "Submit MR", Status: "open"},
				{ID: "gt-mol.9", Title: "Wait for verdict", Status: "open"},
				{ID: "gt-mol.10", Title: "Self-clean", Status: "open"},
			},
			wantErr: false,
		},
		{
			name: "missing self-review step",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Load context", Status: "closed"},
				{ID: "gt-mol.2", Title: "Set up branch", Status: "closed"},
				{ID: "gt-mol.3", Title: "Implement", Status: "closed"},
				{ID: "gt-mol.4", Title: "Self-review", Status: "open"},
				{ID: "gt-mol.5", Title: "Build check", Status: "closed"},
				{ID: "gt-mol.6", Title: "Commit changes", Status: "closed"},
				{ID: "gt-mol.7", Title: "Rebase verify", Status: "closed"},
				{ID: "gt-mol.8", Title: "Submit MR", Status: "open"},
			},
			wantErr:   true,
			wantInErr: []string{"gt-mol.4", "Self-review", "--skip-deps"},
		},
		{
			name: "multiple incomplete steps",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Load context", Status: "closed"},
				{ID: "gt-mol.2", Title: "Set up branch", Status: "open"},
				{ID: "gt-mol.3", Title: "Implement", Status: "in_progress"},
				{ID: "gt-mol.4", Title: "Self-review", Status: "open"},
				{ID: "gt-mol.5", Title: "Submit MR", Status: "open"},
			},
			wantErr:   true,
			wantInErr: []string{"gt-mol.2", "gt-mol.3", "gt-mol.4"},
		},
		{
			name: "no submit step found — checks all steps",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Load context", Status: "closed"},
				{ID: "gt-mol.2", Title: "Implement", Status: "open"},
				{ID: "gt-mol.3", Title: "Build check", Status: "open"},
			},
			wantErr:   true,
			wantInErr: []string{"gt-mol.2", "gt-mol.3"},
		},
		{
			name: "post-submit steps open is OK",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Load context", Status: "closed"},
				{ID: "gt-mol.2", Title: "Submit MR", Status: "open"},
				{ID: "gt-mol.3", Title: "Wait for verdict", Status: "open"},
			},
			wantErr: false,
		},
		{
			name: "case insensitive submit detection",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Implement", Status: "closed"},
				{ID: "gt-mol.2", Title: "SUBMIT MR and enter awaiting_verdict", Status: "open"},
				{ID: "gt-mol.3", Title: "Self-clean", Status: "open"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMoleculePrereqs(tt.children)
			if tt.wantErr && err == nil {
				t.Errorf("validateMoleculePrereqs() = nil, want error")
				return
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateMoleculePrereqs() = %v, want nil", err)
				return
			}
			if err != nil {
				errMsg := err.Error()
				for _, want := range tt.wantInErr {
					if !strings.Contains(errMsg, want) {
						t.Errorf("error message missing %q, got: %s", want, errMsg)
					}
				}
			}
		})
	}
}

func TestNewMergeRequestFieldsIncludesBranchCustody(t *testing.T) {
	fields := newMergeRequestFields(nil, mergeRequestFieldsOptions{
		Branch:        "polecat/fury/gt-12-branch-custody-model@mpkjsi2s",
		Target:        "integration/test-beaddolt-hardenning",
		SourceIssue:   "gt-12-branch-custody-model",
		Rig:           "gastown",
		Worker:        "fury",
		CommitSHA:     "abc123",
		AgentBead:     "agent-fury",
		SkipVerify:    true,
		PreVerified:   true,
		CleanupPolicy: custodyCleanupPolicy("fury", false),
	})

	if fields.Branch != fields.SourceRef {
		t.Fatalf("legacy branch alias = %q, source_ref = %q", fields.Branch, fields.SourceRef)
	}
	if fields.CommitSHA != fields.SourceCommitSHA {
		t.Fatalf("legacy commit alias = %q, source_commit_sha = %q", fields.CommitSHA, fields.SourceCommitSHA)
	}
	if fields.Target != fields.TargetRef {
		t.Fatalf("legacy target alias = %q, target_ref = %q", fields.Target, fields.TargetRef)
	}
	if fields.Phase != "submitted" || fields.CleanupOwner != "fury" || fields.CleanupPolicy != "witness-after-merge" {
		t.Fatalf("custody fields not populated: %+v", fields.Custody())
	}

	desc := beads.FormatMRFields(fields)
	for _, want := range []string{
		"branch: polecat/fury/gt-12-branch-custody-model@mpkjsi2s",
		"source_ref: polecat/fury/gt-12-branch-custody-model@mpkjsi2s",
		"target_ref: integration/test-beaddolt-hardenning",
		"source_commit_sha: abc123",
		"agent_bead: agent-fury",
		"skip_verify: true",
		"phase: submitted",
		"cleanup_policy: witness-after-merge",
	} {
		if !strings.Contains(desc, want) {
			t.Fatalf("description missing %q:\n%s", want, desc)
		}
	}
}
