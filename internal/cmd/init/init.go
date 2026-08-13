package initcmd

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/joelthompson/aigate/internal/config"
	"github.com/joelthompson/aigate/internal/provider/ciinfo"
	"github.com/joelthompson/aigate/internal/wiring"
	"github.com/spf13/cobra"
)

// steps holds which init steps are requested, whether from flags or prompts.
type steps struct {
	config      bool
	claudeHooks bool
	ciContext   bool
	stopPrompt  bool
	gitHooks    bool
}

func NewCommand() *cobra.Command {
	var flags steps

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up aigate for this project",
		RunE: func(cmd *cobra.Command, args []string) error {
			nonInteractive := cmd.Flags().Changed("config") ||
				cmd.Flags().Changed("claude-hooks") ||
				cmd.Flags().Changed("stop-prompt") ||
				cmd.Flags().Changed("git-hooks")

			var prompter func(string) bool
			if nonInteractive {
				prompter = nil
			} else {
				scanner := bufio.NewScanner(cmd.InOrStdin())
				prompter = func(question string) bool {
					return promptYN(cmd.OutOrStdout(), scanner, question)
				}
			}

			return runInit(cmd.OutOrStdout(), prompter, flags)
		},
	}

	cmd.Flags().BoolVar(&flags.config, "config", false, "Create .aigate.yml config file")
	cmd.Flags().BoolVar(&flags.claudeHooks, "claude-hooks", false, "Register Claude Code hooks in .claude/settings.local.json")
	cmd.Flags().BoolVar(&flags.stopPrompt, "stop-prompt", false, "Register Claude Code stop hook for concise recaps")
	cmd.Flags().BoolVar(&flags.gitHooks, "git-hooks", false, "Set up git pre-commit hook")

	return cmd
}

func runInit(w io.Writer, prompter func(string) bool, flags steps) error {
	nonInteractive := prompter == nil

	do := flags

	// Computed unconditionally so the non-interactive gate below behaves
	// identically to the interactive one, even though wiring.Detect (and thus
	// the prompt) is skipped in that mode.
	ciDetected := ciinfo.Detected(".")

	if !nonInteractive {
		cur, err := wiring.Detect(".")
		if err != nil {
			return err
		}

		if wiring.Complete(cur, ciDetected) {
			fmt.Fprintln(w, "Everything is already set up.")
			fmt.Fprintln(w)
		}

		do.config = askUnlessDone(prompter, cur.Config, "Create .aigate.yml config file?")
		do.claudeHooks = askUnlessDone(prompter, cur.SessionHook && cur.SessionContext, "Register Claude Code session-start hook?")
		if ciDetected {
			do.ciContext = askUnlessDone(prompter, cur.CIContext, "Add CircleCI info to Claude's session context?")
		}
		do.stopPrompt = askUnlessDone(prompter, cur.StopHook && cur.StopPrompt, "Register Claude Code stop hook for concise recaps?")
		do.gitHooks = askUnlessDone(prompter, cur.GitHook && cur.PreCommitChecks, "Set up git pre-commit hook?")
	} else {
		do.ciContext = do.claudeHooks && ciDetected
	}

	var hadError bool

	result := printStep(w, "Config", do.config, func() wiring.StepResult {
		configPath, err := filepath.Abs(config.DefaultConfigFile)
		if err != nil {
			return wiring.StepResult{Err: err}
		}
		return wiring.ScaffoldConfig(configPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Claude hooks", do.claudeHooks, func() wiring.StepResult {
		settingsPath, err := wiring.SettingsPath(".")
		if err != nil {
			return wiring.StepResult{Err: err}
		}
		return wiring.RegisterClaudeHooks(settingsPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Session context", do.claudeHooks, func() wiring.StepResult {
		configPath, err := filepath.Abs(config.DefaultConfigFile)
		if err != nil {
			return wiring.StepResult{Err: err}
		}
		return wiring.EnableSessionStart(configPath)
	})
	if result.Err != nil {
		hadError = true
	}

	if ciDetected {
		result = printStep(w, "CI context", do.ciContext, func() wiring.StepResult {
			configPath, err := filepath.Abs(config.DefaultConfigFile)
			if err != nil {
				return wiring.StepResult{Err: err}
			}
			return wiring.EnableCIContext(configPath)
		})
		if result.Err != nil {
			hadError = true
		}
	}

	result = printStep(w, "Stop hook", do.stopPrompt, func() wiring.StepResult {
		settingsPath, err := wiring.SettingsPath(".")
		if err != nil {
			return wiring.StepResult{Err: err}
		}
		return wiring.RegisterStopHook(settingsPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Stop prompt", do.stopPrompt, func() wiring.StepResult {
		configPath, err := filepath.Abs(config.DefaultConfigFile)
		if err != nil {
			return wiring.StepResult{Err: err}
		}
		return wiring.EnableStopPrompt(configPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Git hook", do.gitHooks, func() wiring.StepResult {
		absHookPath, err := filepath.Abs(filepath.Join(".git", "hooks", "pre-commit"))
		if err != nil {
			return wiring.StepResult{Err: err}
		}
		return wiring.SetupGitHook(absHookPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Pre-commit checks", do.gitHooks, func() wiring.StepResult {
		configPath, err := filepath.Abs(config.DefaultConfigFile)
		if err != nil {
			return wiring.StepResult{Err: err}
		}
		return wiring.EnablePreCommitChecks(configPath)
	})
	if result.Err != nil {
		hadError = true
	}

	if hadError {
		return fmt.Errorf("one or more steps failed")
	}
	return nil
}

func printStep(w io.Writer, label string, do bool, fn func() wiring.StepResult) wiring.StepResult {
	if !do {
		r := wiring.StepResult{Skipped: true, Reason: "not requested"}
		fmt.Fprintf(w, "%s: %s\n", label, r)
		return r
	}
	r := fn()
	fmt.Fprintf(w, "%s: %s\n", label, r)
	return r
}

// askUnlessDone prompts only when the step isn't already set up. An
// already-set-up step returns true so it still runs and reports its own
// accurate "already exists" reason; the scaffold funcs are no-ops there.
func askUnlessDone(prompter func(string) bool, done bool, question string) bool {
	if done {
		return true
	}
	return prompter(question)
}

func promptYN(w io.Writer, scanner *bufio.Scanner, question string) bool {
	fmt.Fprintf(w, "%s [Y/n] ", question)
	if !scanner.Scan() {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "" || answer == "y" || answer == "yes"
}
