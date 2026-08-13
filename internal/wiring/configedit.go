package wiring

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joelthompson/aigate/internal/config"
	"gopkg.in/yaml.v3"
)

// sessionStartSection is the claude.session-start block inserted into
// .aigate.yml by the Claude-hooks init step. It mirrors the section the
// config template used to ship inline.
const sessionStartSection = `session-start:
  context:
    - type: project-structure
      max_depth: 3
    # - type: shell
    #   command: "go-task --list"
    #   label: "Available tasks"
`

// preCommitSection is the git.pre-commit block inserted into .aigate.yml by
// the git-hook init step.
const preCommitSection = `pre-commit:
  checks:
    - type: secrets-scan
`

// ciContextEntry is the list item spliced into claude.session-start.context
// by the CI-context init step.
const ciContextEntry = "- type: ci-info"

// errNoContextList is returned by mergeCIContext when
// claude.session-start.context doesn't already exist as a block-style
// sequence to splice an item into.
var errNoContextList = errors.New(`claude.session-start.context is not a list`)

// stopPromptSection is the claude.stop-prompt block inserted into .aigate.yml.
// The prompt is written via strconv.Quote so any future prompt containing a
// literal " or \ still round-trips as a valid YAML double-quoted scalar.
const stopPromptSection = `stop-prompt:
  enabled: true
  # Fed back to Claude when it finishes responding.
  prompt: %s
  # Skip the recap when the reply is already shorter than this; 0 disables.
  min-length: %d
`

func formatStopPromptSection(prompt string, minLength int) string {
	return fmt.Sprintf(stopPromptSection, strconv.Quote(prompt), minLength)
}

func sessionStartPresent(cfg *config.Config) bool {
	return cfg.Claude != nil && cfg.Claude.SessionStart != nil
}

func stopPromptPresent(cfg *config.Config) bool {
	return cfg.Claude != nil && cfg.Claude.StopPrompt != nil
}

func preCommitPresent(cfg *config.Config) bool {
	return cfg.Git != nil && cfg.Git.PreCommit != nil
}

// ciContextPresent reports whether claude.session-start.context already has
// a ci-info entry.
func ciContextPresent(cfg *config.Config) bool {
	if cfg.Claude == nil || cfg.Claude.SessionStart == nil {
		return false
	}
	for _, p := range cfg.Claude.SessionStart.Context {
		if p.Type == "ci-info" {
			return true
		}
	}
	return false
}

// mergeSessionStart returns data with a claude.session-start section
// inserted. See mergeSection for the ok/ordering contract.
func mergeSessionStart(data []byte) ([]byte, bool, error) {
	return mergeSection(data, "claude", sessionStartSection, sessionStartPresent)
}

// mergeStopPrompt returns data with a claude.stop-prompt section inserted.
// See mergeSection for the ok/ordering contract.
func mergeStopPrompt(data []byte, prompt string, minLength int) ([]byte, bool, error) {
	return mergeSection(data, "claude", formatStopPromptSection(prompt, minLength), stopPromptPresent)
}

// mergePreCommit returns data with a git.pre-commit section inserted. See
// mergeSection for the ok/ordering contract.
func mergePreCommit(data []byte) ([]byte, bool, error) {
	return mergeSection(data, "git", preCommitSection, preCommitPresent)
}

