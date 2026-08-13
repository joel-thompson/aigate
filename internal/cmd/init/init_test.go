package initcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/joelthompson/aigate/internal/config"
)

func TestRunInit_StopPromptWritesConfigSectionAndRegistersHook(t *testing.T) {
	t.Chdir(t.TempDir())

	prompter := func(string) bool { return true }

	var out bytes.Buffer
	if err := runInit(&out, prompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(config.DefaultConfigFile)
	if err != nil {
		t.Fatalf("loading generated config: %v", err)
	}
	if cfg.Claude == nil || cfg.Claude.StopPrompt == nil || !cfg.Claude.StopPrompt.Enabled {
		t.Fatal("expected claude.stop-prompt.enabled to be true")
	}

	data, err := os.ReadFile(filepath.Join(".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("reading settings.local.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.local.json: %v", err)
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("expected hooks object in settings.local.json")
	}
	if _, ok := hooks["SessionStart"]; !ok {
		t.Error("expected SessionStart hook to be registered")
	}
	if _, ok := hooks["Stop"]; !ok {
		t.Error("expected Stop hook to be registered")
	}
}

func TestRunInit_AsksAllQuestionsInFreshDir(t *testing.T) {
	t.Chdir(t.TempDir())

	var asked []string
	prompter := func(question string) bool {
		asked = append(asked, question)
		return true
	}

	var out bytes.Buffer
	if err := runInit(&out, prompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(asked) != 4 {
		t.Fatalf("expected 4 questions in a fresh directory, got %d: %v", len(asked), asked)
	}
}

func TestRunInit_SkipsConfigPromptWhenConfigExists(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.WriteFile(config.DefaultConfigFile, []byte("claude:\n"), 0644); err != nil {
		t.Fatalf("seeding config: %v", err)
	}

	var asked []string
	prompter := func(question string) bool {
		asked = append(asked, question)
		return true
	}

	var out bytes.Buffer
	if err := runInit(&out, prompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, q := range asked {
		if strings.Contains(q, "Create .aigate.yml") {
			t.Errorf("expected the config question not to be asked when .aigate.yml already exists, got: %v", asked)
		}
	}
	if !strings.Contains(out.String(), "already exists") {
		t.Errorf("expected the Config step to report its own already-exists reason, got:\n%s", out.String())
	}
}

func TestRunInit_SkipsSessionHookPromptWhenAlreadyRegistered(t *testing.T) {
	t.Chdir(t.TempDir())

	settingsPath, err := claudeSettingsPath()
	if err != nil {
		t.Fatalf("resolving settings path: %v", err)
	}
	if result := RegisterClaudeHooks(settingsPath); result.Err != nil {
		t.Fatalf("seeding session-start hook: %v", result.Err)
	}

	configPath, err := filepath.Abs(config.DefaultConfigFile)
	if err != nil {
		t.Fatalf("resolving config path: %v", err)
	}
	if result := ScaffoldConfig(configPath); result.Err != nil {
		t.Fatalf("seeding config: %v", result.Err)
	}
	if result := EnableSessionStart(configPath); result.Err != nil {
		t.Fatalf("seeding session-start section: %v", result.Err)
	}

	var asked []string
	prompter := func(question string) bool {
		asked = append(asked, question)
		return true
	}

	var out bytes.Buffer
	if err := runInit(&out, prompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, q := range asked {
		if strings.Contains(q, "session-start hook") {
			t.Errorf("expected the session-start question not to be asked when it's already registered, got: %v", asked)
		}
	}
}

func TestRunInit_AsksStopPromptQuestionWhenOnlyHalfDone(t *testing.T) {
	t.Chdir(t.TempDir())

	settingsPath, err := claudeSettingsPath()
	if err != nil {
		t.Fatalf("resolving settings path: %v", err)
	}
	if result := RegisterStopHook(settingsPath); result.Err != nil {
		t.Fatalf("seeding stop hook: %v", result.Err)
	}

	var asked []string
	prompter := func(question string) bool {
		asked = append(asked, question)
		return true
	}

	var out bytes.Buffer
	if err := runInit(&out, prompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, q := range asked {
		if strings.Contains(q, "stop hook for concise recaps") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the stop question to still be asked when only the hook half is done, got: %v", asked)
	}
}

func TestRunInit_SkipsGitHookPromptWhenAlreadyPresent(t *testing.T) {
	t.Chdir(t.TempDir())

	hookPath := filepath.Join(".git", "hooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(hookPath), 0755); err != nil {
		t.Fatalf("creating .git/hooks: %v", err)
	}
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\naigate git pre-commit\n"), 0755); err != nil {
		t.Fatalf("seeding pre-commit hook: %v", err)
	}

	configPath, err := filepath.Abs(config.DefaultConfigFile)
	if err != nil {
		t.Fatalf("resolving config path: %v", err)
	}
	if result := ScaffoldConfig(configPath); result.Err != nil {
		t.Fatalf("seeding config: %v", result.Err)
	}
	if result := EnablePreCommitChecks(configPath); result.Err != nil {
		t.Fatalf("seeding pre-commit section: %v", result.Err)
	}

	var asked []string
	prompter := func(question string) bool {
		asked = append(asked, question)
		return true
	}

	var out bytes.Buffer
	if err := runInit(&out, prompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, q := range asked {
		if strings.Contains(q, "git pre-commit hook") {
			t.Errorf("expected the git hook question not to be asked when it's already set up, got: %v", asked)
		}
	}
}

func TestRunInit_ReportsEverythingAlreadySetUp(t *testing.T) {
	t.Chdir(t.TempDir())

	// SetupGitHook only acts inside a real git repo, so give it a
	// .git/hooks dir to write into.
	if err := os.MkdirAll(filepath.Join(".git", "hooks"), 0755); err != nil {
		t.Fatalf("creating .git/hooks: %v", err)
	}

	// First run sets everything up.
	yesPrompter := func(string) bool { return true }
	var firstOut bytes.Buffer
	if err := runInit(&firstOut, yesPrompter, steps{}); err != nil {
		t.Fatalf("unexpected error on first run: %v", err)
	}

	// Second run should ask nothing and announce that everything is done.
	var asked []string
	prompter := func(question string) bool {
		asked = append(asked, question)
		return true
	}

	var out bytes.Buffer
	if err := runInit(&out, prompter, steps{}); err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}

	if len(asked) != 0 {
		t.Errorf("expected no questions on a fully set up project, got: %v", asked)
	}
	if !strings.Contains(out.String(), "Everything is already set up.") {
		t.Errorf("expected the all-done note, got:\n%s", out.String())
	}
}

func TestRunInit_StopPromptSkipsConfigEditWhenNoConfigFile(t *testing.T) {
	t.Chdir(t.TempDir())

	prompter := func(question string) bool {
		return !strings.Contains(question, "Create .aigate.yml")
	}

	var out bytes.Buffer
	if err := runInit(&out, prompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(config.DefaultConfigFile); !os.IsNotExist(err) {
		t.Fatal("expected no .aigate.yml to be created when the config step is declined")
	}

	if !strings.Contains(out.String(), "rerun with --config") {
		t.Errorf("expected the Stop prompt step to report the skip reason, got:\n%s", out.String())
	}
}

// TestRunInit_DecliningGitLeavesNoGitSectionInConfig is a regression test:
// declining the git-hook question used to still write git.pre-commit into
// .aigate.yml, because the config step wrote one static template containing
// every section regardless of the other answers.
func TestRunInit_DecliningGitLeavesNoGitSectionInConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	prompter := func(question string) bool {
		return !strings.Contains(question, "git pre-commit hook")
	}

	var out bytes.Buffer
	if err := runInit(&out, prompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(config.DefaultConfigFile)
	if err != nil {
		t.Fatalf("loading generated config: %v", err)
	}
	if cfg.Git != nil {
		t.Errorf("expected no git: section when the git-hook question is declined, got: %+v", cfg.Git)
	}
	if !strings.Contains(out.String(), "Pre-commit checks: skipped (not requested)") {
		t.Errorf("expected the Pre-commit checks step to report it was skipped, got:\n%s", out.String())
	}
}

// TestRunInit_DecliningSessionStartLeavesNoSessionStartSectionInConfig is the
// symmetric case: declining the session-start question must not leave
// claude.session-start in .aigate.yml either.
func TestRunInit_DecliningSessionStartLeavesNoSessionStartSectionInConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	prompter := func(question string) bool {
		return !strings.Contains(question, "session-start hook")
	}

	var out bytes.Buffer
	if err := runInit(&out, prompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(config.DefaultConfigFile)
	if err != nil {
		t.Fatalf("loading generated config: %v", err)
	}
	if cfg.Claude != nil && cfg.Claude.SessionStart != nil {
		t.Error("expected no claude.session-start section when the session-start question is declined")
	}
	if !strings.Contains(out.String(), "Session context: skipped (not requested)") {
		t.Errorf("expected the Session context step to report it was skipped, got:\n%s", out.String())
	}
}

// TestRunInit_AllYesProducesTodaysTemplateByteForByte locks the section
// ordering and spacing the per-step splices must reproduce: an all-yes run
// should still generate the exact same .aigate.yml as the old single-template
// approach did.
func TestRunInit_AllYesProducesTodaysTemplateByteForByte(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.MkdirAll(filepath.Join(".git", "hooks"), 0755); err != nil {
		t.Fatalf("creating .git/hooks: %v", err)
	}

	yesPrompter := func(string) bool { return true }
	var out bytes.Buffer
	if err := runInit(&out, yesPrompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(config.DefaultConfigFile)
	if err != nil {
		t.Fatalf("reading generated config: %v", err)
	}

	want := fmt.Sprintf(`# aigate configuration
# See: https://github.com/joelthompson/aigate

claude:
  session-start:
    context:
      - type: project-structure
        max_depth: 3
      # - type: shell
      #   command: "go-task --list"
      #   label: "Available tasks"
  stop-prompt:
    enabled: true
    # Fed back to Claude when it finishes responding.
    prompt: %s
    # Skip the recap when the reply is already shorter than this; 0 disables.
    min-length: %d

git:
  pre-commit:
    checks:
      - type: secrets-scan
`, strconv.Quote(config.DefaultStopPrompt), config.DefaultStopMinLength)

	if string(data) != want {
		t.Errorf("generated config mismatch:\ngot:\n%s\nwant:\n%s", data, want)
	}
}

// TestRunInit_WithCircleCIAddsCIContextStep locks in the wizard's CircleCI
// detection: when .circleci/config.yml exists, an all-yes run must add
// "- type: ci-info" to claude.session-start.context in the right place and
// report the new step, on top of everything
// TestRunInit_AllYesProducesTodaysTemplateByteForByte already pins.
func TestRunInit_WithCircleCIAddsCIContextStep(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.MkdirAll(filepath.Join(".git", "hooks"), 0755); err != nil {
		t.Fatalf("creating .git/hooks: %v", err)
	}
	if err := os.MkdirAll(".circleci", 0755); err != nil {
		t.Fatalf("creating .circleci: %v", err)
	}
	if err := os.WriteFile(filepath.Join(".circleci", "config.yml"), []byte("version: 2.1\njobs:\n  build: {}\nworkflows:\n  main:\n    jobs: [build]\n"), 0644); err != nil {
		t.Fatalf("seeding .circleci/config.yml: %v", err)
	}

	yesPrompter := func(string) bool { return true }
	var out bytes.Buffer
	if err := runInit(&out, yesPrompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.String(), "CI context:") {
		t.Errorf("expected a CI context step to be reported, got:\n%s", out.String())
	}

	data, err := os.ReadFile(config.DefaultConfigFile)
	if err != nil {
		t.Fatalf("reading generated config: %v", err)
	}

	want := fmt.Sprintf(`# aigate configuration
# See: https://github.com/joelthompson/aigate

claude:
  session-start:
    context:
      - type: project-structure
        max_depth: 3
      # - type: shell
      #   command: "go-task --list"
      #   label: "Available tasks"
      - type: ci-info
  stop-prompt:
    enabled: true
    # Fed back to Claude when it finishes responding.
    prompt: %s
    # Skip the recap when the reply is already shorter than this; 0 disables.
    min-length: %d

git:
  pre-commit:
    checks:
      - type: secrets-scan
`, strconv.Quote(config.DefaultStopPrompt), config.DefaultStopMinLength)

	if string(data) != want {
		t.Errorf("generated config mismatch:\ngot:\n%s\nwant:\n%s", data, want)
	}
}

// TestRunInit_WithoutCircleCIPrintsNoCIContextLine is the symmetric case:
// TestRunInit_AllYesProducesTodaysTemplateByteForByte already proves the
// generated config is untouched when there's no .circleci/, but the wizard
// output itself must also omit the "CI context" line entirely rather than
// reporting it as skipped.
func TestRunInit_WithoutCircleCIPrintsNoCIContextLine(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.MkdirAll(filepath.Join(".git", "hooks"), 0755); err != nil {
		t.Fatalf("creating .git/hooks: %v", err)
	}

	yesPrompter := func(string) bool { return true }
	var out bytes.Buffer
	if err := runInit(&out, yesPrompter, steps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out.String(), "CI context") {
		t.Errorf("expected no CI context line when .circleci is absent, got:\n%s", out.String())
	}
}

// TestRunInit_RerunWithCircleCISkipsCIContext exercises the wizard end to
// end across two runs: the first adds ci-info, the second must report it
// already present rather than asking again.
func TestRunInit_RerunWithCircleCISkipsCIContext(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.MkdirAll(filepath.Join(".git", "hooks"), 0755); err != nil {
		t.Fatalf("creating .git/hooks: %v", err)
	}
	if err := os.MkdirAll(".circleci", 0755); err != nil {
		t.Fatalf("creating .circleci: %v", err)
	}
	if err := os.WriteFile(filepath.Join(".circleci", "config.yml"), []byte("version: 2.1\njobs:\n  build: {}\nworkflows:\n  main:\n    jobs: [build]\n"), 0644); err != nil {
		t.Fatalf("seeding .circleci/config.yml: %v", err)
	}

	yesPrompter := func(string) bool { return true }
	var firstOut bytes.Buffer
	if err := runInit(&firstOut, yesPrompter, steps{}); err != nil {
		t.Fatalf("unexpected error on first run: %v", err)
	}

	var asked []string
	prompter := func(question string) bool {
		asked = append(asked, question)
		return true
	}
	var secondOut bytes.Buffer
	if err := runInit(&secondOut, prompter, steps{}); err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}

	for _, q := range asked {
		if strings.Contains(q, "CircleCI info") {
			t.Errorf("expected the CI context question not to be asked on rerun, got: %v", asked)
		}
	}
	if !strings.Contains(secondOut.String(), "ci-info already in") {
		t.Errorf("expected the CI context step to report it was already present, got:\n%s", secondOut.String())
	}
}

// TestRunInit_AcceptingGitOnRerunAddsBothHookAndConfig closes the gap the
// section-per-step design exists to avoid: declining git now and accepting
// it later must add both the hook script and the git.pre-commit config, not
// just one of the two.
func TestRunInit_AcceptingGitOnRerunAddsBothHookAndConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.MkdirAll(filepath.Join(".git", "hooks"), 0755); err != nil {
		t.Fatalf("creating .git/hooks: %v", err)
	}

	declineGit := func(question string) bool {
		return !strings.Contains(question, "git pre-commit hook")
	}
	var firstOut bytes.Buffer
	if err := runInit(&firstOut, declineGit, steps{}); err != nil {
		t.Fatalf("unexpected error on first run: %v", err)
	}

	cfg, err := config.Load(config.DefaultConfigFile)
	if err != nil {
		t.Fatalf("loading config after first run: %v", err)
	}
	if cfg.Git != nil {
		t.Fatal("expected no git: section after declining the git step")
	}

	var asked []string
	acceptAll := func(question string) bool {
		asked = append(asked, question)
		return true
	}
	var secondOut bytes.Buffer
	if err := runInit(&secondOut, acceptAll, steps{}); err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}

	found := false
	for _, q := range asked {
		if strings.Contains(q, "git pre-commit hook") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the git hook question to still be asked on the second run, got: %v", asked)
	}

	hookData, err := os.ReadFile(filepath.Join(".git", "hooks", "pre-commit"))
	if err != nil {
		t.Fatalf("reading pre-commit hook: %v", err)
	}
	if !strings.Contains(string(hookData), "aigate") {
		t.Error("expected the pre-commit hook to invoke aigate")
	}

	cfg, err = config.Load(config.DefaultConfigFile)
	if err != nil {
		t.Fatalf("loading config after second run: %v", err)
	}
	if cfg.Git == nil || cfg.Git.PreCommit == nil {
		t.Error("expected git.pre-commit to be present after accepting the git step on the second run")
	}
}
