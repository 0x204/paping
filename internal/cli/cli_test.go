package cli

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
)

func TestMain(m *testing.M) {
	color.NoColor = true
	m.Run()
}

func TestRun(t *testing.T) {
	port := listen(t)

	var stdout, stderr strings.Builder
	code := Run(t.Context(), []string{"-c", "3", "-i", "1ms", "127.0.0.1", port}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit status = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if want := "Connecting to 127.0.0.1 on TCP " + port + ":\n\n"; !strings.HasPrefix(out, want) {
		t.Errorf("output does not start with %q:\n%s", want, out)
	}
	if got := strings.Count(out, "Connected to 127.0.0.1 time="); got != 3 {
		t.Errorf("got %d successful probes, want 3:\n%s", got, out)
	}
	if want := "Attempted = 3, Connected = 3, Failed = 0 (0.00%)\n"; !strings.Contains(out, want) {
		t.Errorf("output does not contain %q:\n%s", want, out)
	}
	if got := strings.Count(out, " ISP=Unknown\n"); got != 3 {
		t.Errorf("got %d lines with the ISP, want 3:\n%s", got, out)
	}
}

func TestRunWithoutISP(t *testing.T) {
	port := listen(t)

	var stdout, stderr strings.Builder
	code := Run(t.Context(), []string{"-n", "-c", "2", "-i", "1ms", "127.0.0.1", port}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit status = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if got := strings.Count(out, " port="+port+"\n"); got != 2 {
		t.Errorf("got %d lines without the ISP, want 2:\n%s", got, out)
	}
	if strings.Contains(out, "ISP=") {
		t.Errorf("output names the ISP despite -n:\n%s", out)
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := Run(t.Context(), []string{"-version"}, &stdout, &stderr); code != 0 {
		t.Errorf("exit status = %d, want 0", code)
	}
	if want := "paping " + version() + "\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

func TestRunNoConnection(t *testing.T) {
	port := closedPort(t)

	var stdout, stderr strings.Builder
	code := Run(t.Context(), []string{"-c", "2", "-i", "1ms", "127.0.0.1", port}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit status = %d, want 1", code)
	}

	out := stdout.String()
	failure := "Connection to 127.0.0.1:" + port + " failed: "
	if got := strings.Count(out, failure); got != 2 {
		t.Errorf("got %d failed probes, want 2:\n%s", got, out)
	}
	if want := "Attempted = 2, Connected = 0, Failed = 2 (100.00%)\n"; !strings.Contains(out, want) {
		t.Errorf("output does not contain %q:\n%s", want, out)
	}
}

func TestRunHelp(t *testing.T) {
	for _, args := range [][]string{
		{"-h"},
		{"--help"},
		{"example.com", "443", "-h"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr strings.Builder
			if code := Run(t.Context(), args, &stdout, &stderr); code != 0 {
				t.Errorf("exit status = %d, want 0", code)
			}
			if stdout.String() != usage {
				t.Errorf("stdout = %q, want the usage text", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want nothing", stderr.String())
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	defaults := config{host: "example.com", port: 443, network: "ip", interval: defaultInterval, timeout: defaultTimeout}
	custom := config{host: "example.com", port: 443, network: "ip", count: 5, interval: 200 * time.Millisecond, timeout: 2 * time.Second}
	ipv4 := defaults
	ipv4.network = "ip4"
	ipv6NoISP := defaults
	ipv6NoISP.network = "ip6"
	ipv6NoISP.noISP = true
	version := config{network: "ip", interval: defaultInterval, timeout: defaultTimeout, version: true}

	tests := []struct {
		args []string
		want config
	}{
		{[]string{"example.com", "443"}, defaults},
		{[]string{"-c", "5", "-i", "200ms", "-t", "2s", "example.com", "443"}, custom},
		{[]string{"example.com", "443", "-c", "5", "-i", "200ms", "-t", "2s"}, custom},
		{[]string{"-c", "5", "example.com", "-i", "200ms", "443", "-t", "2s"}, custom},
		{[]string{"-c=5", "--i=200ms", "--t", "2s", "example.com", "443"}, custom},
		{[]string{"--", "example.com", "443"}, defaults},
		{[]string{"-c", "5", "-i", "200ms", "-t", "2s", "--", "example.com", "443"}, custom},
		{[]string{"-4", "example.com", "443"}, ipv4},
		{[]string{"example.com", "-4", "443"}, ipv4},
		{[]string{"-6", "-n", "example.com", "443"}, ipv6NoISP},
		{[]string{"example.com", "443", "--6", "-n=true"}, ipv6NoISP},
		{[]string{"-version"}, version},
		{[]string{"example.com", "443", "--version"}, version},
	}
	for _, tt := range tests {
		got, err := parseArgs(tt.args)
		if err != nil {
			t.Errorf("parseArgs(%q): %v", tt.args, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseArgs(%q) = %+v, want %+v", tt.args, got, tt.want)
		}
	}
}

func TestRunInvalidCommandLine(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, "expected a host and a port"},
		{[]string{"example.com"}, "expected a host and a port"},
		{[]string{"example.com", "80", "443"}, "expected a host and a port"},
		{[]string{"example.com", "0"}, `invalid port "0"`},
		{[]string{"example.com", "65536"}, `invalid port "65536"`},
		{[]string{"example.com", "http"}, `invalid port "http"`},
		{[]string{"example.com", "-80"}, `invalid port "-80"`},
		{[]string{"example.com", "-1", "-c", "5"}, `invalid port "-1"`},
		{[]string{"--", "example.com", "-h"}, `invalid port "-h"`},
		{[]string{"--", "example.com", "443", "-c", "5"}, "expected a host and a port"},
		{[]string{"-c", "-1", "example.com", "80"}, "invalid count -1"},
		{[]string{"example.com", "80", "-c", "-1"}, "invalid count -1"},
		{[]string{"-c", "many", "example.com", "80"}, `invalid value "many" for flag -c`},
		{[]string{"example.com", "80", "-c"}, "flag needs an argument: -c"},
		{[]string{"-i", "0s", "example.com", "80"}, "invalid interval 0s"},
		{[]string{"-t", "-1s", "example.com", "80"}, "invalid timeout -1s"},
		{[]string{"-4", "-6", "example.com", "80"}, "-4 and -6 cannot be used together"},
		{[]string{"-n", "example.com"}, "expected a host and a port"},
		{[]string{"-x", "example.com", "80"}, "flag provided but not defined: -x"},
		{[]string{"example.com", "80", "-x"}, "flag provided but not defined: -x"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var stdout, stderr strings.Builder
			if code := Run(t.Context(), tt.args, &stdout, &stderr); code != 2 {
				t.Errorf("exit status = %d, want 2", code)
			}
			if want := "paping: " + tt.want; !strings.HasPrefix(stderr.String(), want) {
				t.Errorf("stderr = %q, want prefix %q", stderr.String(), want)
			}
			if !strings.HasSuffix(stderr.String(), usage) {
				t.Errorf("stderr = %q, want the usage text at the end", stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", stdout.String())
			}
		})
	}
}

func TestRunUnresolvableHost(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := Run(t.Context(), []string{"invalid..host", "80"}, &stdout, &stderr); code != 2 {
		t.Errorf("exit status = %d, want 2", code)
	}
	if want := "paping: lookup invalid..host"; !strings.HasPrefix(stderr.String(), want) {
		t.Errorf("stderr = %q, want prefix %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
}

func TestRunWrongAddressFamily(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := Run(t.Context(), []string{"-6", "192.0.2.1", "80"}, &stdout, &stderr); code != 2 {
		t.Errorf("exit status = %d, want 2", code)
	}
	if want := "paping: 192.0.2.1 is not an IPv6 address\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
}

// listen returns the port of a TCP listener on 127.0.0.1 that accepts
// connections until the test ends.
func listen(t *testing.T) string {
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
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}

// closedPort returns a port on 127.0.0.1 that nothing listens on.
func closedPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}
