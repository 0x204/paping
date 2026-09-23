// Package cli implements the paping command.
package cli

import (
	"context"
	"errors"
	"flag"
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
	defaultInterval = 550 * time.Millisecond
	defaultTimeout  = 5 * time.Second
	ispTimeout      = 5 * time.Second
)

var usage = fmt.Sprintf(`Usage: paping [options] <host> <port>

Repeatedly connects to a TCP port and reports how long each connection takes
to establish.

Options:
  -c count      stop after count probes (default: until interrupted)
  -i interval   time between probes (default %v)
  -t timeout    connection timeout (default %v)
`, defaultInterval, defaultTimeout)

// config holds the settings given on the command line.
type config struct {
	host     string
	port     uint16
	count    int // 0 means no limit
	interval time.Duration
	timeout  time.Duration
}

// Run runs paping with args, the command-line arguments without the program
// name. It returns the exit status: 0 if any connection succeeded, 1 if none
// did, and 2 if the command line is invalid or the host cannot be resolved.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, err := parseArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "paping: %v\n\n%s", err, usage)
		return 2
	}

	addr, err := ping.Resolve(ctx, cfg.host)
	if err != nil {
		fmt.Fprintf(stderr, "paping: %v\n", err)
		return 2
	}

	// Stops the ISP lookup once probing is over.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	client := &ipinfo.Client{HTTPClient: &http.Client{Timeout: ispTimeout}}
	lookup := &ispLookup{fetch: client.Org, addr: addr}
	out := &printer{w: stdout, host: cfg.host, port: cfg.port}
	out.header(addr)

	opts := ping.Options{Interval: cfg.interval, Timeout: cfg.timeout}
	var stats ping.Stats
	for r := range ping.Probes(ctx, netip.AddrPortFrom(addr, cfg.port), opts) {
		stats.Add(r)
		var isp string
		if r.Err == nil {
			// Only a host that answers is looked up on ipinfo.io. The lookup
			// gets the rest of this probe's interval, so the first line
			// normally shows the ISP without holding up the next probe.
			isp = lookup.get(ctx, opts.Interval-r.RTT)
		}
		out.result(r, isp)
		if stats.Attempted == cfg.count {
			break
		}
	}
	out.summary(&stats)

	if stats.Connected == 0 {
		return 1
	}
	return 0
}

// parseArgs parses and validates the command line.
func parseArgs(args []string) (config, error) {
	cfg := config{interval: defaultInterval, timeout: defaultTimeout}

	fs := flag.NewFlagSet("paping", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // Run reports errors along with its own usage text.
	fs.IntVar(&cfg.count, "c", cfg.count, "")
	fs.DurationVar(&cfg.interval, "i", cfg.interval, "")
	fs.DurationVar(&cfg.timeout, "t", cfg.timeout, "")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 2 {
		return config{}, errors.New("expected a host and a port")
	}

	cfg.host = fs.Arg(0)
	port, err := strconv.ParseUint(fs.Arg(1), 10, 16)
	if err != nil || port == 0 {
		return config{}, fmt.Errorf("invalid port %q", fs.Arg(1))
	}
	cfg.port = uint16(port)

	switch {
	case cfg.count < 0:
		return config{}, fmt.Errorf("invalid count %d", cfg.count)
	case cfg.interval <= 0:
		return config{}, fmt.Errorf("invalid interval %v", cfg.interval)
	case cfg.timeout <= 0:
		return config{}, fmt.Errorf("invalid timeout %v", cfg.timeout)
	}
	return cfg, nil
}
