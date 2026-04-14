package cmd

import "testing"

func TestDiscoverHealthDatabasesWith_SortsRuntimeDatabases(t *testing.T) {
	databases := discoverHealthDatabasesWith(3307, func(_ string, _ int) (int, []string, error) {
		return 4, []string{"gastown", "hq", "beads", "coder_dotfiles"}, nil
	})

	want := []string{"beads", "coder_dotfiles", "gastown", "hq"}
	if len(databases) != len(want) {
		t.Fatalf("len(databases) = %d, want %d", len(databases), len(want))
	}
	for i := range want {
		if databases[i] != want[i] {
			t.Fatalf("databases[%d] = %q, want %q", i, databases[i], want[i])
		}
	}
}

func TestDiscoverHealthDatabasesWith_ErrorReturnsNil(t *testing.T) {
	databases := discoverHealthDatabasesWith(3307, func(_ string, _ int) (int, []string, error) {
		return 0, nil, assertErr{}
	})
	if databases != nil {
		t.Fatalf("databases = %v, want nil", databases)
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }
