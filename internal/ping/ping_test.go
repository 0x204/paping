package ping

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		network string
		host    string
		want    string
	}{
		{"ip", "192.0.2.1", "192.0.2.1"},
		{"ip", "2001:db8::1", "2001:db8::1"},
		{"ip", "::ffff:192.0.2.1", "192.0.2.1"},
		{"ip", "fe80::1%eth0", "fe80::1%eth0"},
		{"ip", "localhost", "127.0.0.1"},
		{"ip4", "192.0.2.1", "192.0.2.1"},
		{"ip4", "::ffff:192.0.2.1", "192.0.2.1"},
		{"ip4", "localhost", "127.0.0.1"},
		{"ip6", "2001:db8::1", "2001:db8::1"},
	}
	for _, tt := range tests {
		got, err := Resolve(t.Context(), tt.network, tt.host)
		if err != nil {
			t.Errorf("Resolve(%s, %q): %v", tt.network, tt.host, err)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("Resolve(%s, %q) = %s, want %s", tt.network, tt.host, got, tt.want)
		}
	}
}

func TestResolveWrongFamily(t *testing.T) {
	tests := []struct {
		network string
		host    string
		want    string
	}{
		{"ip4", "2001:db8::1", "2001:db8::1 is not an IPv4 address"},
		{"ip6", "192.0.2.1", "192.0.2.1 is not an IPv6 address"},
		{"ip6", "::ffff:192.0.2.1", "::ffff:192.0.2.1 is not an IPv6 address"},
	}
	for _, tt := range tests {
		addr, err := Resolve(t.Context(), tt.network, tt.host)
		if err == nil {
			t.Errorf("Resolve(%s, %q) = %s, want an error", tt.network, tt.host, addr)
			continue
		}
		if err.Error() != tt.want {
			t.Errorf("Resolve(%s, %q) error = %q, want %q", tt.network, tt.host, err, tt.want)
		}
	}
}

func TestResolveInvalidHost(t *testing.T) {
	if addr, err := Resolve(t.Context(), "ip", "invalid..host"); err == nil {
		t.Errorf("Resolve returned %s, want an error", addr)
	}
}

func TestProbe(t *testing.T) {
	r := Probe(t.Context(), listen(t), time.Second)
	if r.Err != nil {
		t.Fatalf("Probe: %v", r.Err)
	}
	if r.RTT <= 0 {
		t.Errorf("RTT = %v, want > 0", r.RTT)
	}
}

func TestProbeRefused(t *testing.T) {
	r := Probe(t.Context(), closedPort(t), time.Second)
	if r.Err == nil {
		t.Fatal("Probe of a closed port succeeded")
	}
	if r.RTT != 0 {
		t.Errorf("RTT = %v, want 0 for a failed probe", r.RTT)
	}
}

func TestProbes(t *testing.T) {
	target := listen(t)
	var n int
	for r := range Probes(t.Context(), target, Options{Interval: time.Millisecond, Timeout: time.Second}) {
		if r.Err != nil {
			t.Fatalf("probe %d: %v", n, r.Err)
		}
		if n++; n == 3 {
			break
		}
	}
}

func TestProbesStopsWhenContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var n int
	for range Probes(ctx, listen(t), Options{Interval: time.Millisecond, Timeout: time.Second}) {
		n++
		cancel()
	}
	if n != 1 {
		t.Errorf("got %d results, want 1", n)
	}
}

func TestProbesSkipsInterruptedProbe(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for r := range Probes(ctx, listen(t), Options{Interval: time.Millisecond, Timeout: time.Second}) {
		t.Errorf("got result %+v from a canceled context", r)
	}
}

// listen returns the address of a TCP listener that accepts connections until
// the test ends.
func listen(t *testing.T) netip.AddrPort {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	return netip.MustParseAddrPort(ln.Addr().String())
}

// closedPort returns an address on which nothing is listening.
func closedPort(t *testing.T) netip.AddrPort {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := netip.MustParseAddrPort(ln.Addr().String())
	ln.Close()
	return addr
}
