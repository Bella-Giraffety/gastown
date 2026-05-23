package refinery

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

func validateMRFieldsForProcessing(id string, fields *beads.MRFields) error {
	if fields == nil {
		return fmt.Errorf("MR %s missing merge-request metadata; repair branch, target, and source_issue fields", id)
	}

	var missing []string
	if fields.Target == "" {
		missing = append(missing, "target")
	}
	if fields.SourceIssue == "" {
		missing = append(missing, "source_issue")
	}
	if len(missing) > 0 {
		return fmt.Errorf("MR %s missing required metadata: %s; repair the bead instead of defaulting to main", id, strings.Join(missing, ", "))
	}

	return nil
}
