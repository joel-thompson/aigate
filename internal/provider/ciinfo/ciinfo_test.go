package ciinfo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joelthompson/aigate/internal/provider"
)

func makeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// failingCLI stands in for a missing/broken circleci CLI so the existing
// tests in this file, which predate the CLI section, never depend on
// whether the machine running them actually has circleci on $PATH.
func failingCLI(ctx context.Context, args ...string) (string, error) {
	return "", errors.New("circleci: command not found")
}

func run(t *testing.T, root string) *provider.Result {
	t.Helper()
	p := &Provider{Root: root, runCLI: failingCLI}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

// runWithCLI is run's counterpart for tests exercising the CLI section: it
// injects runCLI directly instead of always failing it.
func runWithCLI(t *testing.T, root string, runCLI cliRunner) *provider.Result {
	t.Helper()
	p := &Provider{Root: root, runCLI: runCLI}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

func TestRun_StaticSingleConfig(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: 2.1
jobs:
  build: {}
  test: {}
  lint: {}
  deploy: {}
  package: {}
  publish: {}
workflows:
  main:
    jobs: [build]
`,
	})

	result := run(t, root)
	if result.Label != label {
		t.Errorf("expected label %q, got %q", label, result.Label)
	}
	out := result.Output
	for _, want := range []string{"CircleCI", ".circleci/config.yml", "version 2.1", "static config", "1 workflow, 6 jobs"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRun_SetupWithContinuationConfigurationPath(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: 2.1
setup: true
jobs:
  setup:
    steps:
      - checkout
      - continuation/continue:
          configuration_path: .circleci/continue-config.yml
workflows:
  setup:
    jobs: [setup]
`,
		".circleci/continue-config.yml": `version: 2.1
jobs:
  build: {}
  test: {}
  lint: {}
  deploy: {}
  publish: {}
workflows:
  main:
    jobs: [build]
  release:
    jobs: [deploy]
`,
	})

	result := run(t, root)
	out := result.Output
	for _, want := range []string{"dynamic setup config", "Continues into .circleci/continue-config.yml", "2 workflows, 5 jobs"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRun_SetupWithPathFilteringConfigPath(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: 2.1
setup: true
jobs:
  setup:
    steps:
      - checkout
      - path-filtering/filter:
          config-path: .circleci/continue_config.yml
workflows:
  setup:
    jobs: [setup]
`,
		".circleci/continue_config.yml": `version: 2.1
jobs:
  build: {}
workflows:
  main:
    jobs: [build]
`,
	})

	result := run(t, root)
	out := result.Output
	for _, want := range []string{"dynamic setup config", "Continues into .circleci/continue_config.yml", "1 workflow, 1 job"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRun_SetupNoDeclarationDefaultTargetPresent(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: 2.1
setup: true
jobs:
  setup:
    steps: [checkout]
workflows:
  setup:
    jobs: [setup]
`,
		".circleci/continue_config.yml": `version: 2.1
jobs:
  build: {}
workflows:
  main:
    jobs: [build]
`,
	})

	result := run(t, root)
	out := result.Output
	if !strings.Contains(out, "Continues into .circleci/continue_config.yml") {
		t.Errorf("expected default underscore path used, got:\n%s", out)
	}
	if !strings.Contains(out, "1 workflow, 1 job") {
		t.Errorf("expected counts from default target, got:\n%s", out)
	}
}

