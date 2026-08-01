package server

import (
	"context"
	"log"
	"time"

	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
)

// PauseReasonIdleTTLExceeded is recorded on instances the GC pauses, so a
// person looking at a paused instance can tell "nobody used this" apart from
// "something went wrong".
const PauseReasonIdleTTLExceeded = "idle_ttl_exceeded"

const (
	defaultIdleGCInterval = time.Minute
	idleGCBatchSize       = 500
)

// IdleGCStore is the store behaviour the idle sweep needs.
type IdleGCStore interface {
	ListIdleGCCandidates(ctx context.Context, limit int) ([]store.IdleInstance, error)
	PauseAgentInstance(ctx context.Context, id uuid.UUID, reason string) (store.AgentInstance, error)
}

// RunInstanceIdleGC pauses instances that have sat idle longer than their class
// allows, until ctx is cancelled.
//
// Paused, not terminated: the instance keeps its thread, its default thread and
// its inbox, so ResumeInstance picks up whatever arrived while it was away.
// What the sweep reclaims is the workload behind it, which the Orchestrator
// stops once the instance is no longer active.
func (s *Server) RunInstanceIdleGC(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultIdleGCInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if paused, err := sweepIdleInstances(ctx, s.store); err != nil {
				// Logged, not fatal: a sweep that fails is retried on the next
				// tick, and an instance staying active a minute longer than its
				// limit is not worth taking the service down for.
				log.Printf("instance idle gc: %v", err)
			} else if paused > 0 {
				log.Printf("instance idle gc: paused %d idle instance(s)", paused)
			}
		}
	}
}

// sweepIdleInstances takes the store as an interface so the decision -- which
// instances are past their limit -- can be tested without a database.
func sweepIdleInstances(ctx context.Context, instances IdleGCStore) (int, error) {
	candidates, err := instances.ListIdleGCCandidates(ctx, idleGCBatchSize)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	paused := 0
	for _, candidate := range candidates {
		ttl, err := time.ParseDuration(candidate.IdleTTL)
		if err != nil {
			// The column is free text and nothing enforces it retroactively. A
			// class with an unreadable limit is left alone rather than swept on
			// a guess.
			log.Printf("instance idle gc: instance %s has an unreadable idle ttl %q: %v", candidate.ID, candidate.IdleTTL, err)
			continue
		}
		if ttl <= 0 || now.Sub(candidate.LastActivityAt) < ttl {
			continue
		}
		if _, err := instances.PauseAgentInstance(ctx, candidate.ID, PauseReasonIdleTTLExceeded); err != nil {
			// One instance failing to pause -- resumed a moment ago, deleted
			// under us -- says nothing about the rest of the batch.
			log.Printf("instance idle gc: pause %s: %v", candidate.ID, err)
			continue
		}
		paused++
	}
	return paused, nil
}
