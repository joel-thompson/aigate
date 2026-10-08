package gitstate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Real captured `git status --porcelain=v2 --branch --show-stash` output.
const statusDirty = `# branch.oid 13dc2668e2daa2c0a90bfc287db2121c455f0b87
# branch.head main
1 MM N... 100644 100644 100644 45b983be36b73c0788dc9cbcb76cbb80fc7bb057 5093aa16aa2bbe3633f2f68d6ada76e53537ce9a a.txt
2 R. N... 100644 100644 100644 f719efd430d52bcfc8566a43b2eb655688d38871 f719efd430d52bcfc8566a43b2eb655688d38871 R100 d.txt	b.txt
1 .D N... 100644 100644 000000 587be6b4c3f93f93c489c0111bba5596147a26cb 587be6b4c3f93f93c489c0111bba5596147a26cb e.txt
? c.txt
`

const statusUpstream = `# branch.oid 672df83f90064d65b55e24735d5b7c53ed8c62e6
# branch.head test-hook
# branch.upstream origin/test-hook
# branch.ab +0 -0
`

const statusNoUpstream = `# branch.oid f0362763fc97b41966b5058517f9eeaa7acf0b67
# branch.head main
`

const statusUnborn = `# branch.oid (initial)
# branch.head main
`

const statusDetached = `# branch.oid d5e5adc6a5835c51f637f24d12ac8e27b076a8b1
# branch.head (detached)
? c.txt
`

const statusStash = `# branch.oid d5e5adc6a5835c51f637f24d12ac8e27b076a8b1
# branch.head main
# stash 2
`

// Real captured `git worktree list --porcelain` output.
const worktreeSingle = `worktree /Users/Joel/src/aigate
HEAD f0362763fc97b41966b5058517f9eeaa7acf0b67
branch refs/heads/main

`

const worktreeMainAndLinked = `worktree /Users/Joel/src/capibara
HEAD 32555f32a7d244c24f2834acf2774dd8f4924f72
branch refs/heads/main

worktree /Users/Joel/src/capibara-test-hook
HEAD 672df83f90064d65b55e24735d5b7c53ed8c62e6
branch refs/heads/test-hook

`

func TestParseStatus_UpstreamAndAheadBehind(t *testing.T) {
	got := parseStatus(statusUpstream)

	if got.branch != "test-hook" {
		t.Errorf("expected branch %q, got %q", "test-hook", got.branch)
	}
	if got.upstream != "origin/test-hook" {
		t.Errorf("expected upstream %q, got %q", "origin/test-hook", got.upstream)
	}
	if !got.hasAB {
		t.Errorf("expected hasAB for a branch.ab line")
	}
	if got.ahead != 0 || got.behind != 0 {
		t.Errorf("expected ahead 0 behind 0, got ahead %d behind %d", got.ahead, got.behind)
	}
}

func TestParseStatus_AheadAndBehindCounts(t *testing.T) {
	got := parseStatus("# branch.head main\n# branch.upstream origin/main\n# branch.ab +2 -3\n")

	if got.ahead != 2 {
		t.Errorf("expected ahead 2, got %d", got.ahead)
	}
	if got.behind != 3 {
		t.Errorf("expected behind 3, got %d", got.behind)
	}
}

// No upstream means no branch.ab line at all, which is a different fact from
// being level with one.
func TestParseStatus_NoUpstreamHasNoAheadBehind(t *testing.T) {
	got := parseStatus(statusNoUpstream)

	if got.upstream != "" {
		t.Errorf("expected no upstream, got %q", got.upstream)
	}
	if got.hasAB {
		t.Errorf("expected hasAB false when there is no branch.ab line")
	}
}

func TestParseStatus_Unborn(t *testing.T) {
	got := parseStatus(statusUnborn)

	if !got.unborn {
		t.Errorf("expected unborn for a (initial) branch.oid")
	}
	if got.branch != "main" {
		t.Errorf("expected branch %q, got %q", "main", got.branch)
	}
	if got.oid != "" {
		t.Errorf("expected no oid on an unborn HEAD, got %q", got.oid)
	}
}

