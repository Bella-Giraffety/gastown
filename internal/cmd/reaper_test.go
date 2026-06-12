package cmd

import "testing"

func TestFormatMoleculeStepWispsLine(t *testing.T) {
	if got := formatMoleculeStepWispsLine("gt", 0, false); got != "" {
		t.Fatalf("zero molecule steps should not change output, got %q", got)
	}

	got := formatMoleculeStepWispsLine("gt", 3, false)
	want := "gt: closed 3 molecule-step wisps with closed parent molecules\n"
	if got != want {
		t.Fatalf("formatMoleculeStepWispsLine() = %q, want %q", got, want)
	}

	got = formatMoleculeStepWispsLine("gt", 2, true)
	want = "gt: would close 2 molecule-step wisps with closed parent molecules\n"
	if got != want {
		t.Fatalf("dry-run formatMoleculeStepWispsLine() = %q, want %q", got, want)
	}
}

func TestFormatMoleculeStepSummaryLine(t *testing.T) {
	if got := formatMoleculeStepSummaryLine("", 0, false); got != "" {
		t.Fatalf("zero molecule steps should not change summary output, got %q", got)
	}

	got := formatMoleculeStepSummaryLine("", 4, false)
	want := "Closed molecule-step wisps: 4\n"
	if got != want {
		t.Fatalf("formatMoleculeStepSummaryLine() = %q, want %q", got, want)
	}

	got = formatMoleculeStepSummaryLine("[DRY RUN] ", 5, true)
	want = "[DRY RUN] Would close molecule-step wisps: 5\n"
	if got != want {
		t.Fatalf("dry-run formatMoleculeStepSummaryLine() = %q, want %q", got, want)
	}
}
