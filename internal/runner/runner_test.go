package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/joelthompson/aigate/internal/provider"
)

type stubProvider struct {
	result *provider.Result
	err    error
}

func (s *stubProvider) Run(ctx context.Context) (*provider.Result, error) {
	return s.result, s.err
}

func TestRunEnrich_AllSucceed(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{result: &provider.Result{Output: "output1\n", Label: "First"}},
		&stubProvider{result: &provider.Result{Output: "output2\n", Label: "Second"}},
	}
	var errs bytes.Buffer
	got := RunEnrich(context.Background(), providers, &errs)
	if !strings.Contains(got, "## First\noutput1") {
		t.Errorf("expected labeled first output, got:\n%s", got)
	}
	if !strings.Contains(got, "## Second\noutput2") {
		t.Errorf("expected labeled second output, got:\n%s", got)
	}
	if errs.Len() != 0 {
		t.Errorf("expected no diagnostics, got: %s", errs.String())
	}
}

func TestRunEnrich_SomeFail(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{result: &provider.Result{Output: "good\n", Label: "OK"}},
		&stubProvider{err: errors.New("boom")},
		&stubProvider{result: &provider.Result{Output: "also good\n", Label: "Also OK"}},
	}
	var errs bytes.Buffer
	got := RunEnrich(context.Background(), providers, &errs)
	if !strings.Contains(got, "## OK\ngood") {
		t.Errorf("expected first success, got:\n%s", got)
	}
	if strings.Contains(got, "[error] boom") {
		t.Errorf("expected error line to be excluded from context, got:\n%s", got)
	}
	if !strings.Contains(got, "## Also OK\nalso good") {
		t.Errorf("expected third success, got:\n%s", got)
	}
	if !strings.Contains(errs.String(), "[error] boom") {
		t.Errorf("expected error line in diagnostics, got: %s", errs.String())
	}
}

func TestRunEnrich_AllFail(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{err: errors.New("err1")},
		&stubProvider{err: errors.New("err2")},
	}
	var errs bytes.Buffer
	got := RunEnrich(context.Background(), providers, &errs)
	if got != "" {
		t.Errorf("expected empty context when all providers fail, got:\n%s", got)
	}
	if !strings.Contains(errs.String(), "[error] err1") {
		t.Errorf("expected first error in diagnostics, got: %s", errs.String())
	}
	if !strings.Contains(errs.String(), "[error] err2") {
		t.Errorf("expected second error in diagnostics, got: %s", errs.String())
	}
}

func TestRunEnrich_NoLabel(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{result: &provider.Result{Output: "bare output\n", Label: ""}},
	}
	got := RunEnrich(context.Background(), providers, io.Discard)
	if strings.Contains(got, "##") {
		t.Errorf("expected no label header, got:\n%s", got)
	}
	if !strings.Contains(got, "bare output") {
		t.Errorf("expected bare output, got:\n%s", got)
	}
}

func TestRunEnrich_SkipsEmptyOutputSection(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{result: &provider.Result{Output: "", Label: "CI"}},
		&stubProvider{result: &provider.Result{Output: "output\n", Label: "Second"}},
	}
	got := RunEnrich(context.Background(), providers, io.Discard)
	if strings.Contains(got, "## CI") {
		t.Errorf("expected the empty-output section to be omitted entirely, got:\n%s", got)
	}
	if strings.HasPrefix(got, "\n") {
		t.Errorf("expected no leading blank line from the skipped section, got:\n%q", got)
	}
	if !strings.Contains(got, "## Second\noutput") {
		t.Errorf("expected the non-empty section to still appear, got:\n%s", got)
	}
}

func TestRunEnrich_AllEmptyOutputs(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{result: &provider.Result{Output: "", Label: "CI"}},
		&stubProvider{result: &provider.Result{Output: "", Label: "Other"}},
	}
	got := RunEnrich(context.Background(), providers, io.Discard)
	if got != "" {
		t.Errorf("expected empty string when every provider produces empty output, got %q", got)
	}
}

func TestRunEnrich_Empty(t *testing.T) {
	got := RunEnrich(context.Background(), nil, io.Discard)
	if got != "" {
		t.Errorf("expected empty string for no providers, got %q", got)
	}
}

func TestRunGate_AllPass(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{result: &provider.Result{ExitCode: 0, Label: "check-a"}},
		&stubProvider{result: &provider.Result{ExitCode: 0, Label: "check-b"}},
	}
	var buf bytes.Buffer
	passed := RunGate(context.Background(), providers, &buf)
	if !passed {
		t.Error("expected all checks to pass")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no stderr output, got: %s", buf.String())
	}
}

func TestRunGate_SomeFail(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{result: &provider.Result{ExitCode: 0, Label: "pass"}},
		&stubProvider{result: &provider.Result{ExitCode: 1, Output: "found a problem\n", Label: "fail"}},
		&stubProvider{result: &provider.Result{ExitCode: 0, Label: "also-pass"}},
	}
	var buf bytes.Buffer
	passed := RunGate(context.Background(), providers, &buf)
	if passed {
		t.Error("expected gate to fail when a check fails")
	}
	output := buf.String()
	if !strings.Contains(output, "[fail] FAILED") {
		t.Errorf("expected failure label in output, got: %s", output)
	}
	if !strings.Contains(output, "found a problem") {
		t.Errorf("expected failure detail in output, got: %s", output)
	}
}

func TestRunGate_AllFail(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{result: &provider.Result{ExitCode: 1, Output: "err1\n", Label: "a"}},
		&stubProvider{result: &provider.Result{ExitCode: 2, Output: "err2\n", Label: "b"}},
	}
	var buf bytes.Buffer
	passed := RunGate(context.Background(), providers, &buf)
	if passed {
		t.Error("expected gate to fail")
	}
	output := buf.String()
	if !strings.Contains(output, "[a] FAILED") {
		t.Errorf("expected first failure, got: %s", output)
	}
	if !strings.Contains(output, "[b] FAILED") {
		t.Errorf("expected second failure, got: %s", output)
	}
}

func TestRunGate_ProviderError(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{err: errors.New("provider crashed")},
	}
	var buf bytes.Buffer
	passed := RunGate(context.Background(), providers, &buf)
	if passed {
		t.Error("expected gate to fail on provider error")
	}
	if !strings.Contains(buf.String(), "provider crashed") {
		t.Errorf("expected error message in output, got: %s", buf.String())
	}
}

func TestRunGate_Empty(t *testing.T) {
	var buf bytes.Buffer
	passed := RunGate(context.Background(), nil, &buf)
	if !passed {
		t.Error("expected empty provider list to pass")
	}
}
