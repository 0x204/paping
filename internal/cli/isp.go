package cli

import (
	"context"
	"net/netip"
	"sync"
	"time"
)

const (
	ispAttempts   = 3
	ispRetryDelay = 10 * time.Second
)

// ispLookup finds the organization that owns an address, which paping shows
// as its ISP. The lookup starts on the first call to get and runs in the
// background, so that it never holds up probing.
type ispLookup struct {
	fetch func(context.Context, netip.Addr) (string, error)
	addr  netip.Addr

	start sync.Once
	done  chan struct{} // closed when the lookup has finished
	org   string        // set before done is closed; empty if not found
}

// get returns the organization, or "" while it is unknown. The first call
// starts the lookup and waits up to wait for it to finish.
func (l *ispLookup) get(ctx context.Context, wait time.Duration) string {
	l.start.Do(func() {
		l.done = make(chan struct{})
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
		return l.org
	default:
		return ""
	}
}

// run fetches the organization, retrying failed requests. An answer without
// an organization is final, as asking again would only repeat it.
func (l *ispLookup) run(ctx context.Context) {
	defer close(l.done)
	for attempt := 1; ; attempt++ {
		org, err := l.fetch(ctx, l.addr)
		if err == nil {
			l.org = org
			return
		}
		if attempt == ispAttempts {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(ispRetryDelay):
		}
	}
}
