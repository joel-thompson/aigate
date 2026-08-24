package wiring

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/joelthompson/aigate/internal/config"
	"github.com/joelthompson/aigate/internal/provider/ciinfo"
	"github.com/joelthompson/aigate/internal/provider/dockercompose"
)

// settingsRelPath is settings.local.json's location relative to the project
// root, used in check details and artifact descriptions.
const settingsRelPath = ".claude/settings.local.json"

// configStatus is the tri-state result of reading and parsing .aigate.yml:
// absent, present but unparseable, or present and parsed. Doctor needs this
// distinction to report a parse error once instead of as one missing
// section per feature; configSectionInstalled (detect.go) collapses it back
// to a bool for the write-path parity checks.
type configStatus struct {
	exists bool
	err    error          // set when the file exists but cannot be read or parsed
	cfg    *config.Config // set only when exists && err == nil
}

func loadConfigStatus(path string) configStatus {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return configStatus{}
		}
		return configStatus{exists: true, err: err}
	}
	cfg, err := config.Parse(data)
	if err != nil {
		return configStatus{exists: true, err: err}
	}
	return configStatus{exists: true, cfg: cfg}
}

// settingsStatus mirrors configStatus for .claude/settings.local.json. A
// missing file is not an error (the hook halves report that accurately),
// but a malformed one makes Claude Code ignore every hook in the file,
// which the per-feature checks can't see on their own.
type settingsStatus struct {
	exists bool
	err    error
}

func loadSettingsStatus(path string) settingsStatus {
	exists := true
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			exists = false
		}
	}
	if _, err := readOrCreateSettings(path); err != nil {
		return settingsStatus{exists: exists, err: err}
	}
	return settingsStatus{exists: exists}
}

// artifact is one half of a feature: a single thing aigate installs, named
// in the vocabulary the user sees in the file itself, plus where it lives.
type artifact struct {
	// key is the state field this reports on. It must match that field's Go
	// name exactly, or TestInspect_ReportsOnEveryStateField fails loudly
	// (one field missing from Check.Keys, one stale key left over).
	key     string
	what    string
	path    string
	present bool
	unknown bool // the file this lives in exists but doesn't parse
}

// feature is a user-facing capability: an external registration plus the
// .aigate.yml section that gives it something to do. external is the zero
// value for a section-only feature (CI context has no external half).
type feature struct {
	label        string
	external     artifact
	section      artifact
	remedy       string // the aigate init invocation that installs both halves
	inapplicable string // non-empty reason ⇒ can never be a problem
}

func (f feature) hasExternal() bool {
	return f.external.key != ""
}

func (f feature) keys() []string {
	if !f.hasExternal() {
		return []string{f.section.key}
	}
	return []string{f.external.key, f.section.key}
}

// status is the mismatch rule: both halves present is ok; neither is not
// configured (declined at init, informational); exactly one is a problem;
// either half unknown (its file didn't parse) beats present/absent; a
// declared-inapplicable feature is never a problem regardless.
func (f feature) status() Status {
	if f.inapplicable != "" {
		return StatusNotApplicable
	}
	if f.section.unknown || (f.hasExternal() && f.external.unknown) {
		return StatusUnknown
	}
	if !f.hasExternal() {
		if f.section.present {
			return StatusOK
		}
		return StatusNotConfigured
	}
	switch {
	case f.external.present && f.section.present:
		return StatusOK
	case !f.external.present && !f.section.present:
		return StatusNotConfigured
	default:
		return StatusProblem
	}
}

func (f feature) check() Check {
	c := Check{Label: f.label, Keys: f.keys(), Status: f.status()}
	switch c.Status {
	case StatusNotApplicable:
		c.Detail = f.inapplicable
	case StatusUnknown:
		c.Detail = f.unknownDetail()
	case StatusNotConfigured:
		c.Detail = f.notConfiguredDetail()
	case StatusProblem:
		c.Detail, c.Path = f.problemDetail()
		c.Remedy = f.remedy
	}
	return c
}

func (f feature) notConfiguredDetail() string {
	if !f.hasExternal() {
		return fmt.Sprintf("no %s", f.section.what)
	}
	return fmt.Sprintf("no %s, no %s", f.external.what, f.section.what)
}

