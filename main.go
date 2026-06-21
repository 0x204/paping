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
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fatih/color"
)

type ConnectionStats struct {
	sync.Mutex
	Attempted    int
	Connected    int
	Failed       int
	HasConnected bool
	MinTime      time.Duration
	MaxTime      time.Duration
	TotalTime    time.Duration
}

type IPInfo struct {
	Org string `json:"org"`
}

const (
	ipInfoAPIURL  = "https://ipinfo.io/%s/json"
	dialTimeout   = 5 * time.Second
	httpTimeout   = 5 * time.Second
	pingInterval  = 550 * time.Millisecond
	maxConcurrent = 4
)

var (
	logger = log.New(os.Stdout, "", 0)
	org    atomic.Pointer[string]
)

func currentOrg() string {
	if p := org.Load(); p != nil {
		return *p
	}
	return "Unknown"
}

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

func ping(ctx context.Context, host, ip string, port int, stats *ConnectionStats) {
	dialer := net.Dialer{Timeout: dialTimeout}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	duration := time.Since(start)

	if err != nil {
		if ctx.Err() != nil {
			return
		}
		logger.Print(color.RedString("Connection to %s:%d failed: %v", host, port, err))
		stats.Lock()
		stats.Attempted++
		stats.Failed++
		stats.Unlock()
		return
	}
	_ = conn.Close()

	ms := float64(duration.Microseconds()) / 1000
	logger.Printf("Connected to "+color.GreenString("%s")+" time="+color.GreenString("%.2fms")+" protocol="+color.GreenString("TCP")+" port="+color.GreenString("%d")+" ISP="+color.GreenString("%s")+"\n", host, ms, port, currentOrg())

	stats.Lock()
	stats.Attempted++
	stats.Connected++
	stats.TotalTime += duration
	if !stats.HasConnected || duration < stats.MinTime {
		stats.MinTime = duration
	}
	if duration > stats.MaxTime {
		stats.MaxTime = duration
	}
	stats.HasConnected = true
	stats.Unlock()
}

func fetchOrg(ip string) {
	resp, err := (&http.Client{Timeout: httpTimeout}).Get(fmt.Sprintf(ipInfoAPIURL, ip))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var info IPInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return
	}
	if info.Org != "" {
		org.Store(&info.Org)
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

	go fetchOrg(ip)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	stats := &ConnectionStats{}
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrent)

	probe := func() {
		select {
		case sem <- struct{}{}:
		default:
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			ping(ctx, host, ip, port, stats)
		}()
	}

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	probe()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			printReport(stats)
			return
		case <-ticker.C:
			probe()
		}
	}
}

func printReport(stats *ConnectionStats) {
	stats.Lock()
	defer stats.Unlock()

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
