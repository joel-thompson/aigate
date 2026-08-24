package gitstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// markerFiles maps a single-file marker in the git dir to the operation it
// means, in the order git's own wt_status checks them. git only ever has one
// of these outstanding, so the first match wins.
var markerFiles = []struct{ file, desc string }{
	{"MERGE_HEAD", "merge"},
	{"CHERRY_PICK_HEAD", "cherry-pick"},
	{"REVERT_HEAD", "revert"},
	{"BISECT_LOG", "bisect"},
}

// detectOperation names the half-finished git operation in gitDir, or ""
// when there is none. gitDir must be the *worktree-local* git dir — a rebase
// in one linked worktree leaves no marker in another's.
//
// This is pure os.Stat with no exec: git's state files are the same thing
// `git status` itself reads, so reproducing the check is cheaper than a
// fifth subprocess.
func detectOperation(gitDir string) string {
	if dir := filepath.Join(gitDir, "rebase-merge"); dirExists(dir) {
		// rebase-merge is the merge-backend rebase, interactive or not.
		kind := "rebase"
		if pathExists(filepath.Join(dir, "interactive")) {
			kind = "interactive rebase"
		}
		return describeRebase(dir, kind, "msgnum", "end")
	}

	if dir := filepath.Join(gitDir, "rebase-apply"); dirExists(dir) {
		// rebase-apply is shared by `git am` and the apply-backend rebase,
		// told apart by which sentinel file the operation dropped. Neither
		// being present means git itself can't tell, so neither does aigate.
		switch {
		case pathExists(filepath.Join(dir, "applying")):
			return "git am" + progress(dir, "next", "last")
		case pathExists(filepath.Join(dir, "rebasing")):
			return describeRebase(dir, "rebase", "next", "last")
		default:
			return "rebase or git am" + progress(dir, "next", "last")
		}
	}

	for _, m := range markerFiles {
		if pathExists(filepath.Join(gitDir, m.file)) {
			return m.desc
		}
	}
	return ""
}

// describeRebase names the branch being rebased and how far the rebase got.
// head-name holds the branch that was checked out when the rebase started —
// the one being replayed, not the one it is being replayed onto, which is
// what git's own "rebasing branch 'x'" wording reflects.
func describeRebase(dir, kind, curFile, totalFile string) string {
	desc := kind
	if name, ok := readTrimmed(filepath.Join(dir, "head-name")); ok {
		desc += " of branch " + strings.TrimPrefix(name, "refs/heads/")
	}
	return desc + progress(dir, curFile, totalFile)
}

// progress renders " (2/5)" from the two counter files, or "" when either is
// missing or unreadable — a rebase with no progress to report is still worth
// shouting about.
func progress(dir, curFile, totalFile string) string {
	cur, curOK := readTrimmed(filepath.Join(dir, curFile))
	total, totalOK := readTrimmed(filepath.Join(dir, totalFile))
	if !curOK || !totalOK {
		return ""
	}
	return fmt.Sprintf(" (%s/%s)", cur, total)
}

func readTrimmed(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
