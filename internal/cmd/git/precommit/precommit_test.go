package precommit

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/joelthompson/aigate/internal/config"
	"github.com/joelthompson/aigate/internal/exit"
)

func TestPreCommitCommand_PassesWhenNoSecretsFound(t *testing.T) {
	t.Chdir(t.TempDir())
	initGitRepo(t)
	writeConfig(t, `
git:
  pre-commit:
    checks:
      - type: secrets-scan
`)
	stageFile(t, "file.txt", "hello world\n")

	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got %q", stderr.String())
	}
}

func TestPreCommitCommand_AllowsMissingConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	initGitRepo(t)

	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error (fail open), got %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got %q", stderr.String())
	}
}

func TestPreCommitCommand_BlocksOnUnsupportedCheckType(t *testing.T) {
	t.Chdir(t.TempDir())
	initGitRepo(t)
	writeConfig(t, `
git:
  pre-commit:
    checks:
      - type: shell
        command: "echo hi"
`)
	stageFile(t, "file.txt", "hello world\n")

	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected an error for an unsupported check type")
	}
	if exit.CodeFor(err) != 1 {
		t.Errorf("expected exit code 1, got %d", exit.CodeFor(err))
	}
	// pre-commit doesn't set SilenceErrors/SilenceUsage (tracked as a
	// separate, deliberately deferred bug), so cobra's own usage and error
	// banner leak onto stdout/stderr alongside the check's diagnostic.
	wantOut := "Usage:\n  pre-commit [flags]\n\nFlags:\n  -h, --help   help for pre-commit\n\n"
	if stdout.String() != wantOut {
		t.Errorf("expected stdout %q, got %q", wantOut, stdout.String())
	}
	wantErr := "[check] error: unsupported check type \"shell\"\nError: pre-commit checks failed\n"
	if stderr.String() != wantErr {
		t.Errorf("expected stderr %q, got %q", wantErr, stderr.String())
	}
}

func writeConfig(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(config.DefaultConfigFile, []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
}

func initGitRepo(t *testing.T) {
	t.Helper()
	if err := exec.Command("git", "init").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
}

func stageFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	if err := exec.Command("git", "add", name).Run(); err != nil {
		t.Fatalf("git add %s: %v", name, err)
	}
}
