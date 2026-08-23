package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hrtsegv/flagship/internal/domain"
)

func TestFlagRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout", Version: 1}
	created, err := repo.Create(ctx, f)
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if created != f {
		t.Errorf("Create() = %+v, want %+v", created, f)
	}
}

func TestFlagRepository_Create_DuplicateKeyWithinEnvironment(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f1 := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout"}
	f2 := domain.Flag{ID: "2", EnvironmentID: "e1", Key: "new-checkout"}

	if _, err := repo.Create(ctx, f1); err != nil {
		t.Fatalf("first Create() unexpected error: %v", err)
	}
	_, err := repo.Create(ctx, f2)
	if !errors.Is(err, domain.ErrDuplicateKey) {
		t.Fatalf("second Create() error = %v, want ErrDuplicateKey", err)
	}
}

func TestFlagRepository_Create_SameKeyDifferentEnvironmentsAllowed(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f1 := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout"}
	f2 := domain.Flag{ID: "2", EnvironmentID: "e2", Key: "new-checkout"}

	if _, err := repo.Create(ctx, f1); err != nil {
		t.Fatalf("Create() for e1 unexpected error: %v", err)
	}
	if _, err := repo.Create(ctx, f2); err != nil {
		t.Fatalf("Create() for e2 unexpected error: %v", err)
	}
}

func TestFlagRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout"}
	if _, err := repo.Create(ctx, f); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, "1")
	if err != nil {
		t.Fatalf("GetByID() unexpected error: %v", err)
	}
	if got.Key != "new-checkout" {
		t.Errorf("GetByID() Key = %q, want %q", got.Key, "new-checkout")
	}
}

func TestFlagRepository_GetByID_NotFound(t *testing.T) {
	repo := NewFlagRepository()
	_, err := repo.GetByID(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestFlagRepository_ListByEnvironment(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f1 := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}
	f2 := domain.Flag{ID: "2", EnvironmentID: "e1", Key: "b"}
	f3 := domain.Flag{ID: "3", EnvironmentID: "e2", Key: "a"}

	for _, f := range []domain.Flag{f1, f2, f3} {
		if _, err := repo.Create(ctx, f); err != nil {
			t.Fatal(err)
		}
	}

	list, err := repo.ListByEnvironment(ctx, "e1")
	if err != nil {
		t.Fatalf("ListByEnvironment() unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListByEnvironment() returned %d flags, want 2", len(list))
	}
}

func TestFlagRepository_Update_IncrementsVersion(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout", Enabled: false, Version: 1}
	if _, err := repo.Create(ctx, f); err != nil {
		t.Fatal(err)
	}

	enabled := true
	updated, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if !updated.Enabled {
		t.Errorf("Update() Enabled = false, want true")
	}
	if updated.Version != 2 {
		t.Errorf("Update() Version = %d, want 2", updated.Version)
	}

	stored, err := repo.GetByID(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != 2 {
		t.Errorf("stored Version = %d, want 2", stored.Version)
	}
}

func TestFlagRepository_Update_NotFound(t *testing.T) {
	repo := NewFlagRepository()
	_, err := repo.Update(context.Background(), "missing", domain.FlagPatch{})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestFlagRepository_Update_ConcurrentIncrementsAreSerialized(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout", Version: 1}
	if _, err := repo.Create(ctx, f); err != nil {
		t.Fatal(err)
	}

	const updates = 100
	var wg sync.WaitGroup
	for i := 0; i < updates; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.Update(ctx, "1", domain.FlagPatch{}); err != nil {
				t.Errorf("Update() unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	final, err := repo.GetByID(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	want := 1 + updates
	if final.Version != want {
		t.Fatalf("final Version = %d, want %d (no updates should be lost)", final.Version, want)
	}
}

func TestFlagRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout"}); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, "1"); err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}

	if _, err := repo.GetByID(ctx, "1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID() after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestFlagRepository_Delete_NotFound(t *testing.T) {
	repo := NewFlagRepository()
	err := repo.Delete(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestFlagRepository_Delete_KeyBecomesReusable(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, "1"); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.Create(ctx, domain.Flag{ID: "2", EnvironmentID: "e1", Key: "new-checkout"}); err != nil {
		t.Fatalf("Create() after Delete() unexpected error: %v, want the key to be reusable", err)
	}
}

func TestFlagRepository_Delete_IncrementsConfigVersionAndScopesSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, domain.Flag{ID: "2", EnvironmentID: "e1", Key: "b"}); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, "1"); err != nil {
		t.Fatal(err)
	}

	version, flags, err := repo.Snapshot(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("version after 2 creates + 1 delete = %d, want 3", version)
	}
	if len(flags) != 1 || flags[0].Key != "b" {
		t.Fatalf("flags after delete = %+v, want only %q", flags, "b")
	}
}

// subscribeOnly is a small helper for tests that only care about the
// replay half of SubscribeFrom's return. The live channel is discarded:
// these tests are single-goroutine and synchronous, so nothing is ever
// published to it before t.Cleanup unsubscribes.
func subscribeOnly(t *testing.T, repo *FlagRepository, environmentID string, sinceVersion int) ([]domain.ConfigEvent, bool) {
	t.Helper()
	events, replayable, _, unsubscribe := repo.SubscribeFrom(context.Background(), environmentID, sinceVersion)
	t.Cleanup(unsubscribe)
	return events, replayable
}

func TestFlagRepository_SubscribeFrom_RecordsEachMutation(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, "1"); err != nil {
		t.Fatal(err)
	}

	events, replayable := subscribeOnly(t, repo, "e1", 0)
	if !replayable {
		t.Fatal("replayable = false, want true when the full history is retained")
	}

	want := []domain.ConfigEvent{
		{EnvironmentID: "e1", Version: 1, Type: domain.ConfigEventFlagCreated, FlagKey: "a", Enabled: false},
		{EnvironmentID: "e1", Version: 2, Type: domain.ConfigEventFlagUpdated, FlagKey: "a", Enabled: true},
		{EnvironmentID: "e1", Version: 3, Type: domain.ConfigEventFlagDeleted, FlagKey: "a", Enabled: false},
	}
	if len(events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(events), len(want), events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, events[i], want[i])
		}
	}
}

