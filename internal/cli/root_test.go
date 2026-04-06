package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommandWithoutArgsShowsHelp(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected help output, got %q", stdout.String())
	}
}

func TestRootCommandWithFlagsButNoTargetsErrors(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for missing targets")
	}

	if !strings.Contains(err.Error(), "provide one or more namespace names or set --all-terminating") {
		t.Fatalf("unexpected error: %v", err)
	}
}
