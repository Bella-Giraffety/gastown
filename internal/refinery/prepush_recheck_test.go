package refinery

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	beadsdk "github.com/steveyegge/beads"
	"github.com/steveyegge/gastown/internal/beads"
)

type prepushStore struct {
	beadsdk.Storage
	issues map[string]*beadsdk.Issue
}

type prepushPRProvider struct {
	beforeMerge func()
	mergeCalled bool
}

func (p *prepushPRProvider) FindPRNumber(string) (int, error) {
	return 42, nil
}

func (p *prepushPRProvider) IsPRApproved(int) (bool, error) {
	if p.beforeMerge != nil {
		p.beforeMerge()
	}
	return true, nil
}

func (p *prepushPRProvider) MergePR(int, string) (string, error) {
	p.mergeCalled = true
	return "deadbeef", nil
}

func newPrepushStore(issues ...*beadsdk.Issue) *prepushStore {
	store := &prepushStore{issues: make(map[string]*beadsdk.Issue, len(issues))}
	for _, issue := range issues {
		store.issues[issue.ID] = issue
	}
	return store
}

func (s *prepushStore) GetIssue(_ context.Context, id string) (*beadsdk.Issue, error) {
	issue, ok := s.issues[id]
	if !ok {
		return nil, fmt.Errorf("issue %s not found", id)
	}
	return issue, nil
}

func (s *prepushStore) GetLabels(_ context.Context, id string) ([]string, error) {
	issue, ok := s.issues[id]
	if !ok {
		return nil, fmt.Errorf("issue %s not found", id)
	}
	return append([]string(nil), issue.Labels...), nil
}

func (s *prepushStore) CloseIssue(_ context.Context, id, _, _, _ string) error {
	issue, ok := s.issues[id]
	if !ok {
		return fmt.Errorf("issue %s not found", id)
	}
	now := time.Now()
	issue.Status = beadsdk.StatusClosed
	issue.ClosedAt = &now
	issue.UpdatedAt = now
	return nil
}

func prepushIssue(id, description string, labels ...string) *beadsdk.Issue {
	now := time.Now()
	return &beadsdk.Issue{
		ID:          id,
		Title:       id,
		Description: description,
		Status:      beadsdk.StatusOpen,
		IssueType:   beadsdk.IssueType("task"),
		Priority:    2,
		CreatedAt:   now,
		UpdatedAt:   now,
		Labels:      labels,
	}
}

func prepushMRIssue(id, branch, target, sourceIssue string) *beadsdk.Issue {
	desc := beads.FormatMRFields(&beads.MRFields{
		Branch:      branch,
		Target:      target,
		SourceIssue: sourceIssue,
		Worker:      "polecats/test",
		Rig:         "test-rig",
	})
	return prepushIssue(id, desc, "gt:merge-request")
}

func assertOriginMainUnchangedAndReset(t *testing.T, workDir, before string) {
	t.Helper()
	afterOrigin := run(t, workDir, "git", "rev-parse", "origin/main")
	if afterOrigin != before {
		t.Fatalf("origin/main changed: before %s after %s", before, afterOrigin)
	}
	remoteLine := run(t, workDir, "git", "ls-remote", "origin", "refs/heads/main")
	remoteFields := strings.Fields(remoteLine)
	if len(remoteFields) == 0 || remoteFields[0] != before {
		t.Fatalf("remote main changed: before %s ls-remote %q", before, remoteLine)
	}
	localMain := run(t, workDir, "git", "rev-parse", "main")
	if localMain != before {
		t.Fatalf("local main was not reset to origin/main: local %s origin %s", localMain, before)
	}
}

func TestProcessBatch_RechecksClosedMRBeforePush(t *testing.T) {
	workDir, g, cleanup := testGitRepo(t)
	defer cleanup()
	createFeatureBranch(t, workDir, "feature-stale", "stale.txt", "stale\n")

	e := newTestEngineer(t, workDir, g)
	store := newPrepushStore(
		prepushIssue("gt-src", ""),
		prepushMRIssue("gt-mr-stale", "feature-stale", "main", "gt-src"),
	)
	e.beads = beads.NewWithStore(workDir, store)
	before := run(t, workDir, "git", "rev-parse", "origin/main")

	closed := false
	e.mergeSlotAcquire = func(holder string, addWaiter bool) (*beads.MergeSlotStatus, error) {
		if !closed {
			if err := e.beads.CloseWithReason("rejected: report-only", "gt-mr-stale"); err != nil {
				t.Fatalf("close stale MR: %v", err)
			}
			closed = true
		}
		return &beads.MergeSlotStatus{Available: true, Holder: holder}, nil
	}

	mr := &MRInfo{ID: "gt-mr-stale", Branch: "feature-stale", Target: "main", SourceIssue: "gt-src", Worker: "polecats/test"}
	result := e.ProcessBatch(context.Background(), []*MRInfo{mr}, "main", DefaultBatchConfig())

	if len(result.Merged) != 0 {
		t.Fatalf("expected no merged MRs, got %d", len(result.Merged))
	}
	if result.Error != nil {
		t.Fatalf("expected clean policy dequeue, got error: %v", result.Error)
	}
	if !closed {
		t.Fatal("expected merge slot hook to close MR before push")
	}
	assertOriginMainUnchangedAndReset(t, workDir, before)
	if got := store.issues["gt-src"].Status; got != beadsdk.StatusOpen {
		t.Fatalf("source issue status = %s, want open", got)
	}
}

