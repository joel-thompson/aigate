package wiring

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/joelthompson/aigate/internal/config"
)

// mergeOK runs mergeStopPrompt and fails on error or on ok == false.
func mergeOK(t *testing.T, before string) string {
	t.Helper()
	after, ok, err := mergeStopPrompt([]byte(before), config.DefaultStopPrompt, config.DefaultStopMinLength)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	return string(after)
}

// scaffoldedConfig rebuilds the document a fully-accepted aigate init run
// produces before the stop-prompt step runs: the bare config header with
// claude.session-start and git.pre-commit spliced in by their own init
// steps. Used as the "before" fixture for tests exercising mergeStopPrompt
// against a realistic scaffolded document, so this file doesn't hardcode a
// second copy of those sections.
func scaffoldedConfig(t *testing.T) string {
	t.Helper()
	afterSessionStart, ok, err := mergeSessionStart([]byte(configHeader))
	if err != nil {
		t.Fatalf("unexpected error merging session-start: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true merging session-start")
	}
	afterPreCommit, ok, err := mergePreCommit(afterSessionStart)
	if err != nil {
		t.Fatalf("unexpected error merging pre-commit: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true merging pre-commit")
	}
	return string(afterPreCommit)
}

// assertOriginalLinesIntact fails unless every line of before appears in
// after in the same order. This is the core invariant of a splice: the merge
// may only ADD lines. It catches reindentation, dropped or relocated
// comments, reordered keys, and joined lines all at once.
func assertOriginalLinesIntact(t *testing.T, before, after string) {
	t.Helper()
	if before == "" {
		return
	}
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	ai := 0
	for _, bl := range beforeLines {
		found := false
		for ; ai < len(afterLines); ai++ {
			if afterLines[ai] == bl {
				found = true
				ai++
				break
			}
		}
		if !found {
			t.Fatalf("original line %q not found in order in merged output:\n%s", bl, after)
		}
	}
}

// assertParsesWithStopPrompt round-trips through config.Parse and asserts
// Enabled, Prompt, and MinLength.
func assertParsesWithStopPrompt(t *testing.T, after, wantPrompt string, wantMin int) {
	t.Helper()
	cfg, err := config.Parse([]byte(after))
	if err != nil {
		t.Fatalf("merged output failed to parse: %v", err)
	}
	if cfg.Claude == nil || cfg.Claude.StopPrompt == nil {
		t.Fatal("expected claude.stop-prompt to be present")
	}
	sp := cfg.Claude.StopPrompt
	if !sp.Enabled {
		t.Error("expected enabled to be true")
	}
	if sp.Prompt != wantPrompt {
		t.Errorf("expected prompt %q, got %q", wantPrompt, sp.Prompt)
	}
	if sp.MinLength != wantMin {
		t.Errorf("expected min-length %d, got %d", wantMin, sp.MinLength)
	}
}

func TestMergeStopPrompt_DefaultTemplateExactOutput(t *testing.T) {
	after := mergeOK(t, scaffoldedConfig(t))

	want := fmt.Sprintf(`# aigate configuration
# See: https://github.com/joelthompson/aigate

claude:
  session-start:
    context:
      - type: project-structure
        max_depth: 3
      # - type: shell
      #   command: "go-task --list"
      #   label: "Available tasks"
  stop-prompt:
    enabled: true
    # Fed back to Claude when it finishes responding.
    prompt: %s
    # Skip the recap when the reply is already shorter than this; 0 disables.
    min-length: %d

git:
  pre-commit:
    checks:
      - type: secrets-scan
`, strconv.Quote(config.DefaultStopPrompt), config.DefaultStopMinLength)

	if after != want {
		t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s", after, want)
	}
}

func TestMergeStopPrompt_DefaultTemplateParses(t *testing.T) {
	after := mergeOK(t, scaffoldedConfig(t))
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
}

func TestMergeStopPrompt_KeepsShellCommentBlockUnderContext(t *testing.T) {
	after := mergeOK(t, scaffoldedConfig(t))
	shellIdx := strings.Index(after, "# - type: shell")
	stopIdx := strings.Index(after, "stop-prompt:")
	if shellIdx == -1 || stopIdx == -1 {
		t.Fatalf("expected both markers present: shellIdx=%d stopIdx=%d", shellIdx, stopIdx)
	}
	if shellIdx >= stopIdx {
		t.Errorf("expected the shell comment block to stay above stop-prompt (still under context:)")
	}
}

