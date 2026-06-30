package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
)

func TestListAssignedWorkIncludesDurableAndEphemeral(t *testing.T) {
	installTestBD(t, `#!/bin/sh
cmd=""
for arg in "$@"; do
  case "$arg" in --*) ;; *) cmd="$arg"; break ;; esac
done
case "$cmd" in
  version)
    echo "bd test"
    ;;
  list)
    echo '[{"id":"gt-durable","title":"durable work","status":"hooked","assignee":"testrig/refinery"}]'
    ;;
  query)
    args="$*"
    case "$args" in
      *"ephemeral=true"*) echo '[{"id":"hq-wisp-refinery","title":"wisp work","status":"hooked","assignee":"testrig/refinery","ephemeral":true}]' ;;
      *) echo '[]' ;;
    esac
    ;;
  *)
    echo '[]'
    ;;
esac
`)

	work, err := listAssignedWork(beads.NewIsolated(t.TempDir()), "testrig/refinery", beads.StatusHooked)
	if err != nil {
		t.Fatalf("listAssignedWork returned error: %v", err)
	}
	if len(work) != 2 {
		t.Fatalf("len(work) = %d, want 2: %#v", len(work), work)
	}
	if work[0].ID != "gt-durable" || work[1].ID != "hq-wisp-refinery" {
		t.Fatalf("work IDs = %q, %q; want durable then wisp", work[0].ID, work[1].ID)
	}
}

func TestListAssignedWorkPropagatesEphemeralError(t *testing.T) {
	installTestBD(t, `#!/bin/sh
cmd=""
for arg in "$@"; do
  case "$arg" in --*) ;; *) cmd="$arg"; break ;; esac
done
case "$cmd" in
  version)
    echo "bd test"
    ;;
  list)
    echo '[]'
    ;;
  query)
    echo 'wisps timeout' >&2
    exit 7
    ;;
  *)
    echo '[]'
    ;;
esac
`)

	_, err := listAssignedWork(beads.NewIsolated(t.TempDir()), "testrig/refinery", beads.StatusHooked)
	if err == nil {
		t.Fatal("listAssignedWork returned nil error, want ephemeral query error")
	}
	if !strings.Contains(err.Error(), "wisps timeout") {
		t.Fatalf("error = %q, want wisp diagnostic", err)
	}
}

func TestFindActivePatrolFindsEphemeralRootOnlyWisp(t *testing.T) {
	installTestBD(t, `#!/bin/sh
cmd=""
for arg in "$@"; do
  case "$arg" in --*) ;; *) cmd="$arg"; break ;; esac
done
case "$cmd" in
  version)
    echo "bd test"
    ;;
  list)
    echo '[]'
    ;;
  query)
    args="$*"
    case "$args" in
      *"ephemeral=true"*"status=\"hooked\""*) echo '[{"id":"hq-wisp-refinery","title":"mol-refinery-patrol","status":"hooked","assignee":"testrig/refinery","ephemeral":true}]' ;;
      *) echo '[]' ;;
    esac
    ;;
  *)
    echo '[]'
    ;;
esac
`)

	patrolID, _, found, err := findActivePatrol(PatrolConfig{
		RoleName:      "refinery",
		PatrolMolName: constants.MolRefineryPatrol,
		BeadsDir:      t.TempDir(),
		Assignee:      "testrig/refinery",
	})
	if err != nil {
		t.Fatalf("findActivePatrol returned error: %v", err)
	}
	if !found || patrolID != "hq-wisp-refinery" {
		t.Fatalf("findActivePatrol found=%v id=%q, want hooked wisp", found, patrolID)
	}
}

func TestFindActivePatrolFindsInProgressEphemeralRootOnlyWisp(t *testing.T) {
	installTestBD(t, `#!/bin/sh
cmd=""
for arg in "$@"; do
  case "$arg" in --*) ;; *) cmd="$arg"; break ;; esac
done
case "$cmd" in
  version)
    echo "bd test"
    ;;
  list)
    echo '[]'
    ;;
	  query)
	    args="$*"
	    case "$args" in
	      *"ephemeral=true"*"status=\"hooked\""*) echo '[]' ;;
	      *"ephemeral=true"*"status=\"in_progress\""*) echo '[{"id":"hq-wisp-refinery-active","title":"mol-refinery-patrol","status":"in_progress","assignee":"testrig/refinery","ephemeral":true}]' ;;
      *) echo '[]' ;;
    esac
    ;;
  *)
    echo '[]'
    ;;
esac
`)

	patrolID, _, found, err := findActivePatrol(PatrolConfig{
		RoleName:      "refinery",
		PatrolMolName: constants.MolRefineryPatrol,
		BeadsDir:      t.TempDir(),
		Assignee:      "testrig/refinery",
	})
	if err != nil {
		t.Fatalf("findActivePatrol returned error: %v", err)
	}
	if !found || patrolID != "hq-wisp-refinery-active" {
		t.Fatalf("findActivePatrol found=%v id=%q, want in-progress wisp", found, patrolID)
	}
}