func TestParseStatus_Detached(t *testing.T) {
	got := parseStatus(statusDetached)

	if !got.detached {
		t.Errorf("expected detached for a (detached) branch.head")
	}
	if got.branch != "" {
		t.Errorf("expected no branch when detached, got %q", got.branch)
	}
	if got.oid != "d5e5adc6a5835c51f637f24d12ac8e27b076a8b1" {
		t.Errorf("expected the full oid, got %q", got.oid)
	}
}

func TestParseStatus_Stash(t *testing.T) {
	got := parseStatus(statusStash)

	if got.stash != 2 {
		t.Errorf("expected stash 2, got %d", got.stash)
	}
}

func TestParseStatus_DirtyCounts(t *testing.T) {
	got := parseStatus(statusDirty)

	if got.staged != 2 {
		t.Errorf("expected 2 staged, got %d", got.staged)
	}
	if got.modified != 1 {
		t.Errorf("expected 1 modified, got %d", got.modified)
	}
	if got.deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", got.deleted)
	}
	if got.untracked != 1 {
		t.Errorf("expected 1 untracked, got %d", got.untracked)
	}
	isDirty := got.dirty()
	if !isDirty {
		t.Errorf("expected a dirty tree")
	}
}

// The XY field is the first token of an entry, so a path containing spaces
// changes nothing about the count — the regression this guards against is a
// parser that tries to find the path and miscounts when it can't.
func TestParseStatus_CountsEntryWithSpacesInPath(t *testing.T) {
	line := "1 .M N... 100644 100644 100644 45b983be36b73c0788dc9cbcb76cbb80fc7bb057 45b983be36b73c0788dc9cbcb76cbb80fc7bb057 my notes.md\n"
	got := parseStatus(line)

	if got.modified != 1 {
		t.Errorf("expected 1 modified, got %d", got.modified)
	}
	if got.staged != 0 {
		t.Errorf("expected 0 staged, got %d", got.staged)
	}
}

func TestParseStatus_Conflicted(t *testing.T) {
	line := "u UU N... 100644 100644 100644 100644 df967b96a579e45a18b8251732d16804b2e56a55 351be5bf6e17c59ea560546d69654115ecb2fd8d e45c9c2666d44e0327c1f9c239a74c508336053e f.txt\n"
	got := parseStatus(line)

	if got.conflicted != 1 {
		t.Errorf("expected 1 conflicted, got %d", got.conflicted)
	}
	if got.staged != 0 {
		t.Errorf("expected an unmerged path not to count as staged, got %d", got.staged)
	}
}

// A typechange — a file replaced by a symlink — is a real worktree change
// that porcelain reports as Y='T'. Enumerating only M and D rendered it as
// "clean", which is the one wrong answer that actively misleads.
func TestParseStatus_TypechangeIsDirty(t *testing.T) {
	line := "1 .T N... 100644 100644 120000 45b983be36b73c0788dc9cbcb76cbb80fc7bb057 45b983be36b73c0788dc9cbcb76cbb80fc7bb057 f.txt\n"
	got := parseStatus(line)

	if got.modified != 1 {
		t.Errorf("expected a typechange to count as modified, got %d", got.modified)
	}
	isDirty := got.dirty()
	if !isDirty {
		t.Errorf("expected a typechange to make the tree dirty")
	}
}

// An unrecognised worktree code must default to dirty rather than clean, so a
// porcelain code git adds later can never render a dirty tree as clean.
func TestParseStatus_UnknownWorktreeCodeIsDirty(t *testing.T) {
	line := "1 .X N... 100644 100644 100644 45b983be36b73c0788dc9cbcb76cbb80fc7bb057 45b983be36b73c0788dc9cbcb76cbb80fc7bb057 f.txt\n"
	got := parseStatus(line)

	isDirty := got.dirty()
	if !isDirty {
		t.Errorf("expected an unrecognised worktree code to make the tree dirty")
	}
}