func TestProcessBatch_RechecksMRCloseReasonBeforePush(t *testing.T) {
	workDir, g, cleanup := testGitRepo(t)
	defer cleanup()
	createFeatureBranch(t, workDir, "feature-rejected", "rejected.txt", "rejected\n")

	e := newTestEngineer(t, workDir, g)
	store := newPrepushStore(
		prepushIssue("gt-src", ""),
		prepushMRIssue("gt-mr-rejected", "feature-rejected", "main", "gt-src"),
	)
	e.beads = beads.NewWithStore(workDir, store)
	before := run(t, workDir, "git", "rev-parse", "origin/main")

	mutated := false
	e.mergeSlotAcquire = func(holder string, addWaiter bool) (*beads.MergeSlotStatus, error) {
		if !mutated {
			store.issues["gt-mr-rejected"].Description += "\nclose_reason: rejected"
			store.issues["gt-mr-rejected"].UpdatedAt = time.Now()
			mutated = true
		}
		return &beads.MergeSlotStatus{Available: true, Holder: holder}, nil
	}

	mr := &MRInfo{ID: "gt-mr-rejected", Branch: "feature-rejected", Target: "main", SourceIssue: "gt-src", Worker: "polecats/test"}
	result := e.ProcessBatch(context.Background(), []*MRInfo{mr}, "main", DefaultBatchConfig())

	if len(result.Merged) != 0 {
		t.Fatalf("expected no merged MRs, got %d", len(result.Merged))
	}
	if result.Error != nil {
		t.Fatalf("expected clean policy dequeue, got error: %v", result.Error)
	}
	if !mutated {
		t.Fatal("expected merge slot hook to add close_reason before push")
	}
	assertOriginMainUnchangedAndReset(t, workDir, before)
	if got := store.issues["gt-mr-rejected"].Status; got != beadsdk.StatusClosed {
		t.Fatalf("MR status = %s, want closed", got)
	}
	if got := store.issues["gt-src"].Status; got != beadsdk.StatusOpen {
		t.Fatalf("source issue status = %s, want open", got)
	}
}

func TestRecheckMRStillMergeable_AllowsClosedSourceWithoutPolicyMarker(t *testing.T) {
	workDir, g, cleanup := testGitRepo(t)
	defer cleanup()

	e := newTestEngineer(t, workDir, g)
	source := prepushIssue("gt-src", "")
	now := time.Now()
	source.Status = beadsdk.StatusClosed
	source.ClosedAt = &now
	store := newPrepushStore(
		source,
		prepushMRIssue("gt-mr", "feature", "main", "gt-src"),
	)
	e.beads = beads.NewWithStore(workDir, store)

	mr := &MRInfo{ID: "gt-mr", Branch: "feature", Target: "main", SourceIssue: "gt-src", Worker: "polecats/test"}
	result := e.recheckMRStillMergeable(mr, "main")
	if !result.Success {
		t.Fatalf("closed source without policy marker should remain merge-eligible, got: %+v", result)
	}
	if got := store.issues["gt-mr"].Status; got != beadsdk.StatusOpen {
		t.Fatalf("MR status = %s, want open", got)
	}
}

func TestDoMergePR_RechecksSourceBeforeMergeAPI(t *testing.T) {
	workDir, g, cleanup := testGitRepo(t)
	defer cleanup()

	e := newTestEngineer(t, workDir, g)
	store := newPrepushStore(
		prepushIssue("gt-src", ""),
		prepushMRIssue("gt-mr-pr", "feature-pr", "main", "gt-src"),
	)
	e.beads = beads.NewWithStore(workDir, store)
	requireReview := true
	e.config.RequireReview = &requireReview
	provider := &prepushPRProvider{
		beforeMerge: func() {
			store.issues["gt-src"].Description = "no_merge: true"
			store.issues["gt-src"].UpdatedAt = time.Now()
		},
	}
	e.prProvider = provider

	mr := &MRInfo{ID: "gt-mr-pr", Branch: "feature-pr", Target: "main", SourceIssue: "gt-src", Worker: "polecats/test"}
	result := e.doMergePR(context.Background(), mr)

	if result.Success || !result.NoMerge {
		t.Fatalf("expected clean policy rejection before PR merge API, got: %+v", result)
	}
	if provider.mergeCalled {
		t.Fatal("MergePR was called after source became no_merge")
	}
	if got := store.issues["gt-mr-pr"].Status; got != beadsdk.StatusClosed {
		t.Fatalf("MR status = %s, want closed", got)
	}
	if got := store.issues["gt-src"].Status; got != beadsdk.StatusOpen {
		t.Fatalf("source issue status = %s, want open", got)
	}
}

