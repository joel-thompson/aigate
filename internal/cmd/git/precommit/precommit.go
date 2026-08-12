package precommit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/joelthompson/aigate/internal/config"
	"github.com/joelthompson/aigate/internal/provider"
	"github.com/joelthompson/aigate/internal/provider/secretsscan"
	"github.com/joelthompson/aigate/internal/runner"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pre-commit",
		Short: "Run pre-commit checks against staged changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, err := filepath.Abs(config.DefaultConfigFile)
			if err != nil {
				return fmt.Errorf("resolving config path: %w", err)
			}

			cfg, err := config.Load(cfgPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			}

			if !hasChecks(cfg) {
				return nil
			}

			diff, err := stagedDiff(cmd.Context())
			if err != nil {
				return err
			}

			checks := buildChecks(cfg, diff)
			if !runner.RunGate(cmd.Context(), checks, cmd.ErrOrStderr()) {
				return errors.New("pre-commit checks failed")
			}
			return nil
		},
	}
	return cmd
}

func hasChecks(cfg *config.Config) bool {
	return cfg.Git != nil && cfg.Git.PreCommit != nil && len(cfg.Git.PreCommit.Checks) > 0
}

func stagedDiff(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "diff", "--cached").Output()
	if err != nil {
		return "", fmt.Errorf("reading staged diff: %w", err)
	}
	return string(out), nil
}

type unsupportedCheck struct {
	typeName string
}

func (u *unsupportedCheck) Run(_ context.Context) (*provider.Result, error) {
	return nil, fmt.Errorf("unsupported check type %q", u.typeName)
}

func buildChecks(cfg *config.Config, diff string) []provider.Provider {
	if cfg.Git == nil || cfg.Git.PreCommit == nil {
		return nil
	}

	var checks []provider.Provider
	for _, pc := range cfg.Git.PreCommit.Checks {
		switch pc.Type {
		case "secrets-scan":
			checks = append(checks, &secretsscan.Provider{Diff: diff})
		default:
			checks = append(checks, &unsupportedCheck{typeName: pc.Type})
		}
	}
	return checks
}
