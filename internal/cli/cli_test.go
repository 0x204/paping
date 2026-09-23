package cli

import (
	"context"
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
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)

	// Long enough for the first probe, short of the second.
	ctx, cancel := context.WithTimeout(t.Context(), probeInterval/2)
	defer cancel()

	var stdout, stderr strings.Builder
	if code := Run(ctx, []string{"127.0.0.1", port}, &stdout, &stderr); code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Connected to 127.0.0.1 time=",
		" protocol=TCP port=" + port + " ISP=Unknown\n",
		"Attempted = 1, Connected = 1, Failed = 0 (0.00%)\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no arguments", nil, "Usage: paping <host> <port>\n"},
		{"too many arguments", []string{"example.com", "80", "443"}, "Usage: paping <host> <port>\n"},
		{"port zero", []string{"example.com", "0"}, "Invalid port number: 0\n"},
		{"port too large", []string{"example.com", "65536"}, "Invalid port number: 65536\n"},
		{"port not a number", []string{"example.com", "http"}, "Invalid port number: http\n"},
		{"unresolvable host", []string{"invalid..host", "80"}, "Cannot resolve invalid..host: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			var stdout, stderr strings.Builder
			if code := Run(ctx, tt.args, &stdout, &stderr); code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if !strings.HasPrefix(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want prefix %q", stderr.String(), tt.want)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", stdout.String())
			}
		})
	}
}
