package shell

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/joelthompson/aigate/internal/provider"
)

type Provider struct {
	Command string
	Label   string
}

func (p *Provider) Run(ctx context.Context) (*provider.Result, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", p.Command)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("command %q exited with code %d: %s", p.Command, exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("command %q: %w", p.Command, err)
	}
	return &provider.Result{
		Output: string(out),
		Label:  p.Label,
	}, nil
}