func TestProcessBatch_RechecksSourceFlagsBeforePush(t *testing.T) {
	tests := []struct {
		name        string
		description string
	}{
		{name: "no_merge", description: "no_merge: true"},
		{name: "review_only", description: "review_only: true"},
		{name: "local_merge", description: "merge_strategy: local"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workDir, g, cleanup := testGitRepo(t)
			defer cleanup()
			createFeatureBranch(t, workDir, "feature-"+tc.name, tc.name+".txt", tc.name+"\n")

			e := newTestEngineer(t, workDir, g)
			store := newPrepushStore(
				prepushIssue("gt-src", ""),
				prepushMRIssue("gt-mr", "feature-"+tc.name, "main", "gt-src"),
			)
			e.beads = beads.NewWithStore(workDir, store)
			before := run(t, workDir, "git", "rev-parse", "origin/main")

			mutated := false
			e.mergeSlotAcquire = func(holder string, addWaiter bool) (*beads.MergeSlotStatus, error) {
				if !mutated {
					store.issues["gt-src"].Description = tc.description
					store.issues["gt-src"].UpdatedAt = time.Now()
					mutated = true
				}
				return &beads.MergeSlotStatus{Available: true, Holder: holder}, nil
			}

			mr := &MRInfo{ID: "gt-mr", Branch: "feature-" + tc.name, Target: "main", SourceIssue: "gt-src", Worker: "polecats/test"}
			result := e.ProcessBatch(context.Background(), []*MRInfo{mr}, "main", DefaultBatchConfig())

			if len(result.Merged) != 0 {
				t.Fatalf("expected no merged MRs, got %d", len(result.Merged))
			}
			if result.Error != nil {
				t.Fatalf("expected clean policy dequeue, got error: %v", result.Error)
			}
			if !mutated {
				t.Fatal("expected merge slot hook to mutate source before push")
			}
			assertOriginMainUnchangedAndReset(t, workDir, before)
			if got := store.issues["gt-mr"].Status; got != beadsdk.StatusClosed {
				t.Fatalf("MR status = %s, want closed", got)
			}
			if got := store.issues["gt-src"].Status; got != beadsdk.StatusOpen {
				t.Fatalf("source issue status = %s, want open", got)
			}
		})
	}
}

func TestProcessBatch_RechecksBatchBeforePush(t *testing.T) {
	workDir, g, cleanup := testGitRepo(t)
	defer cleanup()
	createFeatureBranch(t, workDir, "feature-a", "a.txt", "a\n")
	createFeatureBranch(t, workDir, "feature-b", "b.txt", "b\n")

	e := newTestEngineer(t, workDir, g)
	store := newPrepushStore(
		prepushIssue("gt-src-a", ""),
		prepushIssue("gt-src-b", ""),
		prepushMRIssue("gt-mr-a", "feature-a", "main", "gt-src-a"),
		prepushMRIssue("gt-mr-b", "feature-b", "main", "gt-src-b"),
	)
	e.beads = beads.NewWithStore(workDir, store)
	before := run(t, workDir, "git", "rev-parse", "origin/main")

	mutated := false
	e.mergeSlotAcquire = func(holder string, addWaiter bool) (*beads.MergeSlotStatus, error) {
		if !mutated {
			store.issues["gt-src-b"].Description = "no_merge: true"
			store.issues["gt-src-b"].UpdatedAt = time.Now()
			mutated = true
		}
		return &beads.MergeSlotStatus{Available: true, Holder: holder}, nil
	}

	batch := []*MRInfo{
		{ID: "gt-mr-a", Branch: "feature-a", Target: "main", SourceIssue: "gt-src-a", Worker: "polecats/test"},
		{ID: "gt-mr-b", Branch: "feature-b", Target: "main", SourceIssue: "gt-src-b", Worker: "polecats/test"},
	}
	result := e.ProcessBatch(context.Background(), batch, "main", DefaultBatchConfig())

	if len(result.Merged) != 0 {
		t.Fatalf("expected no merged MRs, got %d", len(result.Merged))
	}
	if result.Error != nil {
		t.Fatalf("expected clean policy dequeue, got error: %v", result.Error)
	}
	if !mutated {
		t.Fatal("expected merge slot hook to mutate batch source before push")
	}
	assertOriginMainUnchangedAndReset(t, workDir, before)
	if got := store.issues["gt-mr-b"].Status; got != beadsdk.StatusClosed {
		t.Fatalf("invalidated MR status = %s, want closed", got)
	}
	if got := store.issues["gt-mr-a"].Status; got != beadsdk.StatusOpen {
		t.Fatalf("unaffected MR status = %s, want open", got)
	}
}
