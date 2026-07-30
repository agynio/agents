package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	agentColumns                     = `id, organization_id, name, nickname, role, model, description, configuration, image, init_image, idle_timeout, capabilities, availability, resources_requests_cpu, resources_requests_memory, resources_limits_cpu, resources_limits_memory, environment_id, created_at, updated_at`
	volumeColumns                    = `id, organization_id, persistent, mount_path, size, description, ttl, created_at, updated_at`
	volumeAttachmentColumns          = `id, volume_id, agent_id, mcp_id, hook_id, created_at, updated_at`
	imagePullSecretAttachmentColumns = `id, organization_id, image_pull_secret_id, agent_id, mcp_id, hook_id, environment_id, created_at, updated_at`
	environmentColumns               = `id, organization_id, name, flavor_id, image, flavor_name, runner_id, flavor, created_at, updated_at`
	sandboxColumns                   = `id, organization_id, name, environment_id, owner_id, status, idle_timeout, ttl, last_session_at, environment_name, workload_id, created_at, updated_at`
	mcpColumns                       = `id, agent_id, name, image, command, resources_requests_cpu, resources_requests_memory, resources_limits_cpu, resources_limits_memory, description, created_at, updated_at`
	skillColumns                     = `id, agent_id, name, body, description, created_at, updated_at`
	hookColumns                      = `id, agent_id, event, "function", image, resources_requests_cpu, resources_requests_memory, resources_limits_cpu, resources_limits_memory, description, created_at, updated_at`
	envColumns                       = `id, organization_id, name, description, agent_id, mcp_id, hook_id, environment_id, value, secret_id, created_at, updated_at`
	initScriptColumns                = `id, script, description, agent_id, mcp_id, hook_id, created_at, updated_at`
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func uuidPtrFromPg(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func stringPtrFromPg(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func decodeCapabilities(value []byte) ([]string, error) {
	if value == nil {
		return nil, fmt.Errorf("capabilities is NULL")
	}
	var capabilities []string
	if err := json.Unmarshal(value, &capabilities); err != nil {
		return nil, fmt.Errorf("decode capabilities: %w", err)
	}
	if capabilities == nil {
		return nil, fmt.Errorf("capabilities must be a JSON array")
	}
	return capabilities, nil
}

func encodeCapabilities(capabilities []string) ([]byte, error) {
	if capabilities == nil {
		capabilities = []string{}
	}
	data, err := json.Marshal(capabilities)
	if err != nil {
		return nil, fmt.Errorf("encode capabilities: %w", err)
	}
	return data, nil
}

func scanAgent(row pgx.Row) (Agent, error) {
	var agent Agent
	var idleTimeout pgtype.Text
	var capabilities []byte
	var environmentID pgtype.UUID
	if err := row.Scan(
		&agent.Meta.ID,
		&agent.OrganizationID,
		&agent.Name,
		&agent.Nickname,
		&agent.Role,
		&agent.Model,
		&agent.Description,
		&agent.Configuration,
		&agent.Image,
		&agent.InitImage,
		&idleTimeout,
		&capabilities,
		&agent.Availability,
		&agent.Resources.RequestsCPU,
		&agent.Resources.RequestsMemory,
		&agent.Resources.LimitsCPU,
		&agent.Resources.LimitsMemory,
		&environmentID,
		&agent.Meta.CreatedAt,
		&agent.Meta.UpdatedAt,
	); err != nil {
		return Agent{}, err
	}
	agent.IdleTimeout = stringPtrFromPg(idleTimeout)
	agent.EnvironmentID = uuidPtrFromPg(environmentID)
	decodedCapabilities, err := decodeCapabilities(capabilities)
	if err != nil {
		return Agent{}, err
	}
	agent.Capabilities = decodedCapabilities
	return agent, nil
}

func scanVolume(row pgx.Row) (Volume, error) {
	var volume Volume
	var ttl pgtype.Text
	if err := row.Scan(
		&volume.Meta.ID,
		&volume.OrganizationID,
		&volume.Persistent,
		&volume.MountPath,
		&volume.Size,
		&volume.Description,
		&ttl,
		&volume.Meta.CreatedAt,
		&volume.Meta.UpdatedAt,
	); err != nil {
		return Volume{}, err
	}
	volume.TTL = stringPtrFromPg(ttl)
	return volume, nil
}

func scanVolumeAttachment(row pgx.Row) (VolumeAttachment, error) {
	var attachment VolumeAttachment
	var agentID pgtype.UUID
	var mcpID pgtype.UUID
	var hookID pgtype.UUID
	if err := row.Scan(
		&attachment.Meta.ID,
		&attachment.VolumeID,
		&agentID,
		&mcpID,
		&hookID,
		&attachment.Meta.CreatedAt,
		&attachment.Meta.UpdatedAt,
	); err != nil {
		return VolumeAttachment{}, err
	}
	attachment.AgentID = uuidPtrFromPg(agentID)
	attachment.McpID = uuidPtrFromPg(mcpID)
	attachment.HookID = uuidPtrFromPg(hookID)
	return attachment, nil
}

func scanImagePullSecretAttachment(row pgx.Row) (ImagePullSecretAttachment, error) {
	var attachment ImagePullSecretAttachment
	var agentID pgtype.UUID
	var mcpID pgtype.UUID
	var hookID pgtype.UUID
	var environmentID pgtype.UUID
	if err := row.Scan(
		&attachment.Meta.ID,
		&attachment.OrganizationID,
		&attachment.ImagePullSecretID,
		&agentID,
		&mcpID,
		&hookID,
		&environmentID,
		&attachment.Meta.CreatedAt,
		&attachment.Meta.UpdatedAt,
	); err != nil {
		return ImagePullSecretAttachment{}, err
	}
	attachment.AgentID = uuidPtrFromPg(agentID)
	attachment.McpID = uuidPtrFromPg(mcpID)
	attachment.HookID = uuidPtrFromPg(hookID)
	attachment.EnvironmentID = uuidPtrFromPg(environmentID)
	return attachment, nil
}

func scanEnvironment(row pgx.Row) (Environment, error) {
	var environment Environment
	if err := row.Scan(
		&environment.Meta.ID,
		&environment.OrganizationID,
		&environment.Name,
		&environment.FlavorID,
		&environment.Image,
		&environment.FlavorName,
		&environment.RunnerID,
		&environment.Flavor,
		&environment.Meta.CreatedAt,
		&environment.Meta.UpdatedAt,
	); err != nil {
		return Environment{}, err
	}
	return environment, nil
}

func scanSandbox(row pgx.Row) (Sandbox, error) {
	var sandbox Sandbox
	var lastSessionAt pgtype.Timestamptz
	var workloadID pgtype.UUID
	if err := row.Scan(
		&sandbox.Meta.ID,
		&sandbox.OrganizationID,
		&sandbox.Name,
		&sandbox.EnvironmentID,
		&sandbox.OwnerID,
		&sandbox.Status,
		&sandbox.IdleTimeout,
		&sandbox.TTL,
		&lastSessionAt,
		&sandbox.EnvironmentName,
		&workloadID,
		&sandbox.Meta.CreatedAt,
		&sandbox.Meta.UpdatedAt,
	); err != nil {
		return Sandbox{}, err
	}
	if lastSessionAt.Valid {
		sandbox.LastSessionAt = &lastSessionAt.Time
	}
	sandbox.WorkloadID = uuidPtrFromPg(workloadID)
	return sandbox, nil
}

func scanMcp(row pgx.Row) (Mcp, error) {
	var mcp Mcp
	if err := row.Scan(
		&mcp.Meta.ID,
		&mcp.AgentID,
		&mcp.Name,
		&mcp.Image,
		&mcp.Command,
		&mcp.Resources.RequestsCPU,
		&mcp.Resources.RequestsMemory,
		&mcp.Resources.LimitsCPU,
		&mcp.Resources.LimitsMemory,
		&mcp.Description,
		&mcp.Meta.CreatedAt,
		&mcp.Meta.UpdatedAt,
	); err != nil {
		return Mcp{}, err
	}
	return mcp, nil
}

func scanSkill(row pgx.Row) (Skill, error) {
	var skill Skill
	if err := row.Scan(
		&skill.Meta.ID,
		&skill.AgentID,
		&skill.Name,
		&skill.Body,
		&skill.Description,
		&skill.Meta.CreatedAt,
		&skill.Meta.UpdatedAt,
	); err != nil {
		return Skill{}, err
	}
	return skill, nil
}

func scanHook(row pgx.Row) (Hook, error) {
	var hook Hook
	if err := row.Scan(
		&hook.Meta.ID,
		&hook.AgentID,
		&hook.Event,
		&hook.Function,
		&hook.Image,
		&hook.Resources.RequestsCPU,
		&hook.Resources.RequestsMemory,
		&hook.Resources.LimitsCPU,
		&hook.Resources.LimitsMemory,
		&hook.Description,
		&hook.Meta.CreatedAt,
		&hook.Meta.UpdatedAt,
	); err != nil {
		return Hook{}, err
	}
	return hook, nil
}

func scanEnv(row pgx.Row) (Env, error) {
	var env Env
	var agentID pgtype.UUID
	var mcpID pgtype.UUID
	var hookID pgtype.UUID
	var environmentID pgtype.UUID
	var value pgtype.Text
	var secretID pgtype.UUID
	if err := row.Scan(
		&env.Meta.ID,
		&env.OrganizationID,
		&env.Name,
		&env.Description,
		&agentID,
		&mcpID,
		&hookID,
		&environmentID,
		&value,
		&secretID,
		&env.Meta.CreatedAt,
		&env.Meta.UpdatedAt,
	); err != nil {
		return Env{}, err
	}
	env.AgentID = uuidPtrFromPg(agentID)
	env.McpID = uuidPtrFromPg(mcpID)
	env.HookID = uuidPtrFromPg(hookID)
	env.EnvironmentID = uuidPtrFromPg(environmentID)
	env.Value = stringPtrFromPg(value)
	env.SecretID = uuidPtrFromPg(secretID)
	return env, nil
}

func scanInitScript(row pgx.Row) (InitScript, error) {
	var script InitScript
	var agentID pgtype.UUID
	var mcpID pgtype.UUID
	var hookID pgtype.UUID
	if err := row.Scan(
		&script.Meta.ID,
		&script.Script,
		&script.Description,
		&agentID,
		&mcpID,
		&hookID,
		&script.Meta.CreatedAt,
		&script.Meta.UpdatedAt,
	); err != nil {
		return InitScript{}, err
	}
	script.AgentID = uuidPtrFromPg(agentID)
	script.McpID = uuidPtrFromPg(mcpID)
	script.HookID = uuidPtrFromPg(hookID)
	return script, nil
}

func scanAgentRoleAssignment(row pgx.Row) (AgentRoleAssignment, error) {
	var assignment AgentRoleAssignment
	if err := row.Scan(&assignment.AgentID, &assignment.IdentityID, &assignment.Role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AgentRoleAssignment{}, NotFound("agent role")
		}
		return AgentRoleAssignment{}, err
	}
	return assignment, nil
}

func scanAgentRoleAssignments(rows pgx.Rows) ([]AgentRoleAssignment, error) {
	assignments := []AgentRoleAssignment{}
	for rows.Next() {
		assignment, err := scanAgentRoleAssignment(rows)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, assignment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return assignments, nil
}

func (s *Store) CreateAgent(ctx context.Context, organizationID uuid.UUID, input AgentInput) (Agent, error) {
	capabilitiesJSON, err := encodeCapabilities(input.Capabilities)
	if err != nil {
		return Agent{}, err
	}
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO agents (organization_id, name, nickname, role, model, description, configuration, image, init_image, idle_timeout, capabilities, availability, resources_requests_cpu, resources_requests_memory, resources_limits_cpu, resources_limits_memory, environment_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 RETURNING %s`, agentColumns),
		organizationID,
		input.Name,
		input.Nickname,
		input.Role,
		input.Model,
		input.Description,
		input.Configuration,
		input.Image,
		input.InitImage,
		input.IdleTimeout,
		capabilitiesJSON,
		input.Availability,
		input.Resources.RequestsCPU,
		input.Resources.RequestsMemory,
		input.Resources.LimitsCPU,
		input.Resources.LimitsMemory,
		input.EnvironmentID,
	)
	agent, err := scanAgent(row)
	if err != nil {
		return Agent{}, err
	}
	return agent, nil
}

func (s *Store) GetAgent(ctx context.Context, id uuid.UUID) (Agent, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM agents WHERE id = $1`, agentColumns),
		id,
	)
	agent, err := scanAgent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Agent{}, NotFound("agent")
		}
		return Agent{}, err
	}
	return agent, nil
}

