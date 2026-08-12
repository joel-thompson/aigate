package initcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	var (
		flagConfig      bool
		flagClaudeHooks bool
		flagGitHooks    bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up aigate for this project",
		RunE: func(cmd *cobra.Command, args []string) error {
			nonInteractive := cmd.Flags().Changed("config") ||
				cmd.Flags().Changed("claude-hooks") ||
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

			return runInit(cmd.OutOrStdout(), prompter, flagConfig, flagClaudeHooks, flagGitHooks)
		},
	}

	cmd.Flags().BoolVar(&flagConfig, "config", false, "Create .aigate.yml config file")
	cmd.Flags().BoolVar(&flagClaudeHooks, "claude-hooks", false, "Register Claude Code hooks in settings.json")
	cmd.Flags().BoolVar(&flagGitHooks, "git-hooks", false, "Set up git pre-commit hook")

	return cmd
}

func runInit(w io.Writer, prompter func(string) bool, flagConfig, flagClaudeHooks, flagGitHooks bool) error {
	nonInteractive := prompter == nil

	doConfig := flagConfig
	doClaudeHooks := flagClaudeHooks
	doGitHooks := flagGitHooks

	if !nonInteractive {
		doConfig = prompter("Create .aigate.yml config file?")
		doClaudeHooks = prompter("Register Claude Code session-start hook?")
		doGitHooks = prompter("Set up git pre-commit hook?")
	}

	var hadError bool

	result := printStep(w, "Config", doConfig, func() StepResult {
		configPath, err := filepath.Abs(".aigate.yml")
		if err != nil {
			return StepResult{Err: err}
		}
		return ScaffoldConfig(configPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Claude hooks", doClaudeHooks, func() StepResult {
		home, err := os.UserHomeDir()
		if err != nil {
			return StepResult{Err: fmt.Errorf("finding home directory: %w", err)}
		}
		settingsPath := filepath.Join(home, ".claude", "settings.json")
		return RegisterClaudeHooks(settingsPath)
	})
	if result.Err != nil {
		hadError = true
	}

	result = printStep(w, "Git hook", doGitHooks, func() StepResult {
		absHookPath, err := filepath.Abs(filepath.Join(".git", "hooks", "pre-commit"))
		if err != nil {
			return StepResult{Err: err}
		}
		return SetupGitHook(absHookPath)
	})
	if result.Err != nil {
		hadError = true
	}

	if hadError {
		return fmt.Errorf("one or more steps failed")
	}
	return nil
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

func promptYN(w io.Writer, scanner *bufio.Scanner, question string) bool {
	fmt.Fprintf(w, "%s [Y/n] ", question)
	if !scanner.Scan() {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "" || answer == "y" || answer == "yes"
}
