package secretsscan

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/joelthompson/aigate/internal/provider"
)

type rule struct {
	name    string
	pattern *regexp.Regexp
}

var rules = []rule{
	{"AWS access key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"private key block", regexp.MustCompile(`-----BEGIN.*PRIVATE KEY-----`)},
	{"GitHub personal access token", regexp.MustCompile(`ghp_[A-Za-z0-9_]{36}`)},
	{"API key (sk-prefix)", regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`)},
	{"Slack bot token", regexp.MustCompile(`xoxb-[A-Za-z0-9-]+`)},
	{"Slack user token", regexp.MustCompile(`xoxp-[A-Za-z0-9-]+`)},
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

type finding struct {
	file     string
	line     int
	ruleName string
}

type Provider struct {
	Diff string
}

func (p *Provider) Run(_ context.Context) (*provider.Result, error) {
	findings := scan(p.Diff)
	if len(findings) == 0 {
		return &provider.Result{
			ExitCode: 0,
			Label:    "secrets-scan",
		}, nil
	}

	var lines []string
	for _, f := range findings {
		if f.file != "" {
			lines = append(lines, fmt.Sprintf("  %s:%d: %s", f.file, f.line, f.ruleName))
		} else {
			lines = append(lines, fmt.Sprintf("  line %d: %s", f.line, f.ruleName))
		}
	}
	output := fmt.Sprintf("Potential secrets detected:\n%s", strings.Join(lines, "\n"))

	return &provider.Result{
		Output:   output,
		ExitCode: 1,
		Label:    "secrets-scan",
	}, nil
}

func scan(diff string) []finding {
	var findings []finding
	var currentFile string
	var fileLine int

	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			currentFile = line[6:]
			continue
		}
		if strings.HasPrefix(line, "+++ ") {
			currentFile = ""
			continue
		}

		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			fmt.Sscanf(m[1], "%d", &fileLine)
			continue
		}

		if strings.HasPrefix(line, "-") {
			continue
		}

		if strings.HasPrefix(line, "+") {
			content := line[1:]
			for _, r := range rules {
				if r.pattern.MatchString(content) {
					findings = append(findings, finding{
						file:     currentFile,
						line:     fileLine,
						ruleName: r.name,
					})
					break
				}
			}
			fileLine++
			continue
		}

		// Context line — increments file line counter
		if !strings.HasPrefix(line, "diff ") && !strings.HasPrefix(line, "---") && !strings.HasPrefix(line, "index ") {
			fileLine++
		}
	}
	return findings
}
