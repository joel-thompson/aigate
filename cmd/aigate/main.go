package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/joelthompson/aigate/internal/cmd/root"
	"github.com/joelthompson/aigate/internal/exit"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd := root.NewCommand()
	if err := cmd.ExecuteContext(ctx); err != nil {
		os.Exit(exit.CodeFor(err))
	}
}