// A staged-only change leaves the worktree column at ".", which must not be
// counted as a worktree modification.
func TestParseStatus_StagedOnlyIsNotModified(t *testing.T) {
	line := "1 M. N... 100644 100644 100644 45b983be36b73c0788dc9cbcb76cbb80fc7bb057 45b983be36b73c0788dc9cbcb76cbb80fc7bb057 f.txt\n"
	got := parseStatus(line)

	if got.staged != 1 {
		t.Errorf("expected 1 staged, got %d", got.staged)
	}
	if got.modified != 0 {
		t.Errorf("expected 0 modified for a clean worktree column, got %d", got.modified)
	}
}

func TestParseStatus_Clean(t *testing.T) {
	got := parseStatus(statusNoUpstream)

	isDirty := got.dirty()
	if isDirty {
		t.Errorf("expected a clean tree for a header-only status")
	}
}

func TestParseWorktrees_Single(t *testing.T) {
	got := parseWorktrees(worktreeSingle)

	if len(got) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(got))
	}
	if got[0].path != "/Users/Joel/src/aigate" {
		t.Errorf("expected path %q, got %q", "/Users/Joel/src/aigate", got[0].path)
	}
}

func TestParseWorktrees_MainFirstThenLinked(t *testing.T) {
	got := parseWorktrees(worktreeMainAndLinked)

	if len(got) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(got))
	}
	if got[0].path != "/Users/Joel/src/capibara" {
		t.Errorf("expected the main worktree first, got %q", got[0].path)
	}
	if got[1].path != "/Users/Joel/src/capibara-test-hook" {
		t.Errorf("expected the linked worktree second, got %q", got[1].path)
	}
}

// A detached worktree carries no branch line, which must not stop its path
// from being read — that path is the whole reason this is parsed.
func TestParseWorktrees_Detached(t *testing.T) {
	out := "worktree /tmp/repo\nHEAD d5e5adc6a5835c51f637f24d12ac8e27b076a8b1\ndetached\n"
	got := parseWorktrees(out)

	if len(got) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(got))
	}
	if got[0].path != "/tmp/repo" {
		t.Errorf("expected path %q, got %q", "/tmp/repo", got[0].path)
	}
}

func TestParseWorktrees_Bare(t *testing.T) {
	out := "worktree /tmp/repo.git\nbare\n"
	got := parseWorktrees(out)

	if len(got) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(got))
	}
	if !got[0].bare {
		t.Errorf("expected bare")
	}
}

// A repo whose main worktree is bare still has linked worktrees, and the
// bare entry is the one at index 0 — so "am I the main checkout" must not
// mistake a linked worktree for it.
func TestSplitWorktrees_LinkedWhenToplevelIsNotFirst(t *testing.T) {
	wts := parseWorktrees(worktreeMainAndLinked)

	main, linked := splitWorktrees("/Users/Joel/src/capibara-test-hook", wts)
	if !linked {
		t.Errorf("expected a linked worktree")
	}
	if main.path != "/Users/Joel/src/capibara" {
		t.Errorf("expected main %q, got %q", "/Users/Joel/src/capibara", main.path)
	}
}

func TestSplitWorktrees_MainCheckout(t *testing.T) {
	wts := parseWorktrees(worktreeMainAndLinked)

	main, linked := splitWorktrees("/Users/Joel/src/capibara", wts)
	if linked {
		t.Errorf("expected the main checkout not to report as linked")
	}
	if main.path != "/Users/Joel/src/capibara" {
		t.Errorf("expected main %q, got %q", "/Users/Joel/src/capibara", main.path)
	}
}

// With no worktree list at all — the probe failed — the current checkout is
// the only thing known, so it is reported as the main one rather than
// guessing.
func TestSplitWorktrees_NoListFallsBackToToplevel(t *testing.T) {
	main, linked := splitWorktrees("/repo", nil)

	if linked {
		t.Errorf("expected not linked with no worktree list")
	}
	if main.path != "/repo" {
		t.Errorf("expected main %q, got %q", "/repo", main.path)
	}
}

func TestDetected_GitDirectory(t *testing.T) {
	root := gitRoot(t)

	got := Detected(root)
	if !got {
		t.Errorf("expected an ordinary checkout to be detected")
	}
}

