package ping

import "time"

// Stats summarizes a series of probe results.
type Stats struct {
	Attempted int
	Connected int
	Min       time.Duration // fastest connection
	Max       time.Duration // slowest connection
	Total     time.Duration // sum of all connection times
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
