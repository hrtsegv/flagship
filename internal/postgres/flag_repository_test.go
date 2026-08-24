package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hrtsegv/flagship/internal/domain"
)

// newFlagRepo returns a FlagRepository backed by a fresh, truncated test
// database, with environment e1 already seeded so flag inserts satisfy the
// foreign key.
func newFlagRepo(t *testing.T) (*FlagRepository, *pgxpool.Pool) {
	t.Helper()
	pool := testPool(t)
	seedEnvironment(t, pool, "e1")
	return NewFlagRepository(pool), pool
}

func TestFlagRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo, _ := newFlagRepo(t)

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
	repo, _ := newFlagRepo(t)

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

func TestFlagRepository_GetByID_NotFound(t *testing.T) {
	repo, _ := newFlagRepo(t)
	_, err := repo.GetByID(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestFlagRepository_ListByEnvironment(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	seedEnvironment(t, pool, "e1")
	seedEnvironment(t, pool, "e2")
	repo := NewFlagRepository(pool)

	for _, f := range []domain.Flag{
		{ID: "1", EnvironmentID: "e1", Key: "a"},
		{ID: "2", EnvironmentID: "e1", Key: "b"},
		{ID: "3", EnvironmentID: "e2", Key: "a"},
	} {
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
	repo, _ := newFlagRepo(t)

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
}

func TestFlagRepository_Update_NotFound(t *testing.T) {
	repo, _ := newFlagRepo(t)
	_, err := repo.Update(context.Background(), "missing", domain.FlagPatch{})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestFlagRepository_Update_ConcurrentIncrementsAreSerialized(t *testing.T) {
	ctx := context.Background()
	repo, _ := newFlagRepo(t)

	f := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout", Version: 1}
	if _, err := repo.Create(ctx, f); err != nil {
		t.Fatal(err)
	}

	const updates = 30
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
	repo, _ := newFlagRepo(t)

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
	repo, _ := newFlagRepo(t)
	err := repo.Delete(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestFlagRepository_Delete_KeyBecomesReusable(t *testing.T) {
	ctx := context.Background()
	repo, _ := newFlagRepo(t)

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
	repo, _ := newFlagRepo(t)

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

func subscribeOnly(t *testing.T, repo *FlagRepository, environmentID string, sinceVersion int) ([]domain.ConfigEvent, bool) {
	t.Helper()
	events, replayable, _, unsubscribe := repo.SubscribeFrom(context.Background(), environmentID, sinceVersion)
	t.Cleanup(unsubscribe)
	return events, replayable
}

func TestFlagRepository_SubscribeFrom_RecordsEachMutation(t *testing.T) {
	ctx := context.Background()
	repo, _ := newFlagRepo(t)

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
	repo, _ := newFlagRepo(t)

	for _, key := range []string{"a", "b", "c"} {
		if _, err := repo.Create(ctx, domain.Flag{ID: key, EnvironmentID: "e1", Key: key}); err != nil {
			t.Fatal(err)
		}
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
	repo, _ := newFlagRepo(t)

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
	repo, _ := newFlagRepo(t)

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
	repo, _ := newFlagRepo(t)

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
	repo, _ := newFlagRepo(t)

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	for i := 0; i < maxRetainedEventsPerEnvironment; i++ {
		if _, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled}); err != nil {
			t.Fatal(err)
		}
	}

	if _, replayable := subscribeOnly(t, repo, "e1", 0); replayable {
		t.Error("replayable = true for a client needing trimmed events, want false")
	}

	events, replayable := subscribeOnly(t, repo, "e1", 1)
	if !replayable {
		t.Fatal("replayable = false at the retention boundary, want true")
	}
	if len(events) != maxRetainedEventsPerEnvironment || events[0].Version != 2 {
		t.Fatalf("got %d events starting at version %d, want %d starting at version 2",
			len(events), events[0].Version, maxRetainedEventsPerEnvironment)
	}
}

func TestFlagRepository_SubscribeFrom_DeliversLiveEvents(t *testing.T) {
	ctx := context.Background()
	repo, _ := newFlagRepo(t)

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
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the live event")
	}
}

func TestFlagRepository_SubscribeFrom_UnsubscribeStopsDelivery(t *testing.T) {
	ctx := context.Background()
	repo, _ := newFlagRepo(t)

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
	case <-time.After(100 * time.Millisecond):
	}
}

func TestFlagRepository_SubscribeFrom_ScopedToEnvironment(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	seedEnvironment(t, pool, "e1")
	seedEnvironment(t, pool, "e2")
	repo := NewFlagRepository(pool)

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

// TestFlagRepository_SubscribeFrom_ReplayLiveHandoffHasNoGapOrDuplicate is
// the core correctness proof for SubscribeFrom, ported unchanged from
// internal/memory: a write that happens after subscription must never be
// missing from the live feed, and must never also appear in the replayed
// history.
func TestFlagRepository_SubscribeFrom_ReplayLiveHandoffHasNoGapOrDuplicate(t *testing.T) {
	ctx := context.Background()
	repo, _ := newFlagRepo(t)

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
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for version 2 on the live channel")
	}
}

func TestFlagRepository_Snapshot_EmptyEnvironment(t *testing.T) {
	repo, _ := newFlagRepo(t)

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
	repo, _ := newFlagRepo(t)

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
