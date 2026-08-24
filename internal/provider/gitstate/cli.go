package gitstate

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const cliBinary = "git"

// cliTimeout matches ciinfo's and dockercompose's budget and applies per
// call rather than as a shared pool, so a slow `status` can't eat the
// worktree probe's allowance and make the worktree answer depend on timing.
const cliTimeout = 3 * time.Second

// cliRunner shells out to git. Injectable so tests never depend on the state
// of the repository they happen to run in. dir is the directory to run in:
// git resolves the repository from the working directory, and the hook's own
// cwd isn't necessarily the repo root.
type cliRunner func(ctx context.Context, dir string, args ...string) (string, error)

// execCLI is the real cliRunner. It captures stdout only — never
// CombinedOutput — so git's stderr never reaches Claude's context.
func execCLI(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, cliBinary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		// Load-bearing. `git status` normally refreshes the index and takes
		// index.lock to write it back. This is a background probe firing on
		// session start, so it must not contend with a git command the user
		// is running by hand.
		"GIT_OPTIONAL_LOCKS=0",
		// Never block a session start waiting on a credential prompt.
		"GIT_TERMINAL_PROMPT=0",
		// Keep the captured text plain.
		"NO_COLOR=1",
		"GIT_PAGER=cat",
	)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// cliRunner returns p.runCLI, defaulting to execCLI.
func (p *Provider) cliRunner() cliRunner {
	if p.runCLI != nil {
		return p.runCLI
	}
	return execCLI
}

// run executes one git invocation under cliTimeout. Every probe funnels
// through it and returns (T, bool) rather than (T, error), because an
// *exec.ExitError carries the child's captured stderr: collapsing failure to
// a bool makes it structurally impossible for git's stderr to escape this
// file and land in the model's context.
func (p *Provider) run(ctx context.Context, dir string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()

	out, err := p.cliRunner()(ctx, dir, args...)
	if err != nil {
		return "", false
	}
	return out, true
}

// queryToplevel resolves the current worktree's root and its worktree-local
// git dir. It doubles as the exec-level "is this a usable git repo" gate: it
// fails in a bare repo, which has no worktree to describe.
func (p *Provider) queryToplevel(ctx context.Context, dir string) (toplevel, gitDir string, ok bool) {
	out, ok := p.run(ctx, dir, "rev-parse", "--show-toplevel", "--git-dir")
	if !ok {
		return "", "", false
	}
	lines := nonEmptyLines(out)
	if len(lines) < 2 {
		return "", "", false
	}
	return lines[0], lines[1], true
}

// queryStatus reports the branch, upstream, ahead/behind, stash count and
// per-path changes in one call.
func (p *Provider) queryStatus(ctx context.Context, dir string) (status, bool) {
	out, ok := p.run(ctx, dir, "status", "--porcelain=v2", "--branch", "--show-stash")
	if !ok {
		return status{}, false
	}
	return parseStatus(out), true
}

// queryWorktrees lists every worktree attached to the repository. git emits
// the main worktree first, which is what makes linked-vs-main decidable.
func (p *Provider) queryWorktrees(ctx context.Context, dir string) ([]worktree, bool) {
	out, ok := p.run(ctx, dir, "worktree", "list", "--porcelain")
	if !ok {
		return nil, false
	}
	wts := parseWorktrees(out)
	if len(wts) == 0 {
		return nil, false
	}
	return wts, true
}

// queryIgnored lists the gitignored entries present in dir. --directory plus
// --no-empty-directory prunes a fully-ignored directory to a single entry, so
// this reports "node_modules/" instead of descending into it — the
// difference between ~45ms and several seconds.
//
// -z is load-bearing, not a nicety. Without it core.quotePath renders a path
// with any non-ASCII byte in octal — "caf\303\251.env" for café.env — and a
// quoted path never matches the os.Stat that decides whether this worktree
// already has the entry, so a file that is present gets reported MISSING,
// with escapes.
func (p *Provider) queryIgnored(ctx context.Context, dir string) ([]string, bool) {
	out, ok := p.run(ctx, dir, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory", "--no-empty-directory")
	if !ok {
		return nil, false
	}
	return nulSeparated(out), true
}

// queryRecent returns the last maxRecentCommits commit subjects.
func (p *Provider) queryRecent(ctx context.Context, dir string) ([]string, bool) {
	out, ok := p.run(ctx, dir, "log", "-n", strconv.Itoa(maxRecentCommits), "--format=%h %s")
	if !ok {
		return nil, false
	}
	lines := nonEmptyLines(out)
	if len(lines) == 0 {
		return nil, false
	}
	return lines, true
}

// nulSeparated splits the NUL-terminated output of a -z probe. Unlike
// nonEmptyLines it cannot trim, since a path may legitimately end in a space.
func nulSeparated(out string) []string {
	var entries []string
	for _, e := range strings.Split(out, "\x00") {
		if e == "" {
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

// nonEmptyLines splits out on newlines, trimming each line and dropping the
// empties. git's porcelain output is newline-terminated, so the trailing
// empty line is always there to drop.
func nonEmptyLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
