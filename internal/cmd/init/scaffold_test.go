package initcmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	initcmd "github.com/joelthompson/aigate/internal/cmd/init"
	"github.com/joelthompson/aigate/internal/config"
)

func TestScaffoldConfig_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")

	result := initcmd.ScaffoldConfig(path)

	if result.Skipped || result.Err != nil {
		t.Fatalf("expected file to be created, got: %s", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading created file: %v", err)
	}
	content := string(data)

	if len(content) == 0 {
		t.Fatal("expected non-empty config file")
	}
	if !strings.Contains(content, "project-structure") {
		t.Error("expected config to mention project-structure")
	}
	if !strings.Contains(content, "shell") {
		t.Error("expected config to mention shell provider")
	}
	if !strings.Contains(content, "secrets-scan") {
		t.Error("expected config to mention secrets-scan")
	}
}

func TestScaffoldConfig_SkipsIfExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")

	os.WriteFile(path, []byte("existing: true\n"), 0644)

	result := initcmd.ScaffoldConfig(path)

	if !result.Skipped {
		t.Fatal("expected skip when file exists")
	}
	if result.Reason == "" {
		t.Error("expected skip reason")
	}

	data, _ := os.ReadFile(path)
	if string(data) != "existing: true\n" {
		t.Error("existing file should not be modified")
	}
}

func TestScaffoldConfig_DefaultConfigIsValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".aigate.yml")

	initcmd.ScaffoldConfig(path)

	_, err := config.Load(path)
	if err != nil {
		t.Fatalf("default config template failed validation: %v", err)
	}
}

func TestRegisterClaudeHooks_CreatesNewSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	result := initcmd.RegisterClaudeHooks(path)

	if result.Skipped || result.Err != nil {
		t.Fatalf("expected hooks to be registered, got: %s", result)
	}

	assertSettingsHasAigateHook(t, path)
}

func TestRegisterClaudeHooks_AppendsToExistingSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []string{"Read"},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(path, data, 0644)

	result := initcmd.RegisterClaudeHooks(path)

	if result.Skipped || result.Err != nil {
		t.Fatalf("expected hooks to be registered, got: %s", result)
	}

	data, _ = os.ReadFile(path)
	var settings map[string]any
	json.Unmarshal(data, &settings)

	if _, ok := settings["permissions"]; !ok {
		t.Error("existing permissions key should be preserved")
	}

	assertSettingsHasAigateHook(t, path)
}

func TestRegisterClaudeHooks_AppendsToExistingHooksArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	existing := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "echo hello",
						},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(path, data, 0644)

	result := initcmd.RegisterClaudeHooks(path)

	if result.Skipped || result.Err != nil {
		t.Fatalf("expected hooks to be appended, got: %s", result)
	}

	data, _ = os.ReadFile(path)
	var settings map[string]any
	json.Unmarshal(data, &settings)

	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 2 {
		t.Fatalf("expected 2 hook group entries, got %d", len(sessionStart))
	}
}

func TestRegisterClaudeHooks_SkipsIfAigateAlreadyRegistered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	existing := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "aigate claude session-start",
						},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(path, data, 0644)

	result := initcmd.RegisterClaudeHooks(path)

	if !result.Skipped {
		t.Fatal("expected skip when aigate already registered")
	}
}

func TestSetupGitHook_CreatesNewHook(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	os.MkdirAll(hooksDir, 0755)
	path := filepath.Join(hooksDir, "pre-commit")

	result := initcmd.SetupGitHook(path)

	if result.Skipped || result.Err != nil {
		t.Fatalf("expected hook to be created, got: %s", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading hook: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "#!/") {
		t.Error("expected shebang line")
	}
	if !strings.Contains(content, "aigate git pre-commit") {
		t.Error("expected aigate call in hook")
	}

	info, _ := os.Stat(path)
	if info.Mode()&0111 == 0 {
		t.Error("expected hook to be executable")
	}
}

func TestSetupGitHook_SkipsIfAigateAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	os.MkdirAll(hooksDir, 0755)
	path := filepath.Join(hooksDir, "pre-commit")

	existing := "#!/bin/sh\naigate git pre-commit\n"
	os.WriteFile(path, []byte(existing), 0755)

	result := initcmd.SetupGitHook(path)

	if !result.Skipped {
		t.Fatal("expected skip when aigate already in hook")
	}

	data, _ := os.ReadFile(path)
	if string(data) != existing {
		t.Error("file should not be modified")
	}
}

func TestSetupGitHook_AppendsToExistingHook(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	os.MkdirAll(hooksDir, 0755)
	path := filepath.Join(hooksDir, "pre-commit")

	existing := "#!/bin/sh\nsome-other-tool check\n"
	os.WriteFile(path, []byte(existing), 0755)

	result := initcmd.SetupGitHook(path)

	if result.Skipped || result.Err != nil {
		t.Fatalf("expected aigate to be appended, got: %s", result)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "some-other-tool check") {
		t.Error("existing content should be preserved")
	}
	if !strings.Contains(content, "aigate git pre-commit") {
		t.Error("aigate call should be appended")
	}
}

func TestSetupGitHook_NoGitDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".git", "hooks", "pre-commit")

	result := initcmd.SetupGitHook(path)

	if !result.Skipped {
		t.Fatal("expected skip when .git/hooks dir doesn't exist")
	}
}

func TestSetupGitHook_ReadErrorIsNotSwallowed(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	os.MkdirAll(hooksDir, 0755)
	path := filepath.Join(hooksDir, "pre-commit")

	// Create unreadable file
	os.WriteFile(path, []byte("#!/bin/sh\n"), 0000)

	result := initcmd.SetupGitHook(path)

	if result.Err == nil {
		t.Fatal("expected error for unreadable hook file, got none")
	}
}

// assertSettingsHasAigateHook verifies settings.json contains the aigate
// SessionStart hook in the correct nested schema.
func assertSettingsHasAigateHook(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings JSON: %v", err)
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("expected hooks object")
	}
	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok {
		t.Fatal("expected SessionStart array")
	}

	for _, group := range sessionStart {
		groupMap, ok := group.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := groupMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range hooksList {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if hMap["type"] == "command" && hMap["command"] == "aigate claude session-start" {
				return
			}
		}
	}
	t.Fatal("aigate hook entry not found in SessionStart with correct nested schema")
}
