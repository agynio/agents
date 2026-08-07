package server

import (
	"context"
	"log"

	"github.com/agynio/agents/internal/store"
)

// BackfillEnvironmentAuthorization writes the tuples every environment created
// before the environment type existed is missing.
//
// can_edit_config and can_delete resolve through `owner from org`, so without
// an org tuple an environment answers no to every caller, including the
// organization's owner. There is no creator on the record to restore an owner
// tuple from, and inventing one would hand an environment to whoever happened
// to be looked up first; organization owners reach it through `owner from org`
// instead, which is what the org tuple is for.
//
// Writes are idempotent, so this runs on every startup rather than once behind
// a marker: a tuple that already exists is written again to the same value.
func (s *Server) BackfillEnvironmentAuthorization(ctx context.Context) {
	var cursor *store.PageCursor
	backfilled := 0
	for {
		result, err := s.store.ListAllEnvironments(ctx, backfillPageSize, cursor)
		if err != nil {
			log.Printf("agents: backfill environment authorization: list: %v", err)
			return
		}
		for _, environment := range result.Environments {
			if err := s.writeEnvironmentBaseTuples(ctx, environment); err != nil {
				log.Printf("agents: backfill environment %s: %v", environment.Meta.ID, err)
				continue
			}
			backfilled++
		}
		if result.NextCursor == nil {
			break
		}
		cursor = result.NextCursor
	}
	if backfilled > 0 {
		log.Printf("agents: backfilled authorization for %d environment(s)", backfilled)
	}
}

const backfillPageSize = 100

func (s *Server) writeEnvironmentBaseTuples(ctx context.Context, environment store.Environment) error {
	availability := environment.Availability
	if availability == "" {
		availability = store.EnvironmentAvailabilityInternal
	}
	return s.addEnvironmentBaseAuthorization(ctx, environment.Meta.ID, environment.OrganizationID, availability)
}
