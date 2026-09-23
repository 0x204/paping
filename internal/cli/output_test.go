package cli

import (
	"errors"
	"net"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0x204/paping/internal/ping"
)

func TestPrinterHeader(t *testing.T) {
	tests := []struct {
		host string
		addr string
		want string
	}{
		{"example.com", "93.184.215.14", "Connecting to example.com [93.184.215.14] on TCP 443:\n\n"},
		{"192.0.2.1", "192.0.2.1", "Connecting to 192.0.2.1 on TCP 443:\n\n"},
		{"::ffff:192.0.2.1", "192.0.2.1", "Connecting to ::ffff:192.0.2.1 [192.0.2.1] on TCP 443:\n\n"},
	}
	for _, tt := range tests {
		var b strings.Builder
		p := &printer{w: &b, host: tt.host, port: 443}
		p.header(netip.MustParseAddr(tt.addr))
		if got := b.String(); got != tt.want {
			t.Errorf("header for %s:\ngot  %q\nwant %q", tt.host, got, tt.want)
		}
	}
}

func TestPrinterResult(t *testing.T) {
	refused := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: os.NewSyscallError("connect", errors.New("connection refused")),
	}
	timedOut := &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}

	tests := []struct {
		name string
		host string
		r    ping.Result
		isp  string
		want string
	}{
		{
			name: "connected",
			host: "example.com",
			r:    ping.Result{RTT: 12346 * time.Microsecond},
			isp:  "AS15133 Edgecast Inc.",
			want: "Connected to example.com time=12.35ms protocol=TCP port=443 ISP=AS15133 Edgecast Inc.\n",
		},
		{
			name: "ISP not known",
			host: "192.0.2.1",
			r:    ping.Result{RTT: time.Millisecond},
			want: "Connected to 192.0.2.1 time=1.00ms protocol=TCP port=443 ISP=Unknown\n",
		},
		{
			name: "refused",
			host: "example.com",
			r:    ping.Result{Err: refused},
			want: "Connection to example.com:443 failed: connection refused\n",
		},
		{
			name: "timed out",
			host: "example.com",
			r:    ping.Result{Err: timedOut},
			want: "Connection to example.com:443 failed: timed out\n",
		},
		{
			name: "IPv6",
			host: "2001:db8::1",
			r:    ping.Result{Err: refused},
			want: "Connection to [2001:db8::1]:443 failed: connection refused\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			p := &printer{w: &b, host: tt.host, port: 443}
			p.result(tt.r, tt.isp)
			if got := b.String(); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestPrinterSummary(t *testing.T) {
	tests := []struct {
		name  string
		stats ping.Stats
		want  string
	}{
		{
			name: "no attempts",
			want: "\nConnection statistics:\n" +
				"    No attempts made.\n",
		},
		{
			name:  "none connected",
			stats: ping.Stats{Attempted: 2},
			want: "\nConnection statistics:\n" +
				"    Attempted = 2, Connected = 0, Failed = 2 (100.00%)\n",
		},
		{
			name: "some connected",
			stats: ping.Stats{
				Attempted: 4,
				Connected: 3,
				Min:       10 * time.Millisecond,
				Max:       30 * time.Millisecond,
				Total:     60 * time.Millisecond,
			},
			want: "\nConnection statistics:\n" +
				"    Attempted = 4, Connected = 3, Failed = 1 (25.00%)\n" +
				"Approximate connection times:\n" +
				"    Minimum = 10.00ms, Maximum = 30.00ms, Average = 20.00ms\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			p := &printer{w: &b, host: "example.com", port: 443}
			p.summary(&tt.stats)
			if got := b.String(); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}
