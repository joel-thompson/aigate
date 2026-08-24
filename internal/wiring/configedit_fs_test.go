package wiring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joelthompson/aigate/internal/config"
)

func TestEnableStopPrompt_WritesMergedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")
	if err := os.WriteFile(path, []byte("git:\n  pre-commit:\n    checks:\n      - type: secrets-scan\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := EnableStopPrompt(path)

	if result.Skipped || result.Err != nil {
		t.Fatalf("expected section to be added, got: %s", result)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("merged file failed to parse: %v", err)
	}
	if cfg.Claude == nil || cfg.Claude.StopPrompt == nil || !cfg.Claude.StopPrompt.Enabled {
		t.Fatal("expected claude.stop-prompt.enabled to be true")
	}
}

func TestEnableStopPrompt_SkipsWhenConfigMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")

	result := EnableStopPrompt(path)

	if !result.Skipped {
		t.Fatal("expected skip when config file is missing")
	}
	if result.Reason == "" {
		t.Error("expected a skip reason")
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected no file to be created")
	}
}

func TestEnableStopPrompt_SkipsWhenAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")
	original := "claude:\n  stop-prompt:\n    enabled: true\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	result := EnableStopPrompt(path)

	if !result.Skipped {
		t.Fatal("expected skip when claude.stop-prompt is already present")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Error("expected file to be left untouched")
	}
}

func TestEnableStopPrompt_LeavesFileUntouchedOnParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")
	original := "claude:\n  sesion-start:\n    context: []\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	result := EnableStopPrompt(path)

	if result.Err == nil {
		t.Fatal("expected an error for unparseable config")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Error("expected file to be left untouched on parse error")
	}
}

func TestEnableCIContext_WritesMergedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")
	before := "claude:\n  session-start:\n    context:\n      - type: project-structure\n"
	if err := os.WriteFile(path, []byte(before), 0644); err != nil {
		t.Fatal(err)
	}

	result := EnableCIContext(path)

	if result.Skipped || result.Err != nil {
		t.Fatalf("expected ci-info to be added, got: %s", result)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("merged file failed to parse: %v", err)
	}
	found := false
	for _, p := range cfg.Claude.SessionStart.Context {
		if p.Type == "ci-info" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected ci-info to be present in claude.session-start.context")
	}
}

func TestEnableCIContext_SkipsWhenAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")
	original := "claude:\n  session-start:\n    context:\n      - type: ci-info\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	result := EnableCIContext(path)

	if !result.Skipped {
		t.Fatal("expected skip when ci-info is already present")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Error("expected file to be left untouched")
	}
}

func TestEnableCIContext_SkipsWhenConfigMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")

	result := EnableCIContext(path)

	if !result.Skipped {
		t.Fatal("expected skip when config file is missing")
	}
	if result.Reason == "" {
		t.Error("expected a skip reason")
	}
}

func TestEnableCIContext_TranslatesNoContextListToSkip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")
	original := "claude:\n  stop-prompt:\n    enabled: true\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	result := EnableCIContext(path)

	if result.Err != nil {
		t.Fatalf("expected the missing-context-list error translated to a skip, got error: %v", result.Err)
	}
	if !result.Skipped {
		t.Fatal("expected skip when claude.session-start.context isn't a list")
	}
	if result.Reason == "" {
		t.Error("expected a skip reason")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Error("expected file to be left untouched")
	}
}

func TestEnableCIContext_PreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")
	before := "claude:\n  session-start:\n    context:\n      - type: project-structure\n"
	if err := os.WriteFile(path, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}

	result := EnableCIContext(path)
	if result.Skipped || result.Err != nil {
		t.Fatalf("expected ci-info to be added, got: %s", result)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected file mode 0600 to be preserved, got %v", info.Mode().Perm())
	}
}

func TestEnableStopPrompt_PreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")
	if err := os.WriteFile(path, []byte("git:\n  pre-commit:\n    checks:\n      - type: secrets-scan\n"), 0600); err != nil {
		t.Fatal(err)
	}

	result := EnableStopPrompt(path)
	if result.Skipped || result.Err != nil {
		t.Fatalf("expected section to be added, got: %s", result)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected file mode 0600 to be preserved, got %v", info.Mode().Perm())
	}
}
