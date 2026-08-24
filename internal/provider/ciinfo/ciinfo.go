// Package ciinfo provides session-start context describing a project's CI
// setup: a summary of its CircleCI config, plus — when the circleci CLI is
// installed — its version and a pointer to its own help. Only CircleCI is
// implemented today; the provider type is the generic "ci-info" so other CI
// systems can be added later without a config key rename.
package ciinfo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joelthompson/aigate/internal/provider"
	"gopkg.in/yaml.v3"
)

const label = "CI"

// maxConfigBytes skips oversized files. .circleci/ dirs in the wild contain
// pnpm-lock.yaml and other large non-config YAML; there is no reason to parse
// them on every session start.
const maxConfigBytes = 1 << 20

const circleciDirName = ".circleci"

// defaultContinuePath is where continuation/continue resolves a setup config
// to when the step doesn't declare configuration_path. path-filtering/filter
// has no equivalent default; it always requires config-path.
var defaultContinuePath = circleciPath("continue_config.yml")

type Provider struct {
	Root string

	// runCLI shells out to the circleci CLI. Injectable so tests never
	// depend on whether the machine running them has the CLI installed; nil
	// uses execCLI.
	runCLI cliRunner
}

// cliRunner returns p.runCLI, defaulting to execCLI.
func (p *Provider) cliRunner() cliRunner {
	if p.runCLI != nil {
		return p.runCLI
	}
	return execCLI
}

// rawConfig is the subset of a CircleCI config aigate cares about. Decoding
// is lenient (no KnownFields): anything else in the file is ignored.
type rawConfig struct {
	Version   any            `yaml:"version"` // 2.1 parses as float, "2.1" as string
	Setup     bool           `yaml:"setup"`
	Jobs      map[string]any `yaml:"jobs"`
	Workflows map[string]any `yaml:"workflows"`
}

// parsedFile pairs a config's typed fields with its raw node tree, the
// latter needed to hunt for continuation-path params that aren't part of
// rawConfig's shape.
type parsedFile struct {
	raw  rawConfig
	node *yaml.Node
}

func (p *Provider) Run(ctx context.Context) (*provider.Result, error) {
	root := p.Root
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}
		root = wd
	}

	circleciDir := filepath.Join(root, circleciDirName)
	info, err := os.Stat(circleciDir)
	if err != nil || !info.IsDir() {
		return &provider.Result{Label: label}, nil
	}

	names, err := listConfigCandidates(circleciDir)
	if err != nil {
		return &provider.Result{Label: label}, nil
	}

	if ctx.Err() != nil {
		return &provider.Result{Label: label}, nil
	}

	files, configLike := classifyConfigs(circleciDir, names)
	if len(configLike) == 0 {
		return &provider.Result{Label: label}, nil
	}

	entryName := pickEntry(files, configLike)
	var out string
	if entryName == "" {
		out = noEntryOutput(files, configLike)
	} else {
		out = entryOutput(root, files, configLike, entryName)
	}

	if cli := cliSection(ctx, p.cliRunner()); cli != "" {
		out += "\n" + cli
	}

	return &provider.Result{Label: label, Output: out}, nil
}

// Detected reports whether root has a CircleCI setup aigate can summarize.
// Exported so init's detector shares this exact predicate.
func Detected(root string) bool {
	circleciDir := filepath.Join(root, circleciDirName)
	info, err := os.Stat(circleciDir)
	if err != nil || !info.IsDir() {
		return false
	}
	names, err := listConfigCandidates(circleciDir)
	if err != nil {
		return false
	}
	_, configLike := classifyConfigs(circleciDir, names)
	return len(configLike) > 0
}

// listConfigCandidates returns the .yml/.yaml file names directly inside
// dir, sorted.
func listConfigCandidates(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// classifyConfigs loads each named file from dir and keeps the ones that
// look like CircleCI config. configLike preserves the sorted order of names.
func classifyConfigs(dir string, names []string) (files map[string]*parsedFile, configLike []string) {
	files = map[string]*parsedFile{}
	for _, name := range names {
		pf, ok := loadYAMLConfig(filepath.Join(dir, name))
		if !ok || !isConfigLike(pf.raw) {
			continue
		}
		files[name] = pf
		configLike = append(configLike, name)
	}
	return files, configLike
}

// loadYAMLConfig reads and lenently parses path. It reports false for
// anything that keeps it from being usable: too large, unreadable, empty, or
// not valid YAML.
func loadYAMLConfig(path string) (*parsedFile, bool) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() || fi.Size() > maxConfigBytes {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, false
	}
	if len(node.Content) == 0 {
		return nil, false
	}
	var raw rawConfig
	if err := node.Decode(&raw); err != nil {
		return nil, false
	}
	return &parsedFile{raw: raw, node: &node}, true
}

// isConfigLike rejects the non-config YAML that turns up in real .circleci/
// dirs (test runner configs, lockfiles, workspace manifests) by requiring at
// least one field only a CircleCI config would have. It deliberately does
// not key off version alone.
func isConfigLike(raw rawConfig) bool {
	return len(raw.Jobs) > 0 || len(raw.Workflows) > 0 || raw.Setup
}

