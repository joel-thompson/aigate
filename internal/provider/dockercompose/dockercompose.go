// Package dockercompose provides session-start context describing a
// project's local Docker Compose stack: which file defines it, the exact
// command to start it, and — when the daemon is reachable — which services
// are already running, so the agent leaves those alone instead of
// restarting them or fighting over an already-bound host port.
package dockercompose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/joelthompson/aigate/internal/provider"
)

const label = "Docker stack"

// maxServices caps the rendered list, following projectstructure's
// maxEntriesPerDir: a 40-service stack would otherwise spend 40 lines of
// session-start context.
const maxServices = 30

// maxImageWidth caps the image column's padding. Real image names run long —
// otel/opentelemetry-collector-contrib:0.157.0 is 44 characters — and
// padding every row out to that would open a gutter wide enough to hurt
// readability more than the alignment helps. Wider names run ragged.
const maxImageWidth = 28

// colGap separates the rendered columns.
const colGap = "  "

// composeFileNames are the names docker compose itself auto-discovers, in
// its own precedence order.
var composeFileNames = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

type Provider struct {
	Root  string
	Files []string // from config; empty means let docker discover

	// runCLI shells out to docker. Injectable so tests never depend on
	// whether the machine running them has docker installed; nil uses
	// execCLI.
	runCLI cliRunner
}

// findComposeFile returns the first compose file name present in root, in
// docker's own discovery precedence.
func findComposeFile(root string) (string, bool) {
	for _, name := range composeFileNames {
		if fileExists(filepath.Join(root, name)) {
			return name, true
		}
	}
	return "", false
}

// Detected reports whether root has a compose stack aigate can summarize.
// Exported so init's detector and doctor's report share this exact
// predicate. Only root is checked, not parent directories, matching
// ciinfo.Detected's scope and init's Detected(".") call — Run still benefits
// from docker's own upward walk at exec time, but the gate is deliberately
// narrower than the probe.
func Detected(root string) bool {
	_, ok := findComposeFile(root)
	return ok
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// gateFile returns the compose file description to print, and whether a
// stack is present at all. Configured Files take precedence over docker's
// own discovery: a project that names its file docker-compose-services.yml
// is exactly why Files exists, and gating on the auto-discovered names would
// refuse to run for it.
func (p *Provider) gateFile(root string) (string, bool) {
	if len(p.Files) == 0 {
		return findComposeFile(root)
	}
	for _, f := range p.Files {
		path := f
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, f)
		}
		if !fileExists(path) {
			return "", false
		}
	}
	return strings.Join(p.Files, ", "), true
}

// Run summarizes the stack. Every rung of the degradation ladder returns an
// empty result rather than an error, for the reason ciinfo's cliSection
// documents: RunEnrich prints "[error] %v" to stderr for a returned error,
// and erroring here would produce that noise at every session start on any
// machine without docker.
func (p *Provider) Run(ctx context.Context) (*provider.Result, error) {
	root := p.Root
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}
		root = wd
	}

	// Filesystem gate first, so a project with no compose file never pays
	// for an exec.
	fileDesc, ok := p.gateFile(root)
	if !ok {
		return &provider.Result{Label: label}, nil
	}

	if ctx.Err() != nil {
		return &provider.Result{Label: label}, nil
	}

	cfg, ok := p.queryConfig(ctx, root)
	if !ok {
		// No docker CLI, or the file doesn't parse. The agent can't start
		// the stack anyway, so there is nothing useful to say.
		return &provider.Result{Label: label}, nil
	}

	ps, psOK := p.queryPS(ctx, root)

	return &provider.Result{Label: label, Output: p.render(root, fileDesc, cfg, ps, psOK)}, nil
}

// row is one rendered service line, split into its columns so the whole
// table can be padded to a consistent width across both groups.
type row struct {
	name   string
	source string // "postgres:16" or "build ./migrator"
	ports  string
	status string // health, or state when it isn't "running"
}

