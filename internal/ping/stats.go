package ping

import (
	"math"
	"time"
)

// Stats summarizes a series of probe results.
type Stats struct {
	Attempted int
	Connected int
	Min       time.Duration // fastest connection
	Max       time.Duration // slowest connection
	Total     time.Duration // sum of all connection times

	mean float64 // mean connection time in nanoseconds
	m2   float64 // sum of squared differences from mean
}

// Add records the result of a probe.
func (s *Stats) Add(r Result) {
	s.Attempted++
	if r.Err != nil {
		return
	}
	s.Connected++
	s.Total += r.RTT
	if s.Connected == 1 || r.RTT < s.Min {
		s.Min = r.RTT
	}
	if r.RTT > s.Max {
		s.Max = r.RTT
	}

	// Welford's algorithm, which stays accurate where summing the squares of
	// the times would lose precision to rounding.
	x := float64(r.RTT)
	delta := x - s.mean
	s.mean += delta / float64(s.Connected)
	s.m2 += delta * (x - s.mean)
}

// Failed returns the number of probes that did not connect.
func (s *Stats) Failed() int {
	return s.Attempted - s.Connected
}

// Avg returns the mean connection time, or zero if no probe connected.
func (s *Stats) Avg() time.Duration {
	if s.Connected == 0 {
		return 0
	}
	return s.Total / time.Duration(s.Connected)
}

// StdDev returns the standard deviation of the connection times, which shows
// how much they vary, or zero if no probe connected.
func (s *Stats) StdDev() time.Duration {
	if s.Connected == 0 {
		return 0
	}
	return time.Duration(math.Sqrt(s.m2 / float64(s.Connected)))
}
