package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func scanAgent(row pgx.Row) (Agent, error) {
	var agent Agent
	var config []byte
	if err := row.Scan(
		&agent.Meta.ID,
		&agent.Title,
		&agent.Description,
		&config,
		&agent.Meta.CreatedAt,
		&agent.Meta.UpdatedAt,
	); err != nil {
		return Agent{}, err
	}
	agent.Config = JSONData(config)
	return agent, nil
}

func scanTool(row pgx.Row) (Tool, error) {
	var tool Tool
	var config []byte
	if err := row.Scan(
		&tool.Meta.ID,
		&tool.Type,
		&tool.Name,
		&tool.Description,
		&config,
		&tool.Meta.CreatedAt,
		&tool.Meta.UpdatedAt,
	); err != nil {
		return Tool{}, err
	}
	tool.Config = JSONData(config)
	return tool, nil
}

func scanMcpServer(row pgx.Row) (McpServer, error) {
	var server McpServer
	var config []byte
	if err := row.Scan(
		&server.Meta.ID,
		&server.Title,
		&server.Description,
		&config,
		&server.Meta.CreatedAt,
		&server.Meta.UpdatedAt,
	); err != nil {
		return McpServer{}, err
	}
	server.Config = JSONData(config)
	return server, nil
}

func scanWorkspaceConfiguration(row pgx.Row) (WorkspaceConfiguration, error) {
	var workspace WorkspaceConfiguration
	var config []byte
	if err := row.Scan(
		&workspace.Meta.ID,
		&workspace.Title,
		&workspace.Description,
		&config,
		&workspace.Meta.CreatedAt,
		&workspace.Meta.UpdatedAt,
	); err != nil {
		return WorkspaceConfiguration{}, err
	}
	workspace.Config = JSONData(config)
	return workspace, nil
}

func scanMemoryBucket(row pgx.Row) (MemoryBucket, error) {
	var bucket MemoryBucket
	var config []byte
	if err := row.Scan(
		&bucket.Meta.ID,
		&bucket.Title,
		&bucket.Description,
		&config,
		&bucket.Meta.CreatedAt,
		&bucket.Meta.UpdatedAt,
	); err != nil {
		return MemoryBucket{}, err
	}
	bucket.Config = JSONData(config)
	return bucket, nil
}

func scanVariable(row pgx.Row) (Variable, error) {
	var variable Variable
	if err := row.Scan(
		&variable.Meta.ID,
		&variable.Key,
		&variable.Value,
		&variable.Description,
		&variable.Meta.CreatedAt,
		&variable.Meta.UpdatedAt,
	); err != nil {
		return Variable{}, err
	}
	return variable, nil
}

func scanAttachment(row pgx.Row) (Attachment, error) {
	var attachment Attachment
	if err := row.Scan(
		&attachment.Meta.ID,
		&attachment.Kind,
		&attachment.SourceType,
		&attachment.SourceID,
		&attachment.TargetType,
		&attachment.TargetID,
		&attachment.Meta.CreatedAt,
		&attachment.Meta.UpdatedAt,
	); err != nil {
		return Attachment{}, err
	}
	return attachment, nil
}

func (s *Store) CreateAgent(ctx context.Context, input AgentInput) (Agent, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO agents (title, description, config)
         VALUES ($1, $2, $3)
         RETURNING id, title, description, config, created_at, updated_at`,
		input.Title,
		input.Description,
		input.Config,
	)
	agent, err := scanAgent(row)
	if err != nil {
		return Agent{}, err
	}
	return agent, nil
}

func (s *Store) GetAgent(ctx context.Context, id uuid.UUID) (Agent, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, title, description, config, created_at, updated_at
         FROM agents
         WHERE id = $1`,
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
	builder := updateBuilder{}
	if update.Title != nil {
		builder.add("title", *update.Title)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}
	if update.Config != nil {
		builder.add("config", *update.Config)
	}

	if builder.empty() {
		return Agent{}, fmt.Errorf("agent update requires at least one field")
	}
	query, args := builder.build("agents", "id, title, description, config, created_at, updated_at", id)
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

func (s *Store) DeleteAgent(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("agent")
	}
	return nil
}