func (s *Store) UpdateAgent(ctx context.Context, id uuid.UUID, update AgentUpdate) (Agent, error) {
	if update.EnvironmentID != nil && update.ClearEnvironmentID {
		return Agent{}, fmt.Errorf("agent update cannot set and clear environment_id")
	}
	builder := updateBuilder{}
	if update.Name != nil {
		builder.add("name", *update.Name)
	}
	if update.Nickname != nil {
		builder.add("nickname", *update.Nickname)
	}
	if update.Role != nil {
		builder.add("role", *update.Role)
	}
	if update.Model != nil {
		builder.add("model", *update.Model)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}
	if update.Configuration != nil {
		builder.add("configuration", *update.Configuration)
	}
	if update.Image != nil {
		builder.add("image", *update.Image)
	}
	if update.InitImage != nil {
		builder.add("init_image", *update.InitImage)
	}
	if update.IdleTimeout != nil {
		builder.add("idle_timeout", *update.IdleTimeout)
	}
	if update.Capabilities != nil {
		capabilitiesJSON, err := encodeCapabilities(*update.Capabilities)
		if err != nil {
			return Agent{}, err
		}
		builder.add("capabilities", capabilitiesJSON)
	}
	if update.Availability != nil {
		builder.add("availability", *update.Availability)
	}
	if update.Resources != nil {
		builder.add("resources_requests_cpu", update.Resources.RequestsCPU)
		builder.add("resources_requests_memory", update.Resources.RequestsMemory)
		builder.add("resources_limits_cpu", update.Resources.LimitsCPU)
		builder.add("resources_limits_memory", update.Resources.LimitsMemory)
	}
	if update.EnvironmentID != nil {
		builder.add("environment_id", *update.EnvironmentID)
	}
	if update.ClearEnvironmentID {
		builder.addNull("environment_id")
	}

	if builder.empty() {
		return Agent{}, fmt.Errorf("agent update requires at least one field")
	}
	query, args := builder.build("agents", agentColumns, id)
	row := s.pool.QueryRow(ctx, query, args...)
	agent, err := scanAgent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Agent{}, NotFound("agent")
		}
		return Agent{}, err
	}
	return agent, nil
}