func TestFlagRepository_SubscribeFrom_ReturnsOnlyNewerEvents(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, domain.Flag{ID: "2", EnvironmentID: "e1", Key: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, domain.Flag{ID: "3", EnvironmentID: "e1", Key: "c"}); err != nil {
		t.Fatal(err)
	}

	events, replayable := subscribeOnly(t, repo, "e1", 1)
	if !replayable {
		t.Fatal("replayable = false, want true")
	}
	if len(events) != 2 || events[0].Version != 2 || events[1].Version != 3 {
		t.Fatalf("events = %+v, want versions 2 and 3 only", events)
	}
}

func TestFlagRepository_SubscribeFrom_AlreadyCurrent(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}

	events, replayable := subscribeOnly(t, repo, "e1", 1)
	if !replayable {
		t.Fatal("replayable = false, want true for a client that is already current")
	}
	if len(events) != 0 {
		t.Fatalf("events = %+v, want none", events)
	}
}

func TestFlagRepository_SubscribeFrom_FreshEnvironment(t *testing.T) {
	repo := NewFlagRepository()

	events, replayable := subscribeOnly(t, repo, "e1", 0)
	if !replayable {
		t.Fatal("replayable = false, want true for a fresh environment at version 0")
	}
	if len(events) != 0 {
		t.Fatalf("events = %+v, want none", events)
	}
}

func TestFlagRepository_SubscribeFrom_AheadOfServerIsNotReplayable(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}

	_, replayable := subscribeOnly(t, repo, "e1", 5)
	if replayable {
		t.Fatal("replayable = true, want false when the client claims a version ahead of the server")
	}
}

