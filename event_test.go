package flagship

import (
	"errors"
	"sync"
	"testing"
)

func TestClient_ApplyEvent_SequentialApply(t *testing.T) {
	client := NewClient(NewSnapshot("production", 1, []Flag{{Key: "new-checkout", Enabled: false}}))

	applied, err := client.ApplyEvent(ConfigEvent{
		Environment: "production", Version: 2, Type: EventFlagUpdated, FlagKey: "new-checkout", Enabled: true,
	})
	if err != nil {
		t.Fatalf("ApplyEvent() unexpected error: %v", err)
	}
	if !applied {
		t.Fatal("ApplyEvent() applied = false, want true for the next sequential version")
	}
	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("Bool() after v1->v2 = %v, want true", got)
	}

	applied, err = client.ApplyEvent(ConfigEvent{
		Environment: "production", Version: 3, Type: EventFlagCreated, FlagKey: "new-search", Enabled: true,
	})
	if err != nil {
		t.Fatalf("second ApplyEvent() unexpected error: %v", err)
	}
	if !applied {
		t.Fatal("second ApplyEvent() applied = false, want true")
	}
	if got := client.Bool("new-search", false); got != true {
		t.Errorf("Bool(new-search) after v2->v3 = %v, want true", got)
	}
	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("Bool(new-checkout) after v2->v3 = %v, want true (unchanged by this event)", got)
	}
}

func TestClient_ApplyEvent_DuplicateVersionIgnored(t *testing.T) {
	client := NewClient(NewSnapshot("production", 3, []Flag{{Key: "new-checkout", Enabled: true}}))

	applied, err := client.ApplyEvent(ConfigEvent{
		Environment: "production", Version: 3, Type: EventFlagUpdated, FlagKey: "new-checkout", Enabled: false,
	})
	if err != nil {
		t.Fatalf("ApplyEvent() unexpected error: %v", err)
	}
	if applied {
		t.Fatal("ApplyEvent() applied = true, want false for a duplicate version")
	}
	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("Bool() after ignored duplicate = %v, want true (unchanged)", got)
	}
}

func TestClient_ApplyEvent_OlderVersionIgnored(t *testing.T) {
	client := NewClient(NewSnapshot("production", 5, []Flag{{Key: "new-checkout", Enabled: true}}))

	applied, err := client.ApplyEvent(ConfigEvent{
		Environment: "production", Version: 2, Type: EventFlagUpdated, FlagKey: "new-checkout", Enabled: false,
	})
	if err != nil {
		t.Fatalf("ApplyEvent() unexpected error: %v", err)
	}
	if applied {
		t.Fatal("ApplyEvent() applied = true, want false for an older version")
	}
	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("Bool() after ignored old event = %v, want true (unchanged)", got)
	}
}

func TestClient_ApplyEvent_GapDetected(t *testing.T) {
	client := NewClient(NewSnapshot("production", 1, []Flag{{Key: "new-checkout", Enabled: false}}))

	applied, err := client.ApplyEvent(ConfigEvent{
		Environment: "production", Version: 3, Type: EventFlagUpdated, FlagKey: "new-checkout", Enabled: true,
	})
	if !errors.Is(err, ErrVersionGap) {
		t.Fatalf("ApplyEvent() error = %v, want ErrVersionGap", err)
	}
	if applied {
		t.Fatal("ApplyEvent() applied = true, want false when a gap is detected")
	}
	if got := client.Bool("new-checkout", false); got != false {
		t.Errorf("Bool() after gap = %v, want false (unchanged)", got)
	}
	if v := client.snapshot.Load().Version(); v != 1 {
		t.Errorf("client version after gap = %d, want unchanged at 1", v)
	}
}

func TestClient_ApplyEvent_NoSnapshotYetIsAGap(t *testing.T) {
	client := NewClient(nil)

	applied, err := client.ApplyEvent(ConfigEvent{
		Environment: "production", Version: 1, Type: EventFlagCreated, FlagKey: "new-checkout", Enabled: true,
	})
	if !errors.Is(err, ErrVersionGap) {
		t.Fatalf("ApplyEvent() error = %v, want ErrVersionGap when the client has no snapshot yet", err)
	}
	if applied {
		t.Fatal("ApplyEvent() applied = true, want false")
	}
}

func TestClient_ApplyEvent_WrongEnvironment(t *testing.T) {
	client := NewClient(NewSnapshot("production", 1, []Flag{{Key: "new-checkout", Enabled: false}}))

	applied, err := client.ApplyEvent(ConfigEvent{
		Environment: "staging", Version: 2, Type: EventFlagUpdated, FlagKey: "new-checkout", Enabled: true,
	})
	if !errors.Is(err, ErrWrongEnvironment) {
		t.Fatalf("ApplyEvent() error = %v, want ErrWrongEnvironment", err)
	}
	if applied {
		t.Fatal("ApplyEvent() applied = true, want false")
	}
}

func TestClient_ApplyEvent_Deletion(t *testing.T) {
	client := NewClient(NewSnapshot("production", 1, []Flag{
		{Key: "new-checkout", Enabled: true},
		{Key: "new-search", Enabled: true},
	}))

	applied, err := client.ApplyEvent(ConfigEvent{
		Environment: "production", Version: 2, Type: EventFlagDeleted, FlagKey: "new-checkout",
	})
	if err != nil {
		t.Fatalf("ApplyEvent() unexpected error: %v", err)
	}
	if !applied {
		t.Fatal("ApplyEvent() applied = false, want true")
	}
	if got := client.Bool("new-checkout", false); got != false {
		t.Errorf("Bool(new-checkout) after deletion = %v, want false (falls back to default)", got)
	}
	if got := client.Bool("new-search", false); got != true {
		t.Errorf("Bool(new-search) after unrelated deletion = %v, want true (unchanged)", got)
	}
}

func TestClient_ApplyEvent_ConcurrentApplyAndBool(t *testing.T) {
	client := NewClient(NewSnapshot("production", 0, []Flag{{Key: "new-checkout", Enabled: false}}))

	const (
		updates = 200
		readers = 200
	)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := 1; v <= updates; v++ {
			if _, err := client.ApplyEvent(ConfigEvent{
				Environment: "production", Version: v, Type: EventFlagUpdated, FlagKey: "new-checkout", Enabled: v%2 == 0,
			}); err != nil {
				t.Errorf("ApplyEvent(v=%d) unexpected error: %v", v, err)
			}
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
