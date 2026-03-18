package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type updateBuilder struct {
	setClauses []string
	args       []any
}

func (b *updateBuilder) add(field string, value any) {
	placeholder := len(b.args) + 1
	b.setClauses = append(b.setClauses, fmt.Sprintf("%s = $%d", field, placeholder))
	b.args = append(b.args, value)
}

func (b *updateBuilder) addNull(field string) {
	b.setClauses = append(b.setClauses, fmt.Sprintf("%s = NULL", field))
}

func (b *updateBuilder) empty() bool {
	return len(b.setClauses) == 0
}

func (b *updateBuilder) build(table string, returning string, id uuid.UUID) (string, []any) {
	return buildUpdateQuery(table, returning, b.setClauses, b.args, id)
}

func buildUpdateQuery(table string, returning string, setClauses []string, args []any, id uuid.UUID) (string, []any) {
	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, id)
	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = $%d RETURNING %s",
		table,
		strings.Join(setClauses, ", "),
		len(args),
		returning,
	)
	return query, args
}

func appendClause(clauses []string, args []any, format string, value any) ([]string, []any) {
	placeholder := len(args) + 1
	clauses = append(clauses, fmt.Sprintf(format, placeholder))
	args = append(args, value)
	return clauses, args
}

func listEntities[T any](
	ctx context.Context,
	pool *pgxpool.Pool,
	baseQuery string,
	clauses []string,
	args []any,
	cursor *PageCursor,
	pageSize int32,
	scan func(pgx.Row) (T, error),
	idFunc func(T) uuid.UUID,
) ([]T, *PageCursor, error) {
	limit := NormalizePageSize(pageSize)

	query := strings.Builder{}
	query.WriteString(baseQuery)

	paramIndex := len(args) + 1
	if cursor != nil {
		clauses = append(clauses, fmt.Sprintf("id > $%d", paramIndex))
		args = append(args, cursor.AfterID)
		paramIndex++
	}

	if len(clauses) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(clauses, " AND "))
	}
	query.WriteString(fmt.Sprintf(" ORDER BY id ASC LIMIT $%d", paramIndex))
	args = append(args, int(limit)+1)

	rows, err := pool.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items := make([]T, 0, limit)
	var (
		nextCursor *PageCursor
		lastID     uuid.UUID
		hasMore    bool
	)
	for rows.Next() {
		if int32(len(items)) == limit {
			hasMore = true
			break
		}
		item, err := scan(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, item)
		lastID = idFunc(item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if hasMore {
		nextCursor = &PageCursor{AfterID: lastID}
	}
	return items, nextCursor, nil
}