func (f feature) unknownDetail() string {
	switch {
	case !f.hasExternal():
		return "cannot check .aigate.yml until it parses"
	case f.external.unknown && f.section.unknown:
		return fmt.Sprintf("cannot check .aigate.yml or %s until they parse", settingsRelPath)
	case f.section.unknown:
		if f.external.present {
			return fmt.Sprintf("%s is present; cannot check .aigate.yml until it parses", f.external.what)
		}
		return "cannot check .aigate.yml until it parses"
	default: // f.external.unknown
		if f.section.present {
			return fmt.Sprintf("%s is present; cannot check %s until it parses", f.section.what, settingsRelPath)
		}
		return fmt.Sprintf("cannot check %s until it parses", settingsRelPath)
	}
}

// problemDetail names the present half, the missing half, and what the
// mismatch means in practice, and returns the missing half's path.
func (f feature) problemDetail() (detail, path string) {
	have, missing := f.external, f.section
	consequence := "the hook fires but has nothing to run"
	if !f.external.present {
		have, missing = f.section, f.external
		consequence = "nothing ever calls it"
	}
	return fmt.Sprintf("%s is present, but %s is missing; %s", have.what, missing.what, consequence), missing.path
}

// Applicability records which detected-feature checks apply to a project at
// all, so a feature that can never be a problem here reports "not
// applicable" rather than "not configured".
type Applicability struct {
	CI            bool
	DockerCompose bool
}

// snapshot is everything both doctor's report and init's completeness check
// derive from.
type snapshot struct {
	state           state
	paths           paths
	applicable      Applicability
	configUnknown   bool
	settingsUnknown bool
}

// features maps detected state onto the pairs doctor reports on. This is
// the single place a new state field must be wired in to appear in
// doctor's report or init's "everything is already set up" check.
func (s snapshot) features() []feature {
	return []feature{
		{
			label: "Session context",
			external: artifact{
				key:     "SessionHook",
				what:    "SessionStart hook in " + settingsRelPath,
				path:    s.paths.settings,
				present: s.state.SessionHook,
				unknown: s.settingsUnknown,
			},
			section: artifact{
				key:     "SessionContext",
				what:    "claude.session-start section in " + config.DefaultConfigFile,
				path:    s.paths.config,
				present: s.state.SessionContext,
				unknown: s.configUnknown,
			},
			remedy: "aigate init --claude-hooks",
		},
		{
			label: "CI context",
			section: artifact{
				key:     "CIContext",
				what:    "ci-info entry in claude.session-start.context",
				path:    s.paths.config,
				present: s.state.CIContext,
				unknown: s.configUnknown,
			},
			inapplicable: ciInapplicableReason(s.applicable.CI),
		},
		{
			label: "Stop prompt",
			external: artifact{
				key:     "StopHook",
				what:    "Stop hook in " + settingsRelPath,
				path:    s.paths.settings,
				present: s.state.StopHook,
				unknown: s.settingsUnknown,
			},
			section: artifact{
				key:     "StopPrompt",
				what:    "claude.stop-prompt section in " + config.DefaultConfigFile,
				path:    s.paths.config,
				present: s.state.StopPrompt,
				unknown: s.configUnknown,
			},
			remedy: "aigate init --stop-prompt",
		},
		{
			label: "Pre-commit checks",
			external: artifact{
				key:     "GitHook",
				what:    "aigate line in .git/hooks/pre-commit",
				path:    s.paths.gitHook,
				present: s.state.GitHook,
			},
			section: artifact{
				key:     "PreCommitChecks",
				what:    "git.pre-commit section in " + config.DefaultConfigFile,
				path:    s.paths.config,
				present: s.state.PreCommitChecks,
				unknown: s.configUnknown,
			},
			remedy: "aigate init --git-hooks",
		},
		// Appended last on purpose: doctor's tests address checks by index,
		// so a new feature slotted in ahead of one silently retargets them.
		{
			label: "Docker context",
			section: artifact{
				key:     "DockerContext",
				what:    "docker-compose entry in claude.session-start.context",
				path:    s.paths.config,
				present: s.state.DockerContext,
				unknown: s.configUnknown,
			},
			inapplicable: dockerInapplicableReason(s.applicable.DockerCompose),
		},
	}
}

func ciInapplicableReason(ciApplicable bool) string {
	if ciApplicable {
		return ""
	}
	return "no .circleci directory"
}

func dockerInapplicableReason(dockerApplicable bool) string {
	if dockerApplicable {
		return ""
	}
	return "no compose file in the project root"
}

// Status is a Check's outcome.
type Status string

const (
	StatusOK            Status = "ok"
	StatusProblem       Status = "PROBLEM"
	StatusNotConfigured Status = "not configured" // neither half; declined at init
	StatusNotApplicable Status = "not applicable"
	StatusUnknown       Status = "unknown" // the file the answer lives in doesn't parse
)

