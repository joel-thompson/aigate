package runner

import (
	"context"
	"errors"
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
	got := RunEnrich(context.Background(), providers)
	if !strings.Contains(got, "## First\noutput1") {
		t.Errorf("expected labeled first output, got:\n%s", got)
	}
	if !strings.Contains(got, "## Second\noutput2") {
		t.Errorf("expected labeled second output, got:\n%s", got)
	}
}

func TestRunEnrich_SomeFail(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{result: &provider.Result{Output: "good\n", Label: "OK"}},
		&stubProvider{err: errors.New("boom")},
		&stubProvider{result: &provider.Result{Output: "also good\n", Label: "Also OK"}},
	}
	got := RunEnrich(context.Background(), providers)
	if !strings.Contains(got, "## OK\ngood") {
		t.Errorf("expected first success, got:\n%s", got)
	}
	if !strings.Contains(got, "[error] boom") {
		t.Errorf("expected error line, got:\n%s", got)
	}
	if !strings.Contains(got, "## Also OK\nalso good") {
		t.Errorf("expected third success, got:\n%s", got)
	}
}

func TestRunEnrich_AllFail(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{err: errors.New("err1")},
		&stubProvider{err: errors.New("err2")},
	}
	got := RunEnrich(context.Background(), providers)
	if !strings.Contains(got, "[error] err1") {
		t.Errorf("expected first error, got:\n%s", got)
	}
	if !strings.Contains(got, "[error] err2") {
		t.Errorf("expected second error, got:\n%s", got)
	}
}

func TestRunEnrich_NoLabel(t *testing.T) {
	providers := []provider.Provider{
		&stubProvider{result: &provider.Result{Output: "bare output\n", Label: ""}},
	}
	got := RunEnrich(context.Background(), providers)
	if strings.Contains(got, "##") {
		t.Errorf("expected no label header, got:\n%s", got)
	}
	if !strings.Contains(got, "bare output") {
		t.Errorf("expected bare output, got:\n%s", got)
	}
}

func TestRunEnrich_Empty(t *testing.T) {
	got := RunEnrich(context.Background(), nil)
	if got != "" {
		t.Errorf("expected empty string for no providers, got %q", got)
	}
}