func (p *Provider) render(root, fileDesc string, cfg *composeConfig, ps []psService, psOK bool) string {
	running, notRunning, stack, extra := partition(root, cfg, ps, psOK)
	total := stack + extra

	var lines []string
	lines = append(lines, header(fileDesc, cfg.Name, stack, extra))
	lines = append(lines, fmt.Sprintf("Start: %s   (one service: %s <name>)", p.startCommand(), p.startCommand()))

	if !psOK {
		lines = append(lines, "Docker daemon not reachable; cannot tell which services are already running.")
	}

	budget := maxServices
	running, runElided := capRows(running, budget)
	budget -= len(running)
	notRunning, notElided := capRows(notRunning, budget)

	// Pad across both groups, so the columns line up down the whole table
	// rather than restarting at each heading. Measured after truncation, so
	// a row that was elided can't widen a column no printed row needs.
	all := append(append([]row{}, running...), notRunning...)
	nameW, imageW := columnWidths(all)

	if !psOK {
		lines = append(lines, "", "Services:")
		lines = append(lines, renderRows(notRunning, nameW, imageW, notElided)...)
		return strings.Join(lines, "\n")
	}

	if len(running) > 0 || runElided > 0 {
		lines = append(lines, "", fmt.Sprintf("Running (%d/%d) — already up, do not restart:", len(running)+runElided, total))
		lines = append(lines, renderRows(running, nameW, imageW, runElided)...)
	}
	if len(notRunning) > 0 || notElided > 0 {
		lines = append(lines, "", fmt.Sprintf("Not running (%d/%d):", len(notRunning)+notElided, total))
		lines = append(lines, renderRows(notRunning, nameW, imageW, notElided)...)
	}

	return strings.Join(lines, "\n")
}

// header counts the *default stack* — the services plain `docker compose up
// -d` starts — because that is what the Start line acts on. A profiled
// service that happens to be running is real, but folding it into the same
// count would promise the agent that up -d starts more than it does, so it
// is disclosed separately instead.
func header(fileDesc, project string, stack, extra int) string {
	h := fileDesc
	if project != "" {
		h += fmt.Sprintf(" · project %q", project)
	}
	h += fmt.Sprintf(" · %s", pluralize(stack, "service"))
	if extra > 0 {
		h += fmt.Sprintf(" (+%d running outside it)", extra)
	}
	return h
}

// partition splits the stack into running and not-running rows. config is
// the full set and ps is the running set, so the difference is "not
// running"; plain `ps` omits exited containers, which is exactly the
// grouping wanted. When ps is unavailable the split would be a lie, so
// everything comes back as the single notRunning list for the caller to
// render under one honest heading.
// stack is the number of services in the compose file (what `up -d` starts);
// extra counts running services absent from it.
func partition(root string, cfg *composeConfig, ps []psService, psOK bool) (running, notRunning []row, stack, extra int) {
	names := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	if !psOK {
		for _, name := range names {
			svc := cfg.Services[name]
			notRunning = append(notRunning, row{
				name:   name,
				source: sourceOf(root, svc),
				ports:  configPorts(svc.Ports),
			})
		}
		return nil, notRunning, len(names), 0
	}

	byService := make(map[string]psService, len(ps))
	for _, s := range ps {
		byService[s.Service] = s
	}

	for _, name := range names {
		svc := cfg.Services[name]
		up, isUp := byService[name]
		if !isUp {
			notRunning = append(notRunning, row{
				name:   name,
				source: sourceOf(root, svc),
				ports:  configPorts(svc.Ports),
			})
			continue
		}
		running = append(running, row{
			name:   name,
			source: sourceOf(root, svc),
			ports:  psPorts(up.Publishers),
			status: statusOf(up),
		})
	}

	// A ps entry with no config match is a service behind a `profiles:` key
	// that is up: config omits profiled services entirely, but telling the
	// agent a running service doesn't exist would be worse than a row with
	// less detail.
	var extras []string
	for _, s := range ps {
		if _, inConfig := cfg.Services[s.Service]; !inConfig {
			extras = append(extras, s.Service)
		}
	}
	sort.Strings(extras)
	for _, name := range extras {
		up := byService[name]
		running = append(running, row{
			name:   name,
			source: up.Image,
			ports:  psPorts(up.Publishers),
			status: statusOf(up),
		})
	}

	return running, notRunning, len(names), len(extras)
}

