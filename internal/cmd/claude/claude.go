package claude

import (
	"github.com/joelthompson/aigate/internal/cmd/claude/sessionstart"
	"github.com/joelthompson/aigate/internal/cmd/claude/stop"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claude",
		Short: "Claude Code hooks",
	}

	cmd.AddCommand(sessionstart.NewCommand())
	cmd.AddCommand(stop.NewCommand())

	return cmd
}
