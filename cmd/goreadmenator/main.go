// Command goreadmenator indiziert Quellcode-Repositories (C# und TypeScript/JavaScript)
// und beantwortet Fragen zu Symbolen, Verwendungen und Änderungen.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"goreadmenator/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app := &cli.App{Stdout: os.Stdout, Stderr: os.Stderr}
	os.Exit(app.Run(ctx, os.Args[1:]))
}
