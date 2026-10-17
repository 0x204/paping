package ping

import (
	"errors"
	"testing"
	"time"
)

func TestStats(t *testing.T) {
	var s Stats
	for _, r := range []Result{
		{RTT: 30 * time.Millisecond},
		{Err: errors.New("connection refused")},
		{RTT: 10 * time.Millisecond},
		{RTT: 20 * time.Millisecond},
	} {
		s.Add(r)
	}

	if s.Attempted != 4 || s.Connected != 3 {
		t.Errorf("Attempted, Connected = %d, %d, want 4, 3", s.Attempted, s.Connected)
	}
	if got := s.Failed(); got != 1 {
		t.Errorf("Failed() = %d, want 1", got)
	}
	if s.Min != 10*time.Millisecond || s.Max != 30*time.Millisecond {
		t.Errorf("Min, Max = %v, %v, want 10ms, 30ms", s.Min, s.Max)
	}
	if got := s.Avg(); got != 20*time.Millisecond {
		t.Errorf("Avg() = %v, want 20ms", got)
	}
	// The square root of ((10ms)² + 0 + (10ms)²) / 3.
	if got := s.StdDev().Round(time.Microsecond); got != 8165*time.Microsecond {
		t.Errorf("StdDev() = %v, want 8.165ms", got)
	}
}

func TestStatsEqualTimes(t *testing.T) {
	// The squares of times this long are not exact in a float64.
	var s Stats
	for range 1000 {
		s.Add(Result{RTT: 4999999999 * time.Nanosecond})
	}
	if got := s.StdDev(); got != 0 {
		t.Errorf("StdDev() = %v, want 0", got)
	}
}

func TestStatsWithoutConnections(t *testing.T) {
	var s Stats
	s.Add(Result{Err: errors.New("i/o timeout")})

	if s.Min != 0 || s.Max != 0 {
		t.Errorf("Min, Max = %v, %v, want 0, 0", s.Min, s.Max)
	}
	if got := s.Avg(); got != 0 {
		t.Errorf("Avg() = %v, want 0", got)
	}
	if got := s.StdDev(); got != 0 {
		t.Errorf("StdDev() = %v, want 0", got)
	}
}
