package cli

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/fatih/color"

	"github.com/0x204/paping/internal/ping"
)

var (
	green = color.New(color.FgGreen).SprintFunc()
	red   = color.New(color.FgRed).SprintFunc()
	cyan  = color.New(color.FgCyan).SprintFunc()
)

// printer writes the report for probes of a single host and port.
type printer struct {
	w    io.Writer
	host string
	port uint16
}

// result prints the outcome of a probe. isp is shown for a successful one.
func (p *printer) result(r ping.Result, isp string) {
	if r.Err != nil {
		addr := net.JoinHostPort(p.host, strconv.Itoa(int(p.port)))
		fmt.Fprintln(p.w, red(fmt.Sprintf("Connection to %s failed: %v", addr, r.Err)))
		return
	}
	if isp == "" {
		isp = "Unknown"
	}
	fmt.Fprintf(p.w, "Connected to %s time=%s protocol=%s port=%s ISP=%s\n",
		green(p.host), green(millis(r.RTT)), green("TCP"), green(p.port), green(isp))
}

// summary prints the statistics shown when paping exits.
func (p *printer) summary(s *ping.Stats) {
	fmt.Fprintln(p.w, "\nConnection statistics:")
	if s.Attempted == 0 {
		fmt.Fprintln(p.w, "No attempts made.")
		return
	}
	failed := 100 * float64(s.Failed()) / float64(s.Attempted)
	fmt.Fprintf(p.w, "Attempted = %s, Connected = %s, Failed = %s (%s)\n",
		cyan(s.Attempted), cyan(s.Connected), cyan(s.Failed()), cyan(fmt.Sprintf("%.2f%%", failed)))
	if s.Connected == 0 {
		return
	}
	fmt.Fprintln(p.w, "Approximate connection times:")
	fmt.Fprintf(p.w, " Minimum = %s, Maximum = %s, Average = %s\n",
		cyan(millis(s.Min)), cyan(millis(s.Max)), cyan(millis(s.Avg())))
}

// millis formats d in milliseconds with two decimals, such as "12.34ms".
func millis(d time.Duration) string {
	return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
}
