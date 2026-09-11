package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
	// maxOrgAttempts bounds the lookups spent on a single IP. Failures are not
	// cached, so a transient error recovers on a later probe, but a service
	// that is down or rate-limiting us stops delaying every probe.
	maxOrgAttempts = 3
)

var logger = log.New(os.Stdout, "", 0)

func isValidPort(port int) bool {
	return port >= 1 && port <= 65535
}

func resolveHost(host string) (string, error) {
	if net.ParseIP(host) != nil {
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

// orgEntry is the organization lookup for one IP.
type orgEntry struct {
	org      string // resolved organization, empty until a lookup succeeds
	failures int    // lookups already spent on this IP without success
}

// orgCache maps an IP to its organization. Probes run one at a time on a
// single goroutine, so it needs no locking.
type orgCache map[string]*orgEntry

// lookup returns the organization for ip, fetching it on first use. Only
// successful lookups are cached, so a transient failure is retried on the next
// probe rather than pinning the display to "Unknown" for the whole run.
func (c orgCache) lookup(ctx context.Context, ip string) string {
	e := c[ip]
	if e == nil {
		e = &orgEntry{}
		c[ip] = e
	}
	switch {
	case e.org != "":
		return e.org
	case e.failures >= maxOrgAttempts:
		return unknownOrg
	}

	org, err := fetchOrg(ctx, ip)
	if err != nil || org == "" {
		e.failures++
		return unknownOrg
	}
	e.org = org
	return e.org
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

func ping(ctx context.Context, host, ip string, port int, stats *ConnectionStats, orgs orgCache) {
	dialer := net.Dialer{Timeout: dialTimeout}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	duration := time.Since(start)

	if err != nil {
		if ctx.Err() != nil {
			return
		}
		logger.Print(color.RedString("Connection to %s:%d failed: %v", host, port, err))
		stats.Attempted++
		stats.Failed++
		return
	}
	_ = conn.Close()

	// Resolved only once the connect has succeeded and duration is already
	// measured, so a host that never answers is never disclosed to a third
	// party and the lookup cannot inflate the reported latency.
	org := orgs.lookup(ctx, ip)

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
		logger.Fatal("Usage: paping <host> <port>")
	}

	host := os.Args[1]
	port, err := strconv.Atoi(os.Args[2])
	if err != nil || !isValidPort(port) {
		logger.Fatalf("Invalid port number: %s", os.Args[2])
	}

	ip, err := resolveHost(host)
	if err != nil {
		logger.Fatalf("Cannot resolve %s: %v", host, err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	stats := &ConnectionStats{}
	orgs := orgCache{}

	// One probe at a time. A probe that outlasts pingInterval delays the next
	// one instead of overlapping with it, and the pending tick fires as soon
	// as it finishes, so pacing recovers without any probe being dropped.
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for ctx.Err() == nil {
		ping(ctx, host, ip, port, stats, orgs)
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

	successRate := float64(stats.Connected) / float64(stats.Attempted) * 100
	logger.Printf("Attempted = "+color.CyanString("%d")+", Connected = "+color.CyanString("%d")+", Failed = "+color.CyanString("%d")+" ("+color.CyanString("%.2f%%")+")\n", stats.Attempted, stats.Connected, stats.Failed, successRate)

	if stats.Connected > 0 {
		minMs := float64(stats.MinTime.Microseconds()) / 1000
		maxMs := float64(stats.MaxTime.Microseconds()) / 1000
		avgMs := float64(stats.TotalTime.Microseconds()) / float64(stats.Connected) / 1000
		logger.Print("Approximate connection times:\n")
		logger.Printf(" Minimum = "+color.CyanString("%.2fms")+", Maximum = "+color.CyanString("%.2fms")+", Average = "+color.CyanString("%.2fms")+"\n", minMs, maxMs, avgMs)
	}
}
