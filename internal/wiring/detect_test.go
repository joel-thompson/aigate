package wiring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeSettings marshals settings to JSON and writes it at path, creating the
// parent directory if needed.
func writeSettings(t *testing.T, path string, settings map[string]any) {
	t.Helper()
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshaling settings: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating settings dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("writing settings: %v", err)
	}
}

func TestConfigInstalled_TrueWhenFileExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude:\n"), 0644)

	if !configInstalled(path) {
		t.Error("expected configInstalled to be true when the file exists")
	}
}

func TestConfigInstalled_FalseWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")

	if configInstalled(path) {
		t.Error("expected configInstalled to be false when the file is missing")
	}
}

func TestHookInstalled_TrueWhenRegistered(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.local.json")
	writeSettings(t, path, map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "aigate claude session-start"}}},
			},
		},
	})

	if !hookInstalled(path, "SessionStart", "aigate claude session-start") {
		t.Error("expected hookInstalled to be true when the command is registered")
	}
}

func TestHookInstalled_FalseWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.local.json")

	if hookInstalled(path, "SessionStart", "aigate claude session-start") {
		t.Error("expected hookInstalled to be false when settings.local.json doesn't exist")
	}
}

func TestHookInstalled_FalseOnMalformedSettingsJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.local.json")
	os.WriteFile(path, []byte("{not valid json"), 0644)

	if hookInstalled(path, "SessionStart", "aigate claude session-start") {
		t.Error("expected hookInstalled to be false on malformed settings JSON")
	}
}

func TestStopPromptInstalled_TrueWhenSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude:\n  stop-prompt:\n    enabled: true\n"), 0644)

	if !stopPromptInstalled(path) {
		t.Error("expected stopPromptInstalled to be true when claude.stop-prompt is set")
	}
}

func TestStopPromptInstalled_FalseWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude:\n  session-start:\n    context: []\n"), 0644)

	if stopPromptInstalled(path) {
		t.Error("expected stopPromptInstalled to be false when claude.stop-prompt is absent")
	}
}

func TestStopPromptInstalled_FalseWhenConfigMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")

	if stopPromptInstalled(path) {
		t.Error("expected stopPromptInstalled to be false when the config file doesn't exist")
	}
}

func TestStopPromptInstalled_FalseOnUnparseableYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude: [this is not a mapping\n"), 0644)

	if stopPromptInstalled(path) {
		t.Error("expected stopPromptInstalled to be false on unparseable YAML")
	}
}

func TestSessionContextInstalled_TrueWhenSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude:\n  session-start:\n    context: []\n"), 0644)

	if !sessionContextInstalled(path) {
		t.Error("expected sessionContextInstalled to be true when claude.session-start is set")
	}
}

func TestSessionContextInstalled_FalseWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude:\n  stop-prompt:\n    enabled: true\n"), 0644)

	if sessionContextInstalled(path) {
		t.Error("expected sessionContextInstalled to be false when claude.session-start is absent")
	}
}

func TestSessionContextInstalled_FalseWhenConfigMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")

	if sessionContextInstalled(path) {
		t.Error("expected sessionContextInstalled to be false when the config file doesn't exist")
	}
}

func TestSessionContextInstalled_FalseOnUnparseableYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude: [this is not a mapping\n"), 0644)

	if sessionContextInstalled(path) {
		t.Error("expected sessionContextInstalled to be false on unparseable YAML")
	}
}

func TestPreCommitChecksInstalled_TrueWhenSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("git:\n  pre-commit:\n    checks: []\n"), 0644)

	if !preCommitChecksInstalled(path) {
		t.Error("expected preCommitChecksInstalled to be true when git.pre-commit is set")
	}
}

func TestPreCommitChecksInstalled_FalseWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude:\n  session-start:\n    context: []\n"), 0644)

	if preCommitChecksInstalled(path) {
		t.Error("expected preCommitChecksInstalled to be false when git.pre-commit is absent")
	}
}

func TestPreCommitChecksInstalled_FalseWhenConfigMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")

	if preCommitChecksInstalled(path) {
		t.Error("expected preCommitChecksInstalled to be false when the config file doesn't exist")
	}
}

func TestPreCommitChecksInstalled_FalseOnUnparseableYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("git: [this is not a mapping\n"), 0644)

	if preCommitChecksInstalled(path) {
		t.Error("expected preCommitChecksInstalled to be false on unparseable YAML")
	}
}

func TestGitHookInstalled_TrueWhenAigatePresent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-commit")
	os.WriteFile(path, []byte("#!/bin/sh\naigate git pre-commit\n"), 0755)

	if !gitHookInstalled(path) {
		t.Error("expected gitHookInstalled to be true when the hook invokes aigate")
	}
}

func TestGitHookInstalled_FalseWhenAigateAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-commit")
	os.WriteFile(path, []byte("#!/bin/sh\necho hi\n"), 0755)

	if gitHookInstalled(path) {
		t.Error("expected gitHookInstalled to be false when the hook doesn't mention aigate")
	}
}