// A linked worktree has a .git *file* holding a gitdir: pointer, not a
// directory.
func TestDetected_GitFileWithGitdirPointer(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git"), "gitdir: /repo/.git/worktrees/wt\n")

	got := Detected(root)
	if !got {
		t.Errorf("expected a linked worktree to be detected")
	}
}

func TestDetected_GitFileWithoutGitdirPointer(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git"), "notes about git\n")

	got := Detected(root)
	if got {
		t.Errorf("expected a stray file named .git not to be detected")
	}
}

func TestDetected_ShortGitFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git"), "git")

	got := Detected(root)
	if got {
		t.Errorf("expected a file too short to hold the prefix not to be detected")
	}
}

func TestDetected_Absent(t *testing.T) {
	root := t.TempDir()

	got := Detected(root)
	if got {
		t.Errorf("expected a directory with no .git not to be detected")
	}
}

func TestDetectOperation_None(t *testing.T) {
	gitDir := t.TempDir()

	got := detectOperation(gitDir)
	if got != "" {
		t.Errorf("expected no operation, got %q", got)
	}
}

func TestDetectOperation_InteractiveRebaseWithProgress(t *testing.T) {
	gitDir := t.TempDir()
	dir := filepath.Join(gitDir, "rebase-merge")
	mkdir(t, dir)
	writeFile(t, filepath.Join(dir, "interactive"), "")
	writeFile(t, filepath.Join(dir, "msgnum"), "2\n")
	writeFile(t, filepath.Join(dir, "end"), "5\n")
	writeFile(t, filepath.Join(dir, "head-name"), "refs/heads/feature\n")

	got := detectOperation(gitDir)
	want := "interactive rebase of branch feature (2/5)"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestDetectOperation_NonInteractiveRebase(t *testing.T) {
	gitDir := t.TempDir()
	dir := filepath.Join(gitDir, "rebase-merge")
	mkdir(t, dir)
	writeFile(t, filepath.Join(dir, "head-name"), "refs/heads/feature\n")

	got := detectOperation(gitDir)
	want := "rebase of branch feature"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestDetectOperation_RebaseApplyIsAmWhenApplying(t *testing.T) {
	gitDir := t.TempDir()
	dir := filepath.Join(gitDir, "rebase-apply")
	mkdir(t, dir)
	writeFile(t, filepath.Join(dir, "applying"), "")
	writeFile(t, filepath.Join(dir, "next"), "1\n")
	writeFile(t, filepath.Join(dir, "last"), "3\n")

	got := detectOperation(gitDir)
	want := "git am (1/3)"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestDetectOperation_RebaseApplyIsRebaseWhenRebasing(t *testing.T) {
	gitDir := t.TempDir()
	dir := filepath.Join(gitDir, "rebase-apply")
	mkdir(t, dir)
	writeFile(t, filepath.Join(dir, "rebasing"), "")
	writeFile(t, filepath.Join(dir, "head-name"), "refs/heads/topic\n")

	got := detectOperation(gitDir)
	want := "rebase of branch topic"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestDetectOperation_RebaseApplyWithNoSentinel(t *testing.T) {
	gitDir := t.TempDir()
	mkdir(t, filepath.Join(gitDir, "rebase-apply"))

	got := detectOperation(gitDir)
	want := "rebase or git am"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestDetectOperation_Merge(t *testing.T) {
	gitDir := t.TempDir()
	writeFile(t, filepath.Join(gitDir, "MERGE_HEAD"), "abc\n")

	got := detectOperation(gitDir)
	if got != "merge" {
		t.Errorf("expected %q, got %q", "merge", got)
	}
}

func TestDetectOperation_CherryPick(t *testing.T) {
	gitDir := t.TempDir()
	writeFile(t, filepath.Join(gitDir, "CHERRY_PICK_HEAD"), "abc\n")

	got := detectOperation(gitDir)
	if got != "cherry-pick" {
		t.Errorf("expected %q, got %q", "cherry-pick", got)
	}
}

func TestDetectOperation_Revert(t *testing.T) {
	gitDir := t.TempDir()
	writeFile(t, filepath.Join(gitDir, "REVERT_HEAD"), "abc\n")

	got := detectOperation(gitDir)
	if got != "revert" {
		t.Errorf("expected %q, got %q", "revert", got)
	}
}

func TestDetectOperation_Bisect(t *testing.T) {
	gitDir := t.TempDir()
	writeFile(t, filepath.Join(gitDir, "BISECT_LOG"), "abc\n")

	got := detectOperation(gitDir)
	if got != "bisect" {
		t.Errorf("expected %q, got %q", "bisect", got)
	}
}

// A rebase in one linked worktree lives in that worktree's own git dir, so a
// marker in the shared common dir must not be read out of the wrong one.
func TestDetectOperation_ReadsTheWorktreeLocalGitDir(t *testing.T) {
	common := t.TempDir()
	writeFile(t, filepath.Join(common, "MERGE_HEAD"), "abc\n")
	worktreeGitDir := filepath.Join(common, "worktrees", "wt")
	mkdir(t, worktreeGitDir)

	got := detectOperation(worktreeGitDir)
	if got != "" {
		t.Errorf("expected no operation in the worktree-local git dir, got %q", got)
	}
}

func TestIgnoredDiff_SplitsByWhatThisWorktreeHas(t *testing.T) {
	mainPath := t.TempDir()
	toplevel := t.TempDir()
	mkdir(t, filepath.Join(toplevel, "node_modules"))

	p := &Provider{runCLI: stubCLI(map[string]string{
		ignoredKey: ".env\x00node_modules/\x00.vscode/settings.json\x00",
	}, nil)}

	missing, present := p.ignoredDiff(context.Background(), mainPath, toplevel)

	wantMissing := ".env,.vscode/settings.json"
	gotMissing := strings.Join(missing, ",")
	if gotMissing != wantMissing {
		t.Errorf("expected missing %q, got %q", wantMissing, gotMissing)
	}
	gotPresent := strings.Join(present, ",")
	if gotPresent != "node_modules/" {
		t.Errorf("expected present %q, got %q", "node_modules/", gotPresent)
	}
}

// Without -z, core.quotePath renders this path as "caf\303\251.env", which
// never matches the os.Stat deciding whether the worktree has it — so a file
// that is present gets reported MISSING, with escapes.
func TestIgnoredDiff_NonASCIIPathIsNotEscaped(t *testing.T) {
	mainPath := t.TempDir()
	toplevel := t.TempDir()
	writeFile(t, filepath.Join(toplevel, "café.env"), "")

	p := &Provider{runCLI: stubCLI(map[string]string{
		ignoredKey: "café.env\x00.env\x00",
	}, nil)}

	missing, present := p.ignoredDiff(context.Background(), mainPath, toplevel)

	gotPresent := strings.Join(present, ",")
	if gotPresent != "café.env" {
		t.Errorf("expected the non-ASCII path to be found in this worktree, got %q", gotPresent)
	}
	gotMissing := strings.Join(missing, ",")
	if gotMissing != ".env" {
		t.Errorf("expected only .env to be missing, got %q", gotMissing)
	}
}

// A gitignored path may legitimately contain a space, which -z preserves and
// a line split would too, but a trim would not.
func TestIgnoredDiff_PathWithSpaces(t *testing.T) {
	mainPath := t.TempDir()
	toplevel := t.TempDir()

	p := &Provider{runCLI: stubCLI(map[string]string{
		ignoredKey: "my secrets.env\x00",
	}, nil)}

	missing, _ := p.ignoredDiff(context.Background(), mainPath, toplevel)

	gotMissing := strings.Join(missing, ",")
	if gotMissing != "my secrets.env" {
		t.Errorf("expected the spaced path intact, got %q", gotMissing)
	}
}

func TestIgnoredDiff_ProbeFailureYieldsNothing(t *testing.T) {
	p := &Provider{runCLI: failingCLI}

	missing, present := p.ignoredDiff(context.Background(), t.TempDir(), t.TempDir())
	if missing != nil || present != nil {
		t.Errorf("expected nothing when the probe fails, got missing %v present %v", missing, present)
	}
}

func TestRun_NotAGitRepo(t *testing.T) {
	p := &Provider{Root: t.TempDir(), runCLI: failingCLI}

	got := run(t, p)
	if got != "" {
		t.Errorf("expected empty output outside a git repo, got %q", got)
	}
}

// The filesystem gate runs before any exec, so a non-repo never pays for a
// subprocess.
func TestRun_NotAGitRepoRunsNoCommands(t *testing.T) {
	var calls []call
	p := &Provider{Root: t.TempDir(), runCLI: recordingCLI(nil, &calls)}

	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 0 {
		t.Errorf("expected no git invocations outside a repo, got %d", len(calls))
	}
}

// A bare repo has no worktree, so rev-parse --show-toplevel fails and there
// is nothing to describe.
func TestRun_GitMissingYieldsEmptyOutput(t *testing.T) {
	p := &Provider{Root: gitRoot(t), runCLI: failingCLI}

	got := run(t, p)
	if got != "" {
		t.Errorf("expected empty output when git is unavailable, got %q", got)
	}
}

func TestRun_CancelledContext(t *testing.T) {
	root := gitRoot(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n.git\n",
	}, nil)}

	result, err := p.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "" {
		t.Errorf("expected empty output for a cancelled context, got %q", result.Output)
	}
}

func TestRun_MainCheckoutRender(t *testing.T) {
	root := gitRoot(t)
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n.git\n",
		statusKey: `# branch.oid f0362763fc97b41966b5058517f9eeaa7acf0b67
# branch.head main
# branch.upstream origin/main
# branch.ab +2 -0
1 M. N... 100644 100644 100644 aaaaaaa aaaaaaa internal/config/config.go
1 .M N... 100644 100644 100644 bbbbbbb bbbbbbb internal/wiring/report.go
`,
		worktreeKey: "worktree " + root + "\nHEAD abc\nbranch refs/heads/main\n\nworktree /other\nHEAD def\nbranch refs/heads/topic\n",
		logKey:      "f036276 Add docker-compose provider\n47a5fb7 Add AGENTS.md\n",
	}, nil)}

	got := run(t, p)
	want := "main → origin/main · ahead 2 · 1 staged, 1 modified\n" +
		"Main checkout " + root + "\n" +
		"Recent: f036276 Add docker-compose provider\n" +
		"        47a5fb7 Add AGENTS.md"
	if got != want {
		t.Errorf("expected output:\n%s\n\ngot:\n%s", want, got)
	}
}

