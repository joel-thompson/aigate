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

const configHeader = `# aigate configuration
# See: https://github.com/joelthompson/aigate
`

func ScaffoldConfig(path string) StepResult {
	if configInstalled(path) {
		return StepResult{Skipped: true, Reason: fmt.Sprintf("%s already exists", filepath.Base(path))}
	}

	if err := os.WriteFile(path, []byte(configHeader), 0644); err != nil {
		return StepResult{Err: fmt.Errorf("writing %s: %w", filepath.Base(path), err)}
	}

	return StepResult{Action: fmt.Sprintf("created %s", filepath.Base(path))}
}

func RegisterClaudeHooks(settingsPath string) StepResult {
	return registerHook(settingsPath, "SessionStart", "aigate claude session-start")
}

// RegisterStopHook registers the Stop hook that asks Claude for a concise
// recap before it finishes responding.
func RegisterStopHook(settingsPath string) StepResult {
	return registerHook(settingsPath, "Stop", "aigate claude stop")
}

func registerHook(settingsPath, event, command string) StepResult {
	settings, err := readOrCreateSettings(settingsPath)
	if err != nil {
		return StepResult{Err: fmt.Errorf("reading settings: %w", err)}
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}

	entries, _ := hooks[event].([]any)

	if containsCommand(entries, command) {
		return StepResult{Skipped: true, Reason: fmt.Sprintf("aigate %s hook already registered in %s", event, filepath.Base(settingsPath))}
	}

	entries = append(entries, map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": command,
			},
		},
	})
	hooks[event] = entries

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

	return StepResult{Action: fmt.Sprintf("registered %s hook in %s", event, filepath.Base(settingsPath))}
}

// containsCommand reports whether entries (a hook event's array in the
// Claude settings file) already contains an entry running command. It walks
// both the nested hooks-array schema and the flat command field for
// backwards compat with hand-written entries.
func containsCommand(entries []any, command string) bool {
	for _, group := range entries {
		groupMap, ok := group.(map[string]any)
		if !ok {
			continue
		}
		if hooksList, ok := groupMap["hooks"].([]any); ok {
			for _, h := range hooksList {
				if hMap, ok := h.(map[string]any); ok {
					if cmd, _ := hMap["command"].(string); cmd == command {
						return true
					}
				}
			}
		}
		if cmd, _ := groupMap["command"].(string); cmd == command {
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

// hookHasAigate reports whether a pre-commit hook's content already invokes
// aigate. Shared by SetupGitHook and gitHookInstalled (detect.go) so the two
// can't drift on what counts as "already set up".
func hookHasAigate(content string) bool {
	return strings.Contains(content, "aigate")
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
		if hookHasAigate(string(existing)) {
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
