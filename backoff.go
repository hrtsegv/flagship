package flagship

import (
	"context"
	"math/rand/v2"
	"time"
)

// backoffDuration returns a random duration in [0, min*2^attempt], capped
// at max. Randomizing the full range (full jitter), rather than sleeping
// for the exponential value itself, spreads reconnect load out across
// many SDK instances instead of having them all retry in lockstep -- e.g.
// right after the control plane restarts and every connected SDK drops
// at once.
func backoffDuration(attempt int, minBackoff, maxBackoff time.Duration) time.Duration {
	upper := minBackoff
	for i := 0; i < attempt && upper < maxBackoff; i++ {
		upper *= 2
		if upper <= 0 { // overflow
			upper = maxBackoff
			break
		}
	}
	if upper > maxBackoff {
		upper = maxBackoff
	}
	return rand.N(upper + 1)
}

// waitBackoff sleeps for d, or returns false early if ctx is canceled
// first.
func waitBackoff(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
