package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Store) CreateEnvironment(ctx context.Context, organizationID uuid.UUID, input EnvironmentInput) (Environment, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO environments
		 (organization_id, name, image, runner_id, flavor,
		  workspace_image_id, workspace_image_tag, agent_runtime_image_id, agent_runtime_image_tag, availability,
		  llm_mode, llm_allowed_models, persistent_shells)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, COALESCE($13, TRUE))
		 RETURNING %s`, environmentColumns),
		organizationID,
		input.Name,
		input.Image,
		input.RunnerID,
		input.Flavor,
		input.WorkspaceImageID,
		input.WorkspaceImageTag,
		input.AgentRuntimeImageID,
		input.AgentRuntimeImageTag,
		input.Availability,
		input.LLMMode,
		input.LLMAllowedModels,
		input.PersistentShells,
	)
	environment, err := scanEnvironment(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Environment{}, AlreadyExists("environment")
		}
		return Environment{}, err
	}
	return environment, nil
}

func (s *Store) GetEnvironment(ctx context.Context, id uuid.UUID) (Environment, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM environments WHERE id = $1`, environmentColumns),
		id,
	)
	environment, err := scanEnvironment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Environment{}, NotFound("environment")
		}
		return Environment{}, err
	}
	return environment, nil
}

func (s *Store) UpdateEnvironment(ctx context.Context, id uuid.UUID, update EnvironmentUpdate) (Environment, error) {
	builder := updateBuilder{}
	if update.Name != nil {
		builder.add("name", *update.Name)
	}
	if update.Availability != nil {
		builder.add("availability", string(*update.Availability))
	}
	if update.Image != nil {
		builder.add("image", *update.Image)
	}
	if update.RunnerID != nil {
		builder.add("runner_id", *update.RunnerID)
	}
	if update.Flavor != nil {
		builder.add("flavor", *update.Flavor)
	}
	if update.WorkspaceImageID != nil {
		builder.add("workspace_image_id", *update.WorkspaceImageID)
	}
	if update.WorkspaceImageTag != nil {
		builder.add("workspace_image_tag", *update.WorkspaceImageTag)
	}
	if update.AgentRuntimeImageID != nil {
		builder.add("agent_runtime_image_id", *update.AgentRuntimeImageID)
	}
	if update.AgentRuntimeImageTag != nil {
		builder.add("agent_runtime_image_tag", *update.AgentRuntimeImageTag)
	}
	if update.LLMMode != nil {
		builder.add("llm_mode", string(*update.LLMMode))
	}
	if update.LLMAllowedModels != nil {
		builder.add("llm_allowed_models", *update.LLMAllowedModels)
	}
	if update.PersistentShells != nil {
		builder.add("persistent_shells", *update.PersistentShells)
	}
	if builder.empty() {
		return Environment{}, fmt.Errorf("environment update requires at least one field")
	}

	query, args := builder.build("environments", environmentColumns, id)
	row := s.pool.QueryRow(ctx, query, args...)
	environment, err := scanEnvironment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Environment{}, NotFound("environment")
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Environment{}, AlreadyExists("environment")
		}
		return Environment{}, err
	}
	return environment, nil
}

