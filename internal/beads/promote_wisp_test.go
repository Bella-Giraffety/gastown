package beads

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	beadsdk "github.com/steveyegge/beads"
)

type promoteMockStore struct {
	beadsdk.Storage
	promotedID     string
	promoteActor   string
	promoteErr     error
	commentID      string
	commentAuthor  string
	commentText    string
	commentErr     error
	commitMessages []string
	commitErr      error
	order          []string
}

func (m *promoteMockStore) PromoteFromEphemeral(_ context.Context, id, actor string) error {
	m.order = append(m.order, "promote")
	m.promotedID = id
	m.promoteActor = actor
	return m.promoteErr
}

func (m *promoteMockStore) AddIssueComment(_ context.Context, issueID, author, text string) (*beadsdk.Comment, error) {
	m.order = append(m.order, "comment")
	m.commentID = issueID
	m.commentAuthor = author
	m.commentText = text
	if m.commentErr != nil {
		return nil, m.commentErr
	}
	return &beadsdk.Comment{IssueID: issueID, Author: author, Text: text}, nil
}

func (m *promoteMockStore) Commit(_ context.Context, message string) error {
	m.order = append(m.order, "commit")
	m.commitMessages = append(m.commitMessages, message)
	return m.commitErr
}

type noPromoteStore struct {
	beadsdk.Storage
	commitCalled bool
}

func (s *noPromoteStore) Commit(context.Context, string) error {
	s.commitCalled = true
	return nil
}

type noCommitStore struct {
	beadsdk.Storage
	promoteCalled bool
}

func (s *noCommitStore) PromoteFromEphemeral(context.Context, string, string) error {
	s.promoteCalled = true
	return nil
}

func (s *noCommitStore) AddIssueComment(context.Context, string, string, string) (*beadsdk.Comment, error) {
	return nil, nil
}

func TestPromoteWispUsesSDKPromotionStructuredCommentAndCommit(t *testing.T) {
	t.Setenv("BD_ACTOR", "gastown/polecats/test")
	store := &promoteMockStore{}
	b := NewWithStore(t.TempDir(), store)

	if err := b.PromoteWisp("gt-test", "proven value"); err != nil {
		t.Fatalf("PromoteWisp: %v", err)
	}

	if store.promotedID != "gt-test" {
		t.Fatalf("promoted ID = %q, want gt-test", store.promotedID)
	}
	if store.promoteActor != "gastown/polecats/test" {
		t.Fatalf("promote actor = %q", store.promoteActor)
	}
	if store.commentID != "gt-test" {
		t.Fatalf("comment ID = %q, want gt-test", store.commentID)
	}
	if store.commentAuthor != "gastown/polecats/test" {
		t.Fatalf("comment author = %q", store.commentAuthor)
	}
	if store.commentText != "Promoted from Level 0: proven value" {
		t.Fatalf("comment text = %q", store.commentText)
	}
	if len(store.commitMessages) != 1 || store.commitMessages[0] != "Promote wisp gt-test" {
		t.Fatalf("commit messages = %#v", store.commitMessages)
	}
	if got := strings.Join(store.order, ","); got != "promote,comment,commit" {
		t.Fatalf("operation order = %q", got)
	}
}

func TestPromoteWispFormatsBlankReason(t *testing.T) {
	t.Setenv("BD_ACTOR", "tester")
	store := &promoteMockStore{}
	b := NewWithStore(t.TempDir(), store)

	if err := b.PromoteWisp("gt-test", "  "); err != nil {
		t.Fatalf("PromoteWisp: %v", err)
	}
	if store.commentText != "Promoted from Level 0" {
		t.Fatalf("comment text = %q", store.commentText)
	}
}

func TestPromoteWispRequiresCapabilitiesBeforeMutation(t *testing.T) {
	t.Run("missing promotion", func(t *testing.T) {
		store := &noPromoteStore{}
		b := NewWithStore(t.TempDir(), store)

		err := b.PromoteWisp("gt-test", "reason")
		if err == nil || !strings.Contains(err.Error(), "PromoteFromEphemeral unsupported") {
			t.Fatalf("PromoteWisp error = %v", err)
		}
		if store.commitCalled {
			t.Fatal("Commit called after missing promotion capability")
		}
	})

	t.Run("missing commit", func(t *testing.T) {
		store := &noCommitStore{}
		b := NewWithStore(t.TempDir(), store)

		err := b.PromoteWisp("gt-test", "reason")
		if err == nil || !strings.Contains(err.Error(), "Commit unsupported") {
			t.Fatalf("PromoteWisp error = %v", err)
		}
		if store.promoteCalled {
			t.Fatal("PromoteFromEphemeral called before commit capability check")
		}
	})
}

