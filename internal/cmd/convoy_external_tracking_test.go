package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func writeExternalTrackingBdStub(t *testing.T, scriptBody string) {
	t.Helper()

	binDir := t.TempDir()
	bdPath := filepath.Join(binDir, "bd")
	script := "#!/bin/sh\n" + scriptBody
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func chdirExternalTrackingTest(t *testing.T, dir string) {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
}

func makeExternalTrackingTownWorkspace(t *testing.T) (string, string, string) {
	t.Helper()

	townRoot := t.TempDir()
	townBeads := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeads, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	expectedWD := townRoot
	if resolved, err := filepath.EvalSymlinks(townRoot); err == nil && resolved != "" {
		expectedWD = resolved
	}
	return townRoot, townBeads, expectedWD
}

func TestGetTrackedIssues_FallsBackToShowTrackedDependencies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, townBeads, _ := makeExternalTrackingTownWorkspace(t)
	chdirExternalTrackingTest(t, townRoot)

	scriptBody := fmt.Sprintf(`
case "$*" in
  "--allow-stale version")
    exit 0
    ;;
  "dep list hq-cv-ext --direction=down --type=tracks --allow-stale --json")
    echo '[]'
    ;;
  "show hq-cv-ext --json")
    echo '[{"id":"hq-cv-ext","title":"External convoy","status":"open","issue_type":"convoy","dependencies":[{"id":"external:ghostty:ghostty-123","title":"Ghost 123","status":"open","type":"task","dependency_type":"tracks"},{"id":"external:ghostty:ghostty-456","title":"Ghost 456","status":"closed","type":"task","dependency_type":"tracks"},{"id":"gt-ignore","title":"Ignore me","status":"open","type":"task","dependency_type":"blocks"}]}]'
    ;;
  "--allow-stale show ghostty-123 --json")
    echo '[{"id":"ghostty-123","title":"Ghost 123","status":"open","issue_type":"task"}]'
    ;;
  "--allow-stale show ghostty-456 --json")
    echo '[{"id":"ghostty-456","title":"Ghost 456","status":"closed","issue_type":"task"}]'
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`)
	writeExternalTrackingBdStub(t, scriptBody)

	tracked, err := getTrackedIssues(townBeads, "hq-cv-ext")
	if err != nil {
		t.Fatalf("getTrackedIssues: %v", err)
	}
	if len(tracked) != 2 {
		t.Fatalf("expected 2 tracked issues, got %d", len(tracked))
	}

	ids := []string{tracked[0].ID, tracked[1].ID}
	sort.Strings(ids)
	if ids[0] != "ghostty-123" || ids[1] != "ghostty-456" {
		t.Fatalf("unexpected tracked IDs: %v", ids)
	}

	statusByID := map[string]string{}
	for _, item := range tracked {
		statusByID[item.ID] = item.Status
	}
	if statusByID["ghostty-123"] != "open" || statusByID["ghostty-456"] != "closed" {
		t.Fatalf("unexpected tracked statuses: %#v", statusByID)
	}
}

func TestGetIssueDetailsBatch_CrossRigRouting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, townBeads, _ := makeExternalTrackingTownWorkspace(t)
	routes := `{"prefix":"gm-","path":"gemba"}` + "\n"
	if err := os.WriteFile(filepath.Join(townBeads, "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	rigDir := filepath.Join(townRoot, "gemba")
	beadsDir := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir gemba/.beads: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(rigDir); err == nil && resolved != "" {
		rigDir = resolved
		beadsDir = filepath.Join(resolved, ".beads")
	}
	t.Setenv("EXPECTED_BD_PWD", rigDir)
	t.Setenv("EXPECTED_BEADS_DIR", beadsDir)

	chdirExternalTrackingTest(t, townRoot)

	scriptBody := `
case "$*" in
  "--allow-stale version")
    exit 0
    ;;
  "--allow-stale show gm-abc --json")
    if [ "$(pwd -P)" != "$EXPECTED_BD_PWD" ]; then
      echo "unexpected pwd: $(pwd -P), want $EXPECTED_BD_PWD" >&2
      exit 1
    fi
    if [ "$BEADS_DIR" != "$EXPECTED_BEADS_DIR" ]; then
      echo "unexpected BEADS_DIR: $BEADS_DIR, want $EXPECTED_BEADS_DIR" >&2
      exit 1
    fi
    echo '[{"id":"gm-abc","title":"Gemba task","status":"closed","issue_type":"task"}]'
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`
	writeExternalTrackingBdStub(t, scriptBody)

	result := getIssueDetailsBatch([]string{"gm-abc"})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %#v", len(result), result)
	}
	if result["gm-abc"] == nil {
		t.Fatal("gm-abc missing from result")
	}
	if result["gm-abc"].Status != "closed" {
		t.Fatalf("expected status closed, got %q", result["gm-abc"].Status)
	}
}

