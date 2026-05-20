package cmd

import (
	"strings"
	"testing"
)

func TestTapListShowsPolicyAwarePRWorkflowDescription(t *testing.T) {
	oldGuardsOnly := tapListGuardsOnly
	tapListGuardsOnly = true
	t.Cleanup(func() { tapListGuardsOnly = oldGuardsOnly })

	output := captureStdout(t, func() {
		if err := runTapList(nil, nil); err != nil {
			t.Fatalf("runTapList() = %v", err)
		}
	})

	if !strings.Contains(output, "Enforce configured PR/direct-merge workflow") {
		t.Fatalf("tap list output missing policy-aware PR workflow description:\n%s", output)
	}
}
