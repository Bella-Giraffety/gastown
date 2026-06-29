package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNotifyConvoyCompletion_StampsAndSkipsDuplicate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	statePath := filepath.Join(binDir, "notified.state")
	mailLogPath := filepath.Join(binDir, "mail.log")
	exportLogPath := filepath.Join(binDir, "export.log")
	bdPath := filepath.Join(binDir, "bd")
	gtPath := filepath.Join(binDir, "gt")

	bdScript := `#!/bin/sh
STATE="` + statePath + `"
EXPORT_LOG="` + exportLogPath + `"
if [ "$1" = "--allow-stale" ]; then
  shift
fi
case "$1" in
  version)
    exit 0
    ;;
  show)
    if [ -f "$STATE" ]; then
      printf '%s\n' '[{"id":"hq-cv-dup","description":"Owner: mayor/\ncompletion_notified_at: 2026-05-25T02:30:00Z","created_at":"2026-05-25T02:00:00Z"}]'
    else
      printf '%s\n' '[{"id":"hq-cv-dup","description":"Owner: mayor/","created_at":"2026-05-25T02:00:00Z"}]'
    fi
    exit 0
    ;;
  update)
    touch "$STATE"
    exit 0
    ;;
  export)
    echo "$@" >> "$EXPORT_LOG"
    exit 0
    ;;
  sql)
    printf '%s\n' '[]'
    exit 0
    ;;
esac
exit 0
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	gtScript := `#!/bin/sh
if [ "$1" = "mail" ] && [ "$2" = "send" ]; then
  echo "$@" >> "` + mailLogPath + `"
fi
exit 0
`
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write gt stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	notifyConvoyCompletion(townRoot, "hq-cv-dup", "Duplicate Guard")
	notifyConvoyCompletion(townRoot, "hq-cv-dup", "Duplicate Guard")

	data, err := os.ReadFile(mailLogPath)
	if err != nil {
		t.Fatalf("read mail log: %v", err)
	}
	if got := strings.Count(string(data), "mail send"); got != 1 {
		t.Fatalf("mail sends = %d, want 1; log:\n%s", got, string(data))
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("completion notification state was not recorded: %v", err)
	}
	exportData, err := os.ReadFile(exportLogPath)
	if err != nil {
		t.Fatalf("read export log: %v", err)
	}
	if got := strings.Count(string(exportData), "export -o"); got != 1 {
		t.Fatalf("bd export calls = %d, want 1; log:\n%s", got, string(exportData))
	}
	if !strings.Contains(string(exportData), filepath.Join(townRoot, ".beads", "issues.jsonl")) {
		t.Fatalf("bd export did not target town issues.jsonl; log:\n%s", string(exportData))
	}
}

func TestNotifyConvoyCompletion_RollsBackStampWhenExportFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	updateLogPath := filepath.Join(binDir, "update.log")
	exportLogPath := filepath.Join(binDir, "export.log")
	mailLogPath := filepath.Join(binDir, "mail.log")
	bdScript := `#!/bin/sh
if [ "$1" = "--allow-stale" ]; then
  shift
fi
case "$1" in
  version)
    exit 0
    ;;
  show)
    printf '%s\n' '[{"id":"hq-cv-fail","description":"Owner: mayor/","created_at":"2026-05-25T02:00:00Z"}]'
    exit 0
    ;;
  update)
    echo "$@" >> "` + updateLogPath + `"
    exit 0
    ;;
  export)
    echo "$@" >> "` + exportLogPath + `"
    printf 'export failed\n' >&2
    exit 1
    ;;
  sql)
    printf '%s\n' '[]'
    exit 0
    ;;
esac
exit 1
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	gtScript := `#!/bin/sh
if [ "$1" = "mail" ] && [ "$2" = "send" ]; then
  echo "$@" >> "` + mailLogPath + `"
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "gt"), []byte(gtScript), 0755); err != nil {
		t.Fatalf("write gt stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	notifyConvoyCompletion(townRoot, "hq-cv-fail", "Export Failure")

	updates, err := os.ReadFile(updateLogPath)
	if err != nil {
		t.Fatalf("read update log: %v", err)
	}
	if got := strings.Count(string(updates), "update hq-cv-fail"); got != 2 {
		t.Fatalf("update calls = %d, want stamp and rollback; log:\n%s", got, string(updates))
	}
	if !strings.Contains(string(updates), "completion_notified_at:") {
		t.Fatalf("stamp update missing completion_notified_at:\n%s", string(updates))
	}
	if !strings.Contains(string(updates), "--description=Owner: mayor/") {
		t.Fatalf("rollback update missing original description:\n%s", string(updates))
	}
	exports, err := os.ReadFile(exportLogPath)
	if err != nil {
		t.Fatalf("read export log: %v", err)
	}
	if got := strings.Count(string(exports), "export -o"); got != 1 {
		t.Fatalf("export calls = %d, want 1; log:\n%s", got, string(exports))
	}
	if mailData, err := os.ReadFile(mailLogPath); err == nil && strings.Contains(string(mailData), "mail send") {
		t.Fatalf("mail sent despite failed durable notification stamp:\n%s", string(mailData))
	}
}

func TestCloseConvoyIfComplete_ExportsJSONLBeforeNotification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	orderPath := filepath.Join(binDir, "order.log")
	bdPath := filepath.Join(binDir, "bd")
	gtPath := filepath.Join(binDir, "gt")

	bdScript := `#!/bin/sh
