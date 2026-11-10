// Command pnpm is the scripted fake pnpm that fixtures put first on PATH.
// See package vitefake.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/elentok/gx/testutil/vitefake"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(vitefake.Run(ctx, os.Args[1:], os.Getenv(vitefake.ControlEnv), os.Stdout))
}