func TestMergeStopPrompt_InsertsAtEndOfClaudeSection(t *testing.T) {
	before := `claude:
  session-start:
    context:
      - type: project-structure

git:
  pre-commit:
    checks:
      - type: secrets-scan
`
	after := mergeOK(t, before)
	sessionStartIdx := strings.Index(after, "- type: project-structure")
	stopIdx := strings.Index(after, "stop-prompt:")
	gitIdx := strings.Index(after, "\ngit:")
	if sessionStartIdx == -1 || stopIdx == -1 || gitIdx == -1 {
		t.Fatalf("expected all markers present")
	}
	if stopIdx <= sessionStartIdx {
		t.Errorf("expected stop-prompt after session-start content")
	}
	if stopIdx >= gitIdx {
		t.Errorf("expected stop-prompt before git:")
	}
	assertOriginalLinesIntact(t, before, after)
}

func TestMergeStopPrompt_InsertsBeforeTopLevelComment(t *testing.T) {
	before := `claude:
  session-start:
    context:
      - type: project-structure

# git hooks
git:
  pre-commit:
    checks:
      - type: secrets-scan
`
	after := mergeOK(t, before)
	stopIdx := strings.Index(after, "stop-prompt:")
	commentIdx := strings.Index(after, "# git hooks")
	if stopIdx == -1 || commentIdx == -1 {
		t.Fatalf("expected both markers present")
	}
	if stopIdx >= commentIdx {
		t.Errorf("expected stop-prompt before the top-level comment belonging to git:")
	}
	assertOriginalLinesIntact(t, before, after)
}

func TestMergeStopPrompt_InsertsAfterIndentedTrailingComment(t *testing.T) {
	before := `claude:
  session-start:
    context:
      - type: project-structure
      # trailing comment

git:
  pre-commit:
    checks:
      - type: secrets-scan
`
	after := mergeOK(t, before)
	commentIdx := strings.Index(after, "# trailing comment")
	stopIdx := strings.Index(after, "stop-prompt:")
	if commentIdx == -1 || stopIdx == -1 {
		t.Fatalf("expected both markers present")
	}
	if stopIdx <= commentIdx {
		t.Errorf("expected stop-prompt after the indented trailing comment (it belongs to claude:)")
	}
	assertOriginalLinesIntact(t, before, after)
}

func TestMergeStopPrompt_HandlesClaudeAsLastSection(t *testing.T) {
	before := `claude:
  session-start:
    context:
      - type: project-structure
`
	after := mergeOK(t, before)
	assertOriginalLinesIntact(t, before, after)
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
}

func TestMergeStopPrompt_InsertsBeforeTrailingBlankLines(t *testing.T) {
	before := "claude:\n  session-start:\n    context:\n      - type: project-structure\n\n\n"
	after := mergeOK(t, before)
	if !strings.HasSuffix(after, "\n\n\n") {
		t.Errorf("expected trailing blank lines preserved at the end, got:\n%q", after)
	}
	assertOriginalLinesIntact(t, before, after)
}

func TestMergeStopPrompt_IgnoresNestedClaudeKey(t *testing.T) {
	before := `claude:
  session-start:
    context:
      - type: project-structure

git:
  pre-commit:
    checks:
      - type: shell
        command: "claude --version"
        label: "nested claude: not a real key"
`
	after := mergeOK(t, before)
	stopIdx := strings.Index(after, "\n  stop-prompt:")
	gitIdx := strings.Index(after, "\ngit:")
	if stopIdx == -1 || gitIdx == -1 {
		t.Fatalf("expected both markers present")
	}
	if stopIdx >= gitIdx {
		t.Errorf("expected stop-prompt inserted under the top-level claude:, before git:")
	}
	assertOriginalLinesIntact(t, before, after)
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
}

func TestMergeStopPrompt_AppendsClaudeSectionWhenAbsent(t *testing.T) {
	before := `git:
  pre-commit:
    checks:
      - type: secrets-scan
`
	after := mergeOK(t, before)
	assertOriginalLinesIntact(t, before, after)
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
	if !strings.Contains(after, "\nclaude:\n") {
		t.Errorf("expected a claude: section to be appended, got:\n%s", after)
	}
	if !strings.Contains(after, "git:\n  pre-commit:\n    checks:\n      - type: secrets-scan\n") {
		t.Errorf("expected git section to be untouched, got:\n%s", after)
	}
}

func TestMergeStopPrompt_HandlesEmptyClaudeSection(t *testing.T) {
	before := "claude:\n\ngit:\n  pre-commit:\n    checks:\n      - type: secrets-scan\n"
	after := mergeOK(t, before)
	assertOriginalLinesIntact(t, before, after)
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
	if !strings.Contains(after, "\n  stop-prompt:\n") {
		t.Errorf("expected stop-prompt indented by 2 spaces (fallback), got:\n%s", after)
	}
}

