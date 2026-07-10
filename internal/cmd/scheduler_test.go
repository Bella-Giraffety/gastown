package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/scheduler/capacity"
)

func TestScheduledBeadInfoFromWorkSkipsNonConcreteWork(t *testing.T) {
	fields := &capacity.SlingContextFields{WorkBeadID: "gt-wisp-abc", TargetRig: "gastown"}
	info := beadStatusInfo{Status: "open", IssueType: "task", Labels: []string{"gt:sling-context"}}
	if _, ok := scheduledBeadInfoFromWork("context", fields, info, true, nil); ok {
		t.Fatalf("scheduledBeadInfoFromWork accepted non-concrete work")
	}

	fields.WorkBeadID = "gt-task"
	info = beadStatusInfo{Status: "open", Title: "Concrete task", IssueType: "task"}
	got, ok := scheduledBeadInfoFromWork("context", fields, info, true, nil)
	if !ok {
		t.Fatalf("scheduledBeadInfoFromWork rejected concrete work")
	}
	if got.ID != "gt-task" || got.Title != "Concrete task" || got.TargetRig != "gastown" || got.Blocked {
		t.Fatalf("scheduledBeadInfoFromWork() = %+v, want concrete unblocked scheduled bead", got)
	}
}

func TestScheduledWorkReadinessReasons(t *testing.T) {
	fields := &capacity.SlingContextFields{WorkBeadID: "gt-task", TargetRig: "gastown"}
	tests := []struct {
		name    string
		fields  *capacity.SlingContextFields
		info    beadStatusInfo
		found   bool
		blocked map[string]bool
		wantOK  bool
		wantWhy string
	}{
		{
			name:    "open ready",
			fields:  fields,
			info:    beadStatusInfo{Status: "open"},
			found:   true,
			wantOK:  true,
			wantWhy: "",
		},
		{
			name:    "missing work bead",
			fields:  fields,
			found:   false,
			wantWhy: "not_found",
		},
		{
			name:    "dependency blocked",
			fields:  fields,
			info:    beadStatusInfo{Status: "open"},
			found:   true,
			blocked: map[string]bool{"gt-task": true},
			wantWhy: "blocked_by_dependency",
		},
		{
			name:    "non-open status",
			fields:  fields,
			info:    beadStatusInfo{Status: "hooked"},
			found:   true,
			wantWhy: "status:hooked",
		},
		{
			name:    "force allows non-terminal status",
			fields:  &capacity.SlingContextFields{WorkBeadID: "gt-task", TargetRig: "gastown", Force: true},
			info:    beadStatusInfo{Status: "hooked"},
			found:   true,
			wantOK:  true,
			wantWhy: "",
		},
		{
			name:    "force does not allow terminal status",
			fields:  &capacity.SlingContextFields{WorkBeadID: "gt-task", TargetRig: "gastown", Force: true},
			info:    beadStatusInfo{Status: "closed"},
			found:   true,
			wantWhy: "status:closed",
		},
		{
			name:    "unknown status reason",
			fields:  fields,
			info:    beadStatusInfo{},
			found:   true,
			wantWhy: "status:unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotWhy := scheduledWorkReadiness(tt.fields, tt.info, tt.found, tt.blocked)
			if gotOK != tt.wantOK || gotWhy != tt.wantWhy {
				t.Fatalf("scheduledWorkReadiness() = (%v, %q), want (%v, %q)", gotOK, gotWhy, tt.wantOK, tt.wantWhy)
			}
		})
	}
}

