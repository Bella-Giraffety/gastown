package beads

import (
	"os"
	"strings"
	"testing"
)

func TestPromoteWispUsesSDKPromotion(t *testing.T) {
	data, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("read store.go: %v", err)
	}
	body := sourceBetween(t, string(data), "func (b *Beads) PromoteWisp(", "// storeAddLabel")

	for _, want := range []string{
		"OpenStore(ctx)",
		"PromoteFromEphemeral(context.Context, string, string) error",
		"promoter.PromoteFromEphemeral(ctx, id, actor)",
		"store.RunInTransaction(ctx",
		"tx.ImportIssueComment(ctx, id, actor, comment",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("PromoteWisp missing %q:\n%s", want, body)
		}
	}

	for _, forbidden := range []string{
		`Run("update"`,
		`Run("promote"`,
		`Run("comments"`,
		`--persistent`,
		`depends_on_id`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("PromoteWisp should not use %q:\n%s", forbidden, body)
		}
	}
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