func TestGitHookInstalled_FalseWhenFileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-commit")

	if gitHookInstalled(path) {
		t.Error("expected gitHookInstalled to be false when the hook file doesn't exist")
	}
}

func TestGitHookInstalled_FalseOnUnreadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-commit")
	os.WriteFile(path, []byte("aigate"), 0000)

	if gitHookInstalled(path) {
		t.Error("expected gitHookInstalled to be false when the file can't be read")
	}
}

func TestCIContextInstalled_TrueWhenSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude:\n  session-start:\n    context:\n      - type: ci-info\n"), 0644)

	if !ciContextInstalled(path) {
		t.Error("expected ciContextInstalled to be true when a ci-info entry is present")
	}
}

func TestCIContextInstalled_FalseWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude:\n  session-start:\n    context:\n      - type: project-structure\n"), 0644)

	if ciContextInstalled(path) {
		t.Error("expected ciContextInstalled to be false when no ci-info entry is present")
	}
}

func TestCIContextInstalled_FalseWhenConfigMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")

	if ciContextInstalled(path) {
		t.Error("expected ciContextInstalled to be false when the config file doesn't exist")
	}
}

func TestCIContextInstalled_FalseOnUnparseableYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude: [this is not a mapping\n"), 0644)

	if ciContextInstalled(path) {
		t.Error("expected ciContextInstalled to be false on unparseable YAML")
	}
}

func TestDockerContextInstalled_TrueWhenSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude:\n  session-start:\n    context:\n      - type: docker-compose\n"), 0644)

	if !dockerContextInstalled(path) {
		t.Error("expected dockerContextInstalled to be true when a docker-compose entry is present")
	}
}

func TestDockerContextInstalled_FalseWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude:\n  session-start:\n    context:\n      - type: project-structure\n"), 0644)

	if dockerContextInstalled(path) {
		t.Error("expected dockerContextInstalled to be false when no docker-compose entry is present")
	}
}

func TestDockerContextInstalled_FalseWhenConfigMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")

	if dockerContextInstalled(path) {
		t.Error("expected dockerContextInstalled to be false when the config file doesn't exist")
	}
}

func TestDockerContextInstalled_FalseOnUnparseableYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	os.WriteFile(path, []byte("claude: [this is not a mapping\n"), 0644)

	if dockerContextInstalled(path) {
		t.Error("expected dockerContextInstalled to be false on unparseable YAML")
	}
}

func TestDetectState_AllFalseInFreshDir(t *testing.T) {
	dir := t.TempDir()

	cur, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cur.Config || cur.SessionHook || cur.StopHook || cur.StopPrompt || cur.GitHook {
		t.Errorf("expected a fresh directory to report nothing installed, got %+v", cur)
	}
}

func TestDetectState_AllTrueWhenFullySetUp(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".aigate.yml"), []byte("claude:\n  stop-prompt:\n    enabled: true\n"), 0644)

	writeSettings(t, filepath.Join(dir, ".claude", "settings.local.json"), map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "aigate claude session-start"}}},
			},
			"Stop": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "aigate claude stop"}}},
			},
		},
	})

	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	os.MkdirAll(filepath.Dir(hookPath), 0755)
	os.WriteFile(hookPath, []byte("#!/bin/sh\naigate git pre-commit\n"), 0755)

	cur, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cur.Config || !cur.SessionHook || !cur.StopHook || !cur.StopPrompt || !cur.GitHook {
		t.Errorf("expected fully set up project to report everything installed, got %+v", cur)
	}
}

func TestDetectState_PartialStopSetup(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".aigate.yml"), []byte("claude:\n  session-start:\n    context: []\n"), 0644)

	writeSettings(t, filepath.Join(dir, ".claude", "settings.local.json"), map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "aigate claude stop"}}},
			},
		},
	})

	cur, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cur.StopHook {
		t.Error("expected StopHook to be true when the Stop hook is registered")
	}
	if cur.StopPrompt {
		t.Error("expected StopPrompt to be false when claude.stop-prompt is absent")
	}
}

func TestDetectState_ReportsCIContext(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".aigate.yml"), []byte("claude:\n  session-start:\n    context:\n      - type: ci-info\n"), 0644)

	cur, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cur.CIContext {
		t.Error("expected CIContext to be true when a ci-info entry is present")
	}
}

func TestDetectState_ReportsNoCIContext(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".aigate.yml"), []byte("claude:\n  session-start:\n    context: []\n"), 0644)

	cur, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cur.CIContext {
		t.Error("expected CIContext to be false when no ci-info entry is present")
	}
}

func TestDetectState_ReportsDockerContext(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".aigate.yml"), []byte("claude:\n  session-start:\n    context:\n      - type: docker-compose\n"), 0644)

	cur, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cur.DockerContext {
		t.Error("expected DockerContext to be true when a docker-compose entry is present")
	}
}

func TestDetectState_ReportsNoDockerContext(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".aigate.yml"), []byte("claude:\n  session-start:\n    context: []\n"), 0644)

	cur, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cur.DockerContext {
		t.Error("expected DockerContext to be false when no docker-compose entry is present")
	}
}
