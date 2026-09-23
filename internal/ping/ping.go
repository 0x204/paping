// Package ping measures how long it takes to open a TCP connection.
package ping

import (
	"context"
	"fmt"
	"iter"
	"net"
	"net/netip"
	"time"
)

// Options configures Probes.
type Options struct {
	// Interval is the time between the starts of consecutive probes. It must
	// be positive. A probe that takes longer delays the next one rather than
	// overlapping with it.
	Interval time.Duration

	// Timeout bounds how long each probe waits for the connection.
	Timeout time.Duration
}

// Result is the outcome of a single probe.
type Result struct {
	RTT time.Duration // time to establish the connection; zero if Err is set
	Err error         // why the connection failed, or nil
}

// Resolve returns the address to probe for host, which is either an IP
// address or a hostname. A hostname with both IPv4 and IPv6 addresses
// resolves to its first IPv4 address.
func Resolve(ctx context.Context, host string) (netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap(), nil
	}

	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, err
	}
	if len(addrs) == 0 {
		return netip.Addr{}, fmt.Errorf("no addresses found for %s", host)
	}
	for _, addr := range addrs {
		// The resolver may return IPv4 addresses in IPv4-mapped IPv6 form.
		if addr = addr.Unmap(); addr.Is4() {
			return addr, nil
		}
	}
	return addrs[0], nil
}

// Probe opens a TCP connection to target, timing the handshake, and closes it.
func Probe(ctx context.Context, target netip.AddrPort, timeout time.Duration) Result {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", target.String())
	rtt := time.Since(start)
	if err != nil {
		return Result{Err: err}
	}
	conn.Close()
	return Result{RTT: rtt}
}

// Probes returns an iterator that probes target once every opts.Interval
// until ctx is done. A probe cut short by ctx is not yielded.
func Probes(ctx context.Context, target netip.AddrPort, opts Options) iter.Seq[Result] {
	return func(yield func(Result) bool) {
		ticker := time.NewTicker(opts.Interval)
		defer ticker.Stop()

		for {
			r := Probe(ctx, target, opts.Timeout)
			if r.Err != nil && ctx.Err() != nil {
				return
			}
			if !yield(r) {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}
}
