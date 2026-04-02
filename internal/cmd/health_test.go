package cmd

import (
	"errors"
	"reflect"
	"testing"

	"github.com/steveyegge/gastown/internal/doltserver"
)

func TestHealthDatabasesFiltersOrphansAndSorts(t *testing.T) {
	origList := listHealthDatabases
	origOrphans := findHealthOrphans
	t.Cleanup(func() {
		listHealthDatabases = origList
		findHealthOrphans = origOrphans
	})

	listHealthDatabases = func(string) ([]string, error) {
		return []string{"gastown", "hq", "beads", "stray"}, nil
	}
	findHealthOrphans = func(string) ([]doltserver.OrphanedDatabase, error) {
		return []doltserver.OrphanedDatabase{{Name: "stray"}}, nil
	}

	got := healthDatabases("/tmp/town")
	want := []string{"beads", "gastown", "hq"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("healthDatabases() = %v, want %v", got, want)
	}
}

func TestHealthDatabasesFallsBackToListOnOrphanError(t *testing.T) {
	origList := listHealthDatabases
	origOrphans := findHealthOrphans
	t.Cleanup(func() {
		listHealthDatabases = origList
		findHealthOrphans = origOrphans
	})

	listHealthDatabases = func(string) ([]string, error) {
		return []string{"gastown", "hq", "beads"}, nil
	}
	findHealthOrphans = func(string) ([]doltserver.OrphanedDatabase, error) {
		return nil, errors.New("boom")
	}

	got := healthDatabases("/tmp/town")
	want := []string{"beads", "gastown", "hq"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("healthDatabases() = %v, want %v", got, want)
	}
}
