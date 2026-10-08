// Package gitstate provides session-start context describing the git
// checkout the session is running in: the branch and its relationship to its
// upstream, whether the tree is dirty, whether an operation like a rebase is
// half-finished, and — when the session is in a linked worktree — which
// gitignored local artifacts the main checkout has that this worktree is
// missing. Gitignored files are not shared between worktrees, so a fresh one
// starts without the dependencies, .env files and editor settings the main
// checkout accumulated, and the agent otherwise discovers that by failing.
package gitstate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/joelthompson/aigate/internal/provider"
	"github.com/joelthompson/aigate/internal/provider/dockercompose"
)

// label says the block is a snapshot because hook output can only be
// appended to the conversation, never replaced: this stays in context for
// the whole session while the commits and dirty counts it reports move on.
const label = "Git (snapshot at session start, not live)"

// maxRecentCommits and maxIgnoredEntries cap the rendered output, following
// dockercompose's maxServices: a main checkout with 200 gitignored entries
// would otherwise spend the whole session-start budget here. They are package constants rather than
// config fields because ci-info sets the precedent that a detected, opt-in
// context provider has no knobs.
const (
	maxRecentCommits  = 3
	maxIgnoredEntries = 10
)

// indent prefixes the nested lines of a rendered block.
const indent = "  "

type Provider struct {
	Root string

	// runCLI shells out to git. Injectable so tests never depend on the
	// state of the repository they happen to run in; nil uses execCLI.
	runCLI cliRunner
}

// gitDirName is the entry that marks a checkout, in either of its two
// shapes: a directory in an ordinary checkout, or a regular file containing
// a "gitdir:" pointer in a linked worktree.
const gitDirName = ".git"

// gitFilePrefix opens the .git *file* a linked worktree has in place of a
// directory. Checking for it keeps a stray file that happens to be named
// .git from registering as a checkout, mirroring ciinfo.Detected's
// strictness about what counts as a config.
const gitFilePrefix = "gitdir: "

// Detected reports whether root is a git checkout aigate can summarize.
// Exported so init's detector and doctor's report share this exact
// predicate. Only root is checked, not parent directories, matching
// ciinfo.Detected and dockercompose.Detected's scope and init's
// Detected(".") call — Run still benefits from git's own upward walk at exec
// time, but the gate is deliberately narrower than the probe.
func Detected(root string) bool {
	path := filepath.Join(root, gitDirName)
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return true
	}
	return hasGitFilePrefix(path)
}

// hasGitFilePrefix reads only as far as the prefix it is looking for, so a
// large file that happens to be named .git is never slurped into memory.
func hasGitFilePrefix(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// ReadFull, not Read: Read is permitted to return fewer bytes than the
	// buffer holds without erroring, which would make a real linked worktree
	// look undetected.
	buf := make([]byte, len(gitFilePrefix))
	if _, err := io.ReadFull(f, buf); err != nil {
		return false
	}
	return string(buf) == gitFilePrefix
}

// Run summarizes the checkout. Every rung of the degradation ladder returns
// an empty result rather than an error, for the reason dockercompose.Run
// documents: RunEnrich prints "[error] %v" to stderr for a returned error,
// and erroring here would produce that noise at every session start in any
// directory that isn't a git repo. The os.Getwd failure is the only thing
// impossible enough to be worth returning.
func (p *Provider) Run(ctx context.Context) (*provider.Result, error) {
	root := p.Root
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}
		root = wd
	}

	// Filesystem gate first, so a directory that isn't a checkout never pays
	// for an exec.
	if !Detected(root) {
		return &provider.Result{Label: label}, nil
	}

	if ctx.Err() != nil {
		return &provider.Result{Label: label}, nil
	}

	toplevel, gitDir, ok := p.queryToplevel(ctx, root)
	if !ok {
		// No git binary, or a bare repo, which has no worktree to describe.
		return &provider.Result{Label: label}, nil
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(toplevel, gitDir)
	}

	if ctx.Err() != nil {
		return &provider.Result{Label: label}, nil
	}

	st, ok := p.queryStatus(ctx, toplevel)
	if !ok {
		return &provider.Result{Label: label}, nil
	}

	if ctx.Err() != nil {
		return &provider.Result{Label: label}, nil
	}

	wts, _ := p.queryWorktrees(ctx, toplevel)
	main, linked := splitWorktrees(toplevel, wts)

	// The ignored-file diff only means anything in a linked worktree, and it
	// is the main checkout's contents that answer it, so this probe is
	// skipped entirely in the main checkout.
	var missing, present []string
	if linked && ctx.Err() == nil {
		missing, present = p.ignoredDiff(ctx, main.path, toplevel)
	}

	// An unborn HEAD has nothing to log, so the exec is skipped rather than
	// spent on a guaranteed failure.
	var recent []string
	if !st.unborn && ctx.Err() == nil {
		recent, _ = p.queryRecent(ctx, toplevel)
	}

	out := render(renderInput{
		toplevel:   toplevel,
		main:       main.path,
		mainBare:   main.bare,
		linked:     linked,
		status:     st,
		operation:  detectOperation(gitDir),
		missing:    missing,
		present:    present,
		recent:     recent,
		hasCompose: dockercompose.Detected(toplevel),
	})

	return &provider.Result{Label: label, Output: out}, nil
}

// noiseEntries are OS and editor metadata files that turn up as gitignored
// but are never something a fresh worktree needs: the whole point of the
// diff is to name artifacts worth reproducing, and reporting a missing
// .DS_Store is pure noise crowding out the .env that matters. Matched on the
// base name, since ls-files reports these at whatever depth they appear.
var noiseEntries = map[string]bool{
	".DS_Store":   true,
	"Thumbs.db":   true,
	"desktop.ini": true,
}

// ignoredDiff splits the main checkout's gitignored entries into the ones
// absent from this worktree and the ones already here. Only the main
// checkout is probed; whether an entry exists here is an os.Stat, which is
// both cheaper than a second exec and the question actually being asked.
func (p *Provider) ignoredDiff(ctx context.Context, mainPath, toplevel string) (missing, present []string) {
	entries, ok := p.queryIgnored(ctx, mainPath)
	if !ok {
		return nil, nil
	}
	for _, entry := range entries {
		if noiseEntries[filepath.Base(entry)] {
			continue
		}
		if pathExists(filepath.Join(toplevel, entry)) {
			present = append(present, entry)
			continue
		}
		missing = append(missing, entry)
	}
	return missing, present
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// splitWorktrees reports the main checkout's path and whether toplevel is
// something other than it. `git worktree list` always emits the main
// worktree first, which is what makes this decidable in one call.
//
// Comparing --git-dir against --git-common-dir would be the obvious
// alternative and is a trap: in the main checkout git returns the relative
// string ".git" for both, while in a linked worktree it returns two
// different absolute paths, so that comparison needs path normalisation this
// one doesn't.
func splitWorktrees(toplevel string, wts []worktree) (main worktree, linked bool) {
	if len(wts) == 0 {
		return worktree{path: toplevel}, false
	}
	return wts[0], !samePath(wts[0].path, toplevel)
}

// samePath compares two paths git produced. Both come back absolute and
// cleaned, so this is a string compare with a Clean for safety rather than a
// symlink-resolving one.
func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
