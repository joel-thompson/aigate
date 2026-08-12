package shell

import (
	"context"
	"strings"
	"testing"
)

func TestRun_Success(t *testing.T) {
	p := &Provider{Command: "echo hello", Label: "greeting"}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(result.Output) != "hello" {
		t.Errorf("expected output %q, got %q", "hello", result.Output)
	}
	if result.Label != "greeting" {
		t.Errorf("expected label %q, got %q", "greeting", result.Label)
	}
}

func TestRun_MultiLineOutput(t *testing.T) {
	p := &Provider{Command: "printf 'line1\nline2\nline3'", Label: "multi"}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result.Output), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), result.Output)
	}
}

func TestRun_FailedCommand(t *testing.T) {
	p := &Provider{Command: "exit 42", Label: "fail"}
	_, err := p.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for failed command")
	}
	if !strings.Contains(err.Error(), "exited with code 42") {
		t.Errorf("expected exit code in error, got %q", err.Error())
	}
}

func TestRun_CommandNotFound(t *testing.T) {
	p := &Provider{Command: "aigate_nonexistent_command_xyz", Label: "missing"}
	_, err := p.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for command not found")
	}
}

func TestRun_EmptyOutput(t *testing.T) {
	p := &Provider{Command: "true", Label: "empty"}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "" {
		t.Errorf("expected empty output, got %q", result.Output)
	}
}
