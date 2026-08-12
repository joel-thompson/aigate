package git

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Git hooks",
	}

	return cmd
}