// The heading is the only thing telling the model this block goes stale, so
// it is asserted as a literal rather than against the label constant.
func TestRun_LabelMarksSnapshot(t *testing.T) {
	root := gitRoot(t)
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n.git\n",
		statusKey:   statusNoUpstream,
		worktreeKey: "worktree " + root + "\nHEAD abc\nbranch refs/heads/main\n",
		logKey:      "f036276 Add docker-compose provider\n",
	}, nil)}

	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Git (snapshot at session start, not live)"
	if result.Label != want {
		t.Errorf("expected label %q, got %q", want, result.Label)
	}
}

func TestRun_CleanMainCheckoutWithNoUpstream(t *testing.T) {
	root := gitRoot(t)
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n.git\n",
		statusKey:   statusNoUpstream,
		worktreeKey: "worktree " + root + "\nHEAD abc\nbranch refs/heads/main\n",
		logKey:      "f036276 Add docker-compose provider\n",
	}, nil)}

	got := run(t, p)
	want := "main (no upstream) · clean\n" +
		"Main checkout " + root + "\n" +
		"Recent: f036276 Add docker-compose provider"
	if got != want {
		t.Errorf("expected output:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestRun_UnbornHead(t *testing.T) {
	root := gitRoot(t)
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n.git\n",
		statusKey:   statusUnborn,
		worktreeKey: "worktree " + root + "\n",
	}, nil)}

	got := run(t, p)
	want := "main (no commits yet) · clean\nMain checkout " + root
	if got != want {
		t.Errorf("expected output:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestRun_DetachedHead(t *testing.T) {
	root := gitRoot(t)
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n.git\n",
		statusKey:   statusDetached,
		worktreeKey: "worktree " + root + "\nHEAD d5e5adc6a5835c51f637f24d12ac8e27b076a8b1\ndetached\n",
		logKey:      "d5e5adc a commit\n",
	}, nil)}

	got := run(t, p)
	if !strings.HasPrefix(got, "detached at d5e5adc · 1 untracked\n") {
		t.Errorf("expected a detached HEAD to render its short oid, got:\n%s", got)
	}
}

