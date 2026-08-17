package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ruby-dev/soro/cli"
)

var version = cli.DevelopmentVersion

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	command := cli.New(cli.Settings{Version: version})
	if err := command.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "soro:", err)
		os.Exit(1)
	}
}
