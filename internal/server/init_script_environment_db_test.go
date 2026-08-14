package server

import (
	"context"
	"testing"

	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
)

// An init script targets an agent, an mcp or an environment. Only the first two
// were ever filtered on, so a list scoped to one environment returned every
// script in the database -- and every mutation tried to resolve an agent a
// environment-scoped row does not have, which rolled the write back.
func seedEnvironmentInitScript(ctx context.Context, t *testing.T, backing *store.Store, organizationID, environmentID uuid.UUID, script string) uuid.UUID {
	t.Helper()
	created, err := backing.CreateInitScript(ctx, organizationID, store.InitScriptInput{
		Script:        script,
		Description:   script,
		EnvironmentID: &environmentID,
	})
	if err != nil {
		t.Fatalf("create init script: %v", err)
	}
	return created.Meta.ID
}

func TestListInitScriptsIsScopedToItsEnvironment(t *testing.T) {
	ctx := context.Background()
	_, backing := environmentAuthorizationServer(ctx, t, &recordingAuthorizationWriter{})
	organizationID := uuid.New()
	first := seedEnvironment(ctx, t, backing, organizationID, "first")
	second := seedEnvironment(ctx, t, backing, organizationID, "second")
	seedEnvironmentInitScript(ctx, t, backing, organizationID, first, "echo first")

	result, err := backing.ListInitScripts(ctx, store.InitScriptFilter{EnvironmentID: &first}, 100, nil)
	if err != nil {
		t.Fatalf("list first: %v", err)
	}
	if len(result.InitScripts) != 1 {
		t.Fatalf("expected the environment's own script, got %d", len(result.InitScripts))
	}

	// The leak: an unfiltered query returns this one too.
	other, err := backing.ListInitScripts(ctx, store.InitScriptFilter{EnvironmentID: &second}, 100, nil)
	if err != nil {
		t.Fatalf("list second: %v", err)
	}
	if len(other.InitScripts) != 0 {
		t.Fatalf("expected no scripts for an environment that has none, got %d", len(other.InitScripts))
	}
}

func TestDeleteEnvironmentInitScript(t *testing.T) {
	ctx := context.Background()
	_, backing := environmentAuthorizationServer(ctx, t, &recordingAuthorizationWriter{})
	organizationID := uuid.New()
	environmentID := seedEnvironment(ctx, t, backing, organizationID, "first")
	scriptID := seedEnvironmentInitScript(ctx, t, backing, organizationID, environmentID, "echo first")

	// Resolving an agent for a row that has none used to fail inside the
	// transaction, so the delete rolled back and reported an internal error.
	if err := backing.DeleteInitScript(ctx, scriptID); err != nil {
		t.Fatalf("delete init script: %v", err)
	}
	result, err := backing.ListInitScripts(ctx, store.InitScriptFilter{EnvironmentID: &environmentID}, 100, nil)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(result.InitScripts) != 0 {
		t.Fatalf("expected the script to be gone, got %d", len(result.InitScripts))
	}
}

func TestUpdateEnvironmentInitScript(t *testing.T) {
	ctx := context.Background()
	_, backing := environmentAuthorizationServer(ctx, t, &recordingAuthorizationWriter{})
	organizationID := uuid.New()
	environmentID := seedEnvironment(ctx, t, backing, organizationID, "first")
	scriptID := seedEnvironmentInitScript(ctx, t, backing, organizationID, environmentID, "echo first")

	updated := "echo second"
	if _, err := backing.UpdateInitScript(ctx, scriptID, store.InitScriptUpdate{Script: &updated}); err != nil {
		t.Fatalf("update init script: %v", err)
	}
}