func (s *Store) UpsertAgentRole(ctx context.Context, assignment AgentRoleAssignment) (AgentRoleAssignment, error) {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_roles (agent_id, identity_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (agent_id, identity_id)
		 DO UPDATE SET role = EXCLUDED.role, updated_at = NOW()`,
		assignment.AgentID,
		assignment.IdentityID,
		assignment.Role,
	)
	if err != nil {
		return AgentRoleAssignment{}, err
	}
	return assignment, nil
}

func (s *Store) DeleteAgentRole(ctx context.Context, agentID, identityID uuid.UUID) (AgentRoleAssignment, error) {
	return s.deleteAgentRole(ctx, agentID, identityID)
}

func (s *Store) DeleteAgentRoleIfExists(ctx context.Context, agentID, identityID uuid.UUID) error {
	_, err := s.deleteAgentRole(ctx, agentID, identityID)
	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		return nil
	}
	return err
}

func (s *Store) deleteAgentRole(ctx context.Context, agentID, identityID uuid.UUID) (AgentRoleAssignment, error) {
	row := s.pool.QueryRow(ctx,
		`DELETE FROM agent_roles WHERE agent_id = $1 AND identity_id = $2 RETURNING agent_id, identity_id, role`,
		agentID,
		identityID,
	)
	return scanAgentRoleAssignment(row)
}

func (s *Store) GetAgentRole(ctx context.Context, agentID, identityID uuid.UUID) (AgentRoleAssignment, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT agent_id, identity_id, role FROM agent_roles WHERE agent_id = $1 AND identity_id = $2`,
		agentID,
		identityID,
	)
	return scanAgentRoleAssignment(row)
}

func (s *Store) ListAgentRoles(ctx context.Context, agentID uuid.UUID) ([]AgentRoleAssignment, error) {
	rows, err := s.pool.Query(ctx, `SELECT agent_id, identity_id, role FROM agent_roles WHERE agent_id = $1 ORDER BY identity_id ASC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentRoleAssignments(rows)
}

func (s *Store) ListIdentityAgentRoles(ctx context.Context, organizationID, identityID uuid.UUID) ([]AgentRoleAssignment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT ar.agent_id, ar.identity_id, ar.role
		 FROM agent_roles ar
		 JOIN agents a ON a.id = ar.agent_id
		 WHERE a.organization_id = $1 AND ar.identity_id = $2
		 ORDER BY ar.agent_id ASC`,
		organizationID,
		identityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentRoleAssignments(rows)
}

func (s *Store) DeleteAgent(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ForeignKeyViolation("agent")
		}
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("agent")
	}
	return nil
}

