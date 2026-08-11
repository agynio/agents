package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SandboxLayout is one identity's set of open shells in one sandbox.
//
// The store never looks in the container and never asks whether a named shell
// exists. Attaching creates one that does not, so a layout naming a shell the
// container lost is not an inconsistency to reconcile -- it is the ordinary
// case after a restart.
type SandboxLayout struct {
	SandboxID  uuid.UUID
	IdentityID uuid.UUID
	Version    int64
	Tabs       []SandboxTab
}

type SandboxTab struct {
	ShellID        string     `json:"shell_id"`
	Number         int32      `json:"number"`
	NameOverride   *string    `json:"name_override,omitempty"`
	CWD            *string    `json:"cwd,omitempty"`
	LastAttachedAt *time.Time `json:"last_attached_at,omitempty"`
}

// ErrLayoutVersionConflict reports that the layout moved under a writer. The
// caller refetches and reapplies; it is not an error state, it is the other
// device having got there first.
var ErrLayoutVersionConflict = errors.New("sandbox layout version conflict")

// GetSandboxLayout returns an empty layout at version 0 for a sandbox this
// identity has never worked in, rather than NotFound. No caller needs to tell
// "never opened" from "opened and emptied" apart, and one of those answers
// costs every client an error path.
func (s *Store) GetSandboxLayout(ctx context.Context, sandboxID, identityID uuid.UUID) (SandboxLayout, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT version, tabs FROM sandbox_layouts WHERE sandbox_id = $1 AND identity_id = $2`,
		sandboxID, identityID,
	)

	layout := SandboxLayout{SandboxID: sandboxID, IdentityID: identityID, Tabs: []SandboxTab{}}
	var raw []byte
	if err := row.Scan(&layout.Version, &raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return layout, nil
		}
		return SandboxLayout{}, err
	}
	if err := json.Unmarshal(raw, &layout.Tabs); err != nil {
		return SandboxLayout{}, err
	}
	if layout.Tabs == nil {
		layout.Tabs = []SandboxTab{}
	}
	return layout, nil
}

// SetSandboxLayout replaces the document, guarded by the version the caller
// read. Version 0 means "I found nothing", so it inserts.
func (s *Store) SetSandboxLayout(ctx context.Context, sandboxID, identityID uuid.UUID, version int64, tabs []SandboxTab) (SandboxLayout, error) {
	if tabs == nil {
		tabs = []SandboxTab{}
	}
	encoded, err := json.Marshal(tabs)
	if err != nil {
		return SandboxLayout{}, err
	}

	// One statement rather than a read and a write: the conflict target does
	// the existence check, and the WHERE on the update does the version check,
	// so two devices racing cannot both find the version they expect.
	row := s.pool.QueryRow(ctx,
		`INSERT INTO sandbox_layouts (sandbox_id, identity_id, version, tabs)
		 VALUES ($1, $2, 1, $4)
		 ON CONFLICT (sandbox_id, identity_id) DO UPDATE
		   SET version = sandbox_layouts.version + 1,
		       tabs = EXCLUDED.tabs,
		       updated_at = NOW()
		   WHERE sandbox_layouts.version = $3
		 RETURNING version, tabs`,
		sandboxID, identityID, version, encoded,
	)

	updated := SandboxLayout{SandboxID: sandboxID, IdentityID: identityID}
	var stored []byte
	if err := row.Scan(&updated.Version, &stored); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The row existed and its version was not the caller's. An insert
			// racing another insert lands here too, which is the same answer:
			// refetch, the layout moved.
			return SandboxLayout{}, ErrLayoutVersionConflict
		}
		return SandboxLayout{}, err
	}
	if err := json.Unmarshal(stored, &updated.Tabs); err != nil {
		return SandboxLayout{}, err
	}
	return updated, nil
}

// SetSandboxLayoutDirectories writes cwd onto matching tabs of every layout of
// one sandbox, and returns how many it touched.
//
// Version-free on purpose. The Orchestrator is the sole writer of this field
// and calls immediately before a stop; failing on a concurrent tab reorder
// would lose the snapshot for no benefit, and the snapshot is the only thing
// standing between a restart and tabs that reopen in the wrong place.
//
// Ids it does not find are ignored: a shell the container had and no client
// ever recorded is not a tab, and inventing one would put a tab on a strip
// nobody asked for.
func (s *Store) SetSandboxLayoutDirectories(ctx context.Context, sandboxID uuid.UUID, directories map[string]string) (int, error) {
	if len(directories) == 0 {
		return 0, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT identity_id, version, tabs FROM sandbox_layouts WHERE sandbox_id = $1`,
		sandboxID,
	)
	if err != nil {
		return 0, err
	}

	type pending struct {
		identityID uuid.UUID
		tabs       []SandboxTab
	}
	var updates []pending
	var touched int

	for rows.Next() {
		var identityID uuid.UUID
		var version int64
		var raw []byte
		if err := rows.Scan(&identityID, &version, &raw); err != nil {
			rows.Close()
			return 0, err
		}
		var tabs []SandboxTab
		if err := json.Unmarshal(raw, &tabs); err != nil {
			rows.Close()
			return 0, err
		}
		changed := false
		for i := range tabs {
			cwd, ok := directories[tabs[i].ShellID]
			if !ok || (tabs[i].CWD != nil && *tabs[i].CWD == cwd) {
				continue
			}
			value := cwd
			tabs[i].CWD = &value
			changed = true
			touched++
		}
		if changed {
			updates = append(updates, pending{identityID: identityID, tabs: tabs})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, update := range updates {
		encoded, err := json.Marshal(update.tabs)
		if err != nil {
			return 0, err
		}
		// version is bumped so a client holding the old one refetches and sees
		// the directories rather than writing over them from a stale document.
		if _, err := s.pool.Exec(ctx,
			`UPDATE sandbox_layouts
			   SET tabs = $3, version = version + 1, updated_at = NOW()
			 WHERE sandbox_id = $1 AND identity_id = $2`,
			sandboxID, update.identityID, encoded,
		); err != nil {
			return 0, err
		}
	}
	return touched, nil
}

// ListSandboxLayoutIdentities names who has a layout for this sandbox, so a
// caller that changed all of them knows whom to notify.
func (s *Store) ListSandboxLayoutIdentities(ctx context.Context, sandboxID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT identity_id FROM sandbox_layouts WHERE sandbox_id = $1`, sandboxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var identities []uuid.UUID
	for rows.Next() {
		var identityID uuid.UUID
		if err := rows.Scan(&identityID); err != nil {
			return nil, err
		}
		identities = append(identities, identityID)
	}
	return identities, rows.Err()
}

// DeleteSandboxLayouts drops every layout for a sandbox. Called when it
// terminates: the record is retained for usage history, and a layout is not
// usage history -- nobody will ever reopen it.
func (s *Store) DeleteSandboxLayouts(ctx context.Context, sandboxID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sandbox_layouts WHERE sandbox_id = $1`, sandboxID)
	return err
}
