package wiring

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// setupFullyWired writes a project at dir that Inspect should report as
// entirely ok (no .circleci and no compose file, so the CI-context and
// Docker-context checks report not applicable rather than a problem).
func setupFullyWired(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755); err != nil {
		t.Fatalf("creating .git/hooks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "hooks", "pre-commit"), []byte("#!/bin/sh\naigate git pre-commit\n"), 0755); err != nil {
		t.Fatalf("seeding pre-commit hook: %v", err)
	}

	config := `claude:
  session-start:
    context:
      - type: project-structure
  stop-prompt:
    enabled: true
git:
  pre-commit:
    checks:
      - type: secrets-scan
`
	if err := os.WriteFile(filepath.Join(dir, ".aigate.yml"), []byte(config), 0644); err != nil {
		t.Fatalf("seeding .aigate.yml: %v", err)
	}

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
}

func fakeLookPath(path string, err error) func(string) (string, error) {
	return func(string) (string, error) { return path, err }
}

func findCheck(rep Report, label string) (Check, bool) {
	for _, c := range rep.Checks {
		if c.Label == label {
			return c, true
		}
	}
	return Check{}, false
}

func TestInspect_ReportsOnEveryStateField(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	seen := map[string]int{}
	for _, c := range rep.Checks {
		for _, k := range c.Keys {
			seen[k]++
		}
	}

	typ := reflect.TypeFor[state]()
	if len(seen) != typ.NumField() {
		t.Fatalf("expected %d distinct covered keys, got %d: %v", typ.NumField(), len(seen), seen)
	}
	for field := range typ.Fields() {
		if seen[field.Name] != 1 {
			t.Errorf("expected state field %q to be covered by exactly one check, got %d", field.Name, seen[field.Name])
		}
	}
}

func TestInspect_AllOKWhenFullyWired(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !rep.OK() {
		t.Fatalf("expected a fully wired project to report OK, got problems: %+v", rep.Problems())
	}
	for _, c := range rep.Checks {
		if c.Status == StatusProblem {
			t.Errorf("unexpected problem check: %+v", c)
		}
	}
}

func TestInspect_FlagsExternalWithoutSection(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	// Remove the claude.stop-prompt section, leaving the Stop hook registered.
	if err := os.WriteFile(filepath.Join(dir, ".aigate.yml"), []byte(`claude:
  session-start:
    context:
      - type: project-structure
git:
  pre-commit:
    checks:
      - type: secrets-scan
`), 0644); err != nil {
		t.Fatal(err)
	}

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c, ok := findCheck(rep, "Stop prompt")
	if !ok {
		t.Fatal("expected a Stop prompt check")
	}
	if c.Status != StatusProblem {
		t.Fatalf("expected Stop prompt to be a problem, got %v", c.Status)
	}
	if !strings.Contains(c.Detail, "Stop hook") || !strings.Contains(c.Detail, "claude.stop-prompt") {
		t.Errorf("expected detail to name both halves, got: %s", c.Detail)
	}
	if c.Remedy != "aigate init --stop-prompt" {
		t.Errorf("expected remedy aigate init --stop-prompt, got %q", c.Remedy)
	}
	if !strings.HasSuffix(c.Path, ".aigate.yml") {
		t.Errorf("expected path to point at the missing half (.aigate.yml), got %q", c.Path)
	}
}

func TestInspect_FlagsSectionWithoutExternal(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	// Remove the SessionStart hook, leaving claude.session-start configured.
	writeSettings(t, filepath.Join(dir, ".claude", "settings.local.json"), map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "aigate claude stop"}}},
			},
		},
	})

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c, ok := findCheck(rep, "Session context")
	if !ok {
		t.Fatal("expected a Session context check")
	}
	if c.Status != StatusProblem {
		t.Fatalf("expected Session context to be a problem, got %v", c.Status)
	}
	if !strings.Contains(c.Detail, "SessionStart hook") || !strings.Contains(c.Detail, "claude.session-start") {
		t.Errorf("expected detail to name both halves, got: %s", c.Detail)
	}
	if !strings.HasSuffix(c.Path, "settings.local.json") {
		t.Errorf("expected path to point at the missing half (settings.local.json), got %q", c.Path)
	}
}

func TestInspect_NeitherHalfIsNotConfiguredNotAProblem(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".aigate.yml"), []byte("# aigate configuration\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c, ok := findCheck(rep, "Pre-commit checks")
	if !ok {
		t.Fatal("expected a Pre-commit checks check")
	}
	if c.Status != StatusNotConfigured {
		t.Fatalf("expected Pre-commit checks to be not configured, got %v", c.Status)
	}
	for _, p := range rep.Problems() {
		if p.Label == "Pre-commit checks" {
			t.Error("expected a not-configured check to be excluded from Problems()")
		}
	}
}