func TestGetTrackedIssues_UnknownStatusForUnresolvedDetails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, townBeads, _ := makeExternalTrackingTownWorkspace(t)
	chdirExternalTrackingTest(t, townRoot)

	scriptBody := `
case "$*" in
  "--allow-stale version")
    exit 0
    ;;
  *sql*dependencies*)
    echo '[{"depends_on_id":"ws-missing"},{"depends_on_id":"hq-blank"}]'
    ;;
  "--allow-stale show ws-missing --json"|"show ws-missing --json")
    echo "no issue found matching ws-missing" >&2
    exit 1
    ;;
  "--allow-stale show hq-blank --json"|"show hq-blank --json")
    echo '[{"id":"hq-blank","title":"Blank status","status":"","issue_type":"task"}]'
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`
	writeExternalTrackingBdStub(t, scriptBody)

	tracked, err := getTrackedIssues(townBeads, "hq-cv-unresolved")
	if err != nil {
		t.Fatalf("getTrackedIssues: %v", err)
	}
	if len(tracked) != 2 {
		t.Fatalf("expected 2 tracked issues, got %d: %#v", len(tracked), tracked)
	}

	statusByID := map[string]string{}
	for _, item := range tracked {
		statusByID[item.ID] = item.Status
	}
	if statusByID["ws-missing"] != trackedStatusUnknown {
		t.Fatalf("ws-missing status = %q, want %q", statusByID["ws-missing"], trackedStatusUnknown)
	}
	if statusByID["hq-blank"] != trackedStatusUnknown {
		t.Fatalf("hq-blank status = %q, want %q", statusByID["hq-blank"], trackedStatusUnknown)
	}
}

func TestUnknownTrackedIssueIsNotReady(t *testing.T) {
	cases := []struct {
		name      string
		status    string
		scheduled map[string]bool
		want      bool
	}{
		{name: "unknown", status: trackedStatusUnknown, want: false},
		{name: "blank", status: "", want: false},
		{name: "scheduled", status: "open", scheduled: map[string]bool{"ws-issue": true}, want: false},
		{name: "open unassigned", status: "open", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isReadyIssue(trackedIssueInfo{ID: "ws-issue", Status: tc.status}, tc.scheduled)
			if got != tc.want {
				t.Fatalf("isReadyIssue status %q = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestCloseConvoyIfCompleteReportsUnknownTrackedIssues(t *testing.T) {
	townBeads := t.TempDir()
	tracked := []trackedIssueInfo{
		{ID: "ws-missing", Status: trackedStatusUnknown},
		{ID: "hq-done", Status: "closed"},
	}

	out, err := captureConvoyStdoutErr(t, func() error {
		ready, err := closeConvoyIfComplete(townBeads, "hq-cv-unresolved", "Unresolved", tracked, false)
		if ready {
			t.Fatalf("closeConvoyIfComplete reported ready with unknown tracked status")
		}
		return err
	})
	if err != nil {
		t.Fatalf("closeConvoyIfComplete: %v", err)
	}
	if !strings.Contains(out, "unknown") {
		t.Fatalf("diagnostic missing unknown status: %q", out)
	}
}
