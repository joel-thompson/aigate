package root

import (
	"github.com/joelthompson/aigate/internal/cmd/claude"
	"github.com/joelthompson/aigate/internal/cmd/git"
	initcmd "github.com/joelthompson/aigate/internal/cmd/init"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "aigate",
		Short:        "Claude Code lifecycle management — configurable hooks with pluggable providers",
		SilenceUsage: true,
	}

	cmd.AddCommand(initcmd.NewCommand())
	cmd.AddCommand(claude.NewCommand())
	cmd.AddCommand(git.NewCommand())

	return cmd
}