func TestPromoteWispErrors(t *testing.T) {
	promoteErr := errors.New("promotion failed")
	commitErr := errors.New("commit failed")

	t.Run("promotion", func(t *testing.T) {
		store := &promoteMockStore{promoteErr: promoteErr}
		b := NewWithStore(t.TempDir(), store)

		err := b.PromoteWisp("gt-test", "reason")
		if !errors.Is(err, promoteErr) {
			t.Fatalf("PromoteWisp error = %v", err)
		}
		if len(store.order) != 1 || store.order[0] != "promote" {
			t.Fatalf("operation order = %#v", store.order)
		}
	})

	t.Run("commit", func(t *testing.T) {
		store := &promoteMockStore{commitErr: commitErr}
		b := NewWithStore(t.TempDir(), store)

		err := b.PromoteWisp("gt-test", "reason")
		if !errors.Is(err, commitErr) {
			t.Fatalf("PromoteWisp error = %v", err)
		}
		if got := strings.Join(store.order, ","); got != "promote,comment,commit" {
			t.Fatalf("operation order = %q", got)
		}
	})
}

func TestPromoteWispTreatsCommentFailureAsBestEffort(t *testing.T) {
	store := &promoteMockStore{commentErr: errors.New("comment failed")}
	b := NewWithStore(t.TempDir(), store)

	if err := b.PromoteWisp("gt-test", "reason"); err != nil {
		t.Fatalf("PromoteWisp: %v", err)
	}
	if got := strings.Join(store.order, ","); got != "promote,comment,commit" {
		t.Fatalf("operation order = %q", got)
	}
}

func TestPromoteWispActorFallbacks(t *testing.T) {
	t.Run("bd actor first", func(t *testing.T) {
		b := New(t.TempDir())
		t.Setenv("BD_ACTOR", "bd-actor")
		t.Setenv("BEADS_ACTOR", "beads-actor")

		if got := b.promotionActor(context.Background()); got != "bd-actor" {
			t.Fatalf("actor = %q", got)
		}
	})

	t.Run("beads actor", func(t *testing.T) {
		b := New(t.TempDir())
		t.Setenv("BD_ACTOR", "")
		t.Setenv("BEADS_ACTOR", "beads-actor")

		if got := b.promotionActor(context.Background()); got != "beads-actor" {
			t.Fatalf("actor = %q", got)
		}
	})

	t.Run("git user", func(t *testing.T) {
		workDir := t.TempDir()
		runGit(t, workDir, "init", "-q")
		runGit(t, workDir, "config", "user.name", "Git Actor")
		b := New(workDir)
		t.Setenv("BD_ACTOR", "")
		t.Setenv("BEADS_ACTOR", "")
		t.Setenv("USER", "env-user")

		if got := b.promotionActor(context.Background()); got != "Git Actor" {
			t.Fatalf("actor = %q", got)
		}
	})

	t.Run("user", func(t *testing.T) {
		b := New(t.TempDir())
		isolateGitConfig(t)
		t.Setenv("BD_ACTOR", "")
		t.Setenv("BEADS_ACTOR", "")
		t.Setenv("USER", "env-user")

		if got := b.promotionActor(context.Background()); got != "env-user" {
			t.Fatalf("actor = %q", got)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		b := New(t.TempDir())
		isolateGitConfig(t)
		t.Setenv("BD_ACTOR", "")
		t.Setenv("BEADS_ACTOR", "")
		t.Setenv("USER", "")

		if got := b.promotionActor(context.Background()); got != "unknown" {
			t.Fatalf("actor = %q", got)
		}
	})

	t.Run("isolated", func(t *testing.T) {
		b := NewIsolated(t.TempDir())
		t.Setenv("BD_ACTOR", "bd-actor")
		t.Setenv("BEADS_ACTOR", "beads-actor")
		t.Setenv("USER", "env-user")

		if got := b.promotionActor(context.Background()); got != "unknown" {
			t.Fatalf("actor = %q", got)
		}
	})
}

func TestPromoteWispUsesCanonicalStorePath(t *testing.T) {
	data, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("read store.go: %v", err)
	}
	body := sourceBetween(t, string(data), "func (b *Beads) PromoteWisp(", "func promotionComment(")
	for _, want := range []string{"PromoteFromEphemeral", "AddIssueComment", "Commit"} {
		if !strings.Contains(body, want) {
			t.Fatalf("PromoteWisp missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"RunInTransaction", "ImportIssueComment", ".AddComment(", "bd.Run", "DOLT_COMMIT", "SELECT ", "UPDATE "} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("PromoteWisp should not contain %q:\n%s", forbidden, body)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func isolateGitConfig(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
}

func sourceBetween(t *testing.T, source, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(source, startMarker)
	if start == -1 {
		t.Fatalf("could not find %q", startMarker)
	}
	end := strings.Index(source[start:], endMarker)
	if end == -1 {
		t.Fatalf("could not find %q after %q", endMarker, startMarker)
	}
	return source[start : start+end]
}