ORDER="` + orderPath + `"
if [ "$1" = "--allow-stale" ]; then
  shift
fi
case "$1" in
  version)
    exit 0
    ;;
  close)
    echo close >> "$ORDER"
    exit 0
    ;;
  export)
    echo export:"$@" >> "$ORDER"
    exit 0
    ;;
  show)
    printf '%s\n' '[{"id":"hq-cv-done","description":"Owner: mayor/","created_at":"2026-05-25T02:00:00Z"}]'
    exit 0
    ;;
  update)
    echo update >> "$ORDER"
    exit 0
    ;;
  sql)
    printf '%s\n' '[]'
    exit 0
    ;;
esac
exit 1
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	gtScript := `#!/bin/sh
if [ "$1" = "mail" ] && [ "$2" = "send" ]; then
  echo mail >> "` + orderPath + `"
fi
exit 0
`
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write gt stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	closed, err := closeConvoyIfComplete(townRoot, "hq-cv-done", "Done Convoy", []trackedIssueInfo{
		{ID: "gt-done", Status: "closed"},
	}, false)
	if err != nil {
		t.Fatalf("closeConvoyIfComplete returned error: %v", err)
	}
	if !closed {
		t.Fatal("closeConvoyIfComplete returned closed=false, want true")
	}

	data, err := os.ReadFile(orderPath)
	if err != nil {
		t.Fatalf("read order log: %v", err)
	}
	got := strings.TrimSpace(string(data))
	want := strings.Join([]string{
		"close",
		"export:export -o " + filepath.Join(townRoot, ".beads", "issues.jsonl"),
		"update",
		"export:export -o " + filepath.Join(townRoot, ".beads", "issues.jsonl"),
		"mail",
	}, "\n")
	if got != want {
		t.Fatalf("operation order mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRunConvoyAdd_ExportsAfterReopenAndTracking(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"type":"town","name":"test"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	orderPath := filepath.Join(binDir, "order.log")
	bdPath := filepath.Join(binDir, "bd")
	bdScript := `#!/bin/sh
ORDER="` + orderPath + `"
case "$1" in
  show)
    printf '%s\n' '[{"id":"hq-cv-add","title":"Add Convoy","status":"closed","issue_type":"convoy","description":"Owner: mayor/\ncompletion_notified_at: 2026-05-25T02:30:00Z","labels":[]}]'
    exit 0
    ;;
  update)
    echo update:"$@" >> "$ORDER"
    exit 0
    ;;
  export)
    echo export:"$@" >> "$ORDER"
    exit 0
    ;;
esac
exit 1
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	oldAddTracking := addTrackingRelationFn
	addTrackingRelationFn = func(townRoot, convoyID, issueID string) error {
		f, err := os.OpenFile(orderPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = fmt.Fprintf(f, "track:%s\n", issueID)
		return err
	}
	t.Cleanup(func() { addTrackingRelationFn = oldAddTracking })

	if err := runConvoyAdd(nil, []string{"hq-cv-add", "gt-one"}); err != nil {
		t.Fatalf("runConvoyAdd returned error: %v", err)
	}

	data, err := os.ReadFile(orderPath)
	if err != nil {
		t.Fatalf("read order log: %v", err)
	}
	got := strings.TrimSpace(string(data))
	wantLines := []string{
		"update:update hq-cv-add --status=open",
		"update:update hq-cv-add --description=Owner: mayor/",
		"track:gt-one",
		"export:export -o " + filepath.Join(townRoot, ".beads", "issues.jsonl"),
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Fatalf("order log missing %q:\n%s", want, got)
		}
	}
	if gotExportCount := strings.Count(got, "export:export -o"); gotExportCount != 1 {
		t.Fatalf("export count = %d, want 1; log:\n%s", gotExportCount, got)
	}
}
