package runner

import (
	"context"
	"fmt"
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