func (s *Store) ListAgents(ctx context.Context, organizationID *uuid.UUID, _ AgentFilter, pageSize int32, cursor *PageCursor) (AgentListResult, error) {
	var clauses []string
	var args []any
	if organizationID != nil {
		clauses, args = appendClause(clauses, args, "organization_id = $%d", *organizationID)
	}
	agents, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM agents", agentColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanAgent,
		func(agent Agent) uuid.UUID { return agent.Meta.ID },
	)
	if err != nil {
		return AgentListResult{}, err
	}
	return AgentListResult{Agents: agents, NextCursor: nextCursor}, nil
}

func (s *Store) CreateVolume(ctx context.Context, organizationID uuid.UUID, input VolumeInput) (Volume, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO volumes (organization_id, persistent, mount_path, size, description, ttl)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING %s`, volumeColumns),
		organizationID,
		input.Persistent,
		input.MountPath,
		input.Size,
		input.Description,
		input.TTL,
	)
	volume, err := scanVolume(row)
	if err != nil {
		return Volume{}, err
	}
	return volume, nil
}

func (s *Store) GetVolume(ctx context.Context, id uuid.UUID) (Volume, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM volumes WHERE id = $1`, volumeColumns),
		id,
	)
	volume, err := scanVolume(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Volume{}, NotFound("volume")
		}
		return Volume{}, err
	}
	return volume, nil
}

func (s *Store) UpdateVolume(ctx context.Context, id uuid.UUID, update VolumeUpdate) (Volume, error) {
	builder := updateBuilder{}
	if update.Persistent != nil {
		builder.add("persistent", *update.Persistent)
	}
	if update.MountPath != nil {
		builder.add("mount_path", *update.MountPath)
	}
	if update.Size != nil {
		builder.add("size", *update.Size)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}
	if update.TTL != nil {
		builder.add("ttl", *update.TTL)
	}

	if builder.empty() {
		return Volume{}, fmt.Errorf("volume update requires at least one field")
	}

	return withTx(ctx, s.pool, func(tx pgx.Tx) (Volume, error) {
		query, args := builder.build("volumes", volumeColumns, id)
		row := tx.QueryRow(ctx, query, args...)
		volume, err := scanVolume(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Volume{}, NotFound("volume")
			}
			return Volume{}, err
		}
		agentIDs, err := agentIDsForVolume(ctx, tx, volume.Meta.ID)
		if err != nil {
			return Volume{}, err
		}
		if err := touchAgentsUpdatedAt(ctx, tx, agentIDs); err != nil {
			return Volume{}, err
		}
		return volume, nil
	})
}

func (s *Store) DeleteVolume(ctx context.Context, id uuid.UUID) error {
	_, err := withTx(ctx, s.pool, func(tx pgx.Tx) (struct{}, error) {
		agentIDs, err := agentIDsForVolume(ctx, tx, id)
		if err != nil {
			return struct{}{}, err
		}
		result, err := tx.Exec(ctx, `DELETE FROM volumes WHERE id = $1`, id)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return struct{}{}, ForeignKeyViolation("volume")
			}
			return struct{}{}, err
		}
		if result.RowsAffected() == 0 {
			return struct{}{}, NotFound("volume")
		}
		if err := touchAgentsUpdatedAt(ctx, tx, agentIDs); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (s *Store) ListVolumes(ctx context.Context, organizationID uuid.UUID, _ VolumeFilter, pageSize int32, cursor *PageCursor) (VolumeListResult, error) {
	clauses := []string{"organization_id = $1"}
	args := []any{organizationID}
	volumes, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM volumes", volumeColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanVolume,
		func(volume Volume) uuid.UUID { return volume.Meta.ID },
	)
	if err != nil {
		return VolumeListResult{}, err
	}
	return VolumeListResult{Volumes: volumes, NextCursor: nextCursor}, nil
}

func (s *Store) ListAgentIDsForVolume(ctx context.Context, volumeID uuid.UUID) ([]uuid.UUID, error) {
	return agentIDsForVolume(ctx, s.pool, volumeID)
}

func (s *Store) CreateVolumeAttachment(ctx context.Context, input VolumeAttachmentInput) (VolumeAttachment, error) {
	return withTx(ctx, s.pool, func(tx pgx.Tx) (VolumeAttachment, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`INSERT INTO volume_attachments (volume_id, agent_id, mcp_id, hook_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING %s`, volumeAttachmentColumns),
			input.VolumeID,
			input.AgentID,
			input.McpID,
			input.HookID,
		)
		attachment, err := scanVolumeAttachment(row)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				switch pgErr.Code {
				case "23505":
					return VolumeAttachment{}, AlreadyExists("volume attachment")
				case "23503":
					return VolumeAttachment{}, ForeignKeyViolation("volume attachment")
				}
			}
			return VolumeAttachment{}, err
		}
		agentID, err := resolveAgentID(ctx, tx, attachment.AgentID, attachment.McpID, attachment.HookID)
		if err != nil {
			return VolumeAttachment{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, agentID); err != nil {
			return VolumeAttachment{}, err
		}
		return attachment, nil
	})
}

