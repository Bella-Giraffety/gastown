package doctor

import (
	"context"
	"errors"
	"fmt"

	"github.com/steveyegge/gastown/internal/opencodegc"
	"github.com/steveyegge/gastown/internal/util"
)

type OpenCodeRetentionCheck struct {
	FixableCheck
}

func NewOpenCodeRetentionCheck() *OpenCodeRetentionCheck {
	return &OpenCodeRetentionCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "opencode-retention",
				CheckDescription: "Bound OpenCode session history through supported retention",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

func (c *OpenCodeRetentionCheck) Run(ctx *CheckContext) *CheckResult {
	report, err := opencodegc.Analyze(context.Background(), opencodegc.Options{TownRoot: ctx.TownRoot})
	if errors.Is(err, opencodegc.ErrOpenCodeUnavailable) {
		return &CheckResult{Name: c.Name(), Status: StatusOK, Message: "OpenCode CLI unavailable; retention skipped"}
	}
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "OpenCode retention scan failed",
			Details: []string{err.Error()},
			FixHint: "Fix OpenCode DB/CLI access; retention fails closed without a safe scan",
		}
	}
	if !report.NeedsCleanup() {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("OpenCode history within retention (%d session(s), db %s, wal %s)",
				report.SessionCount, util.FormatBytesHuman(report.DBBytes), util.FormatBytesHuman(report.WALBytes)),
		}
	}
	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d OpenCode session(s) eligible for retention; db %s, wal %s",
			report.Eligible, util.FormatBytesHuman(report.DBBytes), util.FormatBytesHuman(report.WALBytes)),
		Details: report.Details(),
		FixHint: "Run 'gt doctor --fix' to prune old OpenCode sessions through supported retention",
	}
}

func (c *OpenCodeRetentionCheck) Fix(ctx *CheckContext) error {
	_, err := opencodegc.Fix(context.Background(), opencodegc.Options{TownRoot: ctx.TownRoot})
	return err
}
