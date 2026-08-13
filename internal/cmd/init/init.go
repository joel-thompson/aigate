package initcmd

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/joelthompson/aigate/internal/config"
	"github.com/joelthompson/aigate/internal/provider/ciinfo"
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
	// identically to the interactive one, even though detectState (and thus
	// the prompt) is skipped in that mode.
	ciDetected := ciinfo.Detected(".")

	if !nonInteractive {
		cur, err := detectState()
		if err != nil {
			return err
		}

		ciDone := !ciDetected || cur.ciContext

		if cur.config && cur.sessionHook && cur.sessionContext && cur.stopHook && cur.stopPrompt && cur.gitHook && cur.preCommitChecks && ciDone {
			fmt.Fprintln(w, "Everything is already set up.")
			fmt.Fprintln(w)
		}

		do.config = askUnlessDone(prompter, cur.config, "Create .aigate.yml config file?")
		do.claudeHooks = askUnlessDone(prompter, cur.sessionHook && cur.sessionContext, "Register Claude Code session-start hook?")
		if ciDetected {
			do.ciContext = askUnlessDone(prompter, cur.ciContext, "Add CircleCI info to Claude's session context?")
		}
		do.stopPrompt = askUnlessDone(prompter, cur.stopHook && cur.stopPrompt, "Register Claude Code stop hook for concise recaps?")
		do.gitHooks = askUnlessDone(prompter, cur.gitHook && cur.preCommitChecks, "Set up git pre-commit hook?")
	} else {
		do.ciContext = do.claudeHooks && ciDetected
	}

	var hadError bool

	result := printStep(w, "Config", do.config, func() StepResult {
		configPath, err := filepath.Abs(config.DefaultConfigFile)
		if err != nil {
			return StepResult{Err: err}
		}
		return ScaffoldConfig(configPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Claude hooks", do.claudeHooks, func() StepResult {
		settingsPath, err := claudeSettingsPath()
		if err != nil {
			return StepResult{Err: err}
		}
		return RegisterClaudeHooks(settingsPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Session context", do.claudeHooks, func() StepResult {
		configPath, err := filepath.Abs(config.DefaultConfigFile)
		if err != nil {
			return StepResult{Err: err}
		}
		return EnableSessionStart(configPath)
	})
	if result.Err != nil {
		hadError = true
	}

	if ciDetected {
		result = printStep(w, "CI context", do.ciContext, func() StepResult {
			configPath, err := filepath.Abs(config.DefaultConfigFile)
			if err != nil {
				return StepResult{Err: err}
			}
			return EnableCIContext(configPath)
		})
		if result.Err != nil {
			hadError = true
		}
	}

	result = printStep(w, "Stop hook", do.stopPrompt, func() StepResult {
		settingsPath, err := claudeSettingsPath()
		if err != nil {
			return StepResult{Err: err}
		}
		return RegisterStopHook(settingsPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Stop prompt", do.stopPrompt, func() StepResult {
		configPath, err := filepath.Abs(config.DefaultConfigFile)
		if err != nil {
			return StepResult{Err: err}
		}
		return EnableStopPrompt(configPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Git hook", do.gitHooks, func() StepResult {
		absHookPath, err := filepath.Abs(filepath.Join(".git", "hooks", "pre-commit"))
		if err != nil {
			return StepResult{Err: err}
		}
		return SetupGitHook(absHookPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Pre-commit checks", do.gitHooks, func() StepResult {
		configPath, err := filepath.Abs(config.DefaultConfigFile)
		if err != nil {
			return StepResult{Err: err}
		}
		return EnablePreCommitChecks(configPath)
	})
	if result.Err != nil {
		hadError = true
	}

	if hadError {
		return fmt.Errorf("one or more steps failed")
	}
	return nil
}

// claudeSettingsPath returns the absolute path to the project-local Claude
// Code settings file aigate registers hooks in. settings.local.json is the
// per-developer file, so hook registration stays out of git.
func claudeSettingsPath() (string, error) {
	return filepath.Abs(filepath.Join(".claude", "settings.local.json"))
}

func printStep(w io.Writer, label string, do bool, fn func() StepResult) StepResult {
	if !do {
		r := StepResult{Skipped: true, Reason: "not requested"}
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
