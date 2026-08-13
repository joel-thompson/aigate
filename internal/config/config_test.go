package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse_ValidFullConfig(t *testing.T) {
	input := `
claude:
  session-start:
    context:
      - type: project-structure
        max_depth: 3
      - type: shell
        command: "go-task --list"
        label: "Available tasks"

git:
  pre-commit:
    checks:
      - type: secrets-scan
`
	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Claude == nil {
		t.Fatal("expected claude config to be non-nil")
	}
	if cfg.Claude.SessionStart == nil {
		t.Fatal("expected session-start config to be non-nil")
	}
	if len(cfg.Claude.SessionStart.Context) != 2 {
		t.Fatalf("expected 2 context providers, got %d", len(cfg.Claude.SessionStart.Context))
	}

	ps := cfg.Claude.SessionStart.Context[0]
	if ps.Type != "project-structure" {
		t.Errorf("expected type project-structure, got %q", ps.Type)
	}
	if ps.MaxDepth != 3 {
		t.Errorf("expected max_depth 3, got %d", ps.MaxDepth)
	}

	shell := cfg.Claude.SessionStart.Context[1]
	if shell.Type != "shell" {
		t.Errorf("expected type shell, got %q", shell.Type)
	}
	if shell.Command != "go-task --list" {
		t.Errorf("expected command %q, got %q", "go-task --list", shell.Command)
	}
	if shell.Label != "Available tasks" {
		t.Errorf("expected label %q, got %q", "Available tasks", shell.Label)
	}

	if cfg.Git == nil {
		t.Fatal("expected git config to be non-nil")
	}
	if cfg.Git.PreCommit == nil {
		t.Fatal("expected pre-commit config to be non-nil")
	}
	if len(cfg.Git.PreCommit.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(cfg.Git.PreCommit.Checks))
	}
	if cfg.Git.PreCommit.Checks[0].Type != "secrets-scan" {
		t.Errorf("expected type secrets-scan, got %q", cfg.Git.PreCommit.Checks[0].Type)
	}
}

func TestParse_EmptyConfig(t *testing.T) {
	cfg, err := Parse([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude != nil {
		t.Error("expected claude to be nil for empty config")
	}
	if cfg.Git != nil {
		t.Error("expected git to be nil for empty config")
	}
}

func TestParse_ClaudeOnly(t *testing.T) {
	input := `
claude:
  session-start:
    context:
      - type: project-structure
`
	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude == nil {
		t.Fatal("expected claude config to be non-nil")
	}
	if cfg.Git != nil {
		t.Error("expected git to be nil")
	}
}

func TestParse_GitOnly(t *testing.T) {
	input := `
git:
  pre-commit:
    checks:
      - type: secrets-scan
`
	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude != nil {
		t.Error("expected claude to be nil")
	}
	if cfg.Git == nil {
		t.Fatal("expected git config to be non-nil")
	}
}

func TestParse_UnknownType(t *testing.T) {
	input := `
claude:
  session-start:
    context:
      - type: nonexistent-provider
`
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	want := `claude.session-start.context[0]: unknown provider type "nonexistent-provider"`
	if err.Error() != want {
		t.Errorf("expected error %q, got %q", want, err.Error())
	}
}

func TestParse_MissingType(t *testing.T) {
	input := `
claude:
  session-start:
    context:
      - command: "echo hello"
`
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for missing type")
	}
	want := `claude.session-start.context[0]: missing required field "type"`
	if err.Error() != want {
		t.Errorf("expected error %q, got %q", want, err.Error())
	}
}

func TestParse_ShellMissingCommand(t *testing.T) {
	input := `
claude:
  session-start:
    context:
      - type: shell
        label: "My thing"
`
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for shell without command")
	}
	want := `claude.session-start.context[0]: type "shell" requires "command" field`
	if err.Error() != want {
		t.Errorf("expected error %q, got %q", want, err.Error())
	}
}

