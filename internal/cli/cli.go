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
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/0x204/paping/internal/ipinfo"
	"github.com/0x204/paping/internal/ping"
)

const (
	defaultInterval = time.Second
	defaultTimeout  = 5 * time.Second
	ispTimeout      = 5 * time.Second
)

var usage = fmt.Sprintf(`Usage: paping [options] <host> <port>

Repeatedly connects to a TCP port and reports how long each connection takes
to establish.

Options may also follow the host and port. Any argument after "--" is taken
as the host or port, even one that starts with "-".

Options:
  -4            use IPv4 only
  -6            use IPv6 only
  -c count      stop after count probes (default: until interrupted)
  -i interval   time between probes (default %v)
  -n            do not look up the ISP on ipinfo.io
  -t timeout    connection timeout (default %v)
  -version      print the version and exit
`, defaultInterval, defaultTimeout)

// config holds the settings given on the command line.
type config struct {
	host     string
	port     uint16
	network  string // "ip4" or "ip6" to use only that address family, or "ip"
	count    int    // 0 means no limit
	interval time.Duration
	timeout  time.Duration
	noISP    bool
	version  bool
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
	if cfg.version {
		fmt.Fprintf(stdout, "paping %s\n", version())
		return 0
	}

	addr, err := ping.Resolve(ctx, cfg.network, cfg.host)
	if err != nil {
		fmt.Fprintf(stderr, "paping: %v\n", err)
		return 2
	}

	// Stops the ISP lookup once probing is over.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var lookup *ispLookup
	if !cfg.noISP {
		client := &ipinfo.Client{
			HTTPClient: &http.Client{Timeout: ispTimeout},
			Token:      os.Getenv("IPINFO_TOKEN"),
		}
		lookup = &ispLookup{fetch: client.Org, addr: addr}
	}
	out := &printer{w: stdout, host: cfg.host, port: cfg.port, showISP: !cfg.noISP}
	out.header(addr)

	opts := ping.Options{Interval: cfg.interval, Timeout: cfg.timeout}
	var stats ping.Stats
	for r := range ping.Probes(ctx, netip.AddrPortFrom(addr, cfg.port), opts) {
		stats.Add(r)
		var isp string
		if lookup != nil && r.Err == nil {
			// Only a host that answers is looked up on ipinfo.io. The first
			// line waits for the first request, but never past the next probe.
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

// parseArgs parses and validates the command line. With -version, it skips
// the validation, as nothing else on the command line is used.
func parseArgs(args []string) (config, error) {
	cfg := config{network: "ip", interval: defaultInterval, timeout: defaultTimeout}

	var ipv4, ipv6 bool
	fs := flag.NewFlagSet("paping", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // Run reports errors along with its own usage text.
	fs.BoolVar(&ipv4, "4", false, "")
	fs.BoolVar(&ipv6, "6", false, "")
	fs.IntVar(&cfg.count, "c", cfg.count, "")
	fs.DurationVar(&cfg.interval, "i", cfg.interval, "")
	fs.BoolVar(&cfg.noISP, "n", false, "")
	fs.DurationVar(&cfg.timeout, "t", cfg.timeout, "")
	fs.BoolVar(&cfg.version, "version", false, "")

	options, operands := splitArgs(fs, args)
	if err := fs.Parse(options); err != nil {
		return config{}, err
	}
	if cfg.version {
		return cfg, nil
	}
	if len(operands) != 2 {
		return config{}, errors.New("expected a host and a port")
	}

	cfg.host = operands[0]
	port, err := strconv.ParseUint(operands[1], 10, 16)
	if err != nil || port == 0 {
		return config{}, fmt.Errorf("invalid port %q", operands[1])
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

	switch {
	case ipv4 && ipv6:
		return config{}, errors.New("-4 and -6 cannot be used together")
	case ipv4:
		cfg.network = "ip4"
	case ipv6:
		cfg.network = "ip6"
	}
	return cfg, nil
}

// splitArgs separates options from operands, so that options may come before,
// between or after the host and port. Everything after "--" is an operand, as
// is an argument that looks like a negative number but is not an option, such
// as a mistyped port. An option that takes a value and is written as "-c 5"
// takes the next argument with it.
func splitArgs(fs *flag.FlagSet, args []string) (options, operands []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		f := fs.Lookup(name)
		switch {
		case arg == "--":
			return options, append(operands, args[i+1:]...)
		case len(arg) < 2 || arg[0] != '-':
			operands = append(operands, arg)
		case f == nil && '0' <= arg[1] && arg[1] <= '9':
			operands = append(operands, arg)
		default:
			options = append(options, arg)
			if f != nil && !hasValue && !isBoolFlag(f) && i+1 < len(args) {
				i++
				options = append(options, args[i])
			}
		}
	}
	return options, operands
}

// isBoolFlag reports whether f is a boolean option, which takes no value
// unless one is attached, as in "-n=false".
func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// version returns the version paping was built as: a release such as
// "v1.2.0", a pseudo-version for a build between releases, or "(devel)" when
// version control information is not available.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "(devel)"
	}
	return info.Main.Version
}