func (s *Store) GetVolumeAttachment(ctx context.Context, id uuid.UUID) (VolumeAttachment, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM volume_attachments WHERE id = $1`, volumeAttachmentColumns),
		id,
	)
	attachment, err := scanVolumeAttachment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VolumeAttachment{}, NotFound("volume attachment")
		}
		return VolumeAttachment{}, err
	}
	return attachment, nil
}

func (s *Store) DeleteVolumeAttachment(ctx context.Context, id uuid.UUID) error {
	_, err := withTx(ctx, s.pool, func(tx pgx.Tx) (struct{}, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`DELETE FROM volume_attachments WHERE id = $1 RETURNING %s`, volumeAttachmentColumns),
			id,
		)
		attachment, err := scanVolumeAttachment(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return struct{}{}, NotFound("volume attachment")
			}
			return struct{}{}, err
		}
		agentID, err := resolveAgentID(ctx, tx, attachment.AgentID, attachment.McpID, attachment.HookID)
		if err != nil {
			return struct{}{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, agentID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (s *Store) ListVolumeAttachments(ctx context.Context, filter VolumeAttachmentFilter, pageSize int32, cursor *PageCursor) (VolumeAttachmentListResult, error) {
	clauses := []string{}
	args := []any{}
	if filter.VolumeID != nil {
		clauses, args = appendClause(clauses, args, "volume_id = $%d", *filter.VolumeID)
	}
	if filter.AgentID != nil {
		clauses, args = appendClause(clauses, args, "agent_id = $%d", *filter.AgentID)
	}
	if filter.McpID != nil {
		clauses, args = appendClause(clauses, args, "mcp_id = $%d", *filter.McpID)
	}
	if filter.HookID != nil {
		clauses, args = appendClause(clauses, args, "hook_id = $%d", *filter.HookID)
	}

	attachments, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM volume_attachments", volumeAttachmentColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanVolumeAttachment,
		func(attachment VolumeAttachment) uuid.UUID { return attachment.Meta.ID },
	)
	if err != nil {
		return VolumeAttachmentListResult{}, err
	}
	return VolumeAttachmentListResult{VolumeAttachments: attachments, NextCursor: nextCursor}, nil
}

func (s *Store) CreateImagePullSecretAttachment(ctx context.Context, input ImagePullSecretAttachmentInput) (ImagePullSecretAttachment, error) {
	return withTx(ctx, s.pool, func(tx pgx.Tx) (ImagePullSecretAttachment, error) {
		// A caller names only the target, so the organization the row is scoped
		// by is derived from it, in the same transaction that writes the row.
		organizationID, err := organizationIDForTarget(ctx, tx, input.AgentID, input.McpID, input.HookID, input.EnvironmentID)
		if err != nil {
			return ImagePullSecretAttachment{}, err
		}
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`INSERT INTO image_pull_secret_attachments (organization_id, image_pull_secret_id, agent_id, mcp_id, hook_id, environment_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING %s`, imagePullSecretAttachmentColumns),
			organizationID,
			input.ImagePullSecretID,
			input.AgentID,
			input.McpID,
			input.HookID,
			input.EnvironmentID,
		)
		attachment, err := scanImagePullSecretAttachment(row)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				switch pgErr.Code {
				case "23505":
					return ImagePullSecretAttachment{}, AlreadyExists("image pull secret attachment")
				case "23503":
					return ImagePullSecretAttachment{}, ForeignKeyViolation("image pull secret attachment")
				}
			}
			return ImagePullSecretAttachment{}, err
		}
		if err := touchTargetAgent(ctx, tx, attachment.AgentID, attachment.McpID, attachment.HookID, attachment.EnvironmentID); err != nil {
			return ImagePullSecretAttachment{}, err
		}
		return attachment, nil
	})
}

func (s *Store) GetImagePullSecretAttachment(ctx context.Context, id uuid.UUID) (ImagePullSecretAttachment, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM image_pull_secret_attachments WHERE id = $1`, imagePullSecretAttachmentColumns),
		id,
	)
	attachment, err := scanImagePullSecretAttachment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ImagePullSecretAttachment{}, NotFound("image pull secret attachment")
		}
		return ImagePullSecretAttachment{}, err
	}
	return attachment, nil
}

func (s *Store) DeleteImagePullSecretAttachment(ctx context.Context, id uuid.UUID) error {
	_, err := withTx(ctx, s.pool, func(tx pgx.Tx) (struct{}, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`DELETE FROM image_pull_secret_attachments WHERE id = $1 RETURNING %s`, imagePullSecretAttachmentColumns),
			id,
		)
		attachment, err := scanImagePullSecretAttachment(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return struct{}{}, NotFound("image pull secret attachment")
			}
			return struct{}{}, err
		}
		if err := touchTargetAgent(ctx, tx, attachment.AgentID, attachment.McpID, attachment.HookID, attachment.EnvironmentID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (s *Store) ListImagePullSecretAttachments(ctx context.Context, filter ImagePullSecretAttachmentFilter, pageSize int32, cursor *PageCursor) (ImagePullSecretAttachmentListResult, error) {
	clauses := []string{}
	args := []any{}
	if filter.OrganizationID != nil {
		clauses, args = appendClause(clauses, args, "organization_id = $%d", *filter.OrganizationID)
	}
	if filter.ImagePullSecretID != nil {
		clauses, args = appendClause(clauses, args, "image_pull_secret_id = $%d", *filter.ImagePullSecretID)
	}
	if filter.AgentID != nil {
		clauses, args = appendClause(clauses, args, "agent_id = $%d", *filter.AgentID)
	}
	if filter.McpID != nil {
		clauses, args = appendClause(clauses, args, "mcp_id = $%d", *filter.McpID)
	}
	if filter.HookID != nil {
		clauses, args = appendClause(clauses, args, "hook_id = $%d", *filter.HookID)
	}
	if filter.EnvironmentID != nil {
		clauses, args = appendClause(clauses, args, "environment_id = $%d", *filter.EnvironmentID)
	}

	attachments, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM image_pull_secret_attachments", imagePullSecretAttachmentColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanImagePullSecretAttachment,
		func(attachment ImagePullSecretAttachment) uuid.UUID { return attachment.Meta.ID },
	)
	if err != nil {
		return ImagePullSecretAttachmentListResult{}, err
	}
	return ImagePullSecretAttachmentListResult{ImagePullSecretAttachments: attachments, NextCursor: nextCursor}, nil
}

