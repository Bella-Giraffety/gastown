package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/rig"
)

func TestRefineryStartAgentFlag(t *testing.T) {
	flag := refineryStartCmd.Flags().Lookup("agent")
	if flag == nil {
		t.Fatal("expected refinery start to define --agent flag")
	}
	if flag.DefValue != "" {
		t.Errorf("expected default agent override to be empty, got %q", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "overrides town default") {
		t.Errorf("expected --agent usage to mention overrides town default, got %q", flag.Usage)
	}
}

func TestRefineryStartForceFlag(t *testing.T) {
	flag := refineryStartCmd.Flags().Lookup("force")
	if flag == nil {
		t.Fatal("expected refinery start to define --force flag")
	}
	if flag.DefValue != "false" {
		t.Errorf("expected default force to be false, got %q", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "upstream_url guard") {
		t.Errorf("expected --force usage to mention upstream_url guard, got %q", flag.Usage)
	}
}

func TestRefineryAttachAgentFlag(t *testing.T) {
	flag := refineryAttachCmd.Flags().Lookup("agent")
	if flag == nil {
		t.Fatal("expected refinery attach to define --agent flag")
	}
	if flag.DefValue != "" {
		t.Errorf("expected default agent override to be empty, got %q", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "overrides town default") {
		t.Errorf("expected --agent usage to mention overrides town default, got %q", flag.Usage)
	}
}

func TestRefineryRestartAgentFlag(t *testing.T) {
	flag := refineryRestartCmd.Flags().Lookup("agent")
	if flag == nil {
		t.Fatal("expected refinery restart to define --agent flag")
	}
	if flag.DefValue != "" {
		t.Errorf("expected default agent override to be empty, got %q", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "overrides town default") {
		t.Errorf("expected --agent usage to mention overrides town default, got %q", flag.Usage)
	}
}

func TestRefineryRestartForceFlag(t *testing.T) {
	flag := refineryRestartCmd.Flags().Lookup("force")
	if flag == nil {
		t.Fatal("expected refinery restart to define --force flag")
	}
	if flag.DefValue != "false" {
		t.Errorf("expected default force to be false, got %q", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "upstream_url guard") {
		t.Errorf("expected --force usage to mention upstream_url guard, got %q", flag.Usage)
	}
}

func TestRefineryStartOptionsUsesForceFlag(t *testing.T) {
	old := refineryForce
	t.Cleanup(func() { refineryForce = old })

	refineryForce = false
	if refineryStartOptions().AllowForkRig {
		t.Fatal("AllowForkRig = true with refineryForce false")
	}
	refineryForce = true
	if !refineryStartOptions().AllowForkRig {
		t.Fatal("AllowForkRig = false with refineryForce true")
	}
}

func TestRefineryStartCallersSkipForkRig(t *testing.T) {
	r := testForkRig(t)

	msg := startRefineryForRig(r)
	if !strings.Contains(msg, "refinery skipped (fork rig)") {
		t.Fatalf("startRefineryForRig() = %q, want fork-rig skip", msg)
	}

	result := upStartRefinery(r.Name, r)
	if !result.ok || result.detail != "skipped (fork rig)" {
		t.Fatalf("upStartRefinery() = %+v, want ok fork-rig skip", result)
	}
}

func testForkRig(t *testing.T) *rig.Rig {
	t.Helper()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := rig.SaveRigConfig(rigPath, &rig.RigConfig{
		Type:          "rig",
		Version:       rig.CurrentRigConfigVersion,
		Name:          "testrig",
		UpstreamURL:   "https://github.com/upstream/project",
		DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("SaveRigConfig: %v", err)
	}
	return &rig.Rig{Name: "testrig", Path: rigPath}
}

func TestRefineryEnableDisableCommandsRegistered(t *testing.T) {
	for _, name := range []string{"enable", "disable"} {
		cmd, _, err := refineryCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("refinery command %q lookup error: %v", name, err)
		}
		if cmd == nil || cmd.Name() != name {
			t.Fatalf("refinery command %q not registered", name)
		}
		if cmd.Args == nil {
			t.Fatalf("refinery command %q missing Args validation", name)
		}
	}
}
