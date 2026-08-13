package stop

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/joelthompson/aigate/internal/config"
	"github.com/joelthompson/aigate/internal/exit"
)

func TestDecide_ReturnsConfiguredPrompt(t *testing.T) {
	cfg := &config.Config{Claude: &config.ClaudeConfig{StopPrompt: &config.StopPromptConfig{
		Enabled: true,
		Prompt:  "be brief",
	}}}
	prompt, ok := decide(cfg, hookInput{})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if prompt != "be brief" {
		t.Errorf("expected %q, got %q", "be brief", prompt)
	}
}

func TestDecide_ReturnsDefaultPromptWhenPromptOmitted(t *testing.T) {
	cfg := &config.Config{Claude: &config.ClaudeConfig{StopPrompt: &config.StopPromptConfig{
		Enabled: true,
	}}}
	prompt, ok := decide(cfg, hookInput{})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if prompt != config.DefaultStopPrompt {
		t.Errorf("expected default prompt, got %q", prompt)
	}
}

func TestDecide_ReturnsDefaultPromptWhenPromptWhitespaceOnly(t *testing.T) {
	cfg := &config.Config{Claude: &config.ClaudeConfig{StopPrompt: &config.StopPromptConfig{
		Enabled: true,
		Prompt:  "   ",
	}}}
	prompt, ok := decide(cfg, hookInput{})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if prompt != config.DefaultStopPrompt {
		t.Errorf("expected default prompt, got %q", prompt)
	}
}

func TestDecide_AllowsStopWhenStopHookActive(t *testing.T) {
	cfg := &config.Config{Claude: &config.ClaudeConfig{StopPrompt: &config.StopPromptConfig{
		Enabled: true,
		Prompt:  "be brief",
	}}}
	_, ok := decide(cfg, hookInput{StopHookActive: true})
	if ok {
		t.Fatal("expected ok=false when stop_hook_active is true, even with a fully enabled config")
	}
}

func TestDecide_AllowsStopWhenDisabled(t *testing.T) {
	cfg := &config.Config{Claude: &config.ClaudeConfig{StopPrompt: &config.StopPromptConfig{
		Enabled: false,
	}}}
	_, ok := decide(cfg, hookInput{})
	if ok {
		t.Fatal("expected ok=false when disabled")
	}
}

func TestDecide_AllowsStopWhenSectionMissing(t *testing.T) {
	cfg := &config.Config{Claude: &config.ClaudeConfig{}}
	_, ok := decide(cfg, hookInput{})
	if ok {
		t.Fatal("expected ok=false when stop-prompt section is missing")
	}
}

func TestDecide_AllowsStopWhenConfigNil(t *testing.T) {
	_, ok := decide(nil, hookInput{})
	if ok {
		t.Fatal("expected ok=false when config is nil")
	}
}

func TestDecide_AllowsStopWhenReplyShorterThanMinLength(t *testing.T) {
	cfg := &config.Config{Claude: &config.ClaudeConfig{StopPrompt: &config.StopPromptConfig{
		Enabled:   true,
		MinLength: 100,
	}}}
	_, ok := decide(cfg, hookInput{LastAssistantMessage: "short reply"})
	if ok {
		t.Fatal("expected ok=false when the reply is shorter than min-length")
	}
}

func TestDecide_AsksWhenReplyMeetsMinLength(t *testing.T) {
	cfg := &config.Config{Claude: &config.ClaudeConfig{StopPrompt: &config.StopPromptConfig{
		Enabled:   true,
		MinLength: 5,
	}}}
	_, ok := decide(cfg, hookInput{LastAssistantMessage: strings.Repeat("x", 5)})
	if !ok {
		t.Fatal("expected ok=true when the reply meets min-length")
	}
}

func TestDecide_IgnoresMinLengthWhenZero(t *testing.T) {
	cfg := &config.Config{Claude: &config.ClaudeConfig{StopPrompt: &config.StopPromptConfig{
		Enabled:   true,
		MinLength: 0,
	}}}
	_, ok := decide(cfg, hookInput{LastAssistantMessage: "x"})
	if !ok {
		t.Fatal("expected ok=true when min-length is zero (no threshold)")
	}
}

