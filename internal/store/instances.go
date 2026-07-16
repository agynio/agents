package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	agentInstanceColumns = `ai.id, ai.agent_id, ai.organization_id, ai.label, ai.suffix, ai.state, ai.pause_reason, ai.last_activity_at, ai.created_at, ai.updated_at, ai.nickname`
	inboxItemColumns     = `id, agent_instance_id, source_kind, thread_id, message_id, sender_id, body, file_ids, accepted_at, acked_at`
)

func timePtrFromPg(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func uuidStrings(ids []uuid.UUID) []string {
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = id.String()
	}
	return values
}

func parseUUIDStrings(values []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, len(values))
	for i, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, fmt.Errorf("parse file id: %w", err)
		}
		ids[i] = id
	}
	return ids, nil
}

func scanAgentInstance(row pgx.Row) (AgentInstance, error) {
	var instance AgentInstance
	var label pgtype.Text
	var pauseReason pgtype.Text
	if err := row.Scan(
		&instance.Meta.ID,
		&instance.AgentID,
		&instance.OrganizationID,
		&label,
		&instance.Suffix,
		&instance.State,
		&pauseReason,
		&instance.LastActivityAt,
		&instance.Meta.CreatedAt,
		&instance.Meta.UpdatedAt,
		&instance.Nickname,
	); err != nil {
		return AgentInstance{}, err
	}
	instance.Label = stringPtrFromPg(label)
	instance.PauseReason = stringPtrFromPg(pauseReason)
	return instance, nil
}

func scanInboxItem(row pgx.Row) (InboxItem, error) {
	var item InboxItem
	var threadID pgtype.UUID
	var messageID pgtype.UUID
	var fileIDs []string
	var ackedAt pgtype.Timestamptz
	if err := row.Scan(
		&item.ID,
		&item.AgentInstanceID,
		&item.SourceKind,
		&threadID,
		&messageID,
		&item.SenderID,
		&item.Body,
		&fileIDs,
		&item.AcceptedAt,
		&ackedAt,
	); err != nil {
		return InboxItem{}, err
	}
	parsedFileIDs, err := parseUUIDStrings(fileIDs)
	if err != nil {
		return InboxItem{}, err
	}
	item.ThreadID = uuidPtrFromPg(threadID)
	item.MessageID = uuidPtrFromPg(messageID)
	item.FileIDs = parsedFileIDs
	item.AckedAt = timePtrFromPg(ackedAt)
	return item, nil
}

func (s *Store) CreateAgentInstance(ctx context.Context, input AgentInstanceInput) (AgentInstance, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx,
		`INSERT INTO agent_instances (agent_id, organization_id, label, suffix, nickname)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id`,
		input.AgentID,
		input.OrganizationID,
		input.Label,
		input.Suffix,
		input.Nickname,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return AgentInstance{}, AlreadyExists("agent instance")
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return AgentInstance{}, ForeignKeyViolation("agent instance")
		}
		return AgentInstance{}, err
	}
	return s.GetAgentInstance(ctx, id)
}