func (s *Store) CreateMcp(ctx context.Context, input McpInput) (Mcp, error) {
	return withTx(ctx, s.pool, func(tx pgx.Tx) (Mcp, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`INSERT INTO mcps (agent_id, name, image, command, resources_requests_cpu, resources_requests_memory, resources_limits_cpu, resources_limits_memory, description)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING %s`, mcpColumns),
			input.AgentID,
			input.Name,
			input.Image,
			input.Command,
			input.Resources.RequestsCPU,
			input.Resources.RequestsMemory,
			input.Resources.LimitsCPU,
			input.Resources.LimitsMemory,
			input.Description,
		)
		mcp, err := scanMcp(row)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				switch pgErr.Code {
				case "23503":
					return Mcp{}, ForeignKeyViolation("mcp")
				case "23505":
					return Mcp{}, AlreadyExists("mcp")
				}
			}
			return Mcp{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, mcp.AgentID); err != nil {
			return Mcp{}, err
		}
		return mcp, nil
	})
}

func (s *Store) GetMcp(ctx context.Context, id uuid.UUID) (Mcp, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM mcps WHERE id = $1`, mcpColumns),
		id,
	)
	mcp, err := scanMcp(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Mcp{}, NotFound("mcp")
		}
		return Mcp{}, err
	}
	return mcp, nil
}

func (s *Store) UpdateMcp(ctx context.Context, id uuid.UUID, update McpUpdate) (Mcp, error) {
	builder := updateBuilder{}
	if update.Image != nil {
		builder.add("image", *update.Image)
	}
	if update.Command != nil {
		builder.add("command", *update.Command)
	}
	if update.Resources != nil {
		builder.add("resources_requests_cpu", update.Resources.RequestsCPU)
		builder.add("resources_requests_memory", update.Resources.RequestsMemory)
		builder.add("resources_limits_cpu", update.Resources.LimitsCPU)
		builder.add("resources_limits_memory", update.Resources.LimitsMemory)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}

	if builder.empty() {
		return Mcp{}, fmt.Errorf("mcp update requires at least one field")
	}
	return withTx(ctx, s.pool, func(tx pgx.Tx) (Mcp, error) {
		query, args := builder.build("mcps", mcpColumns, id)
		row := tx.QueryRow(ctx, query, args...)
		mcp, err := scanMcp(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Mcp{}, NotFound("mcp")
			}
			return Mcp{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, mcp.AgentID); err != nil {
			return Mcp{}, err
		}
		return mcp, nil
	})
}

func (s *Store) DeleteMcp(ctx context.Context, id uuid.UUID) error {
	_, err := withTx(ctx, s.pool, func(tx pgx.Tx) (struct{}, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`DELETE FROM mcps WHERE id = $1 RETURNING %s`, mcpColumns),
			id,
		)
		mcp, err := scanMcp(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return struct{}{}, NotFound("mcp")
			}
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return struct{}{}, ForeignKeyViolation("mcp")
			}
			return struct{}{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, mcp.AgentID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (s *Store) ListMcps(ctx context.Context, filter McpFilter, pageSize int32, cursor *PageCursor) (McpListResult, error) {
	clauses := []string{}
	args := []any{}
	if filter.AgentID != nil {
		clauses, args = appendClause(clauses, args, "agent_id = $%d", *filter.AgentID)
	}

	mcps, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM mcps", mcpColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanMcp,
		func(mcp Mcp) uuid.UUID { return mcp.Meta.ID },
	)
	if err != nil {
		return McpListResult{}, err
	}
	return McpListResult{Mcps: mcps, NextCursor: nextCursor}, nil
}

func (s *Store) CreateSkill(ctx context.Context, input SkillInput) (Skill, error) {
	return withTx(ctx, s.pool, func(tx pgx.Tx) (Skill, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`INSERT INTO skills (agent_id, name, body, description)
		 VALUES ($1, $2, $3, $4)
		 RETURNING %s`, skillColumns),
			input.AgentID,
			input.Name,
			input.Body,
			input.Description,
		)
		skill, err := scanSkill(row)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return Skill{}, ForeignKeyViolation("skill")
			}
			return Skill{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, skill.AgentID); err != nil {
			return Skill{}, err
		}
		return skill, nil
	})
}

func (s *Store) GetSkill(ctx context.Context, id uuid.UUID) (Skill, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM skills WHERE id = $1`, skillColumns),
		id,
	)
	skill, err := scanSkill(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Skill{}, NotFound("skill")
		}
		return Skill{}, err
	}
	return skill, nil
}

func (s *Store) UpdateSkill(ctx context.Context, id uuid.UUID, update SkillUpdate) (Skill, error) {
	builder := updateBuilder{}
	if update.Name != nil {
		builder.add("name", *update.Name)
	}
	if update.Body != nil {
		builder.add("body", *update.Body)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}

	if builder.empty() {
		return Skill{}, fmt.Errorf("skill update requires at least one field")
	}
	return withTx(ctx, s.pool, func(tx pgx.Tx) (Skill, error) {
		query, args := builder.build("skills", skillColumns, id)
		row := tx.QueryRow(ctx, query, args...)
		skill, err := scanSkill(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Skill{}, NotFound("skill")
			}
			return Skill{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, skill.AgentID); err != nil {
			return Skill{}, err
		}
		return skill, nil
	})
}