func TestFlagRepository_SubscribeFrom_TrimmedHistoryIsNotReplayable(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	// One create + this many updates pushes the history one past the cap,
	// so the create (version 1) is dropped and version 2 becomes oldest.
	enabled := true
	for i := 0; i < maxRetainedEventsPerEnvironment; i++ {
		if _, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled}); err != nil {
			t.Fatal(err)
		}
	}

	// A client at version 0 needs version 1, which is gone.
	if _, replayable := subscribeOnly(t, repo, "e1", 0); replayable {
		t.Error("replayable = true for a client needing trimmed events, want false")
	}

	// A client at version 1 needs version 2 onward, which is still retained.
	events, replayable := subscribeOnly(t, repo, "e1", 1)
	if !replayable {
		t.Fatal("replayable = false at the retention boundary, want true")
	}
	if len(events) != maxRetainedEventsPerEnvironment || events[0].Version != 2 {
		t.Fatalf("got %d events starting at version %d, want %d starting at version 2",
			len(events), events[0].Version, maxRetainedEventsPerEnvironment)
	}
}

func TestFlagRepository_SubscribeFrom_ScopedToEnvironment(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, domain.Flag{ID: "2", EnvironmentID: "e2", Key: "b"}); err != nil {
		t.Fatal(err)
	}

	events, _ := subscribeOnly(t, repo, "e1", 0)
	if len(events) != 1 || events[0].FlagKey != "a" {
		t.Fatalf("e1 events = %+v, want only the flag %q event", events, "a")
	}
}

func TestFlagRepository_SubscribeFrom_ReturnsIndependentSlice(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}

	events, _ := subscribeOnly(t, repo, "e1", 0)
	events[0].FlagKey = "mutated-by-caller"

	again, _ := subscribeOnly(t, repo, "e1", 0)
	if again[0].FlagKey != "a" {
		t.Errorf("stored event was mutated through the returned slice: FlagKey = %q", again[0].FlagKey)
	}
}

func TestFlagRepository_SubscribeFrom_DeliversLiveEvents(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a", Enabled: false}); err != nil {
		t.Fatal(err)
	}

	_, _, live, unsubscribe := repo.SubscribeFrom(ctx, "e1", 1)
	defer unsubscribe()

	enabled := true
	if _, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-live:
		want := domain.ConfigEvent{EnvironmentID: "e1", Version: 2, Type: domain.ConfigEventFlagUpdated, FlagKey: "a", Enabled: true}
		if e != want {
			t.Errorf("live event = %+v, want %+v", e, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the live event")
	}
}

func TestFlagRepository_SubscribeFrom_UnsubscribeStopsDelivery(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}

	_, _, live, unsubscribe := repo.SubscribeFrom(ctx, "e1", 1)
	unsubscribe()

	enabled := true
	if _, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}

	select {
	case e, ok := <-live:
		if ok {
			t.Fatalf("received %+v after unsubscribe, want nothing", e)
		}
	case <-time.After(50 * time.Millisecond):
		// No delivery within a short window is exactly what we want: the
		// channel isn't closed (unsubscribe only stops future publishes,
		// it doesn't close the channel), so a nil receive would block
		// forever if delivery incorrectly happened after removal.
	}
}

func TestFlagRepository_SubscribeFrom_SlowConsumerDoesNotBlockWriter(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}

	_, _, _, unsubscribe := repo.SubscribeFrom(ctx, "e1", 0)
	defer unsubscribe()

	// Fill the subscriber's buffer without ever draining it, then confirm
	// one more write completes promptly instead of hanging.
	enabled := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < subscriberBufferSize+5; i++ {
			enabled = !enabled
			if _, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled}); err != nil {
				t.Errorf("Update() unexpected error: %v", err)
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writes blocked on a slow subscriber instead of dropping events")
	}
}

// TestFlagRepository_SubscribeFrom_ReplayLiveHandoffHasNoGapOrDuplicate is
// the core correctness proof for SubscribeFrom: a write that happens
// after subscription must never be missing from the live feed, and must
// never also appear in the replayed history. The synchronization channel
// pins the write to happen strictly after SubscribeFrom returns, so this
// is deterministic rather than relying on timing.
func TestFlagRepository_SubscribeFrom_ReplayLiveHandoffHasNoGapOrDuplicate(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err) // version 1
	}

	proceed := make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		<-proceed
		enabled := true
		_, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled}) // version 2
		writeDone <- err
	}()

	events, replayable, live, unsubscribe := repo.SubscribeFrom(ctx, "e1", 0)
	defer unsubscribe()
	if !replayable {
		t.Fatal("replayable = false, want true")
	}
	if len(events) != 1 || events[0].Version != 1 {
		t.Fatalf("history at subscribe time = %+v, want exactly version 1", events)
	}

	close(proceed)
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-live:
		if e.Version != 2 {
			t.Fatalf("live event version = %d, want 2 (no gap, no duplicate of version 1)", e.Version)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for version 2 on the live channel")
	}
}

