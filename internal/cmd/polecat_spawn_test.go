package cmd

import "testing"

func TestSpawnedPolecatSessionStartOptionsPreserveIssue(t *testing.T) {
	spawn := &SpawnedPolecatInfo{
		Issue: "gt-olum",
		agent: "codex",
	}

	opts := spawn.sessionStartOptions("/tmp/runtime-config")
	if opts.Issue != "gt-olum" {
		t.Fatalf("Issue = %q, want %q", opts.Issue, "gt-olum")
	}
	if opts.RuntimeConfigDir != "/tmp/runtime-config" {
		t.Fatalf("RuntimeConfigDir = %q, want %q", opts.RuntimeConfigDir, "/tmp/runtime-config")
	}
	if opts.Agent != "codex" {
		t.Fatalf("Agent = %q, want %q", opts.Agent, "codex")
	}
}