func (s *Store) DeleteSkill(ctx context.Context, id uuid.UUID) error {
	_, err := withTx(ctx, s.pool, func(tx pgx.Tx) (struct{}, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`DELETE FROM skills WHERE id = $1 RETURNING %s`, skillColumns),
			id,
		)
		skill, err := scanSkill(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return struct{}{}, NotFound("skill")
			}
			return struct{}{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, skill.AgentID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (s *Store) ListSkills(ctx context.Context, filter SkillFilter, pageSize int32, cursor *PageCursor) (SkillListResult, error) {
	clauses := []string{}
	args := []any{}
	if filter.AgentID != nil {
		clauses, args = appendClause(clauses, args, "agent_id = $%d", *filter.AgentID)
	}

	skills, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM skills", skillColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanSkill,
		func(skill Skill) uuid.UUID { return skill.Meta.ID },
	)
	if err != nil {
		return SkillListResult{}, err
	}
	return SkillListResult{Skills: skills, NextCursor: nextCursor}, nil
}

func (s *Store) CreateHook(ctx context.Context, input HookInput) (Hook, error) {
	return withTx(ctx, s.pool, func(tx pgx.Tx) (Hook, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`INSERT INTO hooks (agent_id, event, "function", image, resources_requests_cpu, resources_requests_memory, resources_limits_cpu, resources_limits_memory, description)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING %s`, hookColumns),
			input.AgentID,
			input.Event,
			input.Function,
			input.Image,
			input.Resources.RequestsCPU,
			input.Resources.RequestsMemory,
			input.Resources.LimitsCPU,
			input.Resources.LimitsMemory,
			input.Description,
		)
		hook, err := scanHook(row)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return Hook{}, ForeignKeyViolation("hook")
			}
			return Hook{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, hook.AgentID); err != nil {
			return Hook{}, err
		}
		return hook, nil
	})
}

func (s *Store) GetHook(ctx context.Context, id uuid.UUID) (Hook, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM hooks WHERE id = $1`, hookColumns),
		id,
	)
	hook, err := scanHook(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Hook{}, NotFound("hook")
		}
		return Hook{}, err
	}
	return hook, nil
}

func (s *Store) UpdateHook(ctx context.Context, id uuid.UUID, update HookUpdate) (Hook, error) {
	builder := updateBuilder{}
	if update.Event != nil {
		builder.add("event", *update.Event)
	}
	if update.Function != nil {
		builder.add("\"function\"", *update.Function)
	}
	if update.Image != nil {
		builder.add("image", *update.Image)
	}
	if update.Resources != nil {
		builder.add("resources_requests_cpu", update.Resources.RequestsCPU)
		builder.add("resources_requests_memory", update.Resources.RequestsMemory)
		builder.add("resources_limits_cpu", update.Resources.LimitsCPU)
		builder.add("resources_limits_memory", update.Resources.LimitsMemory)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}

	if builder.empty() {
		return Hook{}, fmt.Errorf("hook update requires at least one field")
	}
	return withTx(ctx, s.pool, func(tx pgx.Tx) (Hook, error) {
		query, args := builder.build("hooks", hookColumns, id)
		row := tx.QueryRow(ctx, query, args...)
		hook, err := scanHook(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Hook{}, NotFound("hook")
			}
			return Hook{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, hook.AgentID); err != nil {
			return Hook{}, err
		}
		return hook, nil
	})
}

func (s *Store) DeleteHook(ctx context.Context, id uuid.UUID) error {
	_, err := withTx(ctx, s.pool, func(tx pgx.Tx) (struct{}, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`DELETE FROM hooks WHERE id = $1 RETURNING %s`, hookColumns),
			id,
		)
		hook, err := scanHook(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return struct{}{}, NotFound("hook")
			}
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return struct{}{}, ForeignKeyViolation("hook")
			}
			return struct{}{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, hook.AgentID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (s *Store) ListHooks(ctx context.Context, filter HookFilter, pageSize int32, cursor *PageCursor) (HookListResult, error) {
	clauses := []string{}
	args := []any{}
	if filter.AgentID != nil {
		clauses, args = appendClause(clauses, args, "agent_id = $%d", *filter.AgentID)
	}

	hooks, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM hooks", hookColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanHook,
		func(hook Hook) uuid.UUID { return hook.Meta.ID },
	)
	if err != nil {
		return HookListResult{}, err
	}
	return HookListResult{Hooks: hooks, NextCursor: nextCursor}, nil
}

func (s *Store) CreateEnv(ctx context.Context, input EnvInput) (Env, error) {
	return withTx(ctx, s.pool, func(tx pgx.Tx) (Env, error) {
		// A caller names only the target, so the organization the row is scoped
		// by is derived from it, in the same transaction that writes the row.
		organizationID, err := organizationIDForTarget(ctx, tx, input.AgentID, input.McpID, input.HookID, input.EnvironmentID)
		if err != nil {
			return Env{}, err
		}
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`INSERT INTO envs (organization_id, name, description, agent_id, mcp_id, hook_id, environment_id, value, secret_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING %s`, envColumns),
			organizationID,
			input.Name,
			input.Description,
			input.AgentID,
			input.McpID,
			input.HookID,
			input.EnvironmentID,
			input.Value,
			input.SecretID,
		)
		env, err := scanEnv(row)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return Env{}, ForeignKeyViolation("env")
			}
			return Env{}, err
		}
		if err := touchTargetAgent(ctx, tx, env.AgentID, env.McpID, env.HookID, env.EnvironmentID); err != nil {
			return Env{}, err
		}
		return env, nil
	})
}

func (s *Store) GetEnv(ctx context.Context, id uuid.UUID) (Env, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM envs WHERE id = $1`, envColumns),
		id,
	)
	env, err := scanEnv(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Env{}, NotFound("env")
		}
		return Env{}, err
	}
	return env, nil
}

func (s *Store) UpdateEnv(ctx context.Context, id uuid.UUID, update EnvUpdate) (Env, error) {
	builder := updateBuilder{}
	if update.Name != nil {
		builder.add("name", *update.Name)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}
	if update.Value != nil {
		builder.add("value", *update.Value)
		builder.addNull("secret_id")
	}
	if update.SecretID != nil {
		builder.add("secret_id", *update.SecretID)
		builder.addNull("value")
	}

	if builder.empty() {
		return Env{}, fmt.Errorf("env update requires at least one field")
	}
	return withTx(ctx, s.pool, func(tx pgx.Tx) (Env, error) {
		query, args := builder.build("envs", envColumns, id)
		row := tx.QueryRow(ctx, query, args...)
		env, err := scanEnv(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Env{}, NotFound("env")
			}
			return Env{}, err
		}
		if err := touchTargetAgent(ctx, tx, env.AgentID, env.McpID, env.HookID, env.EnvironmentID); err != nil {
			return Env{}, err
		}
		return env, nil
	})
}

