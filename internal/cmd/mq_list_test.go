package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestBuildMQListColumns_IncludesTarget(t *testing.T) {
	tests := []struct {
		name          string
		verify        bool
		wantColumnSeq []string
	}{
		{
			name:   "without verify",
			verify: false,
			wantColumnSeq: []string{
				"ID", "SCORE", "PRI", "CONVOY", "BRANCH", "TARGET", "STATUS", "AGE",
			},
		},
		{
			name:   "with verify",
			verify: true,
			wantColumnSeq: []string{
				"ID", "SCORE", "PRI", "CONVOY", "BRANCH", "TARGET", "STATUS", "GIT", "AGE",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols := buildMQListColumns(tt.verify)
			if len(cols) != len(tt.wantColumnSeq) {
				t.Fatalf("len(columns) = %d, want %d", len(cols), len(tt.wantColumnSeq))
			}
			for i, want := range tt.wantColumnSeq {
				if cols[i].Name != want {
					t.Fatalf("column[%d] = %q, want %q", i, cols[i].Name, want)
				}
			}
		})
	}
}

func TestMergeRequestReadyForSelectionHonorsDependencyDetails(t *testing.T) {
	tests := []struct {
		name  string
		issue *beads.Issue
		want  bool
	}{
		{
			name:  "nil issue is not ready",
			issue: nil,
		},
		{
			name: "open unblocked MR is ready",
			issue: &beads.Issue{
				ID:     "gt-mr-ready",
				Status: "open",
			},
			want: true,
		},
		{
			name: "open MR with unresolved blocker is not ready",
			issue: &beads.Issue{
				ID:     "gt-mr-blocked",
				Status: "open",
				Dependencies: []beads.IssueDep{
					{ID: "gt-blocker", Status: "open", DependencyType: "blocks"},
				},
			},
		},
		{
			name: "parent-child dependency does not block MR selection",
			issue: &beads.Issue{
				ID:     "gt-mr-child",
				Status: "open",
				Dependencies: []beads.IssueDep{
					{ID: "gt-parent", Status: "open", DependencyType: "parent-child"},
				},
			},
			want: true,
		},
		{
			name: "merge-blocks dependency blocks MR selection",
			issue: &beads.Issue{
				ID:     "gt-mr-merge-blocked",
				Status: "open",
				Dependencies: []beads.IssueDep{
					{ID: "gt-merge-blocker", Status: "open", DependencyType: "merge-blocks"},
				},
			},
		},
		{
			name: "closed MR is not ready",
			issue: &beads.Issue{
				ID:     "gt-mr-closed",
				Status: "closed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMergeRequestReadyForSelection(tt.issue); got != tt.want {
				t.Fatalf("isMergeRequestReadyForSelection() = %v, want %v", got, tt.want)
			}
		})
	}
}
