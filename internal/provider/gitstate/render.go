package gitstate

import (
	"fmt"
	"strings"
)

// renderInput is everything Run gathered, so render itself is a pure
// function over facts and can be tested without a fake git.
type renderInput struct {
	toplevel  string
	main      string // the main checkout's path
	mainBare  bool   // the main worktree is a bare repo, so it has no checkout
	linked    bool   // toplevel is a linked worktree, not the main checkout
	status    status
	operation string // "" when nothing is half-finished

	// missing holds the main checkout's gitignored entries this worktree
	// lacks. They are PATHS ONLY — this provider names .env, it never reads
	// it. That is what makes reporting on ignored files safe at all: provider
	// output goes straight into the model's context, so a probe that can see
	// a credential must not be able to print one.
	missing []string

	recent     []string
	hasCompose bool
}

func render(in renderInput) string {
	var lines []string

	// First, and in caps: a half-finished rebase changes what the agent is
	// allowed to do next, so it outranks everything else here.
	if in.operation != "" {
		lines = append(lines, fmt.Sprintf("IN PROGRESS: %s — finish or abort it before committing.", in.operation))
	}

	lines = append(lines, branchLine(in.status))
	lines = append(lines, in.worktreeLines()...)
	lines = append(lines, recentLines(in.recent)...)

	return strings.Join(lines, "\n")
}

// branchLine is the one-line branch picture: where HEAD is, how it compares
// to its upstream, how dirty the tree is, and how much is stashed.
func branchLine(s status) string {
	parts := []string{headDesc(s)}
	if s.hasAB {
		parts = append(parts, aheadBehindDesc(s))
	}
	parts = append(parts, dirtyDesc(s))
	if s.stash > 0 {
		parts = append(parts, plural(s.stash, "stash", "stashes"))
	}
	return strings.Join(parts, " · ")
}

func headDesc(s status) string {
	switch {
	case s.unborn:
		return fmt.Sprintf("%s (no commits yet)", orUnknown(s.branch))
	case s.detached:
		return "detached at " + orUnknown(shortOID(s.oid))
	case s.upstream != "":
		return s.branch + " → " + s.upstream
	default:
		// No branch.upstream line at all: the branch tracks nothing, which is
		// a different fact from being level with an upstream.
		return orUnknown(s.branch) + " (no upstream)"
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}

func aheadBehindDesc(s status) string {
	switch {
	case s.ahead > 0 && s.behind > 0:
		return fmt.Sprintf("ahead %d, behind %d", s.ahead, s.behind)
	case s.ahead > 0:
		return fmt.Sprintf("ahead %d", s.ahead)
	case s.behind > 0:
		return fmt.Sprintf("behind %d", s.behind)
	default:
		return "up to date"
	}
}

// dirtyDesc counts the tree's changes by kind. Staged is counted from the
// index column and modified/deleted from the worktree column, so one file
// that is both staged and further modified shows up in both counts — which
// is the honest answer, since committing now would capture only part of it.
func dirtyDesc(s status) string {
	if !s.dirty() {
		return "clean"
	}
	var parts []string
	for _, c := range []struct {
		n    int
		noun string
	}{
		{s.conflicted, "conflicted"},
		{s.staged, "staged"},
		{s.modified, "modified"},
		{s.deleted, "deleted"},
		{s.untracked, "untracked"},
	} {
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.noun))
		}
	}
	return strings.Join(parts, ", ")
}

// worktreeLines describe which checkout this is. In the main checkout that is
// a single line; in a linked worktree it is the block that motivates this
// whole provider, because a fresh worktree silently lacks the main
// checkout's gitignored artifacts.
//
// Sibling worktrees are deliberately not listed. A session works in one
// checkout, and the others are neither actionable from here nor worth the
// lines — the only thing about them that matters is which one is the main
// checkout, because that is where the missing artifacts live.
func (in renderInput) worktreeLines() []string {
	if !in.linked {
		return []string{"Main checkout " + in.toplevel}
	}

	mainLabel := "Main checkout: "
	if in.mainBare {
		// The bare-repo-plus-worktrees layout has no main checkout at all, so
		// calling it one would be a lie the agent might act on.
		mainLabel = "Main repository (bare, no checkout): "
	}
	lines := []string{
		"Linked worktree " + in.toplevel,
		indent + mainLabel + in.main,
	}

	if len(in.missing) > 0 {
		lines = append(lines, indent+"Gitignored files are per-worktree, not shared. Present in the main checkout but MISSING here: "+joinCapped(in.missing, maxIgnoredEntries))
	}
	if in.hasCompose {
		lines = append(lines, indent+"Docker Compose names its project after the directory, so this worktree gets its own containers, volumes and networks, and collides on host ports with a stack started in another worktree.")
	}

	return lines
}

// recentLines render the last few commit subjects, hanging under a single
// "Recent:" label so the subjects line up as a column.
func recentLines(recent []string) []string {
	if len(recent) == 0 {
		return nil
	}
	const heading = "Recent: "
	lines := []string{heading + recent[0]}
	for _, r := range recent[1:] {
		lines = append(lines, strings.Repeat(" ", len(heading))+r)
	}
	return lines
}

// joinCapped renders at most n items as a comma-separated list, disclosing
// how many it dropped rather than silently truncating.
func joinCapped(items []string, n int) string {
	kept, elided := capStrings(items, n)
	joined := strings.Join(kept, ", ")
	if elided > 0 {
		joined += fmt.Sprintf(", ... (%d more)", elided)
	}
	return joined
}

// capStrings keeps at most n items, reporting how many it dropped.
func capStrings(items []string, n int) (kept []string, elided int) {
	if n < 0 {
		n = 0
	}
	if len(items) <= n {
		return items, 0
	}
	return items[:n], len(items) - n
}

// plural takes both forms rather than suffixing an "s", since the nouns here
// include "stash".
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