func TestRun_LinkedWorktreeRender(t *testing.T) {
	mainPath := t.TempDir()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git"), "gitdir: "+filepath.Join(mainPath, ".git/worktrees/wt")+"\n")
	mkdir(t, filepath.Join(root, "node_modules"))
	gitDir := filepath.Join(mainPath, ".git", "worktrees", "wt")
	mkdir(t, filepath.Join(gitDir, "rebase-merge"))
	writeFile(t, filepath.Join(gitDir, "rebase-merge", "interactive"), "")
	writeFile(t, filepath.Join(gitDir, "rebase-merge", "msgnum"), "2\n")
	writeFile(t, filepath.Join(gitDir, "rebase-merge", "end"), "5\n")
	writeFile(t, filepath.Join(gitDir, "rebase-merge", "head-name"), "refs/heads/test-hook\n")

	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n" + gitDir + "\n",
		statusKey:   statusUpstream + "# stash 1\n",
		worktreeKey: "worktree " + mainPath + "\nHEAD abc\nbranch refs/heads/main\n\n" +
			"worktree " + root + "\nHEAD def\nbranch refs/heads/test-hook\n\n" +
			"worktree /other\nHEAD ghi\nbranch refs/heads/process-webhook\n",
		ignoredKey: ".env\x00node_modules/\x00.claude/settings.local.json\x00",
		logKey:     "672df83 a commit\n",
	}, nil)}

	got := run(t, p)
	want := "IN PROGRESS: interactive rebase of branch test-hook (2/5) — finish or abort it before committing.\n" +
		"test-hook → origin/test-hook · up to date · clean · 1 stash\n" +
		"Linked worktree " + root + "\n" +
		"  Main checkout: " + mainPath + "\n" +
		"  Gitignored files are per-worktree, not shared. Present in the main checkout but MISSING here: .env, .claude/settings.local.json\n" +
		"  Already present here: node_modules/\n" +
		"Recent: 672df83 a commit"
	if got != want {
		t.Errorf("expected output:\n%s\n\ngot:\n%s", want, got)
	}
}