func (s *Store) DeleteEnv(ctx context.Context, id uuid.UUID) error {
	_, err := withTx(ctx, s.pool, func(tx pgx.Tx) (struct{}, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`DELETE FROM envs WHERE id = $1 RETURNING %s`, envColumns),
			id,
		)
		env, err := scanEnv(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return struct{}{}, NotFound("env")
			}
			return struct{}{}, err
		}
		if err := touchTargetAgent(ctx, tx, env.AgentID, env.McpID, env.HookID, env.EnvironmentID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (s *Store) ListEnvs(ctx context.Context, filter EnvFilter, pageSize int32, cursor *PageCursor) (EnvListResult, error) {
	clauses := []string{}
	args := []any{}
	if filter.OrganizationID != nil {
		clauses, args = appendClause(clauses, args, "organization_id = $%d", *filter.OrganizationID)
	}
	if filter.AgentID != nil {
		clauses, args = appendClause(clauses, args, "agent_id = $%d", *filter.AgentID)
	}
	if filter.McpID != nil {
		clauses, args = appendClause(clauses, args, "mcp_id = $%d", *filter.McpID)
	}
	if filter.HookID != nil {
		clauses, args = appendClause(clauses, args, "hook_id = $%d", *filter.HookID)
	}
	if filter.EnvironmentID != nil {
		clauses, args = appendClause(clauses, args, "environment_id = $%d", *filter.EnvironmentID)
	}

	envs, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM envs", envColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanEnv,
		func(env Env) uuid.UUID { return env.Meta.ID },
	)
	if err != nil {
		return EnvListResult{}, err
	}
	return EnvListResult{Envs: envs, NextCursor: nextCursor}, nil
}

func (s *Store) CreateInitScript(ctx context.Context, input InitScriptInput) (InitScript, error) {
	return withTx(ctx, s.pool, func(tx pgx.Tx) (InitScript, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`INSERT INTO init_scripts (script, description, agent_id, mcp_id, hook_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING %s`, initScriptColumns),
			input.Script,
			input.Description,
			input.AgentID,
			input.McpID,
			input.HookID,
		)
		script, err := scanInitScript(row)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return InitScript{}, ForeignKeyViolation("init script")
			}
			return InitScript{}, err
		}
		agentID, err := resolveAgentID(ctx, tx, script.AgentID, script.McpID, script.HookID)
		if err != nil {
			return InitScript{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, agentID); err != nil {
			return InitScript{}, err
		}
		return script, nil
	})
}

func (s *Store) GetInitScript(ctx context.Context, id uuid.UUID) (InitScript, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM init_scripts WHERE id = $1`, initScriptColumns),
		id,
	)
	script, err := scanInitScript(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InitScript{}, NotFound("init script")
		}
		return InitScript{}, err
	}
	return script, nil
}

func (s *Store) UpdateInitScript(ctx context.Context, id uuid.UUID, update InitScriptUpdate) (InitScript, error) {
	builder := updateBuilder{}
	if update.Script != nil {
		builder.add("script", *update.Script)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}

	if builder.empty() {
		return InitScript{}, fmt.Errorf("init script update requires at least one field")
	}
	return withTx(ctx, s.pool, func(tx pgx.Tx) (InitScript, error) {
		query, args := builder.build("init_scripts", initScriptColumns, id)
		row := tx.QueryRow(ctx, query, args...)
		script, err := scanInitScript(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return InitScript{}, NotFound("init script")
			}
			return InitScript{}, err
		}
		agentID, err := resolveAgentID(ctx, tx, script.AgentID, script.McpID, script.HookID)
		if err != nil {
			return InitScript{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, agentID); err != nil {
			return InitScript{}, err
		}
		return script, nil
	})
}

func (s *Store) DeleteInitScript(ctx context.Context, id uuid.UUID) error {
	_, err := withTx(ctx, s.pool, func(tx pgx.Tx) (struct{}, error) {
		row := tx.QueryRow(ctx,
			fmt.Sprintf(`DELETE FROM init_scripts WHERE id = $1 RETURNING %s`, initScriptColumns),
			id,
		)
		script, err := scanInitScript(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return struct{}{}, NotFound("init script")
			}
			return struct{}{}, err
		}
		agentID, err := resolveAgentID(ctx, tx, script.AgentID, script.McpID, script.HookID)
		if err != nil {
			return struct{}{}, err
		}
		if err := touchAgentUpdatedAt(ctx, tx, agentID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (s *Store) ListInitScripts(ctx context.Context, filter InitScriptFilter, pageSize int32, cursor *PageCursor) (InitScriptListResult, error) {
	clauses := []string{}
	args := []any{}
	if filter.AgentID != nil {
		clauses, args = appendClause(clauses, args, "agent_id = $%d", *filter.AgentID)
	}
	if filter.McpID != nil {
		clauses, args = appendClause(clauses, args, "mcp_id = $%d", *filter.McpID)
	}
	if filter.HookID != nil {
		clauses, args = appendClause(clauses, args, "hook_id = $%d", *filter.HookID)
	}

	scripts, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM init_scripts", initScriptColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanInitScript,
		func(script InitScript) uuid.UUID { return script.Meta.ID },
	)
	if err != nil {
		return InitScriptListResult{}, err
	}
	return InitScriptListResult{InitScripts: scripts, NextCursor: nextCursor}, nil
}
