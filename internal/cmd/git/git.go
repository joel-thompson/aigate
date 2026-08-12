package git

import (
	"github.com/joelthompson/aigate/internal/cmd/git/precommit"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Git hooks",
	}

	cmd.AddCommand(precommit.NewCommand())

	return cmd
}
