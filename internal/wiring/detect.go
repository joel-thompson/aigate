package wiring

import (
	"os"
	"path/filepath"

	"github.com/joelthompson/aigate/internal/config"
)

// paths holds the absolute locations of the artifacts aigate installs into a
// project, resolved once from a root so init's writes and doctor's reads
// can't drift onto two different notions of "the config file" or "the
// settings file".
type paths struct {
	root     string
	config   string
	settings string
	gitHook  string
}

// pathsFor resolves root to an absolute path and joins it with each
// artifact's project-relative location.
func pathsFor(root string) (paths, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return paths{}, err
	}
	return paths{
		root:     abs,
		config:   filepath.Join(abs, config.DefaultConfigFile),
		settings: filepath.Join(abs, ".claude", "settings.local.json"),
		gitHook:  filepath.Join(abs, ".git", "hooks", "pre-commit"),
	}, nil
}

// SettingsPath returns the absolute path to the project-local Claude Code
// settings file aigate registers hooks in, resolved from root.
// settings.local.json is the per-developer file, so hook registration stays
// out of git.
func SettingsPath(root string) (string, error) {
	p, err := pathsFor(root)
	if err != nil {
		return "", err
	}
	return p.settings, nil
}

// state records which init artifacts already exist in the project, so
// runInit can skip prompting for anything already set up. Fields are
// exported so internal/cmd/init can read them by name; the type itself stays
// unexported since only Detect constructs one.
type state struct {
	Config          bool
	SessionHook     bool
	SessionContext  bool
	StopHook        bool
	StopPrompt      bool
	GitHook         bool
	PreCommitChecks bool
	CIContext       bool
}

// detectStateAt inspects the project at p for already-configured aigate
// artifacts, resolving the same paths the init steps write to.
func detectStateAt(p paths) state {
	return state{
		Config:          configInstalled(p.config),
		SessionHook:     hookInstalled(p.settings, sessionStartEvent, sessionStartCommand),
		SessionContext:  sessionContextInstalled(p.config),
		StopHook:        hookInstalled(p.settings, stopEvent, stopCommand),
		StopPrompt:      stopPromptInstalled(p.config),
		GitHook:         gitHookInstalled(p.gitHook),
		PreCommitChecks: preCommitChecksInstalled(p.config),
		CIContext:       ciContextInstalled(p.config),
	}
}

// Detect inspects root for already-configured aigate artifacts.
func Detect(root string) (state, error) {
	p, err := pathsFor(root)
	if err != nil {
		return state{}, err
	}
	return detectStateAt(p), nil
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
// "already present". Delegates to loadConfigStatus (report.go) so this bool
// answer and doctor's parse-error detail come from the same read.
func configSectionInstalled(configPath string, present func(*config.Config) bool) bool {
	st := loadConfigStatus(configPath)
	if st.cfg == nil {
		return false
	}
	return present(st.cfg)
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
