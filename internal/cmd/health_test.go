package cmd

import (
	"errors"
	"reflect"
	"testing"
)

func TestListVisibleHealthDatabases_SortsVisibleDatabaseNames(t *testing.T) {
	oldDatabaseCount := healthDatabaseCount
	healthDatabaseCount = func(host string, port int) (int, []string, error) {
		if host != "127.0.0.1" {
			t.Fatalf("host = %q, want 127.0.0.1", host)
		}
		if port != 3307 {
			t.Fatalf("port = %d, want 3307", port)
		}
		return 3, []string{"gastown", "beads", "coder_dotfiles"}, nil
	}
	t.Cleanup(func() { healthDatabaseCount = oldDatabaseCount })

	got := listVisibleHealthDatabases(3307)
	want := []string{"beads", "coder_dotfiles", "gastown"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listVisibleHealthDatabases() = %v, want %v", got, want)
	}
}

func TestListVisibleHealthDatabases_ReturnsNilOnQueryError(t *testing.T) {
	oldDatabaseCount := healthDatabaseCount
	healthDatabaseCount = func(string, int) (int, []string, error) {
		return 0, nil, errors.New("boom")
	}
	t.Cleanup(func() { healthDatabaseCount = oldDatabaseCount })

	if got := listVisibleHealthDatabases(3307); got != nil {
		t.Fatalf("listVisibleHealthDatabases() = %v, want nil", got)
	}
}