func TestMergeStopPrompt_HandlesEmptyFile(t *testing.T) {
	after := mergeOK(t, "")
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
}

func TestMergeStopPrompt_HandlesWhitespaceOnlyFile(t *testing.T) {
	before := "   \n\n  \n"
	after := mergeOK(t, before)
	assertOriginalLinesIntact(t, before, after)
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
}

func TestMergeStopPrompt_HandlesCommentsOnlyFile(t *testing.T) {
	before := "# just a comment\n# another\n"
	after := mergeOK(t, before)
	assertOriginalLinesIntact(t, before, after)
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
}

func TestMergeStopPrompt_HandlesFileWithoutTrailingNewline(t *testing.T) {
	before := "claude:\n  session-start:\n    context:\n      - type: project-structure"
	after := mergeOK(t, before)
	if !strings.Contains(after, "- type: project-structure\n  stop-prompt:") {
		t.Errorf("expected the new section not to be joined onto the last line, got:\n%q", after)
	}
	assertOriginalLinesIntact(t, before, after)
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
}

func TestMergeStopPrompt_HandlesFourSpaceIndentedConfig(t *testing.T) {
	before := `claude:
    session-start:
        context:
            - type: project-structure

git:
    pre-commit:
        checks:
            - type: secrets-scan
`
	after := mergeOK(t, before)
	assertOriginalLinesIntact(t, before, after)
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
	if !strings.Contains(after, "\n    stop-prompt:\n") {
		t.Errorf("expected stop-prompt indented by 4 spaces to match document style, got:\n%s", after)
	}
}

func TestMergeStopPrompt_HandlesDocumentStartMarker(t *testing.T) {
	before := "---\nclaude:\n  session-start:\n    context:\n      - type: project-structure\n"
	after := mergeOK(t, before)
	assertOriginalLinesIntact(t, before, after)
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
}

func TestMergeStopPrompt_HandlesQuotedClaudeKey(t *testing.T) {
	before := "\"claude\":\n  session-start:\n    context:\n      - type: project-structure\n"
	after := mergeOK(t, before)
	assertOriginalLinesIntact(t, before, after)
	assertParsesWithStopPrompt(t, after, config.DefaultStopPrompt, config.DefaultStopMinLength)
}

func TestMergeStopPrompt_ReturnsNotOKWhenAlreadyPresent(t *testing.T) {
	before := `claude:
  stop-prompt:
    enabled: true
`
	after, ok, err := mergeStopPrompt([]byte(before), config.DefaultStopPrompt, config.DefaultStopMinLength)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when already present")
	}
	if string(after) != before {
		t.Errorf("expected output unchanged, got:\n%s", after)
	}
}

func TestMergeStopPrompt_ReturnsNotOKWhenPresentButDisabled(t *testing.T) {
	before := `claude:
  stop-prompt:
    enabled: false
`
	after, ok, err := mergeStopPrompt([]byte(before), config.DefaultStopPrompt, config.DefaultStopMinLength)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false: a deliberate enabled: false should not be re-added or flipped")
	}
	if string(after) != before {
		t.Errorf("expected output unchanged, got:\n%s", after)
	}
}

func TestMergeStopPrompt_ReturnsNotOKWhenPresentAtUnusualIndent(t *testing.T) {
	before := `claude:
    stop-prompt:
        enabled: true
`
	_, ok, err := mergeStopPrompt([]byte(before), config.DefaultStopPrompt, config.DefaultStopMinLength)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false: detection is via config.Parse, not text matching")
	}
}

func TestMergeStopPrompt_ErrorsOnUnparseableYAML(t *testing.T) {
	before := "{{not yaml"
	after, ok, err := mergeStopPrompt([]byte(before), config.DefaultStopPrompt, config.DefaultStopMinLength)
	if err == nil {
		t.Fatal("expected an error for unparseable YAML")
	}
	if ok {
		t.Error("expected ok=false")
	}
	if string(after) != before {
		t.Errorf("expected input returned unchanged, got:\n%s", after)
	}
}

func TestMergeStopPrompt_ErrorsOnUnknownField(t *testing.T) {
	before := `claude:
  sesion-start:
    context:
      - type: project-structure
`
	after, ok, err := mergeStopPrompt([]byte(before), config.DefaultStopPrompt, config.DefaultStopMinLength)
	if err == nil {
		t.Fatal("expected an error for an unknown field (typo)")
	}
	if ok {
		t.Error("expected ok=false")
	}
	if string(after) != before {
		t.Errorf("expected input returned unchanged, got:\n%s", after)
	}
}

