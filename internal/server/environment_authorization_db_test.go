package server

import (
	"context"
	"os"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/agynio/agents/internal/db"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Get, Update and Delete name an environment by id alone and resolve its
// organization from the record, so the refusal cannot be observed without one.
// These run against AGENTS_TEST_DATABASE_URL — a scratch database, emptied
// first — and are skipped when it is unset.
func environmentAuthorizationServer(ctx context.Context, t *testing.T, authz AuthorizationWriter) (*Server, *store.Store) {
	t.Helper()
	url := os.Getenv("AGENTS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AGENTS_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if err := db.ApplyMigrations(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	backing := store.New(pool)
	return New(backing, authz, noopIdentityWriter{}, noopNotificationsClient{}), backing
}

// An environment written straight to the store, so the setup does not depend on
// the authorization the test is about to exercise.
func seedEnvironment(ctx context.Context, t *testing.T, backing *store.Store, organizationID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	runnerID := uuid.New()
	environment, err := backing.CreateEnvironment(ctx, organizationID, store.EnvironmentInput{
		Name:         name,
		Image:        "ghcr.io/agynio/environment:latest",
		RunnerID:     &runnerID,
		Flavor:       "small",
		Availability: store.EnvironmentAvailabilityPrivate,
		LLMMode:      store.LLMModePlatform,
		// NOT NULL, and a nil slice encodes as NULL rather than '{}'.
		LLMAllowedModels: []string{},
	})
	if err != nil {
		t.Fatalf("seed environment: %v", err)
	}
	return environment.Meta.ID
}

// Holding an environment id used to be the whole permission: every one of these
// served a caller from an unrelated organization.
func TestEnvironmentRPCsRefuseAnotherOrganization(t *testing.T) {
	ctx := context.Background()
	member := uuid.New()
	theirs := uuid.New()
	mine := uuid.New()
	// The caller is a member of its own organization and holds nothing on the
	// one that owns the environment.
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{
		organizationPrefix + mine.String(): true,
	}}
	server, backing := environmentAuthorizationServer(ctx, t, authz)
	environmentID := seedEnvironment(ctx, t, backing, theirs, "theirs")
	callerCtx := identityContext(member)

	name := "renamed"
	cases := []struct {
		name string
		call func() error
	}{
		{"Get", func() error {
			_, err := server.GetEnvironment(callerCtx, &agentsv1.GetEnvironmentRequest{Id: environmentID.String()})
			return err
		}},
		{"Update", func() error {
			_, err := server.UpdateEnvironment(callerCtx, &agentsv1.UpdateEnvironmentRequest{Id: environmentID.String(), Name: &name})
			return err
		}},
		{"Delete", func() error {
			_, err := server.DeleteEnvironment(callerCtx, &agentsv1.DeleteEnvironmentRequest{Id: environmentID.String()})
			return err
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.call(); err == nil {
				t.Fatalf("%s served an environment in another organization", testCase.name)
			}
		})
	}

	// The record is intact: the refusals stopped before the store was written.
	if _, err := backing.GetEnvironment(ctx, environmentID); err != nil {
		t.Fatalf("expected the environment to survive: %v", err)
	}
}

// Membership shows an environment exists; changing it is a grant on the
// environment itself. A member holding no role gets the metadata and nothing
// more, which is what keeps one team from renaming another's environment.
func TestEnvironmentRPCsServeAMember(t *testing.T) {
	ctx := context.Background()
	member := uuid.New()
	organizationID := uuid.New()
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{
		organizationPrefix + organizationID.String(): true,
	}}
	server, backing := environmentAuthorizationServer(ctx, t, authz)
	environmentID := seedEnvironment(ctx, t, backing, organizationID, "ours")
	callerCtx := identityContext(member)

	if _, err := server.GetEnvironment(callerCtx, &agentsv1.GetEnvironmentRequest{Id: environmentID.String()}); err != nil {
		t.Fatalf("GetEnvironment refused a member: %v", err)
	}
	name := "renamed"
	if _, err := server.UpdateEnvironment(callerCtx, &agentsv1.UpdateEnvironmentRequest{Id: environmentID.String(), Name: &name}); err == nil {
		t.Fatal("UpdateEnvironment served a member holding no role on the environment")
	}

	// With the role, the same caller edits and deletes it.
	authz.allowedObjects[environmentPrefix+environmentID.String()] = true
	updated, err := server.UpdateEnvironment(callerCtx, &agentsv1.UpdateEnvironmentRequest{Id: environmentID.String(), Name: &name})
	if err != nil {
		t.Fatalf("UpdateEnvironment refused a role holder: %v", err)
	}
	if updated.GetEnvironment().GetName() != name {
		t.Fatalf("expected the name to be %q, got %q", name, updated.GetEnvironment().GetName())
	}
	if _, err := server.DeleteEnvironment(callerCtx, &agentsv1.DeleteEnvironmentRequest{Id: environmentID.String()}); err != nil {
		t.Fatalf("DeleteEnvironment refused a role holder: %v", err)
	}
}

// The Orchestrator resolves an environment while assembling every workload,
// over the mesh and carrying no identity.
func TestGetEnvironmentServesAnInternalCaller(t *testing.T) {
	ctx := context.Background()
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{}}
	server, backing := environmentAuthorizationServer(ctx, t, authz)
	environmentID := seedEnvironment(ctx, t, backing, uuid.New(), "ours")

	if _, err := server.GetEnvironment(ctx, &agentsv1.GetEnvironmentRequest{Id: environmentID.String()}); err != nil {
		t.Fatalf("GetEnvironment refused an internal caller: %v", err)
	}
}

func TestUpdateEnvironmentRefusesAnInternalCaller(t *testing.T) {
	ctx := context.Background()
	authz := &recordingAuthorizationWriter{}
	server, backing := environmentAuthorizationServer(ctx, t, authz)
	environmentID := seedEnvironment(ctx, t, backing, uuid.New(), "ours")

	name := "renamed"
	_, err := server.UpdateEnvironment(ctx, &agentsv1.UpdateEnvironmentRequest{Id: environmentID.String(), Name: &name})
	if status.Code(err) != codes.Unauthenticated && status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected an identity-less write to be refused, got %v", err)
	}
}
