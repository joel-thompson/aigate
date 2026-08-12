package claude

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claude",
		Short: "Claude Code hooks",
	}

	return cmd
}
