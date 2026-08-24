package gitstate

import (
	"strconv"
	"strings"
)

// status is the subset of `git status --porcelain=v2 --branch --show-stash`
// this provider reports on.
type status struct {
	branch   string // "" when detached
	oid      string // full HEAD oid; "" when unborn
	unborn   bool
	detached bool
	upstream string
	ahead    int
	behind   int
	hasAB    bool // whether an upstream comparison was reported at all
	stash    int

	staged     int
	modified   int
	deleted    int
	untracked  int
	conflicted int
}

func (s status) dirty() bool {
	return s.staged+s.modified+s.deleted+s.untracked+s.conflicted > 0
}

// parseStatus reads porcelain v2: "# key value" header lines carry the branch
// picture, "1"/"2" a changed tracked path, "u" an unmerged one and "?" an
// untracked one. Any other leading token — "!" for an ignored path, which
// only appears under --ignored — is skipped.
//
// Only the header values and per-kind counts are read. The paths themselves
// are deliberately left unparsed, which is why nothing here needs to know
// that a porcelain entry puts its path behind a kind-specific number of
// fixed fields (7, 8 and 9 respectively) or that a rename's path field is
// two tab-separated names.
func parseStatus(out string) status {
	var s status
	for _, line := range nonEmptyLines(out) {
		if strings.HasPrefix(line, "# ") {
			s.parseHeader(strings.TrimPrefix(line, "# "))
			continue
		}
		kind, rest, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		switch kind {
		case "?":
			s.untracked++
		case "u":
			s.conflicted++
		case "1", "2":
			s.countTracked(rest)
		}
	}
	return s
}

func (s *status) parseHeader(header string) {
	key, value, found := strings.Cut(header, " ")
	if !found {
		return
	}
	switch key {
	case "branch.oid":
		if value == "(initial)" {
			s.unborn = true
			return
		}
		s.oid = value
	case "branch.head":
		if value == "(detached)" {
			s.detached = true
			return
		}
		s.branch = value
	case "branch.upstream":
		s.upstream = value
	case "branch.ab":
		s.parseAheadBehind(value)
	case "stash":
		if n, err := strconv.Atoi(value); err == nil {
			s.stash = n
		}
	}
}

// parseAheadBehind reads the "+2 -1" field. The whole branch.ab line is
// absent when there is no upstream to compare against, which is why hasAB is
// tracked separately from a zero ahead/behind — "up to date" and "no
// upstream" are different facts.
func (s *status) parseAheadBehind(value string) {
	aheadField, behindField, ok := strings.Cut(value, " ")
	if !ok {
		return
	}
	ahead, aheadErr := strconv.Atoi(strings.TrimPrefix(aheadField, "+"))
	behind, behindErr := strconv.Atoi(strings.TrimPrefix(behindField, "-"))
	if aheadErr != nil || behindErr != nil {
		return
	}
	s.ahead, s.behind, s.hasAB = ahead, behind, true
}

// countTracked counts one changed tracked path from its XY field, the first
// token of the entry: X is the staged state and Y the worktree state, "."
// meaning unchanged on that side.
func (s *status) countTracked(rest string) {
	xy, _, found := strings.Cut(rest, " ")
	if !found || len(xy) < 2 {
		return
	}
	if xy[0] != '.' {
		s.staged++
	}
	switch xy[1] {
	case '.':
		// Unchanged in the worktree; the index column above already counted
		// whatever is staged.
	case 'D':
		s.deleted++
	default:
		// M (modified), T (typechange — a file replaced by a symlink), and
		// any code git adds later. Defaulting to "modified" rather than
		// enumerating codes means an unrecognised one can never render a
		// dirty tree as clean, which is the failure that would actually
		// mislead the agent.
		s.modified++
	}
}

// worktree is one record from `git worktree list --porcelain`, reduced to the
// two facts anything here reads. The per-worktree HEAD and branch are
// deliberately dropped: sibling worktrees aren't reported, so the only
// worktree this provider describes is the current one — and `git status`
// already answered that in more detail than `worktree list` can.
type worktree struct {
	path string
	bare bool
}

// parseWorktrees reads `git worktree list --porcelain`: blank-line-separated
// blocks of "key value" or bare-keyword lines, main worktree first.
func parseWorktrees(out string) []worktree {
	var wts []worktree
	for _, block := range strings.Split(out, "\n\n") {
		var w worktree
		for _, line := range nonEmptyLines(block) {
			key, value, _ := strings.Cut(line, " ")
			switch key {
			case "worktree":
				w.path = value
			case "bare":
				w.bare = true
			}
		}
		if w.path == "" {
			continue
		}
		wts = append(wts, w)
	}
	return wts
}

func shortOID(oid string) string {
	if len(oid) > 7 {
		return oid[:7]
	}
	return oid
}
