package stop

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/joelthompson/aigate/internal/config"
	"github.com/joelthompson/aigate/internal/exit"
	"github.com/spf13/cobra"
)

// hookInput is the JSON payload Claude Code writes to this hook's stdin.
type hookInput struct {
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
	// LastAssistantMessage is the turn's final assistant text, used for the
	// min-length check. The transcript lags, so this field is the source.
	LastAssistantMessage string `json:"last_assistant_message"`
	// StopHookActive is true when Claude is only still running because a stop
	// hook already blocked it. Blocking again loops until Claude Code's
	// eight-block cap fires.
	StopHookActive bool `json:"stop_hook_active"`
}

// NewCommand builds the Stop hook command. It fails open on every error path
// because it runs on every agent turn — a hook-error banner on every response
// is worse than an occasional missed brevity nudge.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Run the Stop hook (asks Claude for a concise recap)",
		// stderr on exit 2 is the message Claude Code feeds back to the
		// model; cobra's own "Error: ..." banner must never land there.
		// SilenceUsage keeps a RunE error (our exit-2 signal) from also
		// dumping the usage string onto stdout, which must stay empty.
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in, ok := readInput(cmd.InOrStdin(), cmd.ErrOrStderr())
			if !ok {
				return nil
			}

			cfg, err := loadConfig()
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "aigate: %v\n", err)
				return nil
			}

			prompt, ok := decide(cfg, in)
			if !ok {
				return nil
			}
			return emit(cmd.ErrOrStderr(), prompt)
		},
	}

	return cmd
}

// readInput parses the hook payload from r. ok is false when stdin is empty
// or the payload isn't valid JSON. A non-empty payload that fails to parse
// gets one diagnostic on errw; empty stdin stays silent, so running this
// command by hand produces no noise.
func readInput(r io.Reader, errw io.Writer) (hookInput, bool) {
	data, err := io.ReadAll(r)
	if err != nil {
		return hookInput{}, false
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return hookInput{}, false
	}

	var in hookInput
	if err := json.Unmarshal(data, &in); err != nil {
		fmt.Fprintf(errw, "aigate: parsing hook input: %v\n", err)
		return hookInput{}, false
	}
	return in, true
}

// loadConfig reads .aigate.yml from the current directory. A missing file is
// not an error — the hook simply allows the stop.
func loadConfig() (*config.Config, error) {
	cfgPath, err := filepath.Abs(config.DefaultConfigFile)
	if err != nil {
		return nil, fmt.Errorf("resolving config path: %w", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return cfg, nil
}

// decide returns the prompt to feed back to Claude, or ok=false to let it stop.
func decide(cfg *config.Config, in hookInput) (string, bool) {
	// Loop guard first, ahead of any config lookup, so no later edit to the
	// config logic can reintroduce the loop.
	if in.StopHookActive {
		return "", false
	}
	if cfg == nil || cfg.Claude == nil || cfg.Claude.StopPrompt == nil ||
		!cfg.Claude.StopPrompt.Enabled {
		return "", false
	}
	sp := cfg.Claude.StopPrompt

	// Short replies don't need a recap. Only applied when the payload
	// actually carried the message, so a missing field asks for a recap
	// rather than silently disabling the hook.
	if sp.MinLength > 0 && in.LastAssistantMessage != "" &&
		utf8.RuneCountInString(in.LastAssistantMessage) < sp.MinLength {
		return "", false
	}

	prompt := strings.TrimSpace(sp.Prompt)
	if prompt == "" {
		prompt = config.DefaultStopPrompt
	}
	return prompt, true
}

// emit blocks the stop. Claude Code reads a Stop hook's stderr on exit 2 as
// the instruction to continue with; stdout is only written to the debug log,
// so it must stay empty.
func emit(errw io.Writer, prompt string) error {
	fmt.Fprintln(errw, prompt)
	return &exit.Error{Code: 2}
}
