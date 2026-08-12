package initcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type StepResult struct {
	Action  string
	Skipped bool
	Reason  string
	Err     error
}

func (r StepResult) String() string {
	if r.Err != nil {
		return fmt.Sprintf("error: %v", r.Err)
	}
	if r.Skipped {
		return fmt.Sprintf("skipped (%s)", r.Reason)
	}
	return r.Action
}

const defaultConfig = `# aigate configuration
# See: https://github.com/joelthompson/aigate

claude:
  session-start:
    context:
      - type: project-structure
        max_depth: 3
      # - type: shell
      #   command: "go-task --list"
      #   label: "Available tasks"

git:
  pre-commit:
    checks:
      - type: secrets-scan
`

func ScaffoldConfig(path string) StepResult {
	if _, err := os.Stat(path); err == nil {
		return StepResult{Skipped: true, Reason: fmt.Sprintf("%s already exists", filepath.Base(path))}
	}

	if err := os.WriteFile(path, []byte(defaultConfig), 0644); err != nil {
		return StepResult{Err: fmt.Errorf("writing %s: %w", filepath.Base(path), err)}
	}

	return StepResult{Action: fmt.Sprintf("created %s", filepath.Base(path))}
}

func RegisterClaudeHooks(settingsPath string) StepResult {
	settings, err := readOrCreateSettings(settingsPath)
	if err != nil {
		return StepResult{Err: fmt.Errorf("reading settings: %w", err)}
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}

	sessionStart, _ := hooks["SessionStart"].([]any)

	if containsAigateHook(sessionStart) {
		return StepResult{Skipped: true, Reason: "aigate hook already registered in settings.json"}
	}

	sessionStart = append(sessionStart, map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": "aigate claude session-start",
			},
		},
	})
	hooks["SessionStart"] = sessionStart

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return StepResult{Err: fmt.Errorf("creating directory: %w", err)}
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return StepResult{Err: fmt.Errorf("marshaling settings: %w", err)}
	}
	data = append(data, '\n')

	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return StepResult{Err: fmt.Errorf("writing settings: %w", err)}
	}

	return StepResult{Action: "registered SessionStart hook in settings.json"}
}

func containsAigateHook(sessionStart []any) bool {
	for _, group := range sessionStart {
		groupMap, ok := group.(map[string]any)
		if !ok {
			continue
		}
		// Check nested hooks array (correct schema)
		if hooksList, ok := groupMap["hooks"].([]any); ok {
			for _, h := range hooksList {
				if hMap, ok := h.(map[string]any); ok {
					if cmd, _ := hMap["command"].(string); strings.Contains(cmd, "aigate") {
						return true
					}
				}
			}
		}
		// Also check flat command field for backwards compat with any manually-written entries
		if cmd, _ := groupMap["command"].(string); strings.Contains(cmd, "aigate") {
			return true
		}
	}
	return false
}

func readOrCreateSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}
	return settings, nil
}

func SetupGitHook(hookPath string) StepResult {
	dir := filepath.Dir(hookPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return StepResult{Skipped: true, Reason: ".git/hooks directory not found (not a git repo?)"}
	}

	existing, err := os.ReadFile(hookPath)
	if err != nil && !os.IsNotExist(err) {
		return StepResult{Err: fmt.Errorf("reading existing hook: %w", err)}
	}

	if err == nil {
		if strings.Contains(string(existing), "aigate") {
			return StepResult{Skipped: true, Reason: "aigate already in pre-commit hook"}
		}
		content := string(existing)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n# aigate pre-commit check\naigate git pre-commit\n"
		if err := os.WriteFile(hookPath, []byte(content), 0755); err != nil {
			return StepResult{Err: fmt.Errorf("appending to hook: %w", err)}
		}
		return StepResult{Action: "appended aigate to existing pre-commit hook"}
	}

	hookScript := "#!/bin/sh\nset -e\n\naigate git pre-commit\n"
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		return StepResult{Err: fmt.Errorf("writing hook: %w", err)}
	}
	return StepResult{Action: "created .git/hooks/pre-commit"}
}
