package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type agentIDQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func touchAgentUpdatedAt(ctx context.Context, tx pgx.Tx, agentID uuid.UUID) error {
	result, err := tx.Exec(ctx, "UPDATE agents SET updated_at = NOW() WHERE id = $1", agentID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("agent")
	}
	return nil
}

func touchAgentsUpdatedAt(ctx context.Context, tx pgx.Tx, agentIDs []uuid.UUID) error {
	for _, agentID := range agentIDs {
		if err := touchAgentUpdatedAt(ctx, tx, agentID); err != nil {
			return err
		}
	}
	return nil
}

func resolveAgentID(ctx context.Context, tx pgx.Tx, agentID *uuid.UUID, mcpID *uuid.UUID) (uuid.UUID, error) {
	if agentID != nil {
		return *agentID, nil
	}
	if mcpID != nil {
		return agentIDForMcp(ctx, tx, *mcpID)
	}
	return uuid.UUID{}, fmt.Errorf("missing target identifier")
}

// touchTargetAgent marks the agent a row belongs to as updated so the agent's
// workload is reassembled. A row targeting an environment belongs to no agent:
// an environment is an organization's, and there is nothing to touch.
func touchTargetAgent(ctx context.Context, tx pgx.Tx, agentID *uuid.UUID, mcpID *uuid.UUID, environmentID *uuid.UUID) error {
	if environmentID != nil {
		return nil
	}
	resolvedAgentID, err := resolveAgentID(ctx, tx, agentID, mcpID)
	if err != nil {
		return err
	}
	return touchAgentUpdatedAt(ctx, tx, resolvedAgentID)
}

// organizationIDForTarget reports the organization a target belongs to. An
// agent and an environment hold one directly; an mcp reaches it
// through their agent.
func organizationIDForTarget(ctx context.Context, tx pgx.Tx, agentID *uuid.UUID, mcpID *uuid.UUID, environmentID *uuid.UUID) (uuid.UUID, error) {
	if environmentID != nil {
		return organizationIDForEnvironment(ctx, tx, *environmentID)
	}
	resolvedAgentID, err := resolveAgentID(ctx, tx, agentID, mcpID)
	if err != nil {
		return uuid.UUID{}, err
	}
	return organizationIDForAgent(ctx, tx, resolvedAgentID)
}

func organizationIDForAgent(ctx context.Context, tx pgx.Tx, agentID uuid.UUID) (uuid.UUID, error) {
	var organizationID uuid.UUID
	row := tx.QueryRow(ctx, "SELECT organization_id FROM agents WHERE id = $1", agentID)
	if err := row.Scan(&organizationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.UUID{}, NotFound("agent")
		}
		return uuid.UUID{}, err
	}
	return organizationID, nil
}

func organizationIDForEnvironment(ctx context.Context, tx pgx.Tx, environmentID uuid.UUID) (uuid.UUID, error) {
	var organizationID uuid.UUID
	row := tx.QueryRow(ctx, "SELECT organization_id FROM environments WHERE id = $1", environmentID)
	if err := row.Scan(&organizationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.UUID{}, NotFound("environment")
		}
		return uuid.UUID{}, err
	}
	return organizationID, nil
}

func agentIDForMcp(ctx context.Context, tx pgx.Tx, mcpID uuid.UUID) (uuid.UUID, error) {
	var agentID uuid.UUID
	row := tx.QueryRow(ctx, "SELECT agent_id FROM mcps WHERE id = $1", mcpID)
	if err := row.Scan(&agentID); err != nil {
		if err == pgx.ErrNoRows {
			return uuid.UUID{}, NotFound("mcp")
		}
		return uuid.UUID{}, err
	}
	return agentID, nil
}

func agentIDsForVolume(ctx context.Context, queryer agentIDQueryer, volumeID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := queryer.Query(ctx,
		`SELECT DISTINCT COALESCE(va.agent_id, mcps.agent_id, hooks.agent_id)
FROM volume_attachments va
LEFT JOIN mcps ON va.mcp_id = mcps.id
WHERE va.volume_id = $1`,
		volumeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := make([]uuid.UUID, 0)
	for rows.Next() {
		var agentID uuid.UUID
		if err := rows.Scan(&agentID); err != nil {
			return nil, err
		}
		agents = append(agents, agentID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}
