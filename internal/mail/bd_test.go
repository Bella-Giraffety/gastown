package mail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBdError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *bdError
		want string
	}{
		{
			name: "stderr present",
			err: &bdError{
				Err:    errors.New("some error"),
				Stderr: "stderr output",
			},
			want: "stderr output",
		},
		{
			name: "no stderr, has error",
			err: &bdError{
				Err:    errors.New("some error"),
				Stderr: "",
			},
			want: "some error",
		},
		{
			name: "no stderr, no error",
			err: &bdError{
				Err:    nil,
				Stderr: "",
			},
			want: "unknown bd error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("bdError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBdError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	bdErr := &bdError{
		Err:    originalErr,
		Stderr: "stderr output",
	}

	unwrapped := bdErr.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("bdError.Unwrap() = %v, want %v", unwrapped, originalErr)
	}
}

func TestBdError_UnwrapNil(t *testing.T) {
	bdErr := &bdError{
		Err:    nil,
		Stderr: "",
	}

	unwrapped := bdErr.Unwrap()
	if unwrapped != nil {
		t.Errorf("bdError.Unwrap() with nil Err should return nil, got %v", unwrapped)
	}
}

func TestBdError_ContainsError(t *testing.T) {
	tests := []struct {
		name     string
		err      *bdError
		substr   string
		contains bool
	}{
		{
			name: "substring present",
			err: &bdError{
				Stderr: "error: bead not found",
			},
			substr:   "bead not found",
			contains: true,
		},
		{
			name: "substring not present",
			err: &bdError{
				Stderr: "error: bead not found",
			},
			substr:   "permission denied",
			contains: false,
		},
		{
			name: "empty stderr",
			err: &bdError{
				Stderr: "",
			},
			substr:   "anything",
			contains: false,
		},
		{
			name: "case sensitive",
			err: &bdError{
				Stderr: "Error: Bead Not Found",
			},
			substr:   "bead not found",
			contains: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.ContainsError(tt.substr)
			if got != tt.contains {
				t.Errorf("bdError.ContainsError(%q) = %v, want %v", tt.substr, got, tt.contains)
			}
		})
	}
}

func TestBdError_ContainsErrorPartialMatch(t *testing.T) {
	err := &bdError{
		Stderr: "fatal: invalid bead ID format: expected prefix-#id",
	}

	// Test partial matches
	if !err.ContainsError("invalid bead ID") {
		t.Error("Should contain partial substring")
	}
	if !err.ContainsError("fatal:") {
		t.Error("Should contain prefix")
	}
	if !err.ContainsError("expected prefix") {
		t.Error("Should contain suffix")
	}
}

func TestBdError_ContainsErrorSpecialChars(t *testing.T) {
	err := &bdError{
		Stderr: "error: bead 'gt-123' not found (exit 1)",
	}

	if !err.ContainsError("'gt-123'") {
		t.Error("Should handle quotes in substring")
	}
	if !err.ContainsError("(exit 1)") {
		t.Error("Should handle parentheses in substring")
	}
}

func TestBdError_ImplementsErrorInterface(t *testing.T) {
	// Verify bdError implements error interface
	var err error = &bdError{
		Err:    errors.New("test"),
		Stderr: "test stderr",
	}

	_ = err.Error() // Should compile and not panic
}

func TestRunBdCommandPinsBeadsDirAndDatabaseEnv(t *testing.T) {
	workDir := t.TempDir()
	beadsDir := filepath.Join(workDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	metadata := `{"dolt_database":"hq","dolt_server_host":"127.0.0.1","dolt_server_port":43113}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	binDir := t.TempDir()
	writeBdEnvStub(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BEADS_DIR", "/wrong/.beads")
	t.Setenv("BEADS_DB", "/wrong/beads.db")
	t.Setenv("BD_DB", "/wrong/bd.db")
	t.Setenv("BEADS_DOLT_SERVER_DATABASE", "gt")
	t.Setenv("BEADS_DOLT_PORT", "3307")

	out, err := runBdCommand(context.Background(), []string{"show", "hq-abc"}, workDir, beadsDir)
	if err != nil {
		t.Fatalf("runBdCommand: %v", err)
	}
	text := string(out)
	for _, want := range []string{
		"BEADS_DIR=" + beadsDir,
		"BEADS_DOLT_SERVER_DATABASE=hq",
		"BEADS_DOLT_PORT=43113",
		"BEADS_DB=",
		"BD_DB=",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("stub output missing %q in:\n%s", want, text)
		}
	}
	for _, bad := range []string{"/wrong", "BEADS_DOLT_SERVER_DATABASE=gt", "BEADS_DOLT_PORT=3307"} {
		if strings.Contains(text, bad) {
			t.Fatalf("stub output contains stale value %q in:\n%s", bad, text)
		}
	}
}

func writeBdEnvStub(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "bd.bat")
		script := `@echo off
echo BEADS_DIR=%BEADS_DIR%
echo BEADS_DB=%BEADS_DB%
echo BD_DB=%BD_DB%
echo BEADS_DOLT_SERVER_DATABASE=%BEADS_DOLT_SERVER_DATABASE%
echo BEADS_DOLT_PORT=%BEADS_DOLT_PORT%
exit /b 0
`
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			t.Fatalf("write bd stub: %v", err)
		}
		return
	}
	path := filepath.Join(dir, "bd")
	script := `#!/bin/sh
printf '%s\n' "BEADS_DIR=$BEADS_DIR"
printf '%s\n' "BEADS_DB=$BEADS_DB"
printf '%s\n' "BD_DB=$BD_DB"
printf '%s\n' "BEADS_DOLT_SERVER_DATABASE=$BEADS_DOLT_SERVER_DATABASE"
printf '%s\n' "BEADS_DOLT_PORT=$BEADS_DOLT_PORT"
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
}

func TestBdError_WithAllFields(t *testing.T) {
	originalErr := errors.New("original error")
	bdErr := &bdError{
		Err:    originalErr,
		Stderr: "command failed: bead not found",
	}

	// Test Error() returns stderr
	got := bdErr.Error()
	want := "command failed: bead not found"
	if got != want {
		t.Errorf("bdError.Error() = %q, want %q", got, want)
	}

	// Test Unwrap() returns original error
	unwrapped := bdErr.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("bdError.Unwrap() = %v, want %v", unwrapped, originalErr)
	}

	// Test ContainsError works
	if !bdErr.ContainsError("bead not found") {
		t.Error("ContainsError should find substring in stderr")
	}
	if bdErr.ContainsError("not present") {
		t.Error("ContainsError should return false for non-existent substring")
	}
}
