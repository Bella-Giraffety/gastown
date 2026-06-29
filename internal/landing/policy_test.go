package landing

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/git"
)

func TestResolveForkBackedUsesUpstreamBaseAndBlocksDefaultPush(t *testing.T) {
	rigPath, workDir := setupPolicyRepo(t)
	run(t, workDir, "git", "remote", "set-url", "origin", "https://github.com/Bella-Giraffety/gastown.git")
	run(t, workDir, "git", "remote", "add", "upstream", "https://github.com/gastownhall/gastown.git")

	policy := Resolve(git.NewGit(workDir), rigPath, "main", "main")

	if !policy.ForkBacked {
		t.Fatal("expected fork-backed policy")
	}
	if policy.CleanBaseRef != "upstream/main" {
		t.Fatalf("CleanBaseRef = %q, want upstream/main", policy.CleanBaseRef)
	}
	if err := policy.CheckDefaultBranchDirectPush("main"); err == nil {
		t.Fatal("expected direct default push to be refused")
	}
	if got := policy.PRHead("polecat/nux/gt-abc@123"); got != "Bella-Giraffety:polecat/nux/gt-abc@123" {
		t.Fatalf("PRHead = %q, want fork owner-qualified head", got)
	}
}

func TestResolveNonForkKeepsOriginBaseAndAllowsDirectPush(t *testing.T) {
	rigPath, workDir := setupPolicyRepo(t)
	run(t, workDir, "git", "remote", "set-url", "origin", "https://github.com/gastownhall/gastown.git")

	policy := Resolve(git.NewGit(workDir), rigPath, "main", "main")

	if policy.ForkBacked {
		t.Fatal("did not expect fork-backed policy")
	}
	if policy.CleanBaseRef != "origin/main" {
		t.Fatalf("CleanBaseRef = %q, want origin/main", policy.CleanBaseRef)
	}
	if err := policy.CheckDefaultBranchDirectPush("main"); err != nil {
		t.Fatalf("direct push should be allowed for non-fork rig: %v", err)
	}
}

func TestNormalizeBaseRefPreservesQualifiedRefs(t *testing.T) {
	rigPath, workDir := setupPolicyRepo(t)
	g := git.NewGit(workDir)

	for _, tc := range []struct {
		in   string
		want string
	}{
		{in: "upstream/main", want: "upstream/main"},
		{in: "origin/main", want: "origin/main"},
		{in: "refs/remotes/upstream/main", want: "upstream/main"},
		{in: "integration/epic", want: "origin/integration/epic"},
	} {
		if got := NormalizeBaseRef(g, rigPath, tc.in, "main"); got != tc.want {
			t.Fatalf("NormalizeBaseRef(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func setupPolicyRepo(t *testing.T) (rigPath, workDir string) {
	t.Helper()
	rigPath = t.TempDir()
	workDir = filepath.Join(rigPath, "work")
	run(t, rigPath, "git", "init", "--initial-branch=main", workDir)
	run(t, workDir, "git", "config", "user.email", "test@test.com")
	run(t, workDir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run(t, workDir, "git", "add", ".")
	run(t, workDir, "git", "commit", "-m", "initial")
	run(t, workDir, "git", "remote", "add", "origin", "https://github.com/gastownhall/gastown.git")

	cfg := map[string]any{
		"type":           "rig",
		"version":        1,
		"name":           "gastown",
		"git_url":        "https://github.com/gastownhall/gastown.git",
		"default_branch": "main",
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigPath, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	return rigPath, workDir
}

func run(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
	return string(out)
}