func (s *Store) ListAgents(ctx context.Context, filter AgentFilter, pageSize int32, cursor *PageCursor) (AgentListResult, error) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if filter.Query != "" {
		placeholder := len(args) + 1
		clauses = append(clauses, fmt.Sprintf("(title ILIKE $%d OR description ILIKE $%d)", placeholder, placeholder))
		args = append(args, "%"+filter.Query+"%")
	}

	agents, nextCursor, err := listEntities(ctx, s.pool,
		`SELECT id, title, description, config, created_at, updated_at FROM agents`,
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

func (s *Store) CreateTool(ctx context.Context, input ToolInput) (Tool, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO tools (type, name, description, config)
         VALUES ($1, $2, $3, $4)
         RETURNING id, type, name, description, config, created_at, updated_at`,
		input.Type,
		input.Name,
		input.Description,
		input.Config,
	)
	tool, err := scanTool(row)
	if err != nil {
		return Tool{}, err
	}
	return tool, nil
}

func (s *Store) GetTool(ctx context.Context, id uuid.UUID) (Tool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, type, name, description, config, created_at, updated_at
         FROM tools
         WHERE id = $1`,
		id,
	)
	tool, err := scanTool(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Tool{}, NotFound("tool")
		}
		return Tool{}, err
	}
	return tool, nil
}

func (s *Store) UpdateTool(ctx context.Context, id uuid.UUID, update ToolUpdate) (Tool, error) {
	builder := updateBuilder{}
	if update.Name != nil {
		builder.add("name", *update.Name)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}
	if update.Config != nil {
		builder.add("config", *update.Config)
	}

	if builder.empty() {
		return Tool{}, fmt.Errorf("tool update requires at least one field")
	}
	query, args := builder.build("tools", "id, type, name, description, config, created_at, updated_at", id)
	row := s.pool.QueryRow(ctx, query, args...)
	tool, err := scanTool(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Tool{}, NotFound("tool")
		}
		return Tool{}, err
	}
	return tool, nil
}

func (s *Store) DeleteTool(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM tools WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("tool")
	}
	return nil
}

func (s *Store) ListTools(ctx context.Context, filter ToolFilter, pageSize int32, cursor *PageCursor) (ToolListResult, error) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if filter.Type != nil {
		clauses, args = appendClause(clauses, args, "type = $%d", *filter.Type)
	}

	tools, nextCursor, err := listEntities(ctx, s.pool,
		`SELECT id, type, name, description, config, created_at, updated_at FROM tools`,
		clauses,
		args,
		cursor,
		pageSize,
		scanTool,
		func(tool Tool) uuid.UUID { return tool.Meta.ID },
	)
	if err != nil {
		return ToolListResult{}, err
	}
	return ToolListResult{Tools: tools, NextCursor: nextCursor}, nil
}

func (s *Store) CreateMcpServer(ctx context.Context, input McpServerInput) (McpServer, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO mcp_servers (title, description, config)
         VALUES ($1, $2, $3)
         RETURNING id, title, description, config, created_at, updated_at`,
		input.Title,
		input.Description,
		input.Config,
	)
	server, err := scanMcpServer(row)
	if err != nil {
		return McpServer{}, err
	}
	return server, nil
}

func (s *Store) GetMcpServer(ctx context.Context, id uuid.UUID) (McpServer, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, title, description, config, created_at, updated_at
         FROM mcp_servers
         WHERE id = $1`,
		id,
	)
	server, err := scanMcpServer(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return McpServer{}, NotFound("mcp server")
		}
		return McpServer{}, err
	}
	return server, nil
}

