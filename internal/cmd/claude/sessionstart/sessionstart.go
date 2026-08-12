package sessionstart

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joelthompson/aigate/internal/config"
	"github.com/joelthompson/aigate/internal/provider"
	"github.com/joelthompson/aigate/internal/provider/projectstructure"
	"github.com/joelthompson/aigate/internal/provider/shell"
	"github.com/joelthompson/aigate/internal/runner"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "session-start",
		Short: "Run session-start context providers",
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

			providers := buildProviders(cfg)
			if len(providers) == 0 {
				return nil
			}

			output := runner.RunEnrich(cmd.Context(), providers)
			fmt.Fprintln(cmd.OutOrStdout(), output)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print context output for auditing (providers are still executed)")

	return cmd
}

type unsupportedProvider struct {
	typeName string
}

func (u *unsupportedProvider) Run(ctx context.Context) (*provider.Result, error) {
	return nil, fmt.Errorf("unsupported provider type %q", u.typeName)
}

func buildProviders(cfg *config.Config) []provider.Provider {
	if cfg.Claude == nil || cfg.Claude.SessionStart == nil {
		return nil
	}

	var providers []provider.Provider
	for _, pc := range cfg.Claude.SessionStart.Context {
		switch pc.Type {
		case "project-structure":
			providers = append(providers, &projectstructure.Provider{
				MaxDepth: pc.MaxDepth,
			})
		case "shell":
			providers = append(providers, &shell.Provider{
				Command: pc.Command,
				Label:   pc.Label,
			})
		default:
			providers = append(providers, &unsupportedProvider{typeName: pc.Type})
		}
	}
	return providers
}