// The compose warning is only true where a compose stack exists, so it is
// gated on the same predicate the docker-compose provider gates on rather
// than told to every repo.
func TestRun_LinkedWorktreeMentionsComposeOnlyWithAComposeFile(t *testing.T) {
	mainPath := t.TempDir()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git"), "gitdir: /repo/.git/worktrees/wt\n")
	writeFile(t, filepath.Join(root, "docker-compose.yml"), "")

	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n/repo/.git/worktrees/wt\n",
		statusKey:   statusUpstream,
		worktreeKey: "worktree " + mainPath + "\nHEAD abc\nbranch refs/heads/main\n\nworktree " + root + "\nHEAD def\nbranch refs/heads/test-hook\n",
		ignoredKey:  "",
		logKey:      "672df83 a commit\n",
	}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "Docker Compose names its project after the directory") {
		t.Errorf("expected the compose port-collision warning, got:\n%s", got)
	}
}

func TestRun_MainCheckoutOmitsComposeWarning(t *testing.T) {
	root := gitRoot(t)
	writeFile(t, filepath.Join(root, "docker-compose.yml"), "")

	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n.git\n",
		statusKey:   statusNoUpstream,
		worktreeKey: "worktree " + root + "\nHEAD abc\nbranch refs/heads/main\n",
		logKey:      "f036276 a commit\n",
	}, nil)}

	got := run(t, p)
	if strings.Contains(got, "Docker Compose") {
		t.Errorf("expected no compose warning in the main checkout, got:\n%s", got)
	}
}

