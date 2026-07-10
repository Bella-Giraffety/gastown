package cmd

import (
	"testing"

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