func (s *Store) UpdateMcpServer(ctx context.Context, id uuid.UUID, update McpServerUpdate) (McpServer, error) {
	builder := updateBuilder{}
	if update.Title != nil {
		builder.add("title", *update.Title)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}
	if update.Config != nil {
		builder.add("config", *update.Config)
	}

	if builder.empty() {
		return McpServer{}, fmt.Errorf("mcp server update requires at least one field")
	}
	query, args := builder.build("mcp_servers", "id, title, description, config, created_at, updated_at", id)
	row := s.pool.QueryRow(ctx, query, args...)
	server, err := scanMcpServer(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return McpServer{}, NotFound("mcp server")
		}
		return McpServer{}, err
	}
	return server, nil
}

func (s *Store) DeleteMcpServer(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("mcp server")
	}
	return nil
}

func (s *Store) ListMcpServers(ctx context.Context, pageSize int32, cursor *PageCursor) (McpServerListResult, error) {
	servers, nextCursor, err := listEntities(ctx, s.pool,
		`SELECT id, title, description, config, created_at, updated_at FROM mcp_servers`,
		nil,
		nil,
		cursor,
		pageSize,
		scanMcpServer,
		func(server McpServer) uuid.UUID { return server.Meta.ID },
	)
	if err != nil {
		return McpServerListResult{}, err
	}
	return McpServerListResult{McpServers: servers, NextCursor: nextCursor}, nil
}

func (s *Store) CreateWorkspaceConfiguration(ctx context.Context, input WorkspaceConfigurationInput) (WorkspaceConfiguration, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO workspace_configurations (title, description, config)
         VALUES ($1, $2, $3)
         RETURNING id, title, description, config, created_at, updated_at`,
		input.Title,
		input.Description,
		input.Config,
	)
	workspace, err := scanWorkspaceConfiguration(row)
	if err != nil {
		return WorkspaceConfiguration{}, err
	}
	return workspace, nil
}

func (s *Store) GetWorkspaceConfiguration(ctx context.Context, id uuid.UUID) (WorkspaceConfiguration, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, title, description, config, created_at, updated_at
         FROM workspace_configurations
         WHERE id = $1`,
		id,
	)
	workspace, err := scanWorkspaceConfiguration(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkspaceConfiguration{}, NotFound("workspace configuration")
		}
		return WorkspaceConfiguration{}, err
	}
	return workspace, nil
}

func (s *Store) UpdateWorkspaceConfiguration(ctx context.Context, id uuid.UUID, update WorkspaceConfigurationUpdate) (WorkspaceConfiguration, error) {
	builder := updateBuilder{}
	if update.Title != nil {
		builder.add("title", *update.Title)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}
	if update.Config != nil {
		builder.add("config", *update.Config)
	}

	if builder.empty() {
		return WorkspaceConfiguration{}, fmt.Errorf("workspace configuration update requires at least one field")
	}
	query, args := builder.build("workspace_configurations", "id, title, description, config, created_at, updated_at", id)
	row := s.pool.QueryRow(ctx, query, args...)
	workspace, err := scanWorkspaceConfiguration(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkspaceConfiguration{}, NotFound("workspace configuration")
		}
		return WorkspaceConfiguration{}, err
	}
	return workspace, nil
}

func (s *Store) DeleteWorkspaceConfiguration(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM workspace_configurations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("workspace configuration")
	}
	return nil
}

func (s *Store) ListWorkspaceConfigurations(ctx context.Context, pageSize int32, cursor *PageCursor) (WorkspaceConfigurationListResult, error) {
	workspaces, nextCursor, err := listEntities(ctx, s.pool,
		`SELECT id, title, description, config, created_at, updated_at FROM workspace_configurations`,
		nil,
		nil,
		cursor,
		pageSize,
		scanWorkspaceConfiguration,
		func(workspace WorkspaceConfiguration) uuid.UUID { return workspace.Meta.ID },
	)
	if err != nil {
		return WorkspaceConfigurationListResult{}, err
	}
	return WorkspaceConfigurationListResult{WorkspaceConfigurations: workspaces, NextCursor: nextCursor}, nil
}

