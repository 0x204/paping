// Package cli implements the paping command.
package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/0x204/paping/internal/ipinfo"
	"github.com/0x204/paping/internal/ping"
)

const (
	probeInterval = 550 * time.Millisecond
	probeTimeout  = 5 * time.Second
	ispTimeout    = 5 * time.Second
)

// Run runs paping with args, the command-line arguments without the program
// name, and returns its exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "Usage: paping <host> <port>")
		return 1
	}
	host := args[0]
	port, err := strconv.ParseUint(args[1], 10, 16)
	if err != nil || port == 0 {
		fmt.Fprintf(stderr, "Invalid port number: %s\n", args[1])
		return 1
	}
	addr, err := ping.Resolve(ctx, host)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot resolve %s: %v\n", host, err)
		return 1
	}

	// Stops the ISP lookup once probing is over.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	opts := ping.Options{Interval: probeInterval, Timeout: probeTimeout}
	client := &ipinfo.Client{HTTPClient: &http.Client{Timeout: ispTimeout}}
	lookup := &ispLookup{fetch: client.Org, addr: addr}
	out := &printer{w: stdout, host: host, port: uint16(port)}

	var stats ping.Stats
	for r := range ping.Probes(ctx, netip.AddrPortFrom(addr, uint16(port)), opts) {
		stats.Add(r)
		var isp string
		if r.Err == nil {
			// Only a host that answers is looked up on ipinfo.io. The lookup
			// gets the rest of this probe's interval, so the first line
			// normally shows the ISP without holding up the next probe.
			isp = lookup.get(ctx, opts.Interval-r.RTT)
		}
		out.result(r, isp)
	}
	out.summary(&stats)
	return 0
}
