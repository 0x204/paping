package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
)

type ConnectionStats struct {
	Attempted int
	Connected int
	Failed    int
	MinTime   time.Duration
	MaxTime   time.Duration
	TotalTime time.Duration
}

type IPInfo struct {
	Org string `json:"org"`
}

const (
	ipInfoAPIURL = "https://ipinfo.io/%s/json"
	dialTimeout  = 5 * time.Second
	httpTimeout  = 5 * time.Second
	pingInterval = 550 * time.Millisecond
	unknownOrg   = "Unknown"
	// maxOrgAttempts bounds the requests spent on the probed IP, and
	// orgRetryDelay spaces them out so a short outage cannot use them all up.
	maxOrgAttempts = 3
	orgRetryDelay  = 10 * time.Second
)

var (
	logger    = log.New(color.Output, "", 0)
	errLogger = log.New(os.Stderr, "", 0)
)

func isValidPort(port int) bool {
	return port >= 1 && port <= 65535
}

func resolveHost(host string) (string, error) {
	// netip, unlike net.ParseIP, accepts a zone such as fe80::1%eth0, which
	// the resolver would drop.
	if _, err := netip.ParseAddr(host); err == nil {
		return host, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4.String(), nil
		}
	}
	if len(ips) > 0 {
		return ips[0].String(), nil
	}
	return "", fmt.Errorf("no addresses for %s", host)
}

// orgLookup fetches the organization that owns the probed IP. It starts on the
// first successful probe, so an address that never answers is not disclosed to
// ipinfo.io, and runs in the background, so it never holds up probing.
type orgLookup struct {
	ip    string
	start sync.Once
	done  chan struct{} // closed when the lookup has finished
	org   string        // valid once done is closed; empty if not found
}

func newOrgLookup(ip string) *orgLookup {
	return &orgLookup{ip: ip, done: make(chan struct{})}
}

// get returns the organization, or unknownOrg while it is not known. The first
// call starts the lookup and waits up to wait for it to finish.
func (l *orgLookup) get(ctx context.Context, wait time.Duration) string {
	l.start.Do(func() {
		go l.run(ctx)
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-l.done:
		case <-timer.C:
		case <-ctx.Done():
		}
	})
	select {
	case <-l.done:
		if l.org != "" {
			return l.org
		}
	default:
	}
	return unknownOrg
}

func (l *orgLookup) run(ctx context.Context) {
	defer close(l.done)
	// ipinfo.io knows no organization for loopback, private or link-local
	// addresses, so asking would only disclose internal addressing.
	if ip := net.ParseIP(l.ip); !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return
	}
	for attempt := 1; ; attempt++ {
		// An answer without an organization is final too: asking again would
		// only repeat it.
		org, err := fetchOrg(ctx, l.ip)
		if err == nil {
			l.org = org
			return
		}
		if attempt == maxOrgAttempts {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(orgRetryDelay):
		}
	}
}

func fetchOrg(ctx context.Context, ip string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(ipInfoAPIURL, ip), nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ipinfo.io returned %s", resp.Status)
	}

	var info IPInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	return info.Org, nil
}

func ping(ctx context.Context, host, ip string, port int, stats *ConnectionStats, isp *orgLookup) {
	dialer := net.Dialer{Timeout: dialTimeout}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	duration := time.Since(start)

	if err != nil {
		if ctx.Err() != nil {
			return
		}
		logger.Print(color.RedString("Connection to %s failed: %v", net.JoinHostPort(host, strconv.Itoa(port)), err))
		stats.Attempted++
		stats.Failed++
		return
	}
	_ = conn.Close()

	// Give the ISP lookup the rest of this probe's interval, so the first line
	// normally shows it without the next probe being held up.
	org := isp.get(ctx, pingInterval-duration)

	ms := float64(duration.Microseconds()) / 1000
	logger.Printf("Connected to "+color.GreenString("%s")+" time="+color.GreenString("%.2fms")+" protocol="+color.GreenString("TCP")+" port="+color.GreenString("%d")+" ISP="+color.GreenString("%s")+"\n", host, ms, port, org)

	stats.Attempted++
	stats.Connected++
	stats.TotalTime += duration
	if stats.Connected == 1 || duration < stats.MinTime {
		stats.MinTime = duration
	}
	if duration > stats.MaxTime {
		stats.MaxTime = duration
	}
}

func main() {
	if len(os.Args) != 3 {
		errLogger.Fatal("Usage: paping <host> <port>")
	}

	host := os.Args[1]
	port, err := strconv.Atoi(os.Args[2])
	if err != nil || !isValidPort(port) {
		errLogger.Fatalf("Invalid port number: %s", os.Args[2])
	}

	ip, err := resolveHost(host)
	if err != nil {
		errLogger.Fatalf("Cannot resolve %s: %v", host, err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	stats := &ConnectionStats{}
	isp := newOrgLookup(ip)

	// One probe at a time. A probe that outlasts pingInterval delays the next
	// one instead of overlapping with it, and the pending tick fires as soon
	// as it finishes, so pacing recovers without any probe being dropped.
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for ctx.Err() == nil {
		ping(ctx, host, ip, port, stats, isp)
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}

	printReport(stats)
}

func printReport(stats *ConnectionStats) {
	logger.Print("\nConnection statistics:\n")
	if stats.Attempted == 0 {
		logger.Print("No attempts made.\n")
		return
	}

	failureRate := float64(stats.Failed) / float64(stats.Attempted) * 100
	logger.Printf("Attempted = "+color.CyanString("%d")+", Connected = "+color.CyanString("%d")+", Failed = "+color.CyanString("%d")+" ("+color.CyanString("%.2f%%")+")\n", stats.Attempted, stats.Connected, stats.Failed, failureRate)

	if stats.Connected > 0 {
		minMs := float64(stats.MinTime.Microseconds()) / 1000
		maxMs := float64(stats.MaxTime.Microseconds()) / 1000
		avgMs := float64(stats.TotalTime.Microseconds()) / float64(stats.Connected) / 1000
		logger.Print("Approximate connection times:\n")
		logger.Printf(" Minimum = "+color.CyanString("%.2fms")+", Maximum = "+color.CyanString("%.2fms")+", Average = "+color.CyanString("%.2fms")+"\n", minMs, maxMs, avgMs)
	}
}
