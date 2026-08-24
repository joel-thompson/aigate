package ciinfo

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubCLI dispatches on the joined args (trimmed, so the trailing "word
// being completed" argument doesn't have to appear in every test's map
// keys), following runner_test.go's hand-written stubProvider: a plain
// closure rather than a mock framework.
func stubCLI(out map[string]string, errs map[string]error) cliRunner {
	return func(_ context.Context, args ...string) (string, error) {
		key := strings.TrimSpace(strings.Join(args, " "))
		if err, ok := errs[key]; ok {
			return "", err
		}
		return out[key], nil
	}
}

func TestCliSection_VersionFails(t *testing.T) {
	runCLI := stubCLI(nil, map[string]error{"version": errors.New("exec: \"circleci\": executable file not found in $PATH")})
	got := cliSection(context.Background(), runCLI)
	if got != "" {
		t.Errorf("expected empty section when version fails, got %q", got)
	}
}

func TestCliSection_BlankVersion(t *testing.T) {
	runCLI := stubCLI(map[string]string{"version": "   \n"}, nil)
	got := cliSection(context.Background(), runCLI)
	if got != "" {
		t.Errorf("expected empty section for blank version output, got %q", got)
	}
}

func TestCliSection_Success(t *testing.T) {
	version := "circleci 1.0.47692 (e05a7e5cc8fc)"
	runCLI := stubCLI(map[string]string{"version": version + "\n"}, nil)

	got := cliSection(context.Background(), runCLI)
	want := "CLI · " + version + "\n" + cliHelpLine
	if got != want {
		t.Errorf("expected:\n%q\ngot:\n%q", want, got)
	}
}

func TestCliSection_OnlyProbesVersion(t *testing.T) {
	var calls [][]string
	runCLI := func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, args)
		return "circleci 1.0.47692 (e05a7e5cc8fc)\n", nil
	}

	cliSection(context.Background(), runCLI)
	if len(calls) != 1 {
		t.Fatalf("expected exactly one call, got %d: %v", len(calls), calls)
	}
	if len(calls[0]) != 1 || calls[0][0] != "version" {
		t.Errorf("expected the single call to be [version], got %v", calls[0])
	}
}

func TestRun_CLISectionAppendedWhenCircleCIDetected(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: 2.1
jobs:
  build: {}
workflows:
  main:
    jobs: [build]
`,
	})
	runCLI := stubCLI(map[string]string{
		"version": "circleci 1.0.47692 (e05a7e5cc8fc)\n",
	}, nil)

	result := runWithCLI(t, root, runCLI)
	if !strings.Contains(result.Output, "CircleCI") {
		t.Errorf("expected the existing CircleCI summary preserved, got:\n%s", result.Output)
	}
	if !strings.Contains(result.Output, "CLI · circleci 1.0.47692") {
		t.Errorf("expected the CLI section appended, got:\n%s", result.Output)
	}
	if !strings.Contains(result.Output, cliHelpLine) {
		t.Errorf("expected the help pointer appended, got:\n%s", result.Output)
	}
}

func TestRun_CLISectionNotAppendedWithoutCircleCI(t *testing.T) {
	root := t.TempDir()
	runCLI := stubCLI(map[string]string{
		"version": "circleci 1.0.47692 (e05a7e5cc8fc)\n",
	}, nil)

	result := runWithCLI(t, root, runCLI)
	if result.Output != "" {
		t.Errorf("expected no output when there's no .circleci, even with a runner that would succeed, got:\n%s", result.Output)
	}
}