func TestDecide_IgnoresMinLengthWhenLastMessageEmpty(t *testing.T) {
	cfg := &config.Config{Claude: &config.ClaudeConfig{StopPrompt: &config.StopPromptConfig{
		Enabled:   true,
		MinLength: 100,
	}}}
	_, ok := decide(cfg, hookInput{LastAssistantMessage: ""})
	if !ok {
		t.Fatal("expected ok=true when last_assistant_message is empty: the threshold can't be trusted, so ask")
	}
}

func TestDecide_MinLengthCountsRunesNotBytes(t *testing.T) {
	cfg := &config.Config{Claude: &config.ClaudeConfig{StopPrompt: &config.StopPromptConfig{
		Enabled:   true,
		MinLength: 5,
	}}}
	// "é" is 2 bytes in UTF-8: 4 runes is 8 bytes, so a byte-based check
	// would (wrongly) treat this as meeting a 5-rune threshold.
	msg := strings.Repeat("é", 4)
	_, ok := decide(cfg, hookInput{LastAssistantMessage: msg})
	if ok {
		t.Fatal("expected ok=false: 4 runes is below the 5-rune min-length")
	}
}

func TestReadInput_ParsesStopHookActive(t *testing.T) {
	var errBuf bytes.Buffer
	in, ok := readInput(strings.NewReader(`{"hook_event_name":"Stop","stop_hook_active":true,"last_assistant_message":"hi"}`), &errBuf)
	if !ok {
		t.Fatal("expected ok=true for valid JSON")
	}
	if !in.StopHookActive {
		t.Error("expected StopHookActive to be true")
	}
	if in.LastAssistantMessage != "hi" {
		t.Errorf("expected last_assistant_message %q, got %q", "hi", in.LastAssistantMessage)
	}
	if errBuf.Len() != 0 {
		t.Errorf("expected no stderr output, got %q", errBuf.String())
	}
}

func TestReadInput_EmptyStdinIsNotOK(t *testing.T) {
	var errBuf bytes.Buffer
	_, ok := readInput(strings.NewReader(""), &errBuf)
	if ok {
		t.Fatal("expected ok=false for empty stdin")
	}
	if errBuf.Len() != 0 {
		t.Errorf("expected no stderr output for empty stdin, got %q", errBuf.String())
	}
}

func TestReadInput_MalformedJSONIsNotOK(t *testing.T) {
	var errBuf bytes.Buffer
	_, ok := readInput(strings.NewReader("not json"), &errBuf)
	if ok {
		t.Fatal("expected ok=false for malformed JSON")
	}
	if errBuf.Len() == 0 {
		t.Error("expected a diagnostic on stderr for malformed JSON")
	}
}

func TestStopCommand_BlocksWithPromptOnStderr(t *testing.T) {
	t.Chdir(t.TempDir())
	writeConfig(t, `
claude:
  stop-prompt:
    enabled: true
    prompt: "custom recap prompt"
`)

	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"Stop","stop_hook_active":false}`))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	err := cmd.Execute()

	if exit.CodeFor(err) != 2 {
		t.Fatalf("expected exit code 2, got %d (err=%v)", exit.CodeFor(err), err)
	}
	if stderr.String() != "custom recap prompt\n" {
		t.Errorf("expected stderr %q, got %q", "custom recap prompt\n", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got %q", stdout.String())
	}
}

func TestStopCommand_AllowsStopWhenStopHookActive(t *testing.T) {
	t.Chdir(t.TempDir())
	writeConfig(t, `
claude:
  stop-prompt:
    enabled: true
    prompt: "custom recap prompt"
`)

	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"Stop","stop_hook_active":true}`))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("expected no output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestStopCommand_AllowsStopWhenConfigMissing(t *testing.T) {
	t.Chdir(t.TempDir())

	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"Stop","stop_hook_active":false}`))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestStopCommand_AllowsStopWhenConfigInvalid(t *testing.T) {
	t.Chdir(t.TempDir())
	writeConfig(t, `
claude:
  stop-prompt:
    enbaled: true
`)

	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"Stop","stop_hook_active":false}`))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error (fail open), got %v", err)
	}
}

func writeConfig(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(config.DefaultConfigFile, []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
}