func (s *Store) CreateMemoryBucket(ctx context.Context, input MemoryBucketInput) (MemoryBucket, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO memory_buckets (title, description, config)
         VALUES ($1, $2, $3)
         RETURNING id, title, description, config, created_at, updated_at`,
		input.Title,
		input.Description,
		input.Config,
	)
	bucket, err := scanMemoryBucket(row)
	if err != nil {
		return MemoryBucket{}, err
	}
	return bucket, nil
}

func (s *Store) GetMemoryBucket(ctx context.Context, id uuid.UUID) (MemoryBucket, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, title, description, config, created_at, updated_at
         FROM memory_buckets
         WHERE id = $1`,
		id,
	)
	bucket, err := scanMemoryBucket(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MemoryBucket{}, NotFound("memory bucket")
		}
		return MemoryBucket{}, err
	}
	return bucket, nil
}

func (s *Store) UpdateMemoryBucket(ctx context.Context, id uuid.UUID, update MemoryBucketUpdate) (MemoryBucket, error) {
	builder := updateBuilder{}
	if update.Title != nil {
		builder.add("title", *update.Title)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}
	if update.Config != nil {
		builder.add("config", *update.Config)
	}

	if builder.empty() {
		return MemoryBucket{}, fmt.Errorf("memory bucket update requires at least one field")
	}
	query, args := builder.build("memory_buckets", "id, title, description, config, created_at, updated_at", id)
	row := s.pool.QueryRow(ctx, query, args...)
	bucket, err := scanMemoryBucket(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MemoryBucket{}, NotFound("memory bucket")
		}
		return MemoryBucket{}, err
	}
	return bucket, nil
}

func (s *Store) DeleteMemoryBucket(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM memory_buckets WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("memory bucket")
	}
	return nil
}

func (s *Store) ListMemoryBuckets(ctx context.Context, pageSize int32, cursor *PageCursor) (MemoryBucketListResult, error) {
	buckets, nextCursor, err := listEntities(ctx, s.pool,
		`SELECT id, title, description, config, created_at, updated_at FROM memory_buckets`,
		nil,
		nil,
		cursor,
		pageSize,
		scanMemoryBucket,
		func(bucket MemoryBucket) uuid.UUID { return bucket.Meta.ID },
	)
	if err != nil {
		return MemoryBucketListResult{}, err
	}
	return MemoryBucketListResult{MemoryBuckets: buckets, NextCursor: nextCursor}, nil
}

func (s *Store) CreateVariable(ctx context.Context, input VariableInput) (Variable, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO variables (key, value, description)
         VALUES ($1, $2, $3)
         RETURNING id, key, value, description, created_at, updated_at`,
		input.Key,
		input.Value,
		input.Description,
	)
	variable, err := scanVariable(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Variable{}, AlreadyExists("variable")
		}
		return Variable{}, err
	}
	return variable, nil
}

func (s *Store) GetVariable(ctx context.Context, id uuid.UUID) (Variable, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, key, value, description, created_at, updated_at
         FROM variables
         WHERE id = $1`,
		id,
	)
	variable, err := scanVariable(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Variable{}, NotFound("variable")
		}
		return Variable{}, err
	}
	return variable, nil
}

func (s *Store) UpdateVariable(ctx context.Context, id uuid.UUID, update VariableUpdate) (Variable, error) {
	builder := updateBuilder{}
	if update.Key != nil {
		builder.add("key", *update.Key)
	}
	if update.Value != nil {
		builder.add("value", *update.Value)
	}
	if update.Description != nil {
		builder.add("description", *update.Description)
	}

	if builder.empty() {
		return Variable{}, fmt.Errorf("variable update requires at least one field")
	}
	query, args := builder.build("variables", "id, key, value, description, created_at, updated_at", id)
	row := s.pool.QueryRow(ctx, query, args...)
	variable, err := scanVariable(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Variable{}, AlreadyExists("variable")
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return Variable{}, NotFound("variable")
		}
		return Variable{}, err
	}
	return variable, nil
}

