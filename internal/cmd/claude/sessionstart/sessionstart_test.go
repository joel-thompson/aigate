package sessionstart

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/joelthompson/aigate/internal/config"
)

func TestSessionStartCommand_RunsShellProviderAndPrintsOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	writeConfig(t, `
claude:
  session-start:
    context:
      - type: shell
        command: "echo hello"
        label: "Test"
`)

	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	want := "## Test\nhello\n"
	if stdout.String() != want {
		t.Errorf("expected stdout %q, got %q", want, stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got %q", stderr.String())
	}
}

func TestSessionStartCommand_AllowsMissingConfig(t *testing.T) {
	t.Chdir(t.TempDir())

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

func TestSessionStartCommand_UnsupportedProviderType(t *testing.T) {
	t.Chdir(t.TempDir())
	writeConfig(t, `
claude:
  session-start:
    context:
      - type: secrets-scan
`)

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
	want := "[error] unsupported provider type \"secrets-scan\"\n"
	if stderr.String() != want {
		t.Errorf("expected stderr %q, got %q", want, stderr.String())
	}
}

// TestSessionStartCommand_DockerComposeTypeIsSupported guards the
// buildProviders switch: docker-compose being in validTypes is not enough,
// it must also be wired, or it falls through to unsupportedProvider.
func TestSessionStartCommand_DockerComposeTypeIsSupported(t *testing.T) {
	t.Chdir(t.TempDir())
	writeConfig(t, `
claude:
  session-start:
    context:
      - type: docker-compose
`)

	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error (fail open), got %v", err)
	}
	// The temp dir has no compose file, so the provider gates out and emits
	// nothing. What matters is that no unsupported-type error is reported.
	if strings.Contains(stderr.String(), "unsupported provider type") {
		t.Errorf("expected docker-compose to be wired into buildProviders, got stderr %q", stderr.String())
	}
}

// TestSessionStartCommand_GitStateTypeIsSupported guards the buildProviders
// switch: git-state being in validTypes is not enough, it must also be
// wired, or it falls through to unsupportedProvider.
func TestSessionStartCommand_GitStateTypeIsSupported(t *testing.T) {
	t.Chdir(t.TempDir())
	writeConfig(t, `
claude:
  session-start:
    context:
      - type: git-state
`)

	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error (fail open), got %v", err)
	}
	// The temp dir is not a git repo, so the provider gates out and emits
	// nothing. What matters is that no unsupported-type error is reported.
	if strings.Contains(stderr.String(), "unsupported provider type") {
		t.Errorf("expected git-state to be wired into buildProviders, got stderr %q", stderr.String())
	}
}

func writeConfig(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(config.DefaultConfigFile, []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
}
