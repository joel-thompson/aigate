package sessionstart

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joelthompson/aigate/internal/config"
	"github.com/joelthompson/aigate/internal/provider"
	"github.com/joelthompson/aigate/internal/provider/ciinfo"
	"github.com/joelthompson/aigate/internal/provider/dockercompose"
	"github.com/joelthompson/aigate/internal/provider/projectstructure"
	"github.com/joelthompson/aigate/internal/provider/shell"
	"github.com/joelthompson/aigate/internal/runner"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
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
				return fmt.Errorf("loading config: %w", err)
			}

			providers := buildProviders(cfg)
			if len(providers) == 0 {
				return nil
			}

			output := runner.RunEnrich(cmd.Context(), providers, cmd.ErrOrStderr())
			if output != "" {
				fmt.Fprintln(cmd.OutOrStdout(), output)
			}
			return nil
		},
	}

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
		case "ci-info":
			providers = append(providers, &ciinfo.Provider{})
		case "docker-compose":
			providers = append(providers, &dockercompose.Provider{Files: pc.Files})
		default:
			providers = append(providers, &unsupportedProvider{typeName: pc.Type})
		}
	}
	return providers
}
