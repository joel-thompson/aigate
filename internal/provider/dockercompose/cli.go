package dockercompose

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"
)

const cliBinary = "docker"

// cliTimeout matches ciinfo's budget and applies per call rather than as a
// shared pool, so a slow `config` can't eat `ps`'s allowance and make the
// running-state answer depend on timing.
const cliTimeout = 3 * time.Second

// cliRunner shells out to the docker CLI. Injectable so tests never depend
// on whether the machine running them has docker installed. dir is the
// directory to run in: compose resolves its file relative to the working
// directory, and the hook's own cwd isn't necessarily the project root.
type cliRunner func(ctx context.Context, dir string, args ...string) (string, error)

// execCLI is the real cliRunner. It captures stdout only, so compose's
// stderr warnings — an obsolete `version:` key, or an interpolation warning
// that could echo a resolved value — never reach Claude's context.
func execCLI(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, cliBinary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
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

// composeArgs is the argv prefix shared by the config probe, the ps probe,
// and the rendered Start line, so the three can never disagree about which
// compose files are in play.
func (p *Provider) composeArgs() []string {
	args := []string{"compose"}
	for _, f := range p.Files {
		args = append(args, "-f", f)
	}
	return args
}

// startCommand is the command aigate tells Claude to run to bring the stack
// up. It is built from composeArgs so it is provably the same invocation
// aigate itself queried.
func (p *Provider) startCommand() string {
	return cliBinary + " " + strings.Join(append(p.composeArgs(), "up", "-d"), " ")
}

// composeConfig is the subset of `compose config --format json` aigate
// reports on.
type composeConfig struct {
	Name     string                    `json:"name"`
	Services map[string]composeService `json:"services"`
}

// composeService is the subset of a compose service aigate reports on.
// `environment` is deliberately absent: config --format json resolves it in
// full, credentials included, and nothing here should be able to print it.
type composeService struct {
	Image string        `json:"image"`
	Build *composeBuild `json:"build"`
	Ports []composePort `json:"ports"`
}

type composeBuild struct {
	// Context comes back absolute and must be relativised before printing.
	Context string `json:"context"`
}

type composePort struct {
	HostIP    string `json:"host_ip"`
	Target    int    `json:"target"`
	Published string `json:"published"` // "5432", "8000-8002", or "" for random
	Protocol  string `json:"protocol"`
}

// psService is the subset of one `compose ps --format json` record aigate
// reports on.
type psService struct {
	Service    string        `json:"Service"`
	State      string        `json:"State"`
	Health     string        `json:"Health"`
	Image      string        `json:"Image"`
	Publishers []psPublisher `json:"Publishers"`
}

type psPublisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

// queryConfig parses the stack without needing the daemon: `compose config`
// is a pure local parse and succeeds with docker not running. It collapses
// every failure to ok=false so no *exec.ExitError — and so none of the child
// stderr it carries — can escape this file.
func (p *Provider) queryConfig(ctx context.Context, dir string) (*composeConfig, bool) {
	ctx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()

	args := append(p.composeArgs(), "config", "--format", "json")
	out, err := p.cliRunner()(ctx, dir, args...)
	if err != nil {
		return nil, false
	}
	var cfg composeConfig
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		return nil, false
	}
	return &cfg, true
}

// queryPS lists the running services. It fails when the daemon is
// unreachable, which is why its failure is a distinct rung of the ladder
// rather than fatal: the stack is still worth describing, just without the
// running/not-running split. Failures collapse to ok=false for the same
// stderr-containment reason as queryConfig.
func (p *Provider) queryPS(ctx context.Context, dir string) ([]psService, bool) {
	ctx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()

	args := append(p.composeArgs(), "ps", "--format", "json")
	out, err := p.cliRunner()(ctx, dir, args...)
	if err != nil {
		return nil, false
	}
	svcs, err := decodePS(out)
	if err != nil {
		return nil, false
	}
	return svcs, true
}

// decodePS accepts both the NDJSON that compose v2.21+ emits and the single
// JSON array older versions emitted.
func decodePS(out string) ([]psService, error) {
	trimmed := strings.TrimSpace(out)
	if strings.HasPrefix(trimmed, "[") {
		var arr []psService
		if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
			return nil, err
		}
		return arr, nil
	}
	var svcs []psService
	dec := json.NewDecoder(strings.NewReader(trimmed))
	for dec.More() {
		var s psService
		if err := dec.Decode(&s); err != nil {
			return nil, err
		}
		svcs = append(svcs, s)
	}
	return svcs, nil
}
