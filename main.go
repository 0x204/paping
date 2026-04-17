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
	"syscall"
	"time"

	"github.com/fatih/color"
)

type ConnectionStats struct {
	sync.Mutex
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

var (
	logger           = log.New(os.Stdout, "", 0)
	ipInfoCache      = make(map[string]*IPInfo)
	ipInfoCacheMutex sync.Mutex
)

const (
	ipInfoAPIURL = "http://ipinfo.io/%s/json"
	dialTimeout  = 5 * time.Second
	httpTimeout  = 5 * time.Second
	pingInterval = 550 * time.Millisecond
)

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

func ping(host, ip string, port int, stats *ConnectionStats) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), dialTimeout)
	duration := time.Since(start)

	if err != nil {
		logger.Print(color.RedString("Connection to %s:%d failed: %v\n", host, port, err))
		stats.Lock()
		stats.Attempted++
		stats.Failed++
		stats.Unlock()
		return
	}
	_ = conn.Close()

	org := "Unknown"
	if info, err := getIPInfo(ip); err == nil && info.Org != "" {
		org = info.Org
	}

	ms := float64(duration.Microseconds()) / 1000
	logger.Printf("Connected to "+color.GreenString("%s")+" time="+color.GreenString("%.2fms")+" protocol="+color.GreenString("TCP")+" port="+color.GreenString("%d")+" ISP="+color.GreenString("%s")+"\n", host, ms, port, org)

	stats.Lock()
	stats.Attempted++
	stats.Connected++
	stats.TotalTime += duration
	if stats.MinTime == 0 || duration < stats.MinTime {
		stats.MinTime = duration
	}
	if duration > stats.MaxTime {
		stats.MaxTime = duration
	}
	stats.Unlock()
}

func getIPInfo(ip string) (*IPInfo, error) {
	ipInfoCacheMutex.Lock()
	if info, ok := ipInfoCache[ip]; ok {
		ipInfoCacheMutex.Unlock()
		return info, nil
	}
	ipInfoCacheMutex.Unlock()

	resp, err := (&http.Client{Timeout: httpTimeout}).Get(fmt.Sprintf(ipInfoAPIURL, ip))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var info IPInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	ipInfoCacheMutex.Lock()
	ipInfoCache[ip] = &info
	ipInfoCacheMutex.Unlock()
	return &info, nil
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
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	go ping(host, ip, port, stats)
	for {
		select {
		case <-ctx.Done():
			printReport(stats)
			return
		case <-ticker.C:
			go ping(host, ip, port, stats)
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
