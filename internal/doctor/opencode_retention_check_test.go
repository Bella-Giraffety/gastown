package doctor

import (
	"strings"
	"testing"
)

func TestOpenCodeRetentionCheckMetadata(t *testing.T) {
	check := NewOpenCodeRetentionCheck()
	if check.Name() != "opencode-retention" {
		t.Fatalf("Name() = %q", check.Name())
	}
	if check.Category() != CategoryCleanup {
		t.Fatalf("Category() = %q, want %q", check.Category(), CategoryCleanup)
	}
	if !check.CanFix() {
		t.Fatal("OpenCode retention check should be fixable")
	}
}

func TestOpenCodeRetentionCheckUnavailableIsOK(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	check := NewOpenCodeRetentionCheck()

	result := check.Run(&CheckContext{TownRoot: t.TempDir()})
	if result.Status != StatusOK {
		t.Fatalf("Status = %v, want StatusOK: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "retention skipped") {
		t.Fatalf("Message = %q, want retention skipped", result.Message)
	}
}
