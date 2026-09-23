package cli

import (
	"net"
	"strconv"
	"strings"
	"testing"

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
	var stdout, stderr strings.Builder
	if code := Run(t.Context(), []string{"-h"}, &stdout, &stderr); code != 0 {
		t.Errorf("exit status = %d, want 0", code)
	}
	if stdout.String() != usage {
		t.Errorf("stdout = %q, want the usage text", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
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
		{[]string{"-c", "-1", "example.com", "80"}, "invalid count -1"},
		{[]string{"-c", "many", "example.com", "80"}, `invalid value "many" for flag -c`},
		{[]string{"-i", "0s", "example.com", "80"}, "invalid interval 0s"},
		{[]string{"-t", "-1s", "example.com", "80"}, "invalid timeout -1s"},
		{[]string{"-x", "example.com", "80"}, "flag provided but not defined: -x"},
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
