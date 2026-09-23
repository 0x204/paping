// Paping measures how long it takes to open a TCP connection to a host, much
// like ping does for ICMP echo requests.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatih/color"

	"github.com/0x204/paping/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], color.Output, os.Stderr)
	stop()
	os.Exit(code)
}