func (s *Store) DeleteVariable(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM variables WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("variable")
	}
	return nil
}

func (s *Store) ListVariables(ctx context.Context, filter VariableFilter, pageSize int32, cursor *PageCursor) (VariableListResult, error) {
	clauses := make([]string, 0, 1)
	args := make([]any, 0, 1)
	if filter.Query != "" {
		placeholder := len(args) + 1
		clauses = append(clauses, fmt.Sprintf("key ILIKE $%d", placeholder))
		args = append(args, "%"+filter.Query+"%")
	}

	variables, nextCursor, err := listEntities(ctx, s.pool,
		`SELECT id, key, value, description, created_at, updated_at FROM variables`,
		clauses,
		args,
		cursor,
		pageSize,
		scanVariable,
		func(variable Variable) uuid.UUID { return variable.Meta.ID },
	)
	if err != nil {
		return VariableListResult{}, err
	}
	return VariableListResult{Variables: variables, NextCursor: nextCursor}, nil
}

func (s *Store) ResolveVariableByKey(ctx context.Context, key string) (string, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT value FROM variables WHERE key = $1`, key)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) CreateAttachment(ctx context.Context, input AttachmentInput) (Attachment, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO attachments (kind, source_type, source_id, target_type, target_id)
         VALUES ($1, $2, $3, $4, $5)
         RETURNING id, kind, source_type, source_id, target_type, target_id, created_at, updated_at`,
		input.Kind,
		input.SourceType,
		input.SourceID,
		input.TargetType,
		input.TargetID,
	)
	attachment, err := scanAttachment(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Attachment{}, AlreadyExists("attachment")
		}
		return Attachment{}, err
	}
	return attachment, nil
}

func (s *Store) GetAttachment(ctx context.Context, id uuid.UUID) (Attachment, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, kind, source_type, source_id, target_type, target_id, created_at, updated_at
         FROM attachments
         WHERE id = $1`,
		id,
	)
	attachment, err := scanAttachment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Attachment{}, NotFound("attachment")
		}
		return Attachment{}, err
	}
	return attachment, nil
}

func (s *Store) DeleteAttachment(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM attachments WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("attachment")
	}
	return nil
}

func (s *Store) ListAttachments(ctx context.Context, filter AttachmentFilter, pageSize int32, cursor *PageCursor) (AttachmentListResult, error) {
	clauses := make([]string, 0, 5)
	args := make([]any, 0, 5)

	if filter.Kind != nil {
		clauses, args = appendClause(clauses, args, "kind = $%d", *filter.Kind)
	}
	if filter.SourceType != nil {
		clauses, args = appendClause(clauses, args, "source_type = $%d", *filter.SourceType)
	}
	if filter.SourceID != nil {
		clauses, args = appendClause(clauses, args, "source_id = $%d", *filter.SourceID)
	}
	if filter.TargetType != nil {
		clauses, args = appendClause(clauses, args, "target_type = $%d", *filter.TargetType)
	}
	if filter.TargetID != nil {
		clauses, args = appendClause(clauses, args, "target_id = $%d", *filter.TargetID)
	}

	attachments, nextCursor, err := listEntities(ctx, s.pool,
		`SELECT id, kind, source_type, source_id, target_type, target_id, created_at, updated_at FROM attachments`,
		clauses,
		args,
		cursor,
		pageSize,
		scanAttachment,
		func(attachment Attachment) uuid.UUID { return attachment.Meta.ID },
	)
	if err != nil {
		return AttachmentListResult{}, err
	}
	return AttachmentListResult{Attachments: attachments, NextCursor: nextCursor}, nil
}
