package cmd

import (
	"strings"
	"testing"
)

func TestBeadsScopeNote_GasTownHQ(t *testing.T) {
	note := beadsScopeNote([]string{"beads", "gastown", "hq"})

	for _, want := range []string{"database hq", "bd -C ~/gt show hq-abc", "bd --global targets Beads' beads_global", "gt dolt status"} {
		if !strings.Contains(note, want) {
			t.Fatalf("beadsScopeNote() missing %q in:\n%s", want, note)
		}
	}
}

func TestBeadsScopeNote_NoHQ(t *testing.T) {
	if note := beadsScopeNote([]string{"gastown", "beads"}); note != "" {
		t.Fatalf("beadsScopeNote() = %q, want empty", note)
	}
}
