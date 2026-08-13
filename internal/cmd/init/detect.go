package initcmd

import (
	"os"
	"path/filepath"

	"github.com/joelthompson/aigate/internal/config"
)

// state records which init artifacts already exist in the project, so
// runInit can skip prompting for anything already set up.
type state struct {
	config          bool
	sessionHook     bool
	sessionContext  bool
	stopHook        bool
	stopPrompt      bool
	gitHook         bool
	preCommitChecks bool
	ciContext       bool
}

// detectState inspects the project for already-configured aigate artifacts,
// resolving the same paths the init steps write to.
func detectState() (state, error) {
	configPath, err := filepath.Abs(config.DefaultConfigFile)
	if err != nil {
		return state{}, err
	}
	settingsPath, err := claudeSettingsPath()
	if err != nil {
		return state{}, err
	}
	gitHookPath, err := filepath.Abs(filepath.Join(".git", "hooks", "pre-commit"))
	if err != nil {
		return state{}, err
	}

	return state{
		config:          configInstalled(configPath),
		sessionHook:     hookInstalled(settingsPath, "SessionStart", "aigate claude session-start"),
		sessionContext:  sessionContextInstalled(configPath),
		stopHook:        hookInstalled(settingsPath, "Stop", "aigate claude stop"),
		stopPrompt:      stopPromptInstalled(configPath),
		gitHook:         gitHookInstalled(gitHookPath),
		preCommitChecks: preCommitChecksInstalled(configPath),
		ciContext:       ciContextInstalled(configPath),
	}, nil
}

// configInstalled reports whether a config file already exists at path.
func configInstalled(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// hookInstalled reports whether command is already registered for event in
// the Claude settings file at settingsPath. It reuses the same settings
// reader and duplicate-detection walk as registerHook (scaffold.go), so
// detection can never drift from what actually counts as "already
// registered".
func hookInstalled(settingsPath, event, command string) bool {
	settings, err := readOrCreateSettings(settingsPath)
	if err != nil {
		return false
	}
	hooks, _ := settings["hooks"].(map[string]any)
	entries, _ := hooks[event].([]any)
	return containsCommand(entries, command)
}

// configSectionInstalled reports whether the config file at configPath
// parses and present reports its section as already there. Any read or
// parse error counts as not installed. Shared by sessionContextInstalled,
// stopPromptInstalled, and preCommitChecksInstalled so detection can never
// drift from what the matching merge* func (configedit.go) considers
// "already present".
func configSectionInstalled(configPath string, present func(*config.Config) bool) bool {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}
	cfg, err := config.Parse(data)
	if err != nil {
		return false
	}
	return present(cfg)
}

// sessionContextInstalled reports whether .aigate.yml already has a
// claude.session-start section. This mirrors the check in mergeSessionStart
// (configedit.go).
func sessionContextInstalled(configPath string) bool {
	return configSectionInstalled(configPath, sessionStartPresent)
}

// stopPromptInstalled reports whether .aigate.yml already has a
// claude.stop-prompt section. This mirrors the check in mergeStopPrompt
// (configedit.go).
func stopPromptInstalled(configPath string) bool {
	return configSectionInstalled(configPath, stopPromptPresent)
}

// preCommitChecksInstalled reports whether .aigate.yml already has a
// git.pre-commit section. This mirrors the check in mergePreCommit
// (configedit.go).
func preCommitChecksInstalled(configPath string) bool {
	return configSectionInstalled(configPath, preCommitPresent)
}

// ciContextInstalled reports whether .aigate.yml's
// claude.session-start.context already has a ci-info entry. This mirrors
// the check in mergeCIContext (configedit.go).
func ciContextInstalled(configPath string) bool {
	return configSectionInstalled(configPath, ciContextPresent)
}

// gitHookInstalled reports whether the pre-commit hook at hookPath already
// invokes aigate.
func gitHookInstalled(hookPath string) bool {
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return false
	}
	return hookHasAigate(string(data))
}