// ListAgentNamesByEnvironment names the agents that would be invalidated by a
// change to the environment, so the refusal can say which ones.
func (s *Store) ListAgentNamesByEnvironment(ctx context.Context, environmentID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT name FROM agents WHERE environment_id = $1 ORDER BY name`, environmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (s *Store) DeleteEnvironment(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM environments WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ForeignKeyViolation("environment")
		}
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("environment")
	}
	return nil
}

// ListAllEnvironments pages every environment regardless of organization. Used
// by the authorization backfill, which repairs rows across all of them.
func (s *Store) ListAllEnvironments(ctx context.Context, pageSize int32, cursor *PageCursor) (EnvironmentListResult, error) {
	environments, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM environments", environmentColumns),
		nil,
		nil,
		cursor,
		pageSize,
		scanEnvironment,
		func(environment Environment) uuid.UUID { return environment.Meta.ID },
	)
	if err != nil {
		return EnvironmentListResult{}, err
	}
	return EnvironmentListResult{Environments: environments, NextCursor: nextCursor}, nil
}

func (s *Store) ListEnvironments(ctx context.Context, organizationID uuid.UUID, _ EnvironmentFilter, pageSize int32, cursor *PageCursor) (EnvironmentListResult, error) {
	environments, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM environments", environmentColumns),
		[]string{"organization_id = $1"},
		[]any{organizationID},
		cursor,
		pageSize,
		scanEnvironment,
		func(environment Environment) uuid.UUID { return environment.Meta.ID },
	)
	if err != nil {
		return EnvironmentListResult{}, err
	}
	return EnvironmentListResult{Environments: environments, NextCursor: nextCursor}, nil
}

func (s *Store) CreateSandbox(ctx context.Context, organizationID uuid.UUID, input SandboxInput) (Sandbox, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO sandboxes (organization_id, name, environment_id, owner_id, status, idle_timeout, ttl, environment_name)
		 SELECT $1, $2, e.id, $4, $5, $6, $7, e.name
		 FROM environments e
		 WHERE e.id = $3 AND e.organization_id = $1
		 RETURNING %s`, sandboxColumns),
		organizationID,
		input.Name,
		input.EnvironmentID,
		input.OwnerID,
		input.Status,
		input.IdleTimeout,
		input.TTL,
	)
	sandbox, err := scanSandbox(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Sandbox{}, NotFound("environment")
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Sandbox{}, AlreadyExists("sandbox")
		}
		return Sandbox{}, err
	}
	return sandbox, nil
}

func (s *Store) GetSandbox(ctx context.Context, id uuid.UUID) (Sandbox, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM sandboxes WHERE id = $1`, sandboxColumns),
		id,
	)
	sandbox, err := scanSandbox(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Sandbox{}, NotFound("sandbox")
		}
		return Sandbox{}, err
	}
	return sandbox, nil
}

func (s *Store) GetSandboxByName(ctx context.Context, organizationID uuid.UUID, name string) (Sandbox, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM sandboxes WHERE organization_id = $1 AND name = $2`, sandboxColumns),
		organizationID,
		name,
	)
	sandbox, err := scanSandbox(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Sandbox{}, NotFound("sandbox")
		}
		return Sandbox{}, err
	}
	return sandbox, nil
}

func (s *Store) UpdateSandbox(ctx context.Context, id uuid.UUID, update SandboxUpdate) (Sandbox, error) {
	if update.WorkloadID != nil && update.ClearWorkloadID {
		return Sandbox{}, fmt.Errorf("sandbox update cannot set and clear workload_id")
	}
	builder := updateBuilder{}
	if update.Status != nil {
		builder.add("status", *update.Status)
	}
	if update.LastSessionAt != nil {
		builder.add("last_session_at", *update.LastSessionAt)
	}
	if update.WorkloadID != nil {
		builder.add("workload_id", *update.WorkloadID)
	}
	if update.ClearWorkloadID {
		builder.addNull("workload_id")
	}
	if builder.empty() {
		return Sandbox{}, fmt.Errorf("sandbox update requires at least one field")
	}

	query, args := builder.build("sandboxes", sandboxColumns, id)
	row := s.pool.QueryRow(ctx, query, args...)
	sandbox, err := scanSandbox(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Sandbox{}, NotFound("sandbox")
		}
		return Sandbox{}, err
	}
	return sandbox, nil
}

func (s *Store) DeleteSandboxRecord(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM sandboxes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("sandbox")
	}
	return nil
}

// A nil organization lists every organization's sandboxes. Only the
// Orchestrator asks for that: it reconciles sandboxes wherever they are, and
// deriving the set of organizations any other way means one it has not heard of
// is never reconciled at all.
func (s *Store) ListSandboxes(ctx context.Context, organizationID *uuid.UUID, filter SandboxFilter, pageSize int32, cursor *PageCursor) (SandboxListResult, error) {
	clauses := []string{}
	args := []any{}
	if organizationID != nil {
		clauses, args = appendClause(clauses, args, "organization_id = $%d", *organizationID)
	}
	if filter.OwnerID != nil {
		clauses, args = appendClause(clauses, args, "owner_id = $%d", *filter.OwnerID)
	}
	if !filter.IncludeTerminated {
		clauses = append(clauses, "status <> 'terminated'")
	}

	sandboxes, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM sandboxes", sandboxColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanSandbox,
		func(sandbox Sandbox) uuid.UUID { return sandbox.Meta.ID },
	)
	if err != nil {
		return SandboxListResult{}, err
	}
	return SandboxListResult{Sandboxes: sandboxes, NextCursor: nextCursor}, nil
}
