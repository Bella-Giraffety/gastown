//go:build cgo

package beads

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	beadsdk "github.com/steveyegge/beads"
)

func TestPromoteWispEmbeddedStorePromotesAndAddsStructuredComment(t *testing.T) {
	bdPath, err := exec.LookPath("bd")
	if err != nil {
		t.Skip("bd executable not available")
	}

	workDir := t.TempDir()
	runCmd(t, workDir, nil, "git", "init", "-q")
	runCmd(t, workDir, nil, "git", "config", "user.name", "Real Store Tester")
	runCmd(t, workDir, nil, "git", "config", "user.email", "real-store@example.invalid")

	beadsDir := filepath.Join(workDir, ".beads")
	bdEnv := cleanBDTestEnv(beadsDir, "real-store-actor")
	runCmd(t, workDir, bdEnv, bdPath, "init", "--prefix", "pw", "--skip-agents", "--skip-hooks", "--non-interactive", "--quiet")
	assertEmbeddedDoltMode(t, beadsDir)
	out := runCmd(t, workDir, bdEnv, bdPath, "create", "--title", "Promote me", "--type", "task", "--ephemeral", "--json", "--quiet")

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(extractJSONObject(out), &created); err != nil {
		t.Fatalf("parse bd create JSON: %v\n%s", err, out)
	}
	if created.ID == "" {
		t.Fatalf("bd create returned empty id: %s", out)
	}

	t.Setenv("BD_ACTOR", "real-store-actor")
	b := NewWithBeadsDir(workDir, beadsDir)
	if err := b.PromoteWisp(created.ID, "proven value"); err != nil {
		t.Fatalf("PromoteWisp: %v", err)
	}

	store, err := beadsdk.OpenFromConfig(context.Background(), beadsDir)
	if err != nil {
		t.Fatalf("OpenFromConfig: %v", err)
	}
	defer store.Close()

	issue, err := store.GetIssue(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetIssue after promote: %v", err)
	}
	if issue.Ephemeral {
		t.Fatalf("issue %s is still ephemeral after PromoteWisp", created.ID)
	}

	comments, err := store.GetIssueComments(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetIssueComments after promote: %v", err)
	}
	for _, c := range comments {
		if c.Text == "Promoted from Level 0: proven value" && c.Author == "real-store-actor" {
			return
		}
	}
	t.Fatalf("structured promotion comment not found in %#v", comments)
}

func runCmd(t *testing.T, dir string, env []string, name string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

func cleanBDTestEnv(beadsDir, actor string) []string {
	blocked := []string{
		"BEADS_DIR=", "BEADS_DB=", "BD_DB=", "BEADS_DOLT_DATA_DIR=", "GT_DOLT_DATA=",
		"BEADS_DOLT_SERVER_DATABASE=", "BEADS_ACTOR=", "BD_ACTOR=", "BD_NON_INTERACTIVE=",
	}
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		blockedEntry := false
		for _, prefix := range blocked {
			if strings.HasPrefix(entry, prefix) {
				blockedEntry = true
				break
			}
		}
		if !blockedEntry {
			env = append(env, entry)
		}
	}
	return append(env, "BEADS_DIR="+beadsDir, "BD_ACTOR="+actor, "BD_NON_INTERACTIVE=1")
}

func assertEmbeddedDoltMode(t *testing.T, beadsDir string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}
	var metadata struct {
		DoltMode string `json:"dolt_mode"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("parse metadata.json: %v", err)
	}
	if metadata.DoltMode != "" && metadata.DoltMode != "embedded" {
		t.Fatalf("dolt_mode = %q, want embedded", metadata.DoltMode)
	}
}

func extractJSONObject(out []byte) []byte {
	idx := strings.IndexByte(string(out), '{')
	if idx < 0 {
		return out
	}
	return out[idx:]
}
