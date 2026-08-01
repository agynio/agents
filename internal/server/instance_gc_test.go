package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
)

type fakeIdleGCStore struct {
	candidates []store.IdleInstance
	listErr    error
	paused     map[uuid.UUID]string
	pauseErr   map[uuid.UUID]error
}

func (f *fakeIdleGCStore) ListIdleGCCandidates(context.Context, int) ([]store.IdleInstance, error) {
	return f.candidates, f.listErr
}

func (f *fakeIdleGCStore) PauseAgentInstance(_ context.Context, id uuid.UUID, reason string) (store.AgentInstance, error) {
	if err, ok := f.pauseErr[id]; ok {
		return store.AgentInstance{}, err
	}
	if f.paused == nil {
		f.paused = map[uuid.UUID]string{}
	}
	f.paused[id] = reason
	return store.AgentInstance{}, nil
}

func TestSweepIdleInstances(t *testing.T) {
	idle := uuid.New()
	busy := uuid.New()
	exactly := uuid.New()
	now := time.Now().UTC()

	instances := &fakeIdleGCStore{candidates: []store.IdleInstance{
		{ID: idle, LastActivityAt: now.Add(-2 * time.Hour), IdleTTL: "30m"},
		{ID: busy, LastActivityAt: now.Add(-time.Minute), IdleTTL: "30m"},
		// A Go duration Postgres would not read as an interval, which is why
		// the comparison is done here rather than in the query.
		{ID: exactly, LastActivityAt: now.Add(-2 * time.Hour), IdleTTL: "1h30m"},
	}}

	paused, err := sweepIdleInstances(context.Background(), instances)
	if err != nil {
		t.Fatalf("sweepIdleInstances failed: %v", err)
	}
	if paused != 2 {
		t.Fatalf("expected 2 instances paused, got %d", paused)
	}
	if reason := instances.paused[idle]; reason != PauseReasonIdleTTLExceeded {
		t.Fatalf("expected %q, got %q", PauseReasonIdleTTLExceeded, reason)
	}
	if _, ok := instances.paused[busy]; ok {
		t.Fatal("an instance active a minute ago was paused")
	}
	if _, ok := instances.paused[exactly]; !ok {
		t.Fatal("expected the 1h30m limit to be read as a Go duration")
	}
}

// The column is free text and nothing enforces it retroactively, so a class
// with an unreadable limit is left alone rather than swept on a guess.
func TestSweepIdleInstancesSkipsUnreadableTTL(t *testing.T) {
	bad := uuid.New()
	good := uuid.New()
	now := time.Now().UTC()

	instances := &fakeIdleGCStore{candidates: []store.IdleInstance{
		{ID: bad, LastActivityAt: now.Add(-99 * time.Hour), IdleTTL: "forever"},
		{ID: good, LastActivityAt: now.Add(-99 * time.Hour), IdleTTL: "1h"},
	}}

	paused, err := sweepIdleInstances(context.Background(), instances)
	if err != nil {
		t.Fatalf("sweepIdleInstances failed: %v", err)
	}
	if paused != 1 {
		t.Fatalf("expected 1 instance paused, got %d", paused)
	}
	if _, ok := instances.paused[bad]; ok {
		t.Fatal("an instance with an unreadable ttl was paused")
	}
}

// One instance failing to pause -- resumed a moment ago, deleted under us --
// says nothing about the rest of the batch.
func TestSweepIdleInstancesContinuesPastAFailure(t *testing.T) {
	first := uuid.New()
	second := uuid.New()
	now := time.Now().UTC()

	instances := &fakeIdleGCStore{
		candidates: []store.IdleInstance{
			{ID: first, LastActivityAt: now.Add(-time.Hour), IdleTTL: "1m"},
			{ID: second, LastActivityAt: now.Add(-time.Hour), IdleTTL: "1m"},
		},
		pauseErr: map[uuid.UUID]error{first: errors.New("gone")},
	}

	paused, err := sweepIdleInstances(context.Background(), instances)
	if err != nil {
		t.Fatalf("sweepIdleInstances failed: %v", err)
	}
	if paused != 1 {
		t.Fatalf("expected 1 instance paused, got %d", paused)
	}
	if _, ok := instances.paused[second]; !ok {
		t.Fatal("the batch stopped at the first failure")
	}
}
