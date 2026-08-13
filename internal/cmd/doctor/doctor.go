package doctor

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/joelthompson/aigate/internal/exit"
	"github.com/joelthompson/aigate/internal/wiring"
	"github.com/spf13/cobra"
)

// NewCommand builds the doctor command. SilenceErrors/SilenceUsage mirror
// stop.go: a runDoctor error return is only ever the exit-1 signal, and
// cobra's own "Error: ..." banner must not land on top of the report.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "doctor",
		Short:         "Diagnose why aigate's hooks aren't firing",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.OutOrStdout(), cmd.ErrOrStderr(), exec.LookPath)
		},
	}
	return cmd
}

func runDoctor(out, errw io.Writer, lookPath func(string) (string, error)) error {
	rep, err := wiring.Inspect(".", lookPath)
	if err != nil {
		fmt.Fprintf(errw, "aigate doctor: %v\n", err)
		return &exit.Error{Code: 1}
	}

	render(out, rep)

	if !rep.OK() {
		return &exit.Error{Code: 1}
	}
	return nil
}

func render(w io.Writer, rep wiring.Report) {
	fmt.Fprintln(w, "aigate doctor")
	fmt.Fprintf(w, "cwd: %s\n\n", rep.Root)

	for _, c := range rep.Checks {
		writeCheck(w, c)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, summary(rep))
}

func writeCheck(w io.Writer, c wiring.Check) {
	if c.Status == wiring.StatusProblem {
		fmt.Fprintf(w, "%s: %s\n", c.Label, c.Status)
		if c.Detail != "" {
			fmt.Fprintf(w, "  %s\n", c.Detail)
		}
		if c.Path != "" {
			fmt.Fprintf(w, "  path: %s\n", c.Path)
		}
		if c.Remedy != "" {
			fmt.Fprintf(w, "  fix: %s\n", c.Remedy)
		}
		return
	}

	if c.Detail == "" {
		fmt.Fprintf(w, "%s: %s\n", c.Label, c.Status)
		return
	}
	fmt.Fprintf(w, "%s: %s (%s)\n", c.Label, c.Status, c.Detail)
}

func summary(rep wiring.Report) string {
	switch n := len(rep.Problems()); n {
	case 0:
		return "No problems found."
	case 1:
		return "1 problem found."
	default:
		return fmt.Sprintf("%d problems found.", n)
	}
}