func TestParse_UnknownTypeInGitChecks(t *testing.T) {
	input := `
git:
  pre-commit:
    checks:
      - type: unknown-check
`
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for unknown type in git checks")
	}
	want := `git.pre-commit.checks[0]: unknown provider type "unknown-check"`
	if err.Error() != want {
		t.Errorf("expected error %q, got %q", want, err.Error())
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := Parse([]byte("{{not yaml"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")
	content := `
claude:
  session-start:
    context:
      - type: project-structure
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude == nil || cfg.Claude.SessionStart == nil {
		t.Fatal("expected claude.session-start to be non-nil")
	}
	if len(cfg.Claude.SessionStart.Context) != 1 {
		t.Fatalf("expected 1 context provider, got %d", len(cfg.Claude.SessionStart.Context))
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/.aigate.yml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParse_UnknownYAMLKey(t *testing.T) {
	input := `
claude:
  sesion-start:
    context:
      - type: project-structure
`
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for unknown YAML key (typo)")
	}
}

func TestParse_UnknownNestedYAMLKey(t *testing.T) {
	input := `
git:
  pre-commit:
    chekcs:
      - type: secrets-scan
`
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for unknown nested YAML key (typo)")
	}
}

func TestParse_ShellWhitespaceOnlyCommand(t *testing.T) {
	input := `
claude:
  session-start:
    context:
      - type: shell
        command: "   "
`
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for whitespace-only command")
	}
}

func TestParse_StopPromptEnabledWithPrompt(t *testing.T) {
	input := `
claude:
  stop-prompt:
    enabled: true
    prompt: "be brief"
    min-length: 100
`
	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude == nil || cfg.Claude.StopPrompt == nil {
		t.Fatal("expected claude.stop-prompt to be non-nil")
	}
	sp := cfg.Claude.StopPrompt
	if !sp.Enabled {
		t.Error("expected enabled to be true")
	}
	if sp.Prompt != "be brief" {
		t.Errorf("expected prompt %q, got %q", "be brief", sp.Prompt)
	}
	if sp.MinLength != 100 {
		t.Errorf("expected min-length 100, got %d", sp.MinLength)
	}
}

func TestParse_StopPromptEnabledWithoutPrompt(t *testing.T) {
	input := `
claude:
  stop-prompt:
    enabled: true
`
	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude == nil || cfg.Claude.StopPrompt == nil {
		t.Fatal("expected claude.stop-prompt to be non-nil")
	}
	if cfg.Claude.StopPrompt.Prompt != "" {
		t.Errorf("expected empty prompt, got %q", cfg.Claude.StopPrompt.Prompt)
	}
}

func TestParse_StopPromptMinLength(t *testing.T) {
	input := `
claude:
  stop-prompt:
    enabled: true
    min-length: 250
`
	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude.StopPrompt.MinLength != 250 {
		t.Errorf("expected min-length 250, got %d", cfg.Claude.StopPrompt.MinLength)
	}
}

func TestParse_StopPromptDisabled(t *testing.T) {
	input := `
claude:
  stop-prompt:
    enabled: false
`
	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude.StopPrompt == nil {
		t.Fatal("expected claude.stop-prompt to be non-nil")
	}
	if cfg.Claude.StopPrompt.Enabled {
		t.Error("expected enabled to be false")
	}
}

func TestParse_SessionStartAndStopPromptTogether(t *testing.T) {
	input := `
claude:
  session-start:
    context:
      - type: project-structure
  stop-prompt:
    enabled: true
`
	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude.SessionStart == nil {
		t.Fatal("expected session-start to be non-nil")
	}
	if cfg.Claude.StopPrompt == nil {
		t.Fatal("expected stop-prompt to be non-nil")
	}
}

func TestParse_UnknownStopPromptKey(t *testing.T) {
	input := `
claude:
  stop-prompt:
    enbaled: true
`
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for unknown YAML key (typo)")
	}
}

func TestParse_CIInfoType(t *testing.T) {
	input := `
claude:
  session-start:
    context:
      - type: ci-info
`
	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Claude.SessionStart.Context) != 1 {
		t.Fatalf("expected 1 context provider, got %d", len(cfg.Claude.SessionStart.Context))
	}
	if cfg.Claude.SessionStart.Context[0].Type != "ci-info" {
		t.Errorf("expected type ci-info, got %q", cfg.Claude.SessionStart.Context[0].Type)
	}
}

func TestParse_ShellWithCommandNoLabel(t *testing.T) {
	input := `
claude:
  session-start:
    context:
      - type: shell
        command: "echo hello"
`
	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude.SessionStart.Context[0].Label != "" {
		t.Errorf("expected empty label, got %q", cfg.Claude.SessionStart.Context[0].Label)
	}
}