func TestRun_CapsIgnoredEntries(t *testing.T) {
	mainPath := t.TempDir()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git"), "gitdir: /repo/.git/worktrees/wt\n")

	var entries []string
	for i := 0; i < maxIgnoredEntries+3; i++ {
		entries = append(entries, "ignored"+string(rune('a'+i)))
	}

	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n/repo/.git/worktrees/wt\n",
		statusKey:   statusUpstream,
		worktreeKey: "worktree " + mainPath + "\nHEAD abc\nbranch refs/heads/main\n\nworktree " + root + "\nHEAD def\nbranch refs/heads/test-hook\n",
		ignoredKey:  strings.Join(entries, "\x00") + "\x00",
		logKey:      "672df83 a commit\n",
	}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "... (3 more)") {
		t.Errorf("expected the ignored list to be capped with an elision, got:\n%s", got)
	}
}

// The bare-repo-plus-worktrees layout has no main checkout, so labelling the
// bare repo as one would be a lie the agent might act on.
func TestRun_LinkedWorktreeOffABareRepo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git"), "gitdir: /repo.git/worktrees/wt\n")

	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n/repo.git/worktrees/wt\n",
		statusKey:   statusUpstream,
		worktreeKey: "worktree /repo.git\nbare\n\nworktree " + root + "\nHEAD def\nbranch refs/heads/test-hook\n",
		ignoredKey:  "",
		logKey:      "672df83 a commit\n",
	}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "Main repository (bare, no checkout): /repo.git") {
		t.Errorf("expected a bare main worktree to be labelled as such, got:\n%s", got)
	}
}

// Sibling worktrees are not the session's business: it works in one
// checkout, and the others are neither actionable from here nor worth the
// lines.
func TestRun_OmitsSiblingWorktrees(t *testing.T) {
	root := gitRoot(t)
	blocks := []string{"worktree " + root + "\nHEAD abc\nbranch refs/heads/main"}
	for i := 0; i < 8; i++ {
		blocks = append(blocks, "worktree /wt"+string(rune('a'+i))+"\nHEAD abc\nbranch refs/heads/b"+string(rune('a'+i)))
	}

	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		toplevelKey: root + "\n.git\n",
		statusKey:   statusNoUpstream,
		worktreeKey: strings.Join(blocks, "\n\n") + "\n",
		logKey:      "f036276 a commit\n",
	}, nil)}

	got := run(t, p)
	want := "main (no upstream) · clean\n" +
		"Main checkout " + root + "\n" +
		"Recent: f036276 a commit"
	if got != want {
		t.Errorf("expected output:\n%s\n\ngot:\n%s", want, got)
	}
}

// OS metadata is gitignored everywhere and is never an artifact a fresh
// worktree needs, so it is filtered out of both halves of the diff rather
// than crowding out the .env that matters.
func TestIgnoredDiff_FiltersOSMetadataNoise(t *testing.T) {
	mainPath := t.TempDir()
	toplevel := t.TempDir()
	writeFile(t, filepath.Join(toplevel, "Thumbs.db"), "")

	p := &Provider{runCLI: stubCLI(map[string]string{
		ignoredKey: ".DS_Store\x00.env\x00Thumbs.db\x00desktop.ini\x00src/.DS_Store\x00",
	}, nil)}

	missing, present := p.ignoredDiff(context.Background(), mainPath, toplevel)

	gotMissing := strings.Join(missing, ",")
	if gotMissing != ".env" {
		t.Errorf("expected only .env to be missing, got %q", gotMissing)
	}
	if len(present) != 0 {
		t.Errorf("expected no noise entries reported as present, got %v", present)
	}
}

// run executes the provider and returns its rendered output, failing the
// test on the error path Run reserves for the impossible.
func run(t *testing.T, p *Provider) string {
	t.Helper()
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Label != label {
		t.Errorf("expected label %q, got %q", label, result.Label)
	}
	return result.Output
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}
