package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DeleteOrganizationResources removes every agent-owned row the organization
// holds, leaf-first, in one transaction.
//
// The per-resource delete paths do not compose into an organization-wide sweep:
// envs, init_scripts, mcps and skills reference agents with NO ACTION, and
// agents and sandboxes reference environments the same way, so deleting either
// parent while a child remains is a foreign key violation. Ordering the whole
// set once is what the individual paths would otherwise have to rediscover row
// by row, and the invariants they enforce -- an agent with no live instances, an
// environment nothing references -- hold here because the order satisfies them
// rather than because each row is checked.
func (s *Store) DeleteOrganizationResources(ctx context.Context, organizationID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		statements := []string{
			// Leaf-first. envs, init_scripts, mcps and skills reference agents
			// with NO ACTION, so an agent cannot go while any of them remain.
			`DELETE FROM envs WHERE organization_id = $1`,
			`DELETE FROM init_scripts WHERE organization_id = $1`,
			`DELETE FROM mcps WHERE organization_id = $1`,
			`DELETE FROM skills s USING agents a WHERE s.agent_id = a.id AND a.organization_id = $1`,
			// Sandboxes take their layouts; instances take their inbox items;
			// agents take their roles and any remaining instances.
			`DELETE FROM sandboxes WHERE organization_id = $1`,
			`DELETE FROM agent_instances WHERE organization_id = $1`,
			`DELETE FROM agents WHERE organization_id = $1`,
			// Environments last of all: agents and sandboxes reference them
			// with NO ACTION. Their roles and any remaining volumes follow
			// through ON DELETE CASCADE.
			`DELETE FROM volumes WHERE organization_id = $1`,
			`DELETE FROM environments WHERE organization_id = $1`,
		}

		for _, statement := range statements {
			if _, err := tx.Exec(ctx, statement, organizationID); err != nil {
				return err
			}
		}
		return nil
	})
}