func (s *Store) GetAgentInstance(ctx context.Context, id uuid.UUID) (AgentInstance, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM agent_instances ai WHERE ai.id = $1`, agentInstanceColumns),
		id,
	)
	instance, err := scanAgentInstance(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AgentInstance{}, NotFound("agent instance")
		}
		return AgentInstance{}, err
	}
	return instance, nil
}

func (s *Store) PauseAgentInstance(ctx context.Context, id uuid.UUID, reason string) (AgentInstance, error) {
	var updatedID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`UPDATE agent_instances SET state = $1, pause_reason = $2, updated_at = NOW()
			WHERE id = $3 AND state <> $4 RETURNING id`,
		AgentInstanceStatePaused,
		reason,
		id,
		AgentInstanceStateTerminated,
	).Scan(&updatedID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AgentInstance{}, NotFound("agent instance")
		}
		return AgentInstance{}, err
	}
	return s.GetAgentInstance(ctx, updatedID)
}

func (s *Store) ResumeAgentInstance(ctx context.Context, id uuid.UUID) (AgentInstance, error) {
	var updatedID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`UPDATE agent_instances SET state = $1, pause_reason = NULL, updated_at = NOW()
			WHERE id = $2 AND state <> $3 RETURNING id`,
		AgentInstanceStateActive,
		id,
		AgentInstanceStateTerminated,
	).Scan(&updatedID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AgentInstance{}, NotFound("agent instance")
		}
		return AgentInstance{}, err
	}
	return s.GetAgentInstance(ctx, updatedID)
}

func (s *Store) DeleteAgentInstance(ctx context.Context, id uuid.UUID) (AgentInstance, error) {
	var updatedID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`UPDATE agent_instances SET state = $1, pause_reason = NULL, updated_at = NOW()
			WHERE id = $2 AND state <> $1 RETURNING id`,
		AgentInstanceStateTerminated,
		id,
	).Scan(&updatedID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AgentInstance{}, NotFound("agent instance")
		}
		return AgentInstance{}, err
	}
	return s.GetAgentInstance(ctx, updatedID)
}

func (s *Store) ListAgentInstances(ctx context.Context, filter AgentInstanceFilter, pageSize int32, cursor *PageCursor) (AgentInstanceListResult, error) {
	var clauses []string
	var args []any
	if filter.OrganizationID != nil {
		clauses, args = appendClause(clauses, args, "ai.organization_id = $%d", *filter.OrganizationID)
	}
	if filter.AgentID != nil {
		clauses, args = appendClause(clauses, args, "ai.agent_id = $%d", *filter.AgentID)
	}
	if len(filter.StateIn) > 0 {
		clauses, args = appendClause(clauses, args, "ai.state = ANY($%d)", agentInstanceStateStrings(filter.StateIn))
	}
	if filter.HasUnacked != nil {
		condition := `EXISTS (SELECT 1 FROM inbox_items ii WHERE ii.agent_instance_id = ai.id AND ii.acked_at IS NULL)`
		if !*filter.HasUnacked {
			condition = "NOT " + condition
		}
		clauses = append(clauses, condition)
	}
	if cursor != nil {
		clauses, args = appendClause(clauses, args, "ai.id > $%d", cursor.AfterID)
	}
	limit := NormalizePageSize(pageSize)
	args = append(args, int(limit)+1)
	query := strings.Builder{}
	query.WriteString(fmt.Sprintf("SELECT %s FROM agent_instances ai", agentInstanceColumns))
	if len(clauses) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(clauses, " AND "))
	}
	query.WriteString(fmt.Sprintf(" ORDER BY ai.id ASC LIMIT $%d", len(args)))

	rows, err := s.pool.Query(ctx, query.String(), args...)
	if err != nil {
		return AgentInstanceListResult{}, err
	}
	defer rows.Close()

	instances := make([]AgentInstance, 0, limit)
	var lastID uuid.UUID
	var hasMore bool
	for rows.Next() {
		if int32(len(instances)) == limit {
			hasMore = true
			break
		}
		instance, err := scanAgentInstance(rows)
		if err != nil {
			return AgentInstanceListResult{}, err
		}
		instances = append(instances, instance)
		lastID = instance.Meta.ID
	}
	if err := rows.Err(); err != nil {
		return AgentInstanceListResult{}, err
	}
	var nextCursor *PageCursor
	if hasMore {
		nextCursor = &PageCursor{AfterID: lastID}
	}
	return AgentInstanceListResult{Instances: instances, NextCursor: nextCursor}, nil
}

func agentInstanceStateStrings(states []AgentInstanceState) []string {
	values := make([]string, len(states))
	for i, state := range states {
		values[i] = string(state)
	}
	return values
}

func (s *Store) HasNonTerminatedAgentInstances(ctx context.Context, agentID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM agent_instances WHERE agent_id = $1 AND state <> $2)`,
		agentID,
		AgentInstanceStateTerminated,
	).Scan(&exists)
	return exists, err
}

func (s *Store) CreateInboxItem(ctx context.Context, input InboxItemInput) (InboxItem, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO inbox_items (agent_instance_id, source_kind, thread_id, message_id, sender_id, body, file_ids)
			SELECT $1, $2, $3, $4, $5, $6, $7
			WHERE EXISTS (SELECT 1 FROM agent_instances WHERE id = $1 AND state <> $8)
			RETURNING %s`, inboxItemColumns),
		input.AgentInstanceID,
		input.SourceKind,
		input.ThreadID,
		input.MessageID,
		input.SenderID,
		input.Body,
		uuidStrings(input.FileIDs),
		AgentInstanceStateTerminated,
	)
	item, err := scanInboxItem(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return InboxItem{}, NotFound("agent instance")
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return InboxItem{}, ForeignKeyViolation("inbox item")
		}
		return InboxItem{}, err
	}
	if err := s.touchInstanceActivity(ctx, input.AgentInstanceID); err != nil {
		return InboxItem{}, err
	}
	return item, nil
}

func (s *Store) FanoutInboxItem(ctx context.Context, input InboxItemInput) (InboxItem, bool, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO inbox_items (agent_instance_id, source_kind, thread_id, message_id, sender_id, body, file_ids)
			SELECT $1, $2, $3, $4, $5, $6, $7
			WHERE EXISTS (SELECT 1 FROM agent_instances WHERE id = $1 AND state <> $8)
			ON CONFLICT (agent_instance_id, thread_id, message_id) WHERE source_kind = 'thread'
			DO UPDATE SET updated_at = inbox_items.updated_at
			RETURNING %s, (xmax = 0) AS inserted`, inboxItemColumns),
		input.AgentInstanceID,
		input.SourceKind,
		input.ThreadID,
		input.MessageID,
		input.SenderID,
		input.Body,
		uuidStrings(input.FileIDs),
		AgentInstanceStateTerminated,
	)
	var inserted bool
	item, err := scanInboxItemWithInserted(row, &inserted)
	if err != nil {
		if err == pgx.ErrNoRows {
			return InboxItem{}, false, NotFound("agent instance")
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return InboxItem{}, false, ForeignKeyViolation("inbox item")
		}
		return InboxItem{}, false, err
	}
	if inserted {
		if err := s.touchInstanceActivity(ctx, input.AgentInstanceID); err != nil {
			return InboxItem{}, false, err
		}
	}
	return item, inserted, nil
}