func TestMergeStopPrompt_TwiceIsStable(t *testing.T) {
	after := mergeOK(t, scaffoldedConfig(t))

	second, ok, err := mergeStopPrompt([]byte(after), config.DefaultStopPrompt, config.DefaultStopMinLength)
	if err != nil {
		t.Fatalf("unexpected error on second merge: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false on second merge")
	}
	if string(second) != after {
		t.Errorf("expected second merge to leave output unchanged")
	}
}

// mergeCIContextOK runs mergeCIContext and fails on error or on ok == false.
func mergeCIContextOK(t *testing.T, before string) string {
	t.Helper()
	after, ok, err := mergeCIContext([]byte(before))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	return string(after)
}

func TestMergeCIContext_AppendsAtListIndentTwoSpace(t *testing.T) {
	before := `claude:
  session-start:
    context:
      - type: project-structure
        max_depth: 3
`
	after := mergeCIContextOK(t, before)
	assertOriginalLinesIntact(t, before, after)
	if !strings.Contains(after, "\n      - type: ci-info\n") {
		t.Errorf("expected the new entry indented to match existing list items, got:\n%s", after)
	}

	cfg, err := config.Parse([]byte(after))
	if err != nil {
		t.Fatalf("merged output failed to parse: %v", err)
	}
	found := false
	for _, p := range cfg.Claude.SessionStart.Context {
		if p.Type == "ci-info" {
			found = true
		}
	}
	if !found {
		t.Error("expected ci-info to be present in the merged config")
	}
}

func TestMergeCIContext_AppendsAtListIndentFourSpace(t *testing.T) {
	before := `claude:
    session-start:
        context:
            - type: project-structure
`
	after := mergeCIContextOK(t, before)
	assertOriginalLinesIntact(t, before, after)
	if !strings.Contains(after, "\n            - type: ci-info\n") {
		t.Errorf("expected the new entry indented to match the 4-space document, got:\n%s", after)
	}
}

func TestMergeCIContext_IdempotentWhenAlreadyPresent(t *testing.T) {
	before := `claude:
  session-start:
    context:
      - type: ci-info
`
	after, ok, err := mergeCIContext([]byte(before))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when ci-info is already present")
	}
	if string(after) != before {
		t.Errorf("expected output unchanged, got:\n%s", after)
	}
}

func TestMergeCIContext_ErrorsOnUnparseableYAML(t *testing.T) {
	before := "{{not yaml"
	after, ok, err := mergeCIContext([]byte(before))
	if err == nil {
		t.Fatal("expected an error for unparseable YAML")
	}
	if ok {
		t.Error("expected ok=false")
	}
	if string(after) != before {
		t.Errorf("expected input returned unchanged, got:\n%s", after)
	}
	if errors.Is(err, errNoContextList) {
		t.Error("expected a parse error, not errNoContextList")
	}
}

func TestMergeCIContext_NoContextListCases(t *testing.T) {
	cases := map[string]string{
		"missing claude":        "git:\n  pre-commit:\n    checks:\n      - type: secrets-scan\n",
		"missing session-start": "claude:\n  stop-prompt:\n    enabled: true\n",
		"missing context":       "claude:\n  session-start: {}\n",
		"null context":          "claude:\n  session-start:\n    context:\n",
		"flow-style context":    "claude:\n  session-start:\n    context: []\n",
	}
	for name, before := range cases {
		t.Run(name, func(t *testing.T) {
			after, ok, err := mergeCIContext([]byte(before))
			if !errors.Is(err, errNoContextList) {
				t.Fatalf("expected errNoContextList, got: %v", err)
			}
			if ok {
				t.Error("expected ok=false")
			}
			if string(after) != before {
				t.Errorf("expected input returned unchanged, got:\n%s", after)
			}
		})
	}
}

// mergeDockerContextOK runs mergeDockerContext and fails on error or on
// ok == false.
func mergeDockerContextOK(t *testing.T, before string) string {
	t.Helper()
	after, ok, err := mergeDockerContext([]byte(before))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	return string(after)
}

