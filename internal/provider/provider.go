package provider

import "context"

type Result struct {
	Output   string
	ExitCode int
	Label    string
}

type Provider interface {
	Run(ctx context.Context) (*Result, error)
}