func TestRun_SetupNoDeclarationTargetAbsent(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: 2.1
setup: true
jobs:
  setup:
    steps: [checkout]
workflows:
  setup:
    jobs: [setup]
`,
	})

	result := run(t, root)
	out := result.Output
	if !strings.Contains(out, "not in this repo") {
		t.Errorf("expected 'not in this repo' wording, got:\n%s", out)
	}
	if !strings.Contains(out, "default path") {
		t.Errorf("expected the default-path caveat, got:\n%s", out)
	}
	if !strings.Contains(out, ".circleci/continue_config.yml") {
		t.Errorf("expected the default underscore path named, got:\n%s", out)
	}
}

func TestRun_DeclaredTargetAbsent(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: 2.1
setup: true
jobs:
  setup:
    steps:
      - continuation/continue:
          configuration_path: .circleci/nonexistent.yml
workflows:
  setup:
    jobs: [setup]
`,
	})

	result := run(t, root)
	out := result.Output
	if !strings.Contains(out, "Continues into .circleci/nonexistent.yml") {
		t.Errorf("expected declared target named, got:\n%s", out)
	}
	if !strings.Contains(out, "not in this repo") {
		t.Errorf("expected 'not in this repo' wording, got:\n%s", out)
	}
	if strings.Contains(out, "default path") {
		t.Errorf("expected no default-path caveat for a declared target, got:\n%s", out)
	}
}

func TestRun_ExtraConfigsListedAndExcludeEntryAndTarget(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: 2.1
setup: true
jobs:
  setup:
    steps:
      - continuation/continue:
          configuration_path: .circleci/continue-config.yml
workflows:
  setup:
    jobs: [setup]
`,
		".circleci/continue-config.yml": `version: 2.1
jobs:
  build: {}
workflows:
  main:
    jobs: [build]
`,
		".circleci/scheduled-jobs.yml": `version: 2.1
jobs:
  nightly: {}
workflows:
  nightly:
    jobs: [nightly]
`,
		".circleci/promotion-config.yml": `version: 2.1
jobs:
  promote: {}
workflows:
  promote:
    jobs: [promote]
`,
	})

	result := run(t, root)
	out := result.Output
	if !strings.Contains(out, "Other configs in .circleci/: promotion-config.yml, scheduled-jobs.yml") {
		t.Errorf("expected the other-configs line to list extras sorted, got:\n%s", out)
	}
	otherLine := out[strings.Index(out, "Other configs"):]
	if strings.Contains(otherLine, "continue-config.yml") {
		t.Errorf("expected the continuation target excluded from other configs, got:\n%s", otherLine)
	}
}

func TestRun_NonConfigsIgnored(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: 2.1
jobs:
  build: {}
workflows:
  main:
    jobs: [build]
`,
		".circleci/test-suites.yml": `name: suite
discover: "**/*_test.go"
run: go test
`,
		".circleci/pnpm-lock.yaml": `lockfileVersion: '6.0'
dependencies: {}
`,
	})

	result := run(t, root)
	out := result.Output
	if strings.Contains(out, "test-suites.yml") {
		t.Errorf("expected test-suites.yml to be ignored, got:\n%s", out)
	}
	if strings.Contains(out, "pnpm-lock.yaml") {
		t.Errorf("expected pnpm-lock.yaml to be ignored, got:\n%s", out)
	}
	if !strings.Contains(out, "1 workflow, 1 job") {
		t.Errorf("expected counts unaffected by non-configs, got:\n%s", out)
	}
}

func TestRun_NoConfigYMLMultipleCandidates(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/dogfood.yml": `version: 2.1
setup: true
jobs:
  setup:
    steps: [checkout]
workflows:
  setup:
    jobs: [setup]
`,
		".circleci/rollback.yml": `version: 2.1
jobs:
  rollback: {}
workflows:
  rollback:
    jobs: [rollback]
`,
	})

	result := run(t, root)
	out := result.Output
	if !strings.Contains(out, "has no config.yml") {
		t.Errorf("expected 'has no config.yml' wording, got:\n%s", out)
	}
	if !strings.Contains(out, "set project-side") {
		t.Errorf("expected 'set project-side' wording, got:\n%s", out)
	}
	if !strings.Contains(out, "dogfood.yml (dynamic setup config)") {
		t.Errorf("expected dogfood.yml annotated as dynamic setup config, got:\n%s", out)
	}
	if !strings.Contains(out, "rollback.yml") {
		t.Errorf("expected rollback.yml listed, got:\n%s", out)
	}
	if strings.Contains(out, "rollback.yml (dynamic") {
		t.Errorf("expected rollback.yml not annotated as dynamic, got:\n%s", out)
	}
}

func TestRun_LegacyWorkflowsVersionKeyNotCounted(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: 2
jobs:
  build: {}
workflows:
  version: 2
  build:
    jobs: [build]
`,
	})

	result := run(t, root)
	out := result.Output
	if !strings.Contains(out, "1 workflow, 1 job") {
		t.Errorf("expected the legacy workflows.version key excluded from the count, got:\n%s", out)
	}
}