func scanInboxItemWithInserted(row pgx.Row, inserted *bool) (InboxItem, error) {
	var item InboxItem
	var threadID pgtype.UUID
	var messageID pgtype.UUID
	var fileIDs []string
	var ackedAt pgtype.Timestamptz
	if err := row.Scan(
		&item.ID,
		&item.AgentInstanceID,
		&item.SourceKind,
		&threadID,
		&messageID,
		&item.SenderID,
		&item.Body,
		&fileIDs,
		&item.AcceptedAt,
		&ackedAt,
		inserted,
	); err != nil {
		return InboxItem{}, err
	}
	parsedFileIDs, err := parseUUIDStrings(fileIDs)
	if err != nil {
		return InboxItem{}, err
	}
	item.ThreadID = uuidPtrFromPg(threadID)
	item.MessageID = uuidPtrFromPg(messageID)
	item.FileIDs = parsedFileIDs
	item.AckedAt = timePtrFromPg(ackedAt)
	return item, nil
}

func (s *Store) ListUnackedInboxItems(ctx context.Context, agentInstanceID uuid.UUID, pageSize int32, cursor *InboxPageCursor) (InboxItemListResult, error) {
	limit := NormalizePageSize(pageSize)
	args := []any{agentInstanceID}
	clauses := []string{"agent_instance_id = $1", "acked_at IS NULL"}
	if cursor != nil {
		clauses = append(clauses, fmt.Sprintf("(accepted_at, id) > ($%d, $%d)", len(args)+1, len(args)+2))
		args = append(args, cursor.AfterAcceptedAt, cursor.AfterID)
	}
	args = append(args, int(limit)+1)
	query := fmt.Sprintf(
		"SELECT %s FROM inbox_items WHERE %s ORDER BY accepted_at ASC, id ASC LIMIT $%d",
		inboxItemColumns,
		strings.Join(clauses, " AND "),
		len(args),
	)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return InboxItemListResult{}, err
	}
	defer rows.Close()

	items := make([]InboxItem, 0, limit)
	var lastAcceptedAt time.Time
	var lastID uuid.UUID
	var hasMore bool
	for rows.Next() {
		if int32(len(items)) == limit {
			hasMore = true
			break
		}
		item, err := scanInboxItem(rows)
		if err != nil {
			return InboxItemListResult{}, err
		}
		items = append(items, item)
		lastAcceptedAt = item.AcceptedAt
		lastID = item.ID
	}
	if err := rows.Err(); err != nil {
		return InboxItemListResult{}, err
	}
	var nextCursor *InboxPageCursor
	if hasMore {
		nextCursor = &InboxPageCursor{AfterAcceptedAt: lastAcceptedAt, AfterID: lastID}
	}
	return InboxItemListResult{Items: items, NextCursor: nextCursor}, nil
}

func (s *Store) AckInboxItems(ctx context.Context, agentInstanceID uuid.UUID, itemIDs []uuid.UUID) (int32, error) {
	if len(itemIDs) == 0 {
		return 0, nil
	}
	result, err := s.pool.Exec(ctx,
		`UPDATE inbox_items SET acked_at = NOW(), updated_at = NOW()
		 WHERE agent_instance_id = $1 AND id = ANY($2) AND acked_at IS NULL`,
		agentInstanceID,
		uuidStrings(itemIDs),
	)
	if err != nil {
		return 0, err
	}
	return int32(result.RowsAffected()), nil
}

func (s *Store) CountUnackedInboxItems(ctx context.Context, agentInstanceID uuid.UUID, threadID *uuid.UUID) (int32, error) {
	clauses := []string{"agent_instance_id = $1", "acked_at IS NULL"}
	args := []any{agentInstanceID}
	if threadID != nil {
		clauses = append(clauses, fmt.Sprintf("thread_id = $%d", len(args)+1))
		args = append(args, *threadID)
	}
	var count int32
	err := s.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*)::int FROM inbox_items WHERE %s", strings.Join(clauses, " AND ")),
		args...,
	).Scan(&count)
	return count, err
}

func (s *Store) touchInstanceActivity(ctx context.Context, agentInstanceID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE agent_instances SET last_activity_at = NOW(), updated_at = NOW() WHERE id = $1 AND state <> $2`,
		agentInstanceID,
		AgentInstanceStateTerminated,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("agent instance")
	}
	return nil
}