// mergeCIContext returns data with a "- type: ci-info" item appended to the
// claude.session-start.context list. Unlike mergeSection, the target here is
// a list item nested inside a top-level block, not a second-level block
// under a top-level key, so it walks the node tree itself rather than
// delegating to mergeSection.
func mergeCIContext(data []byte) ([]byte, bool, error) {
	cfg, err := config.Parse(data)
	if err != nil {
		return data, false, err
	}
	if ciContextPresent(cfg) {
		return data, false, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return data, false, err
	}

	root := rootMapping(&doc)
	if root == nil {
		return data, false, errNoContextList
	}
	_, claudeVal := mappingEntry(root, "claude")
	if claudeVal == nil || claudeVal.Kind != yaml.MappingNode {
		return data, false, errNoContextList
	}
	_, sessionStartVal := mappingEntry(claudeVal, "session-start")
	if sessionStartVal == nil || sessionStartVal.Kind != yaml.MappingNode {
		return data, false, errNoContextList
	}
	contextKey, contextVal := mappingEntry(sessionStartVal, "context")
	if contextKey == nil || contextVal == nil || contextVal.Kind != yaml.SequenceNode || contextVal.Style == yaml.FlowStyle {
		return data, false, errNoContextList
	}

	lines := strings.Split(string(data), "\n")

	indent := contextKey.Column - 1 + 2
	if len(contextVal.Content) > 0 {
		first := contextVal.Content[0]
		indent = leadingSpaces(lines[first.Line-1])
	}

	insertAt := findBlockEnd(lines, contextKey.Line, contextKey.Column-1)
	entryLines := indentLines([]string{ciContextEntry}, indent)

	out := make([]string, 0, len(lines)+len(entryLines))
	out = append(out, lines[:insertAt]...)
	out = append(out, entryLines...)
	out = append(out, lines[insertAt:]...)

	return []byte(strings.Join(out, "\n")), true, nil
}

// mergeSection returns data with section inserted at the end of the topKey
// block (or a fresh topKey block appended, if topKey is absent). ok is false
// when present(cfg) reports the section is already there, in which case data
// is returned unchanged.
//
// This locates insertion points in a parsed yaml.Node tree and splices raw
// lines, rather than re-marshaling the document, so existing formatting and
// comments survive byte for byte. Lines are split on "\n"; CRLF input is out
// of scope.
func mergeSection(data []byte, topKey string, section string, present func(*config.Config) bool) ([]byte, bool, error) {
	cfg, err := config.Parse(data)
	if err != nil {
		return data, false, err
	}
	if present(cfg) {
		return data, false, nil
	}

	sectionLines := strings.Split(strings.TrimSuffix(section, "\n"), "\n")

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return data, false, err
	}

	root := rootMapping(&doc)
	if root == nil {
		return appendTopLevelSection(data, topKey, sectionLines), true, nil
	}

	topIdx := -1
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == topKey {
			topIdx = i
			break
		}
	}
	if topIdx == -1 {
		return appendTopLevelSection(data, topKey, sectionLines), true, nil
	}

	keyNode := root.Content[topIdx]
	valNode := root.Content[topIdx+1]

	indent := 2
	if valNode.Kind == yaml.MappingNode && len(valNode.Content) > 0 {
		indent = valNode.Content[0].Column - 1
	}

	lines := strings.Split(string(data), "\n")
	insertAt := findSectionEnd(lines, keyNode.Line)

	out := make([]string, 0, len(lines)+len(sectionLines))
	out = append(out, lines[:insertAt]...)
	out = append(out, indentLines(sectionLines, indent)...)
	out = append(out, lines[insertAt:]...)

	return []byte(strings.Join(out, "\n")), true, nil
}

// rootMapping returns the document's root mapping node, or nil if the
// document has no content (empty, whitespace-only, or comments-only) or its
// root is not a mapping.
func rootMapping(doc *yaml.Node) *yaml.Node {
	if len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	return root
}

// mappingEntry returns the key and value nodes for key in mapping m, or nil,
// nil if m isn't a mapping or has no such key.
func mappingEntry(m *yaml.Node, key string) (keyNode, valNode *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// findBlockEnd scans forward from keyLine (the 1-based line number of a
// key) to the first non-blank line indented no deeper than keyIndent — the
// next sibling key, or a comment at the same indent that belongs to it —
// then backs up over any trailing blank lines. The result is a 0-based
// index into lines: the point at which to insert new content, which places
// it after any deeper-indented trailing comments belonging to the block.
func findBlockEnd(lines []string, keyLine, keyIndent int) int {
	end := len(lines)
	for i := keyLine; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			continue
		}
		if leadingSpaces(line) <= keyIndent {
			end = i
			break
		}
	}
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end
}