func TestFlagRepository_SubscribeFrom_ConcurrentWritesNeverDuplicate(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}

	events, _, live, unsubscribe := repo.SubscribeFrom(ctx, "e1", 0)
	defer unsubscribe()

	seen := make(map[int]bool)
	for _, e := range events {
		seen[e.Version] = true
	}

	const writers, updatesPerWriter = 4, 20
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			enabled := false
			for i := 0; i < updatesPerWriter; i++ {
				enabled = !enabled
				if _, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled}); err != nil {
					t.Errorf("Update() unexpected error: %v", err)
				}
			}
		}()
	}

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		// Writers publish synchronously inside Update, so by the time
		// wg.Wait() (below) returns, everything that will ever be sent
		// has already been sent; this only needs to wait long enough to
		// read whatever's sitting in the channel buffer, not for more
		// writers to show up.
		for {
			select {
			case e, ok := <-live:
				if !ok {
					return
				}
				if seen[e.Version] {
					t.Errorf("version %d delivered more than once (via history+live combined)", e.Version)
				}
				seen[e.Version] = true
			case <-time.After(200 * time.Millisecond):
				return
			}
		}
	}()

	wg.Wait()
	<-drained
	// No assertion on len(seen): a slow-consumer buffer can legitimately
	// drop some live events under this much concurrency. What must never
	// happen, and what every earlier assertion in this test verified, is
	// a duplicate delivery of any single version.
}

func TestFlagRepository_Snapshot_EmptyEnvironment(t *testing.T) {
	repo := NewFlagRepository()

	version, flags, err := repo.Snapshot(context.Background(), "e1")
	if err != nil {
		t.Fatalf("Snapshot() unexpected error: %v", err)
	}
	if version != 0 {
		t.Errorf("Snapshot() version = %d, want 0 for an environment with no flags yet", version)
	}
	if len(flags) != 0 {
		t.Errorf("Snapshot() flags = %v, want empty", flags)
	}
}

func TestFlagRepository_Snapshot_IncrementsOnCreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	version, _, err := repo.Snapshot(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("version after first create = %d, want 1", version)
	}

	if _, err := repo.Create(ctx, domain.Flag{ID: "2", EnvironmentID: "e1", Key: "b"}); err != nil {
		t.Fatal(err)
	}
	version, flags, err := repo.Snapshot(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("version after second create = %d, want 2", version)
	}
	if len(flags) != 2 {
		t.Fatalf("flags after second create = %d, want 2", len(flags))
	}

	enabled := true
	if _, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	version, _, err = repo.Snapshot(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("version after update = %d, want 3", version)
	}
}

func TestFlagRepository_Snapshot_ScopedToEnvironment(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, domain.Flag{ID: "2", EnvironmentID: "e2", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, domain.Flag{ID: "3", EnvironmentID: "e2", Key: "b"}); err != nil {
		t.Fatal(err)
	}

	v1, f1, err := repo.Snapshot(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if v1 != 1 || len(f1) != 1 {
		t.Fatalf("e1 snapshot = (version %d, %d flags), want (1, 1)", v1, len(f1))
	}

	v2, f2, err := repo.Snapshot(ctx, "e2")
	if err != nil {
		t.Fatal(err)
	}
	if v2 != 2 || len(f2) != 2 {
		t.Fatalf("e2 snapshot = (version %d, %d flags), want (2, 2)", v2, len(f2))
	}
}

func TestProjectRepository_Create_ConcurrentDistinctKeys(t *testing.T) {
	ctx := context.Background()
	repo := NewProjectRepository()

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := domain.Project{ID: fmt.Sprintf("id-%d", i), Key: fmt.Sprintf("key-%d", i)}
			if _, err := repo.Create(ctx, p); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("unexpected error creating project with distinct key: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != n {
		t.Fatalf("List() returned %d projects, want %d", len(list), n)
	}
}
