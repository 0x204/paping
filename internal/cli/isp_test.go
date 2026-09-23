package cli

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"testing/synctest"
	"time"
)

// fakeFetch answers after delay, failing its first failures calls.
type fakeFetch struct {
	delay    time.Duration
	failures int
	org      string
	calls    int
}

func (f *fakeFetch) fetch(ctx context.Context, _ netip.Addr) (string, error) {
	f.calls++
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(f.delay):
	}
	if f.calls <= f.failures {
		return "", errors.New("503 Service Unavailable")
	}
	return f.org, nil
}

func newTestLookup(f *fakeFetch) *ispLookup {
	return &ispLookup{fetch: f.fetch, addr: netip.MustParseAddr("192.0.2.1")}
}

func TestISPLookupFirstGetWaits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newTestLookup(&fakeFetch{delay: 100 * time.Millisecond, org: "AS64500 Example"})

		start := time.Now()
		if got := l.get(t.Context(), time.Second); got != "AS64500 Example" {
			t.Errorf("get() = %q, want %q", got, "AS64500 Example")
		}
		if elapsed := time.Since(start); elapsed != 100*time.Millisecond {
			t.Errorf("get() took %v, want 100ms", elapsed)
		}
	})
}

func TestISPLookupWaitIsBounded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newTestLookup(&fakeFetch{delay: 2 * time.Second, org: "AS64500 Example"})

		start := time.Now()
		if got := l.get(t.Context(), 500*time.Millisecond); got != "" {
			t.Errorf("get() = %q before the lookup finished, want \"\"", got)
		}
		if got := l.get(t.Context(), time.Minute); got != "" {
			t.Errorf("second get() = %q before the lookup finished, want \"\"", got)
		}
		if elapsed := time.Since(start); elapsed != 500*time.Millisecond {
			t.Errorf("get() calls took %v, want 500ms", elapsed)
		}

		time.Sleep(2 * time.Second)
		if got := l.get(t.Context(), 0); got != "AS64500 Example" {
			t.Errorf("get() = %q after the lookup finished, want %q", got, "AS64500 Example")
		}
	})
}

func TestISPLookupRetries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeFetch{failures: 1, org: "AS64500 Example"}
		l := newTestLookup(f)

		if got := l.get(t.Context(), 0); got != "" {
			t.Errorf("get() = %q after a failed lookup, want \"\"", got)
		}
		time.Sleep(ispRetryDelay)
		synctest.Wait()
		if got := l.get(t.Context(), 0); got != "AS64500 Example" {
			t.Errorf("get() = %q after a retry, want %q", got, "AS64500 Example")
		}
		if f.calls != 2 {
			t.Errorf("fetch called %d times, want 2", f.calls)
		}
	})
}

func TestISPLookupGivesUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeFetch{failures: ispAttempts + 1, org: "AS64500 Example"}
		l := newTestLookup(f)

		l.get(t.Context(), 0)
		time.Sleep(time.Hour)
		synctest.Wait()
		if f.calls != ispAttempts {
			t.Errorf("fetch called %d times, want %d", f.calls, ispAttempts)
		}
		if got := l.get(t.Context(), 0); got != "" {
			t.Errorf("get() = %q, want \"\"", got)
		}
	})
}

func TestISPLookupDoesNotRetryEmptyAnswer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeFetch{}
		l := newTestLookup(f)

		l.get(t.Context(), 0)
		time.Sleep(time.Hour)
		synctest.Wait()
		if f.calls != 1 {
			t.Errorf("fetch called %d times, want 1", f.calls)
		}
	})
}