// pickEntry chooses the config to treat as the pipeline entry point:
// config.yml if it's config-like, else the sole config-like file, else none.
func pickEntry(files map[string]*parsedFile, configLike []string) string {
	if _, ok := files["config.yml"]; ok {
		return "config.yml"
	}
	if len(configLike) == 1 {
		return configLike[0]
	}
	return ""
}

func circleciPath(name string) string {
	return circleciDirName + "/" + name
}

// entryOutput assembles the pointer-only summary once an entry config has
// been chosen.
func entryOutput(root string, files map[string]*parsedFile, configLike []string, entryName string) string {
	entry := files[entryName]
	entryPath := circleciPath(entryName)

	usedNames := map[string]bool{entryName: true}

	header := "CircleCI · " + entryPath
	if entry.raw.Version != nil {
		header += fmt.Sprintf(" · version %v", entry.raw.Version)
	}

	var lines []string

	if entry.raw.Setup {
		header += " · dynamic setup config"
		lines = append(lines, header)

		target, declared := resolveContinuationTarget(entry.node)
		if name, ok := configInCircleCIDir(target); ok {
			usedNames[name] = true
		}

		if targetFile, found := loadYAMLConfig(filepath.Join(root, filepath.FromSlash(target))); found {
			workflows, jobs := counts(targetFile.raw)
			lines = append(lines, fmt.Sprintf("Continues into %s · %s. Read the files for detail.", target, countsPhrase(workflows, jobs)))
		} else if declared {
			lines = append(lines, fmt.Sprintf("Continues into %s (not in this repo).", target))
		} else {
			lines = append(lines, fmt.Sprintf("Continues into %s (default path; not declared in config, not in this repo).", target))
		}
	} else {
		header += " · static config"
		lines = append(lines, header)
		workflows, jobs := counts(entry.raw)
		lines = append(lines, fmt.Sprintf("%s. Read the file for detail.", countsPhrase(workflows, jobs)))
	}

	var others []string
	for _, name := range configLike {
		if !usedNames[name] {
			others = append(others, name)
		}
	}
	if len(others) > 0 {
		lines = append(lines, "Other configs in "+circleciDirName+"/: "+strings.Join(others, ", "))
	}

	return strings.Join(lines, "\n")
}

// noEntryOutput assembles output for a .circleci/ with no usable config.yml
// and more than one candidate, so the pipeline entry point must be set in
// CircleCI's project settings rather than being discoverable from the repo.
func noEntryOutput(files map[string]*parsedFile, configLike []string) string {
	parts := make([]string, len(configLike))
	for i, name := range configLike {
		part := name
		if files[name].raw.Setup {
			part += " (dynamic setup config)"
		}
		parts[i] = part
	}
	return "CircleCI · " + circleciDirName + "/ has no config.yml; the pipeline entry point is set project-side.\n" +
		"Configs present: " + strings.Join(parts, ", ")
}

// resolveContinuationTarget returns where entryNode's setup config continues
// into: the declared configuration_path/config-path if the step tree
// declares one, else defaultContinuePath.
func resolveContinuationTarget(entryNode *yaml.Node) (target string, declared bool) {
	if p := findContinuationPath(entryNode); p != "" {
		return p, true
	}
	return defaultContinuePath, false
}

// findContinuationPath recursively walks n for a mapping entry keyed
// configuration_path (the continuation/continue step param) or config-path
// (the path-filtering/filter job param) with a scalar value. A generic walk
// covers both orbs plus hand-rolled jobs without hardcoding orb names or
// step shapes.
func findContinuationPath(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			val := n.Content[i+1]
			if key.Kind == yaml.ScalarNode && val.Kind == yaml.ScalarNode &&
				(key.Value == "configuration_path" || key.Value == "config-path") {
				return val.Value
			}
		}
	}
	for _, c := range n.Content {
		if found := findContinuationPath(c); found != "" {
			return found
		}
	}
	return ""
}

// configInCircleCIDir reports whether target names a file directly inside
// .circleci/, returning its bare name. Used to exclude a resolved
// continuation target from the "Other configs" listing.
func configInCircleCIDir(target string) (name string, ok bool) {
	prefix := circleciDirName + "/"
	if !strings.HasPrefix(target, prefix) {
		return "", false
	}
	name = strings.TrimPrefix(target, prefix)
	if strings.ContainsAny(name, "/\\") {
		return "", false
	}
	return name, true
}

// counts returns workflow and job counts for raw. Workflows subtracts a
// legacy top-level version key when present: CircleCI 2.0 configs put
// version: 2 inside workflows:, and counting it as a workflow would be wrong.
func counts(raw rawConfig) (workflows, jobs int) {
	workflows = len(raw.Workflows)
	if _, ok := raw.Workflows["version"]; ok {
		workflows--
	}
	jobs = len(raw.Jobs)
	return workflows, jobs
}

func countsPhrase(workflows, jobs int) string {
	return fmt.Sprintf("%s, %s", pluralize(workflows, "workflow"), pluralize(jobs, "job"))
}

func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
