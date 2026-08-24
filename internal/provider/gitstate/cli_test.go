package gitstate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The argv keys the stub dispatches on, spelled once so a test can't drift
// from the probe it means to stub.
const (
	toplevelKey = "rev-parse --show-toplevel --git-dir"
	statusKey   = "status --porcelain=v2 --branch --show-stash"
	worktreeKey = "worktree list --porcelain"
	ignoredKey  = "ls-files -z --others --ignored --exclude-standard --directory --no-empty-directory"
	logKey      = "log -n 3 --format=%h %s"
)

// stubCLI dispatches on the joined args, following ciinfo's and
// dockercompose's cli_test.go: a plain closure rather than a mock framework.
func stubCLI(out map[string]string, errs map[string]error) cliRunner {
	return func(_ context.Context, _ string, args ...string) (string, error) {
		key := strings.TrimSpace(strings.Join(args, " "))
		if err, ok := errs[key]; ok {
			return "", err
		}
		return out[key], nil
	}
}

// recordingCLI wraps out with a call log, so a test can assert on which
// probes ran and which directory each ran in.
func recordingCLI(out map[string]string, calls *[]call) cliRunner {
	return func(_ context.Context, dir string, args ...string) (string, error) {
		key := strings.TrimSpace(strings.Join(args, " "))
		*calls = append(*calls, call{dir: dir, args: key})
		return out[key], nil
	}
}

type call struct {
	dir  string
	args string
}

// failingCLI stands in for a missing git binary, so tests never depend on
// whether the machine running them has git installed.
func failingCLI(_ context.Context, _ string, _ ...string) (string, error) {
	return "", errors.New("git: command not found")
}

// gitRoot is a temp dir holding the minimum that gets past Detected's
// filesystem gate. Its contents are never parsed — the stubbed CLI supplies
// every fact — so an empty .git directory is enough.
func gitRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestQueryToplevel_ReturnsBothLines(t *testing.T) {
	p := &Provider{runCLI: stubCLI(map[string]string{
		toplevelKey: "/repo\n.git\n",
	}, nil)}

	toplevel, gitDir, ok := p.queryToplevel(context.Background(), "/repo")
	if !ok {
		t.Fatalf("expected ok for a two-line rev-parse")
	}
	if toplevel != "/repo" {
		t.Errorf("expected toplevel %q, got %q", "/repo", toplevel)
	}
	if gitDir != ".git" {
		t.Errorf("expected git dir %q, got %q", ".git", gitDir)
	}
}

func TestQueryToplevel_ShortOutputIsNotOK(t *testing.T) {
	p := &Provider{runCLI: stubCLI(map[string]string{toplevelKey: "/repo\n"}, nil)}

	_, _, ok := p.queryToplevel(context.Background(), "/repo")
	if ok {
		t.Errorf("expected not ok when rev-parse emits only one line")
	}
}

func TestQueryToplevel_FailureIsNotOK(t *testing.T) {
	p := &Provider{runCLI: failingCLI}

	_, _, ok := p.queryToplevel(context.Background(), "/repo")
	if ok {
		t.Errorf("expected not ok when git is missing")
	}
}

// The stderr an *exec.ExitError carries is exactly what must not reach the
// model, so the probe's failure signal is a bool and the error is dropped
// inside this file.
func TestQueryStatus_FailureIsNotOK(t *testing.T) {
	p := &Provider{runCLI: failingCLI}

	_, ok := p.queryStatus(context.Background(), "/repo")
	if ok {
		t.Errorf("expected not ok when git is missing")
	}
}

func TestQueryWorktrees_EmptyOutputIsNotOK(t *testing.T) {
	p := &Provider{runCLI: stubCLI(map[string]string{worktreeKey: "\n"}, nil)}

	_, ok := p.queryWorktrees(context.Background(), "/repo")
	if ok {
		t.Errorf("expected not ok when worktree list is empty")
	}
}

func TestQueryRecent_EmptyOutputIsNotOK(t *testing.T) {
	p := &Provider{runCLI: stubCLI(map[string]string{logKey: "\n"}, nil)}

	_, ok := p.queryRecent(context.Background(), "/repo")
	if ok {
		t.Errorf("expected not ok when log emits nothing")
	}
}

// The ignored-file probe is the expensive one and the only one that reads
// another directory, so both facts about it are pinned: it does not run in
// the main checkout, and when it does run it runs against the main
// checkout's path.
func TestRun_SkipsIgnoredProbeInMainCheckout(t *testing.T) {
	root := gitRoot(t)
	var calls []call
	p := &Provider{Root: root, runCLI: recordingCLI(map[string]string{
		toplevelKey: root + "\n.git\n",
		statusKey:   "# branch.oid abc\n# branch.head main\n",
		worktreeKey: "worktree " + root + "\nHEAD abc\nbranch refs/heads/main\n",
		logKey:      "abc123 a commit\n",
	}, &calls)}

	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range calls {
		if c.args == ignoredKey {
			t.Errorf("expected no ls-files probe in the main checkout, got one in %q", c.dir)
		}
	}
}

func TestRun_ProbesIgnoredFilesInTheMainCheckout(t *testing.T) {
	mainPath := "/repo"
	root := gitRoot(t)
	var calls []call
	p := &Provider{Root: root, runCLI: recordingCLI(map[string]string{
		toplevelKey: root + "\n" + filepath.Join(mainPath, ".git/worktrees/wt") + "\n",
		statusKey:   "# branch.oid abc\n# branch.head feature\n",
		worktreeKey: "worktree " + mainPath + "\nHEAD abc\nbranch refs/heads/main\n\nworktree " + root + "\nHEAD abc\nbranch refs/heads/feature\n",
		ignoredKey:  ".env\x00",
		logKey:      "abc123 a commit\n",
	}, &calls)}

	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var ignoredDirs []string
	for _, c := range calls {
		if c.args == ignoredKey {
			ignoredDirs = append(ignoredDirs, c.dir)
		}
	}
	if len(ignoredDirs) != 1 {
		t.Fatalf("expected exactly one ls-files probe, got %d", len(ignoredDirs))
	}
	if ignoredDirs[0] != mainPath {
		t.Errorf("expected ls-files to run in the main checkout %q, got %q", mainPath, ignoredDirs[0])
	}
}

// An unborn HEAD has nothing to log, so the exec is skipped rather than spent
// on a guaranteed failure.
func TestRun_SkipsLogWhenHeadIsUnborn(t *testing.T) {
	root := gitRoot(t)
	var calls []call
	p := &Provider{Root: root, runCLI: recordingCLI(map[string]string{
		toplevelKey: root + "\n.git\n",
		statusKey:   "# branch.oid (initial)\n# branch.head main\n",
		worktreeKey: "worktree " + root + "\n",
	}, &calls)}

	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range calls {
		if c.args == logKey {
			t.Errorf("expected no log probe on an unborn HEAD")
		}
	}
}
