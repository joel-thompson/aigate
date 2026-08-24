package doctor

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joelthompson/aigate/internal/exit"
	"github.com/joelthompson/aigate/internal/wiring"
)

func okReport() wiring.Report {
	return wiring.Report{
		Root: "/repo",
		Checks: []wiring.Check{
			{Label: "Binary", Status: wiring.StatusOK, Detail: "aigate resolves to /usr/local/bin/aigate"},
			{Label: "Config", Status: wiring.StatusOK, Detail: ".aigate.yml"},
			{Label: "Claude settings", Status: wiring.StatusOK, Detail: ".claude/settings.local.json"},
			{Label: "Session context", Status: wiring.StatusOK},
			{Label: "CI context", Status: wiring.StatusNotApplicable, Detail: "no .circleci directory"},
			{Label: "Stop prompt", Status: wiring.StatusOK},
			{Label: "Pre-commit checks", Status: wiring.StatusNotConfigured, Detail: "no aigate line in .git/hooks/pre-commit, no git.pre-commit section in .aigate.yml"},
		},
	}
}

func problemReport() wiring.Report {
	rep := okReport()
	rep.Checks[5] = wiring.Check{
		Label:  "Stop prompt",
		Status: wiring.StatusProblem,
		Detail: "Stop hook in .claude/settings.local.json is present, but claude.stop-prompt section in .aigate.yml is missing; the hook fires but has nothing to run",
		Path:   "/repo/.aigate.yml",
		Remedy: "aigate init --stop-prompt",
	}
	return rep
}

func TestRender_PrintsOneLineForEachPassingCheck(t *testing.T) {
	var buf bytes.Buffer
	render(&buf, okReport())

	out := buf.String()
	for _, want := range []string{
		"Binary: ok (aigate resolves to /usr/local/bin/aigate)",
		"Config: ok (.aigate.yml)",
		"Claude settings: ok (.claude/settings.local.json)",
		"Session context: ok",
		"CI context: not applicable (no .circleci directory)",
		"Stop prompt: ok",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRender_IndentsDetailPathAndFixUnderAProblem(t *testing.T) {
	var buf bytes.Buffer
	render(&buf, problemReport())

	out := buf.String()
	_, rest, ok := strings.Cut(out, "Stop prompt: PROBLEM\n")
	if !ok {
		t.Fatalf("expected a Stop prompt: PROBLEM line, got:\n%s", out)
	}
	lines := strings.Split(rest, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines after the PROBLEM line, got: %v", lines)
	}
	if !strings.HasPrefix(lines[0], "  Stop hook in .claude/settings.local.json is present") {
		t.Errorf("expected detail line first, got: %q", lines[0])
	}
	if lines[1] != "  path: /repo/.aigate.yml" {
		t.Errorf("expected path line second, got: %q", lines[1])
	}
	if lines[2] != "  fix: aigate init --stop-prompt" {
		t.Errorf("expected fix line third, got: %q", lines[2])
	}
}

func TestRender_OmitsFixLineWhenRemedyIsEmpty(t *testing.T) {
	rep := okReport()
	rep.Checks[1] = wiring.Check{
		Label:  "Config",
		Status: wiring.StatusProblem,
		Detail: ".aigate.yml exists but does not parse: boom",
		Path:   "/repo/.aigate.yml",
	}

	var buf bytes.Buffer
	render(&buf, rep)

	if strings.Contains(buf.String(), "fix:") {
		t.Errorf("expected no fix line when Remedy is empty, got:\n%s", buf.String())
	}
}

func TestRender_PrintsCwdHeader(t *testing.T) {
	var buf bytes.Buffer
	render(&buf, okReport())

	if !strings.HasPrefix(buf.String(), "aigate doctor\ncwd: /repo\n\n") {
		t.Errorf("expected output to start with the title and cwd header, got:\n%s", buf.String())
	}
}

func TestRender_SummarizesNoProblemsFound(t *testing.T) {
	var buf bytes.Buffer
	render(&buf, okReport())

	if !strings.HasSuffix(strings.TrimRight(buf.String(), "\n"), "No problems found.") {
		t.Errorf("expected the summary to end with No problems found., got:\n%s", buf.String())
	}
}

func TestRender_SummarizesOneProblem(t *testing.T) {
	var buf bytes.Buffer
	render(&buf, problemReport())

	if !strings.HasSuffix(strings.TrimRight(buf.String(), "\n"), "1 problem found.") {
		t.Errorf("expected the summary to end with '1 problem found.', got:\n%s", buf.String())
	}
}

func TestRender_PluralizesProblemCount(t *testing.T) {
	rep := problemReport()
	rep.Checks[4] = wiring.Check{Label: "CI context", Status: wiring.StatusProblem, Detail: "boom"}

	var buf bytes.Buffer
	render(&buf, rep)

	if !strings.HasSuffix(strings.TrimRight(buf.String(), "\n"), "2 problems found.") {
		t.Errorf("expected the summary to end with '2 problems found.', got:\n%s", buf.String())
	}
}

func TestRunDoctor_ReturnsExitCodeOneWhenAProblemIsFound(t *testing.T) {
	t.Chdir(t.TempDir())

	var out, errw bytes.Buffer
	err := runDoctor(&out, &errw, func(string) (string, error) { return "", errors.New("not found") })

	if exit.CodeFor(err) != 1 {
		t.Fatalf("expected exit code 1, got %v (err=%v)", exit.CodeFor(err), err)
	}
	if out.Len() == 0 {
		t.Error("expected a non-empty report on stdout")
	}
	if errw.Len() != 0 {
		t.Errorf("expected empty stderr, got: %s", errw.String())
	}
}

func TestRunDoctor_ReturnsNilWhenEverythingIsWired(t *testing.T) {
	dir := t.TempDir()
	seedFullyWiredProject(t, dir)
	t.Chdir(dir)

	var out, errw bytes.Buffer
	err := runDoctor(&out, &errw, func(string) (string, error) { return "/usr/local/bin/aigate", nil })

	if err != nil {
		t.Fatalf("expected no error for a fully wired project, got %v", err)
	}
	if !strings.Contains(out.String(), "No problems found.") {
		t.Errorf("expected the report to say no problems were found, got:\n%s", out.String())
	}
	if errw.Len() != 0 {
		t.Errorf("expected empty stderr, got: %s", errw.String())
	}
}

func TestDoctorCommand_ExitsNonZeroWithoutCobraErrorBanner(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cmd := NewCommand()
	var out, errw bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errw)
	cmd.SetArgs(nil)

	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected an error (used as the exit-1 signal) for an empty project")
	}
	if exit.CodeFor(err) != 1 {
		t.Errorf("expected exit code 1, got %d", exit.CodeFor(err))
	}
	if strings.Contains(errw.String(), "Error:") {
		t.Errorf("expected no cobra error banner on stderr, got: %s", errw.String())
	}
}

func seedFullyWiredProject(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "hooks", "pre-commit"), []byte("#!/bin/sh\naigate git pre-commit\n"), 0755); err != nil {
		t.Fatal(err)
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
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	settings := `{
  "hooks": {
    "SessionStart": [{"hooks": [{"type": "command", "command": "aigate claude session-start"}]}],
    "Stop": [{"hooks": [{"type": "command", "command": "aigate claude stop"}]}]
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, ".claude", "settings.local.json"), []byte(settings), 0644); err != nil {
		t.Fatal(err)
	}
}