// findSectionEnd scans forward from topLine (the 1-based line number of the
// top-level key) to the first line whose first character is non-whitespace —
// the next top-level key, or a column-1 comment that belongs to it — then
// backs up over any trailing blank lines. The result is a 0-based index into
// lines: the point at which to insert the new section, which places it after
// any indented trailing comments belonging to the block.
func findSectionEnd(lines []string, topLine int) int {
	return findBlockEnd(lines, topLine, 0)
}

// leadingSpaces counts line's leading space/tab characters.
func leadingSpaces(line string) int {
	n := 0
	for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		n++
	}
	return n
}

func indentLines(lines []string, n int) []string {
	prefix := strings.Repeat(" ", n)
	out := make([]string, len(lines))
	for i, l := range lines {
		if l == "" {
			out[i] = l
			continue
		}
		out[i] = prefix + l
	}
	return out
}

// appendTopLevelSection appends a fresh key: section, with sectionLines
// nested under it, at the end of data. Used when the document has no key: at
// all, so there is no risk of a duplicate key.
func appendTopLevelSection(data []byte, key string, sectionLines []string) []byte {
	s := string(data)
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	if s != "" {
		s += "\n"
	}
	s += key + ":\n"
	s += strings.Join(indentLines(sectionLines, 2), "\n") + "\n"
	return []byte(s)
}

// EnableSessionStart adds the claude.session-start section to .aigate.yml.
func EnableSessionStart(configPath string) StepResult {
	return editConfig(configPath, mergeSessionStart,
		"added claude.session-start to .aigate.yml",
		"claude.session-start already set in .aigate.yml")
}

// EnableStopPrompt adds the claude.stop-prompt section to .aigate.yml.
func EnableStopPrompt(configPath string) StepResult {
	return editConfig(configPath, func(data []byte) ([]byte, bool, error) {
		return mergeStopPrompt(data, config.DefaultStopPrompt, config.DefaultStopMinLength)
	}, "added claude.stop-prompt to .aigate.yml",
		"claude.stop-prompt already set in .aigate.yml")
}

// EnablePreCommitChecks adds the git.pre-commit section to .aigate.yml.
func EnablePreCommitChecks(configPath string) StepResult {
	return editConfig(configPath, mergePreCommit,
		"added git.pre-commit to .aigate.yml",
		"git.pre-commit already set in .aigate.yml")
}

// EnableCIContext adds a ci-info entry to claude.session-start.context in
// .aigate.yml. If the context list doesn't exist in a splice-able shape,
// this is reported as a skip rather than an error, since the fix is a
// one-line manual edit rather than something worth failing init over.
func EnableCIContext(configPath string) StepResult {
	res := editConfig(configPath, mergeCIContext,
		"added ci-info to claude.session-start.context",
		"ci-info already in claude.session-start.context")
	if res.Err != nil && errors.Is(res.Err, errNoContextList) {
		return StepResult{Skipped: true, Reason: `claude.session-start.context not found (add "- type: ci-info" by hand)`}
	}
	return res
}

// editConfig reads configPath, runs merge over its contents, and writes the
// result back. It reports the shared skip/error cases so each Enable*
// function only needs to supply its merge func and result strings.
func editConfig(configPath string, merge func([]byte) ([]byte, bool, error), action, alreadyReason string) StepResult {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return StepResult{Skipped: true, Reason: fmt.Sprintf("%s not found (rerun with --config to create one)", filepath.Base(configPath))}
		}
		return StepResult{Err: fmt.Errorf("reading %s: %w", filepath.Base(configPath), err)}
	}

	merged, ok, err := merge(data)
	if err != nil {
		return StepResult{Err: fmt.Errorf("parsing %s: %w", filepath.Base(configPath), err)}
	}
	if !ok {
		return StepResult{Skipped: true, Reason: alreadyReason}
	}

	if err := os.WriteFile(configPath, merged, 0644); err != nil {
		return StepResult{Err: fmt.Errorf("writing %s: %w", filepath.Base(configPath), err)}
	}

	return StepResult{Action: action}
}
