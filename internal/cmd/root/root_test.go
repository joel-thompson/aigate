package root

import (
	"bytes"
	"testing"
)

func TestNewCommand_PrintsVersion(t *testing.T) {
	cmd := NewCommand("1.2.3")

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := "aigate version 1.2.3\n"
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