func TestTargetRigBeadsDirUsesRoutesOnly(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}
	dotfilesRig := filepath.Join(townRoot, "dotfiles", "mayor", "rig")
	if err := os.MkdirAll(filepath.Join(dotfilesRig, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(`{"prefix":"do-","path":"dotfiles/mayor/rig"}
`), 0644); err != nil {
		t.Fatal(err)
	}

	if got := targetRigBeadsDir(townRoot, "dotfiles"); got != filepath.Join(dotfilesRig, ".beads") {
		t.Fatalf("targetRigBeadsDir(dotfiles) = %q, want canonical route dir", got)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "ghost", ".beads"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := targetRigBeadsDir(townRoot, "ghost"); got != "" {
		t.Fatalf("targetRigBeadsDir(ghost) = %q, want no filesystem fallback without route", got)
	}
}

func TestSlingContextFieldsChronologicalLessUsesOldest(t *testing.T) {
	old := &capacity.SlingContextFields{WorkBeadID: "gt-task", EnqueuedAt: "2026-01-01T00:00:00Z"}
	forcedNew := &capacity.SlingContextFields{WorkBeadID: "gt-task", EnqueuedAt: "2026-01-02T00:00:00Z", Force: true}
	if !slingContextFieldsChronologicalLess(old, forcedNew, "ctx-old", "ctx-new") {
		t.Fatalf("older context should sort first globally even when newer context is forced")
	}
	if slingContextFieldsChronologicalLess(forcedNew, old, "ctx-new", "ctx-old") {
		t.Fatalf("newer forced context should not jump older unrelated queue entries")
	}

	oldForced := &capacity.SlingContextFields{WorkBeadID: "gt-task", EnqueuedAt: "2026-01-01T00:00:00Z", Force: true}
	newForced := &capacity.SlingContextFields{WorkBeadID: "gt-task", EnqueuedAt: "2026-01-02T00:00:00Z", Force: true}
	if !slingContextFieldsChronologicalLess(oldForced, newForced, "ctx-old", "ctx-new") {
		t.Fatalf("oldest forced context should sort first when both are forced")
	}
}

func TestPreferredSlingContextIndexesPrefersCanonicalThenForce(t *testing.T) {
	townRoot := t.TempDir()
	townBeadsDir := filepath.Join(townRoot, ".beads")
	dotfilesBeadsDir := filepath.Join(townRoot, "dotfiles", "mayor", "rig", ".beads")
	for _, dir := range []string{townBeadsDir, dotfilesBeadsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(townBeadsDir, "routes.jsonl"), []byte(`{"prefix":"do-","path":"dotfiles/mayor/rig"}
`), 0644); err != nil {
		t.Fatal(err)
	}

	wrongForced := slingContextRecord{issue: &beads.Issue{ID: "ctx-wrong"}, workDir: townRoot, beadsDir: townBeadsDir}
	canonical := slingContextRecord{issue: &beads.Issue{ID: "ctx-canonical"}, workDir: filepath.Dir(dotfilesBeadsDir), beadsDir: dotfilesBeadsDir}
	keep := preferredSlingContextIndexes(townRoot,
		[]slingContextRecord{wrongForced, canonical},
		[]*capacity.SlingContextFields{
			{WorkBeadID: "do-csbb", TargetRig: "dotfiles", EnqueuedAt: "2026-01-01T00:00:00Z", Force: true},
			{WorkBeadID: "do-csbb", TargetRig: "dotfiles", EnqueuedAt: "2026-01-02T00:00:00Z"},
		})
	if keep[0] || !keep[1] {
		t.Fatalf("keep = %v, want canonical context preferred over wrong-DB forced context", keep)
	}

	canonicalForced := slingContextRecord{issue: &beads.Issue{ID: "ctx-canonical-force"}, workDir: filepath.Dir(dotfilesBeadsDir), beadsDir: dotfilesBeadsDir}
	keep = preferredSlingContextIndexes(townRoot,
		[]slingContextRecord{canonical, canonicalForced},
		[]*capacity.SlingContextFields{
			{WorkBeadID: "do-csbb", TargetRig: "dotfiles", EnqueuedAt: "2026-01-01T00:00:00Z"},
			{WorkBeadID: "do-csbb", TargetRig: "dotfiles", EnqueuedAt: "2026-01-02T00:00:00Z", Force: true},
		})
	if keep[0] || !keep[1] {
		t.Fatalf("keep = %v, want forced canonical context preferred", keep)
	}
}

func TestScheduledBeadInfoFromWorkReportsBlockedReason(t *testing.T) {
	fields := &capacity.SlingContextFields{WorkBeadID: "gt-task", TargetRig: "gastown"}
	info := beadStatusInfo{Status: "open", Title: "Concrete task", IssueType: "task"}
	got, ok := scheduledBeadInfoFromWork("context", fields, info, true, map[string]bool{"gt-task": true})
	if !ok {
		t.Fatalf("scheduledBeadInfoFromWork rejected concrete work")
	}
	if !got.Blocked || got.BlockedReason != "blocked_by_dependency" {
		t.Fatalf("scheduledBeadInfoFromWork() = %+v, want blocked_by_dependency", got)
	}
}
