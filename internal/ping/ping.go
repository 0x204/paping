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
// address or a hostname. network is "ip4" or "ip6" to accept only addresses
// of that family, or "ip" to accept both, in which case a hostname with both
// IPv4 and IPv6 addresses resolves to its first IPv4 address.
func Resolve(ctx context.Context, network, host string) (netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		addr = addr.Unmap()
		if !inNetwork(addr, network) {
			return netip.Addr{}, fmt.Errorf("%s is not an %s address", host, familyNames[network])
		}
		return addr, nil
	}

	addrs, err := net.DefaultResolver.LookupNetIP(ctx, network, host)
	if err != nil {
		return netip.Addr{}, err
	}
	var found netip.Addr
	for _, addr := range addrs {
		// The resolver may return IPv4 addresses in IPv4-mapped IPv6 form.
		addr = addr.Unmap()
		if !inNetwork(addr, network) {
			continue
		}
		if addr.Is4() {
			return addr, nil
		}
		if !found.IsValid() {
			found = addr
		}
	}
	if !found.IsValid() {
		return netip.Addr{}, fmt.Errorf("no addresses found for %s", host)
	}
	return found, nil
}

// familyNames names the address family of each network that accepts only one.
var familyNames = map[string]string{"ip4": "IPv4", "ip6": "IPv6"}

// inNetwork reports whether addr belongs to network, which is "ip4", "ip6"
// or "ip".
func inNetwork(addr netip.Addr, network string) bool {
	switch network {
	case "ip4":
		return addr.Is4()
	case "ip6":
		return addr.Is6()
	}
	return true
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
