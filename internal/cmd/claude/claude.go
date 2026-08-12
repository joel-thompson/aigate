package claude

import (
	"github.com/joelthompson/aigate/internal/cmd/claude/sessionstart"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claude",
		Short: "Claude Code hooks",
	}

	cmd.AddCommand(sessionstart.NewCommand())

	return cmd
}