func TestInspect_ReportsProblemWhenConfigMissing(t *testing.T) {
	dir := t.TempDir()

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c, ok := findCheck(rep, "Config")
	if !ok {
		t.Fatal("expected a Config check")
	}
	if c.Status != StatusProblem {
		t.Fatalf("expected missing config to be a problem, got %v", c.Status)
	}
	if c.Remedy != "aigate init --config" {
		t.Errorf("expected remedy aigate init --config, got %q", c.Remedy)
	}
	if !strings.HasSuffix(c.Path, ".aigate.yml") {
		t.Errorf("expected path to end in .aigate.yml, got %q", c.Path)
	}
}

func TestInspect_ReportsMalformedConfigAsOneProblem(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	// CI context must be applicable (not "not applicable") for this test to
	// prove it becomes unknown, like the other three features, rather than
	// staying not-applicable regardless of whether .aigate.yml parses.
	if err := os.MkdirAll(filepath.Join(dir, ".circleci"), 0755); err != nil {
		t.Fatal(err)
	}
	circleciConfig := "version: 2.1\njobs:\n  build:\n    docker:\n      - image: cimg/base:stable\nworkflows:\n  main:\n    jobs:\n      - build\n"
	if err := os.WriteFile(filepath.Join(dir, ".circleci", "config.yml"), []byte(circleciConfig), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, ".aigate.yml"), []byte("claude: [this is not a mapping\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	problems := rep.Problems()
	if len(problems) != 1 {
		t.Fatalf("expected exactly one problem, got %d: %+v", len(problems), problems)
	}
	if problems[0].Label != "Config" {
		t.Errorf("expected the sole problem to be Config, got %s", problems[0].Label)
	}

	for _, label := range []string{"Session context", "CI context", "Stop prompt", "Pre-commit checks"} {
		c, ok := findCheck(rep, label)
		if !ok {
			t.Fatalf("expected a %s check", label)
		}
		if c.Status != StatusUnknown {
			t.Errorf("expected %s to be unknown when .aigate.yml doesn't parse, got %v", label, c.Status)
		}
	}
}

func TestInspect_ReportsMalformedSettingsAsOneProblem(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	if err := os.WriteFile(filepath.Join(dir, ".claude", "settings.local.json"), []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	problems := rep.Problems()
	if len(problems) != 1 {
		t.Fatalf("expected exactly one problem, got %d: %+v", len(problems), problems)
	}
	if problems[0].Label != "Claude settings" {
		t.Errorf("expected the sole problem to be Claude settings, got %s", problems[0].Label)
	}

	for _, label := range []string{"Session context", "Stop prompt"} {
		c, ok := findCheck(rep, label)
		if !ok {
			t.Fatalf("expected a %s check", label)
		}
		if c.Status != StatusUnknown {
			t.Errorf("expected %s to be unknown when settings.local.json doesn't parse, got %v", label, c.Status)
		}
	}

	// Pre-commit checks has no half living in settings.local.json, so it's
	// unaffected by the malformed file.
	c, ok := findCheck(rep, "Pre-commit checks")
	if !ok {
		t.Fatal("expected a Pre-commit checks check")
	}
	if c.Status == StatusUnknown {
		t.Error("expected Pre-commit checks to be unaffected by a malformed settings.local.json")
	}
}

func TestInspect_ReportsProblemWhenBinaryNotOnPath(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	rep, err := Inspect(dir, fakeLookPath("", errors.New("not found")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c, ok := findCheck(rep, "Binary")
	if !ok {
		t.Fatal("expected a Binary check")
	}
	if c.Status != StatusProblem {
		t.Fatalf("expected Binary to be a problem, got %v", c.Status)
	}
}

func TestInspect_ReportsResolvedBinaryPath(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c, ok := findCheck(rep, "Binary")
	if !ok {
		t.Fatal("expected a Binary check")
	}
	if c.Status != StatusOK {
		t.Fatalf("expected Binary to be ok, got %v", c.Status)
	}
	if !strings.Contains(c.Detail, "/usr/local/bin/aigate") {
		t.Errorf("expected detail to include the resolved path, got %q", c.Detail)
	}
}

func TestInspect_MarksCIContextNotApplicableWithoutCircleCI(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c, ok := findCheck(rep, "CI context")
	if !ok {
		t.Fatal("expected a CI context check")
	}
	if c.Status != StatusNotApplicable {
		t.Fatalf("expected CI context to be not applicable without .circleci, got %v", c.Status)
	}
}

func TestInspect_ReportsCIContextNotConfiguredWhenCircleCIPresent(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	if err := os.MkdirAll(filepath.Join(dir, ".circleci"), 0755); err != nil {
		t.Fatal(err)
	}
	circleciConfig := "version: 2.1\njobs:\n  build:\n    docker:\n      - image: cimg/base:stable\nworkflows:\n  main:\n    jobs:\n      - build\n"
	if err := os.WriteFile(filepath.Join(dir, ".circleci", "config.yml"), []byte(circleciConfig), 0644); err != nil {
		t.Fatal(err)
	}

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c, ok := findCheck(rep, "CI context")
	if !ok {
		t.Fatal("expected a CI context check")
	}
	if c.Status != StatusNotConfigured {
		t.Fatalf("expected CI context to be not configured when .circleci has a real config and no ci-info entry, got %v", c.Status)
	}
}

func TestInspect_MarksDockerContextNotApplicableWithoutComposeFile(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c, ok := findCheck(rep, "Docker context")
	if !ok {
		t.Fatal("expected a Docker context check")
	}
	if c.Status != StatusNotApplicable {
		t.Fatalf("expected Docker context to be not applicable without a compose file, got %v", c.Status)
	}
}

func TestInspect_ReportsDockerContextNotConfiguredWhenComposeFilePresent(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	compose := "services:\n  db:\n    image: postgres:16\n"
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c, ok := findCheck(rep, "Docker context")
	if !ok {
		t.Fatal("expected a Docker context check")
	}
	if c.Status != StatusNotConfigured {
		t.Fatalf("expected Docker context to be not configured when a compose file exists and no docker-compose entry does, got %v", c.Status)
	}
}

func TestInspect_ResolvesEveryPathUnderTheGivenRoot(t *testing.T) {
	dir := t.TempDir()
	setupFullyWired(t, dir)

	// Break the session-start pair so the report includes a Path.
	writeSettings(t, filepath.Join(dir, ".claude", "settings.local.json"), map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "aigate claude stop"}}},
			},
		},
	})

	rep, err := Inspect(dir, fakeLookPath("/usr/local/bin/aigate", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(rep.Root, dir) {
		t.Errorf("expected Root to be under %q, got %q", dir, rep.Root)
	}
	for _, c := range rep.Checks {
		if c.Path != "" && !strings.HasPrefix(c.Path, rep.Root) {
			t.Errorf("expected %s's path %q to be under report root %q", c.Label, c.Path, rep.Root)
		}
	}
}

func TestComplete_TrueWhenFullyWired(t *testing.T) {
	s := state{
		Config: true, SessionHook: true, SessionContext: true,
		StopHook: true, StopPrompt: true, GitHook: true, PreCommitChecks: true,
	}
	if !Complete(s, Applicability{}) {
		t.Error("expected Complete to be true when every field is wired and CI isn't applicable")
	}
}

func TestComplete_FalseWhenOneHalfMissing(t *testing.T) {
	s := state{
		Config: true, SessionHook: true, SessionContext: true,
		StopHook: true, StopPrompt: false, GitHook: true, PreCommitChecks: true,
	}
	if Complete(s, Applicability{}) {
		t.Error("expected Complete to be false when one half of a pair is missing")
	}
}

func TestComplete_FalseWhenConfigMissing(t *testing.T) {
	s := state{
		SessionHook: true, SessionContext: true,
		StopHook: true, StopPrompt: true, GitHook: true, PreCommitChecks: true,
	}
	if Complete(s, Applicability{}) {
		t.Error("expected Complete to be false when the config file doesn't exist")
	}
}

func TestComplete_IgnoresCIContextWhenNotApplicable(t *testing.T) {
	s := state{
		Config: true, SessionHook: true, SessionContext: true,
		StopHook: true, StopPrompt: true, GitHook: true, PreCommitChecks: true,
		CIContext: false,
	}
	if !Complete(s, Applicability{}) {
		t.Error("expected Complete to be true when CI context is missing but not applicable")
	}
}

func TestComplete_FalseWhenCIContextMissingAndApplicable(t *testing.T) {
	s := state{
		Config: true, SessionHook: true, SessionContext: true,
		StopHook: true, StopPrompt: true, GitHook: true, PreCommitChecks: true,
		CIContext: false,
	}
	if Complete(s, Applicability{CI: true}) {
		t.Error("expected Complete to be false when CI is applicable but ci-info is missing")
	}
}

func TestLoadConfigStatus_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")

	st := loadConfigStatus(path)

	if st.exists {
		t.Error("expected exists to be false for a missing file")
	}
	if st.err != nil {
		t.Errorf("expected no error for a missing file, got %v", st.err)
	}
	if st.cfg != nil {
		t.Error("expected cfg to be nil for a missing file")
	}
}

func TestLoadConfigStatus_MalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	if err := os.WriteFile(path, []byte("claude: [this is not a mapping\n"), 0644); err != nil {
		t.Fatal(err)
	}

	st := loadConfigStatus(path)

	if !st.exists {
		t.Error("expected exists to be true for a malformed file")
	}
	if st.err == nil {
		t.Error("expected an error for a malformed file")
	}
	if st.cfg != nil {
		t.Error("expected cfg to be nil for a malformed file")
	}
}

func TestLoadConfigStatus_ValidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".aigate.yml")
	if err := os.WriteFile(path, []byte("claude:\n  stop-prompt:\n    enabled: true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	st := loadConfigStatus(path)

	if !st.exists || st.err != nil || st.cfg == nil {
		t.Fatalf("expected a valid parsed config, got %+v", st)
	}
	if st.cfg.Claude == nil || st.cfg.Claude.StopPrompt == nil || !st.cfg.Claude.StopPrompt.Enabled {
		t.Error("expected the parsed config to reflect the file's contents")
	}
}
