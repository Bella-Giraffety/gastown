package cmd

import (
	"strings"
	"testing"
)

func TestRoutingHelpWarnsAboutOffPrefixForceIDs(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"root help", rootCmd.Long},
		{"sling help", slingCmd.Long},
		{"doctor help", doctorCmd.Long},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range []string{"bd create --force --id", "wrong", "physical", "database"} {
				if !strings.Contains(tt.text, want) {
					t.Fatalf("expected %s to contain %q, got:\n%s", tt.name, want, tt.text)
				}
			}
		})
	}
}

func TestOffPrefixPlacementHintNamesMitigation(t *testing.T) {
	msg := offPrefixPlacementHint("gt-abc", "gastown")
	for _, want := range []string{
		"BD_DEBUG_ROUTING=1 bd show gt-abc",
		"wrong physical DB",
		"bd create --force --id gt-abc",
		"create it from gastown",
		"duplicate off-prefix rows",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected placement hint to contain %q, got:\n%s", want, msg)
		}
	}
}