func TestRun_QuotedVersionString(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: "2.1"
jobs:
  build: {}
workflows:
  main:
    jobs: [build]
`,
	})

	result := run(t, root)
	if !strings.Contains(result.Output, "version 2.1") {
		t.Errorf("expected quoted version string rendered without quotes, got:\n%s", result.Output)
	}
}

func TestRun_NoVersionAtAll(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `jobs:
  build: {}
workflows:
  main:
    jobs: [build]
`,
	})

	result := run(t, root)
	if strings.Contains(result.Output, "version") {
		t.Errorf("expected no version segment when version is absent, got:\n%s", result.Output)
	}
	if !strings.Contains(result.Output, "static config") {
		t.Errorf("expected static config still reported, got:\n%s", result.Output)
	}
}

func TestRun_NoCircleCIDir(t *testing.T) {
	root := t.TempDir()
	result := run(t, root)
	if result.Output != "" {
		t.Errorf("expected empty output when .circleci is missing, got:\n%s", result.Output)
	}
}

func TestRun_EmptyCircleCIDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".circleci"), 0755); err != nil {
		t.Fatal(err)
	}
	result := run(t, root)
	if result.Output != "" {
		t.Errorf("expected empty output for an empty .circleci dir, got:\n%s", result.Output)
	}
}

func TestRun_CircleCIIsAFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".circleci"), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	result := run(t, root)
	if result.Output != "" {
		t.Errorf("expected empty output when .circleci is a file, got:\n%s", result.Output)
	}
}

func TestRun_MalformedYAMLInOneFileDoesNotFailProvider(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": `version: 2.1
jobs:
  build: {}
workflows:
  main:
    jobs: [build]
`,
		".circleci/broken.yml": "jobs: [this is not: valid: mapping\n",
	})

	p := &Provider{Root: root, runCLI: failingCLI}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "1 workflow, 1 job") {
		t.Errorf("expected the valid config still summarized, got:\n%s", result.Output)
	}
}

func TestRun_OversizedFileSkipped(t *testing.T) {
	root := t.TempDir()
	big := "version: 2.1\njobs:\n  build: {}\nworkflows:\n  main:\n    jobs: [build]\n"
	big += "# padding\n" + strings.Repeat("#", maxConfigBytes+1) + "\n"
	makeFiles(t, root, map[string]string{
		".circleci/config.yml": big,
	})

	result := run(t, root)
	if result.Output != "" {
		t.Errorf("expected an oversized config.yml to be skipped entirely, got:\n%s", result.Output)
	}
}

func TestRun_CancelledContext(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := &Provider{Root: root}
	result, err := p.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "" {
		t.Errorf("expected cancelled context to produce no output, got:\n%s", result.Output)
	}
}

func TestDetected_True(t *testing.T) {
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
	if !Detected(root) {
		t.Error("expected Detected to be true")
	}
}

func TestDetected_FalseMissingDir(t *testing.T) {
	root := t.TempDir()
	if Detected(root) {
		t.Error("expected Detected to be false when .circleci is missing")
	}
}

func TestDetected_FalseNonConfigOnlyDir(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{
		".circleci/test-suites.yml": `name: suite
discover: "**/*_test.go"
run: go test
`,
	})
	if Detected(root) {
		t.Error("expected Detected to be false when nothing in .circleci looks like a CircleCI config")
	}
}

func TestDetected_FalseCircleCIIsAFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".circleci"), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	if Detected(root) {
		t.Error("expected Detected to be false when .circleci is a file")
	}
}