// sourceOf describes where a service's container comes from: its image, or
// its build context made relative to root so absolute developer paths never
// land in the output.
func sourceOf(root string, svc composeService) string {
	if svc.Image != "" {
		return svc.Image
	}
	if svc.Build == nil || svc.Build.Context == "" {
		return ""
	}
	ctxPath := svc.Build.Context
	if rel, err := filepath.Rel(root, ctxPath); err == nil {
		ctxPath = "./" + filepath.ToSlash(rel)
	}
	return "build " + ctxPath
}

// statusOf annotates a running service. State wins over Health when it isn't
// "running": "restarting" is the more urgent fact than a stale health value.
func statusOf(s psService) string {
	if s.State != "" && s.State != "running" {
		return s.State
	}
	return s.Health
}

// psPorts renders the ports a running service actually publishes. This comes
// from ps rather than config because it is the truth: the compose file may
// have been edited since `up`.
func psPorts(pubs []psPublisher) string {
	var parts []string
	for _, pub := range pubs {
		if pub.PublishedPort == 0 {
			continue
		}
		part := fmt.Sprintf("%d→%d", pub.PublishedPort, pub.TargetPort)
		if host := hostPrefix(pub.URL); host != "" {
			part = host + ":" + part
		}
		parts = append(parts, withProtocol(part, pub.Protocol))
	}
	return joinPorts(parts)
}

// configPorts renders the ports a not-running service would publish, which
// can only come from the compose file.
func configPorts(ports []composePort) string {
	var parts []string
	for _, port := range ports {
		if port.Published == "" {
			parts = append(parts, withProtocol(fmt.Sprintf("→%d (random host port)", port.Target), port.Protocol))
			continue
		}
		part := fmt.Sprintf("%s→%d", port.Published, port.Target)
		if host := hostPrefix(port.HostIP); host != "" {
			part = host + ":" + part
		}
		parts = append(parts, withProtocol(part, port.Protocol))
	}
	return joinPorts(parts)
}

func joinPorts(parts []string) string {
	if len(parts) == 0 {
		return "no published ports"
	}
	return strings.Join(parts, ", ")
}

// hostPrefix drops the host for the two values that carry no information:
// unspecified, and the all-interfaces wildcard.
func hostPrefix(host string) string {
	if host == "" || host == "0.0.0.0" {
		return ""
	}
	return host
}

// withProtocol annotates anything that isn't plain TCP, so a udp port isn't
// mistaken for one a client can connect to over tcp.
func withProtocol(part, protocol string) string {
	if protocol == "" || protocol == "tcp" {
		return part
	}
	return part + "/" + protocol
}

func columnWidths(rows []row) (nameW, imageW int) {
	for _, r := range rows {
		if n := utf8.RuneCountInString(r.name); n > nameW {
			nameW = n
		}
		if n := utf8.RuneCountInString(r.source); n > imageW {
			imageW = n
		}
	}
	if imageW > maxImageWidth {
		imageW = maxImageWidth
	}
	return nameW, imageW
}

func renderRows(rows []row, nameW, imageW, elided int) []string {
	lines := make([]string, 0, len(rows)+1)
	for _, r := range rows {
		line := colGap + pad(r.name, nameW) + colGap + pad(r.source, imageW) + colGap + r.ports
		if r.status != "" {
			line += colGap + r.status
		}
		lines = append(lines, strings.TrimRight(line, " "))
	}
	if elided > 0 {
		lines = append(lines, fmt.Sprintf("%s... (%d more)", colGap, elided))
	}
	return lines
}

// capRows keeps at most n rows, reporting how many it dropped.
func capRows(rows []row, n int) (kept []row, elided int) {
	if n < 0 {
		n = 0
	}
	if len(rows) <= n {
		return rows, 0
	}
	return rows[:n], len(rows) - n
}

// pad right-pads s to w display columns, counting runes so the arrow in a
// port mapping doesn't skew the column.
func pad(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