// Check is one line of a Report: either a feature pair or a standalone
// check on one of the files the pairs live in (the binary itself, or
// .aigate.yml/settings.local.json's own readability).
type Check struct {
	Label  string
	Keys   []string // state fields this check covers, so coverage is provable
	Status Status
	Detail string // one line, empty on an unremarkable ok
	Path   string // absolute path Detail refers to, when relevant
	Remedy string // set only when an init flag actually fixes it
}

// Report is aigate doctor's full output for one project root.
type Report struct {
	Root   string
	Checks []Check
}

// Problems returns the checks with StatusProblem, in report order.
func (r Report) Problems() []Check {
	var out []Check
	for _, c := range r.Checks {
		if c.Status == StatusProblem {
			out = append(out, c)
		}
	}
	return out
}

// OK reports whether the report found no problems.
func (r Report) OK() bool {
	return len(r.Problems()) == 0
}

// Inspect is the read-only counterpart to runInit: it reports the wiring
// state of root without changing anything. lookPath resolves the aigate
// binary the hooks invoke by bare name; nil uses exec.LookPath. Injectable
// so tests never depend on whether the machine running them has aigate
// installed.
func Inspect(root string, lookPath func(string) (string, error)) (Report, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	p, err := pathsFor(root)
	if err != nil {
		return Report{}, err
	}

	cfgStatus := loadConfigStatus(p.config)
	setStatus := loadSettingsStatus(p.settings)

	snap := snapshot{
		state: detectStateAt(p),
		paths: p,
		applicable: Applicability{
			CI:            ciinfo.Detected(p.root),
			DockerCompose: dockercompose.Detected(p.root),
		},
		configUnknown:   cfgStatus.exists && cfgStatus.err != nil,
		settingsUnknown: setStatus.exists && setStatus.err != nil,
	}

	checks := []Check{binaryCheck(lookPath), configCheck(cfgStatus, p), settingsCheck(setStatus, p)}
	for _, f := range snap.features() {
		checks = append(checks, f.check())
	}

	return Report{Root: p.root, Checks: checks}, nil
}

func binaryCheck(lookPath func(string) (string, error)) Check {
	path, err := lookPath(binaryName)
	if err != nil {
		return Check{
			Label:  "Binary",
			Status: StatusProblem,
			Detail: fmt.Sprintf("%q not found on $PATH", binaryName),
		}
	}
	return Check{
		Label:  "Binary",
		Status: StatusOK,
		Detail: fmt.Sprintf("%s resolves to %s", binaryName, path),
	}
}

func configCheck(st configStatus, p paths) Check {
	c := Check{Label: "Config", Keys: []string{"Config"}}
	switch {
	case !st.exists:
		c.Status = StatusProblem
		c.Detail = fmt.Sprintf("%s does not exist, so nothing aigate registers has anything to run", config.DefaultConfigFile)
		c.Path = p.config
		c.Remedy = "aigate init --config"
	case st.err != nil:
		c.Status = StatusProblem
		c.Detail = fmt.Sprintf("%s exists but does not parse: %v", config.DefaultConfigFile, st.err)
		c.Path = p.config
	default:
		c.Status = StatusOK
		c.Detail = config.DefaultConfigFile
	}
	return c
}

func settingsCheck(st settingsStatus, p paths) Check {
	c := Check{Label: "Claude settings"}
	switch {
	case !st.exists:
		c.Status = StatusNotConfigured
		c.Detail = fmt.Sprintf("%s does not exist", settingsRelPath)
	case st.err != nil:
		c.Status = StatusProblem
		c.Detail = fmt.Sprintf("%s exists but does not parse: %v", settingsRelPath, st.err)
		c.Path = p.settings
	default:
		c.Status = StatusOK
		c.Detail = settingsRelPath
	}
	return c
}

// Complete reports whether every artifact init writes is already in place:
// the config file exists and every feature pair is either fully wired or
// not applicable. This is the conjunction runInit's "everything is already
// set up" check derives from, so init and doctor can't structurally
// disagree about what "done" means.
func Complete(s state, ap Applicability) bool {
	if !s.Config {
		return false
	}
	snap := snapshot{state: s, applicable: ap}
	for _, f := range snap.features() {
		switch f.status() {
		case StatusOK, StatusNotApplicable:
			continue
		default:
			return false
		}
	}
	return true
}