func TestRunHookShowSeesEphemeralHookedWork(t *testing.T) {
	installTestBD(t, `#!/bin/sh
cmd=""
for arg in "$@"; do
  case "$arg" in --*) ;; *) cmd="$arg"; break ;; esac
done
case "$cmd" in
  version)
    echo "bd test"
    ;;
  list)
    echo '[]'
    ;;
  query)
    args="$*"
    case "$args" in
      *"ephemeral=true"*"status=\"hooked\""*"assignee=\"testrig/refinery\""*) echo '[{"id":"hq-wisp-refinery","title":"mol-refinery-patrol","status":"hooked","assignee":"testrig/refinery","ephemeral":true}]' ;;
      *) echo '[]' ;;
    esac
    ;;
  *)
    echo '[]'
    ;;
esac
`)

	townRoot := t.TempDir()
	for _, dir := range []string{filepath.Join(townRoot, ".beads"), filepath.Join(townRoot, "mayor"), filepath.Join(townRoot, "testrig")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"name":"test"}`), 0o644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir town root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	prevJSON := moleculeJSON
	moleculeJSON = true
	t.Cleanup(func() { moleculeJSON = prevJSON })

	out := captureStdout(t, func() {
		if err := runHookShow(nil, []string{"testrig/refinery"}); err != nil {
			t.Fatalf("runHookShow returned error: %v", err)
		}
	})
	var got struct {
		Agent  string `json:"agent"`
		BeadID string `json:"bead_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse hook show output %q: %v", out, err)
	}
	if got.BeadID != "hq-wisp-refinery" || got.Status != beads.StatusHooked {
		t.Fatalf("hook show bead=%q status=%q, want hooked wisp", got.BeadID, got.Status)
	}
}

func TestRunHookShowSeesEphemeralInProgressWork(t *testing.T) {
	installTestBD(t, `#!/bin/sh
cmd=""
for arg in "$@"; do
  case "$arg" in --*) ;; *) cmd="$arg"; break ;; esac
done
case "$cmd" in
  version)
    echo "bd test"
    ;;
  list)
    echo '[]'
    ;;
	  query)
	    args="$*"
	    case "$args" in
	      *"ephemeral=true"*"status=\"hooked\""*) echo '[]' ;;
	      *"ephemeral=true"*"status=\"in_progress\""*"assignee=\"testrig/refinery\""*) echo '[{"id":"hq-wisp-refinery-active","title":"mol-refinery-patrol","status":"in_progress","assignee":"testrig/refinery","ephemeral":true}]' ;;
      *) echo '[]' ;;
    esac
    ;;
  *)
    echo '[]'
    ;;
esac
`)

	townRoot := t.TempDir()
	for _, dir := range []string{filepath.Join(townRoot, ".beads"), filepath.Join(townRoot, "mayor"), filepath.Join(townRoot, "testrig")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"name":"test"}`), 0o644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir town root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	prevJSON := moleculeJSON
	moleculeJSON = true
	t.Cleanup(func() { moleculeJSON = prevJSON })

	out := captureStdout(t, func() {
		if err := runHookShow(nil, []string{"testrig/refinery"}); err != nil {
			t.Fatalf("runHookShow returned error: %v", err)
		}
	})
	var got struct {
		Agent  string `json:"agent"`
		BeadID string `json:"bead_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse hook show output %q: %v", out, err)
	}
	if got.BeadID != "hq-wisp-refinery-active" || got.Status != "in_progress" {
		t.Fatalf("hook show bead=%q status=%q, want in-progress wisp", got.BeadID, got.Status)
	}
}

func installTestBD(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("mock bd script uses POSIX shell")
	}
	beads.ResetBdAllowStaleCacheForTest()
	t.Cleanup(beads.ResetBdAllowStaleCacheForTest)

	binDir := t.TempDir()
	path := filepath.Join(binDir, "bd")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake bd: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}
