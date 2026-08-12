package runner

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/joelthompson/aigate/internal/provider"
)

func RunEnrich(ctx context.Context, providers []provider.Provider) string {
	var sections []string
	for _, p := range providers {
		result, err := p.Run(ctx)
		if err != nil {
			sections = append(sections, fmt.Sprintf("[error] %v", err))
			continue
		}
		output := strings.TrimRight(result.Output, "\n")
		if result.Label != "" {
			sections = append(sections, fmt.Sprintf("## %s\n%s", result.Label, output))
		} else {
			sections = append(sections, output)
		}
	}
	return strings.Join(sections, "\n\n")
}

// RunGate runs all check providers and returns true if all passed.
// Failures are written to w.
func RunGate(ctx context.Context, providers []provider.Provider, w io.Writer) bool {
	passed := true
	for _, p := range providers {
		result, err := p.Run(ctx)
		if err != nil {
			fmt.Fprintf(w, "[check] error: %v\n", err)
			passed = false
			continue
		}
		if result.ExitCode != 0 {
			label := result.Label
			if label == "" {
				label = "check"
			}
			fmt.Fprintf(w, "[%s] FAILED\n", label)
			if result.Output != "" {
				fmt.Fprintln(w, result.Output)
			}
			passed = false
		}
	}
	return passed
}
