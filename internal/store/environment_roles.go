package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Role storage mirrors agents: a row per grant, with the OpenFGA tuple written
// alongside it by the service.
func (s *Store) UpsertEnvironmentRole(ctx context.Context, assignment EnvironmentRoleAssignment) (*EnvironmentRole, error) {
	var previous EnvironmentRole
	err := s.pool.QueryRow(ctx,
		`SELECT role FROM environment_roles WHERE environment_id = $1 AND identity_id = $2`,
		assignment.EnvironmentID, assignment.IdentityID,
	).Scan(&previous)
	hadPrevious := true
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		hadPrevious = false
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO environment_roles (environment_id, identity_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (environment_id, identity_id)
		 DO UPDATE SET role = EXCLUDED.role, updated_at = NOW()`,
		assignment.EnvironmentID, assignment.IdentityID, assignment.Role,
	); err != nil {
		return nil, err
	}
	if !hadPrevious {
		return nil, nil
	}
	return &previous, nil
}

func (s *Store) DeleteEnvironmentRole(ctx context.Context, environmentID, identityID uuid.UUID) (EnvironmentRole, error) {
	var role EnvironmentRole
	err := s.pool.QueryRow(ctx,
		`DELETE FROM environment_roles WHERE environment_id = $1 AND identity_id = $2 RETURNING role`,
		environmentID, identityID,
	).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", NotFound("environment role")
		}
		return "", err
	}
	return role, nil
}

func (s *Store) ListEnvironmentRoles(ctx context.Context, environmentID uuid.UUID) ([]EnvironmentRoleAssignment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT environment_id, identity_id, role FROM environment_roles WHERE environment_id = $1 ORDER BY identity_id ASC`,
		environmentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var assignments []EnvironmentRoleAssignment
	for rows.Next() {
		var assignment EnvironmentRoleAssignment
		if err := rows.Scan(&assignment.EnvironmentID, &assignment.IdentityID, &assignment.Role); err != nil {
			return nil, err
		}
		assignments = append(assignments, assignment)
	}
	return assignments, rows.Err()
}
