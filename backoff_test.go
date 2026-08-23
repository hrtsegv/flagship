package flagship

import (
	"context"
	"testing"
	"time"
)

func TestBackoffDuration_FirstAttemptIsWithinMin(t *testing.T) {
	min, max := 100*time.Millisecond, 10*time.Second
	for i := 0; i < 50; i++ {
		d := backoffDuration(0, min, max)
		if d < 0 || d > min {
			t.Fatalf("backoffDuration(0, ...) = %v, want in [0, %v]", d, min)
		}
	}
}

func TestBackoffDuration_GrowsWithAttemptThenCaps(t *testing.T) {
	min, max := 10*time.Millisecond, 100*time.Millisecond

	// attempt=0 is bounded by min; attempt large enough to have doubled
	// past max is bounded by max. Run many samples since the function is
	// randomized (full jitter) and only the upper bound is deterministic.
	for i := 0; i < 100; i++ {
		if d := backoffDuration(0, min, max); d > min {
			t.Fatalf("backoffDuration(0, %v, %v) = %v, want <= %v", min, max, d, min)
		}
	}
	for i := 0; i < 100; i++ {
		if d := backoffDuration(10, min, max); d > max {
			t.Fatalf("backoffDuration(10, %v, %v) = %v, want <= %v (capped)", min, max, d, max)
		}
	}
}

func TestBackoffDuration_NeverNegative(t *testing.T) {
	// A large attempt count would overflow a naive min<<attempt shift;
	// this must saturate at max instead of wrapping around to a negative
	// duration.
	d := backoffDuration(1000, time.Second, time.Minute)
	if d < 0 || d > time.Minute {
		t.Fatalf("backoffDuration(1000, ...) = %v, want in [0, 1m]", d)
	}
}

func TestWaitBackoff_ReturnsTrueAfterElapsing(t *testing.T) {
	if !waitBackoff(context.Background(), time.Millisecond) {
		t.Fatal("waitBackoff() = false, want true when the context is never canceled")
	}
}

func TestWaitBackoff_ReturnsFalseOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if waitBackoff(ctx, time.Second) {
		t.Fatal("waitBackoff() = true, want false for an already-canceled context")
	}
}