func TestMergeDockerContext_AppendsAtListIndentTwoSpace(t *testing.T) {
	before := `claude:
  session-start:
    context:
      - type: project-structure
        max_depth: 3
`
	after := mergeDockerContextOK(t, before)
	assertOriginalLinesIntact(t, before, after)
	if !strings.Contains(after, "\n      - type: docker-compose\n") {
		t.Errorf("expected the new entry indented to match existing list items, got:\n%s", after)
	}

	cfg, err := config.Parse([]byte(after))
	if err != nil {
		t.Fatalf("merged output failed to parse: %v", err)
	}
	found := false
	for _, p := range cfg.Claude.SessionStart.Context {
		if p.Type == "docker-compose" {
			found = true
		}
	}
	if !found {
		t.Error("expected docker-compose to be present in the merged config")
	}
}

func TestMergeDockerContext_IdempotentWhenAlreadyPresent(t *testing.T) {
	before := `claude:
  session-start:
    context:
      - type: docker-compose
`
	after, ok, err := mergeDockerContext([]byte(before))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when docker-compose is already present")
	}
	if string(after) != before {
		t.Errorf("expected output unchanged, got:\n%s", after)
	}
}

func TestMergeDockerContext_NoContextList(t *testing.T) {
	before := "claude:\n  session-start: {}\n"

	after, ok, err := mergeDockerContext([]byte(before))
	if !errors.Is(err, errNoContextList) {
		t.Fatalf("expected errNoContextList, got: %v", err)
	}
	if ok {
		t.Error("expected ok=false")
	}
	if string(after) != before {
		t.Errorf("expected input returned unchanged, got:\n%s", after)
	}
}

// TestMergeContextEntry_CIAndDockerCoexist is the point of parameterising
// the splice rather than cloning it: each merge only claims its own entry as
// "already present", so running both leaves both in the list.
func TestMergeContextEntry_CIAndDockerCoexist(t *testing.T) {
	before := `claude:
  session-start:
    context:
      - type: project-structure
`
	withCI := mergeCIContextOK(t, before)
	after := mergeDockerContextOK(t, withCI)

	assertOriginalLinesIntact(t, before, after)
	if !strings.Contains(after, "\n      - type: ci-info\n") {
		t.Errorf("expected the ci-info entry to survive the docker merge, got:\n%s", after)
	}
	if !strings.Contains(after, "\n      - type: docker-compose\n") {
		t.Errorf("expected the docker-compose entry to be appended, got:\n%s", after)
	}

	cfg, err := config.Parse([]byte(after))
	if err != nil {
		t.Fatalf("merged output failed to parse: %v", err)
	}
	var types []string
	for _, p := range cfg.Claude.SessionStart.Context {
		types = append(types, p.Type)
	}
	joined := strings.Join(types, ",")
	want := "project-structure,ci-info,docker-compose"
	if joined != want {
		t.Errorf("expected context types %q, got %q", want, joined)
	}
}

func TestFindBlockEnd_AtIndentZero(t *testing.T) {
	lines := strings.Split(`claude:
  session-start:
    context:
      - type: project-structure
git:
  pre-commit:
    checks:
      - type: secrets-scan
`, "\n")
	// claude: is on line 1 (index 0), 1-based line number 1.
	end := findBlockEnd(lines, 1, 0)
	if lines[end] != "git:" {
		t.Errorf("expected findBlockEnd to land on 'git:', got %q (index %d)", lines[end], end)
	}
}

func TestFindBlockEnd_AtIndentFour(t *testing.T) {
	lines := strings.Split(`claude:
  session-start:
    context:
      - type: project-structure
      # trailing comment
  stop-prompt:
    enabled: true
`, "\n")
	// context: is on line 3 (index 2), 1-based line number 3, indent 4.
	end := findBlockEnd(lines, 3, 4)
	if strings.TrimSpace(lines[end]) != "stop-prompt:" {
		t.Errorf("expected findBlockEnd to land on 'stop-prompt:', got %q (index %d)", lines[end], end)
	}
}

func TestFormatStopPromptSection_EscapesQuotesAndBackslashes(t *testing.T) {
	prompt := `he said "hi" \ bye`
	section := formatStopPromptSection(prompt, 400)
	sectionLines := strings.Split(strings.TrimSuffix(section, "\n"), "\n")

	doc := "claude:\n" + strings.Join(indentLines(sectionLines, 2), "\n") + "\n"

	cfg, err := config.Parse([]byte(doc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude == nil || cfg.Claude.StopPrompt == nil {
		t.Fatal("expected claude.stop-prompt to be present")
	}
	if cfg.Claude.StopPrompt.Prompt != prompt {
		t.Errorf("expected prompt %q, got %q", prompt, cfg.Claude.StopPrompt.Prompt)
	}
}
