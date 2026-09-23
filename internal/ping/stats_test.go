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

	want := Stats{
		Attempted: 4,
		Connected: 3,
		Min:       10 * time.Millisecond,
		Max:       30 * time.Millisecond,
		Total:     60 * time.Millisecond,
	}
	if s != want {
		t.Errorf("Stats = %+v, want %+v", s, want)
	}
	if got := s.Failed(); got != 1 {
		t.Errorf("Failed() = %d, want 1", got)
	}
	if got := s.Avg(); got != 20*time.Millisecond {
		t.Errorf("Avg() = %v, want 20ms", got)
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
}
