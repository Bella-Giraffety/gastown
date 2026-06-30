package cmd

import (
	"encoding/json"
	"fmt"
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

func TestRefineryPatrolNewHookAndReportLookupUseSameAssignedWisp(t *testing.T) {
	hookedPath := filepath.Join(t.TempDir(), "hooked")
	installTestGT(t, `#!/bin/sh
case "$*" in
  "formula list") echo 'mol-refinery-patrol    Refinery patrol' ;;
  *) echo "unexpected gt $*" >&2; exit 1 ;;
esac
`)
	installTestBD(t, fmt.Sprintf(`#!/bin/sh
hooked=%q
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
      *"ephemeral=true"*"status=\"hooked\""*"assignee=\"testrig/refinery\""*)
        if [ -f "$hooked" ]; then
          echo '[{"id":"hq-wisp-refinery-new","title":"mol-refinery-patrol","status":"hooked","assignee":"testrig/refinery","ephemeral":true}]'
        else
          echo '[]'
        fi
        ;;
      *) echo '[]' ;;
    esac
    ;;
  mol)
    args="$*"
    case "$args" in
      *"wisp"*"create"*) echo 'Root issue: hq-wisp-refinery-new' ;;
      *) echo '[]' ;;
    esac
    ;;
  update)
    args="$*"
    case "$args" in
      *"hq-wisp-refinery-new"*"--status=hooked"*"--assignee=testrig/refinery"*) touch "$hooked" ;;
    esac
    echo '{}'
    ;;
  *)
    echo '[]'
    ;;
esac
`, hookedPath))

	cfg := PatrolConfig{
		RoleName:      "refinery-test",
		PatrolMolName: constants.MolRefineryPatrol,
		BeadsDir:      t.TempDir(),
		Assignee:      "testrig/refinery",
	}
	patrolID, err := autoSpawnPatrol(cfg)
	if err != nil {
		t.Fatalf("autoSpawnPatrol returned error: %v", err)
	}
	if patrolID != "hq-wisp-refinery-new" {
		t.Fatalf("autoSpawnPatrol id=%q, want hq-wisp-refinery-new", patrolID)
	}
	if _, err := os.Stat(hookedPath); err != nil {
		t.Fatalf("autoSpawnPatrol did not hook patrol wisp: %v", err)
	}

	activeID, _, found, err := findActivePatrol(cfg)
	if err != nil {
		t.Fatalf("findActivePatrol returned error: %v", err)
	}
	if !found || activeID != patrolID {
		t.Fatalf("findActivePatrol found=%v id=%q, want spawned hooked patrol %q", found, activeID, patrolID)
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

func TestRunMoleculeStatusSeesEphemeralHookedWork(t *testing.T) {
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
		if err := runMoleculeStatus(nil, []string{"testrig/refinery"}); err != nil {
			t.Fatalf("runMoleculeStatus returned error: %v", err)
		}
	})
	var got MoleculeStatusInfo
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse hook status output %q: %v", out, err)
	}
	if !got.HasWork || got.PinnedBead == nil {
		t.Fatalf("hook status has_work=%v pinned=%#v, want hooked wisp", got.HasWork, got.PinnedBead)
	}
	if got.PinnedBead.ID != "hq-wisp-refinery" || got.PinnedBead.Status != beads.StatusHooked {
		t.Fatalf("hook status bead=%q status=%q, want hooked wisp", got.PinnedBead.ID, got.PinnedBead.Status)
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

func installTestGT(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("mock gt script uses POSIX shell")
	}

	binDir := t.TempDir()
	path := filepath.Join(binDir, "gt")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gt: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}
