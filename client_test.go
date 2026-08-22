package flagship

import (
	"sync"
	"testing"
)

func TestClient_Bool_ExistingEnabledFlag(t *testing.T) {
	snap := NewSnapshot("production", 1, []Flag{{Key: "new-checkout", Enabled: true}})
	client := NewClient(snap)

	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("Bool() = %v, want true", got)
	}
}

func TestClient_Bool_ExistingDisabledFlag(t *testing.T) {
	snap := NewSnapshot("production", 1, []Flag{{Key: "new-checkout", Enabled: false}})
	client := NewClient(snap)

	if got := client.Bool("new-checkout", true); got != false {
		t.Errorf("Bool() = %v, want false", got)
	}
}

func TestClient_Bool_MissingFlagReturnsDefault(t *testing.T) {
	snap := NewSnapshot("production", 1, []Flag{{Key: "other-flag", Enabled: true}})
	client := NewClient(snap)

	if got := client.Bool("does-not-exist", false); got != false {
		t.Errorf("Bool() = %v, want false (default)", got)
	}
	if got := client.Bool("does-not-exist", true); got != true {
		t.Errorf("Bool() = %v, want true (default)", got)
	}
}

func TestClient_Bool_NoConfigurationReturnsDefault(t *testing.T) {
	client := NewClient(nil)

	if got := client.Bool("new-checkout", false); got != false {
		t.Errorf("Bool() = %v, want false (default)", got)
	}
	if got := client.Bool("new-checkout", true); got != true {
		t.Errorf("Bool() = %v, want true (default)", got)
	}
}

func TestClient_UpdateSnapshot_NewerReplacesCurrent(t *testing.T) {
	v1 := NewSnapshot("production", 1, []Flag{{Key: "new-checkout", Enabled: false}})
	v2 := NewSnapshot("production", 2, []Flag{{Key: "new-checkout", Enabled: true}})
	client := NewClient(v1)

	if applied := client.UpdateSnapshot(v2); !applied {
		t.Fatal("UpdateSnapshot() = false, want true for a newer version")
	}
	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("Bool() after update = %v, want true", got)
	}
}

func TestClient_UpdateSnapshot_OlderDoesNotReplaceCurrent(t *testing.T) {
	v2 := NewSnapshot("production", 2, []Flag{{Key: "new-checkout", Enabled: true}})
	v1 := NewSnapshot("production", 1, []Flag{{Key: "new-checkout", Enabled: false}})
	client := NewClient(v2)

	if applied := client.UpdateSnapshot(v1); applied {
		t.Fatal("UpdateSnapshot() = true, want false for an older version")
	}
	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("Bool() after rejected update = %v, want true (unchanged)", got)
	}
}

func TestClient_UpdateSnapshot_EqualVersionIsRejected(t *testing.T) {
	v2a := NewSnapshot("production", 2, []Flag{{Key: "new-checkout", Enabled: true}})
	v2b := NewSnapshot("production", 2, []Flag{{Key: "new-checkout", Enabled: false}})
	client := NewClient(v2a)

	if applied := client.UpdateSnapshot(v2b); applied {
		t.Fatal("UpdateSnapshot() = true, want false for an equal version")
	}
	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("Bool() after rejected equal-version update = %v, want true (unchanged)", got)
	}
}

func TestClient_UpdateSnapshot_FirstInstallWithNoPriorSnapshot(t *testing.T) {
	client := NewClient(nil)
	v1 := NewSnapshot("production", 1, []Flag{{Key: "new-checkout", Enabled: true}})

	if applied := client.UpdateSnapshot(v1); !applied {
		t.Fatal("UpdateSnapshot() = false, want true when installing the first snapshot")
	}
	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("Bool() = %v, want true", got)
	}
}

func TestClient_UpdateSnapshot_NilIsRejected(t *testing.T) {
	snap := NewSnapshot("production", 1, []Flag{{Key: "new-checkout", Enabled: true}})
	client := NewClient(snap)

	if applied := client.UpdateSnapshot(nil); applied {
		t.Fatal("UpdateSnapshot(nil) = true, want false")
	}
	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("Bool() after rejected nil update = %v, want true (unchanged)", got)
	}
}

func TestClient_Bool_ConcurrentReads(t *testing.T) {
	snap := NewSnapshot("production", 1, []Flag{
		{Key: "a", Enabled: true},
		{Key: "b", Enabled: false},
	})
	client := NewClient(snap)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := client.Bool("a", false); got != true {
				t.Errorf("Bool(a) = %v, want true", got)
			}
			if got := client.Bool("b", true); got != false {
				t.Errorf("Bool(b) = %v, want false", got)
			}
			if got := client.Bool("missing", true); got != true {
				t.Errorf("Bool(missing) = %v, want true (default)", got)
			}
		}()
	}
	wg.Wait()
}

// TestClient_ConcurrentUpdateAndBool exercises UpdateSnapshot racing against
// Bool() reads from many goroutines. Its main value is under
// `go test -race`: it must complete without the race detector firing, and
// the final state must reflect the last version installed by the single
// writer goroutine.
func TestClient_ConcurrentUpdateAndBool(t *testing.T) {
	initial := NewSnapshot("production", 0, []Flag{{Key: "new-checkout", Enabled: false}})
	client := NewClient(initial)

	const (
		updates = 200
		readers = 200
	)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := 1; v <= updates; v++ {
			client.UpdateSnapshot(NewSnapshot("production", v, []Flag{{Key: "new-checkout", Enabled: v%2 == 0}}))
		}
	}()

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.Bool("new-checkout", false)
		}()
	}
	wg.Wait()

	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("final Bool() = %v, want true (version %d is even)", got, updates)
	}
}
