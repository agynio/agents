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

const testInitImage = "ghcr.io/agynio/agent-init-codex:latest"

// agentEnvironmentServer builds a server over the database named by
// AGENTS_TEST_DATABASE_URL and empties it first. The variable must name a
// scratch database, never one holding anything worth keeping. It is unset in
// ordinary runs and these tests are then skipped, so the package needs no
// database to be tested.
//
// The reference is recorded and served across the store, the composite foreign
// key and the proto converter at once, so it is exercised through the RPCs
// rather than through any one of them.
func agentEnvironmentServer(ctx context.Context, t *testing.T) *Server {
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
	return New(store.New(pool), noopAuthorizationWriter{}, noopIdentityWriter{}, noopNotificationsClient{})
}

func createTestEnvironment(ctx context.Context, t *testing.T, server *Server, organizationID uuid.UUID, name string) string {
	t.Helper()
	response, err := server.CreateEnvironment(ctx, &agentsv1.CreateEnvironmentRequest{
		OrganizationId: organizationID.String(),
		RunnerId:       uuid.NewString(),
		Name:           name,
		Image:          "ghcr.io/agynio/environment:latest",
		Flavor:         "small",
	})
	if err != nil {
		t.Fatalf("create environment: %v", err)
	}
	return response.GetEnvironment().GetMeta().GetId()
}

func createAgentRequest(organizationID uuid.UUID, name string) *agentsv1.CreateAgentRequest {
	return &agentsv1.CreateAgentRequest{
		OrganizationId: organizationID.String(),
		Name:           name,
		Model:          uuid.NewString(),
		InitImage:      testInitImage,
		Availability:   agentsv1.AgentAvailability_AGENT_AVAILABILITY_INTERNAL,
	}
}

// An agent's environment is what supplies its image and compute, replacing the
// copy each agent used to carry inline.
func TestCreateAgentPersistsTheEnvironmentReference(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()
	environmentID := createTestEnvironment(ctx, t, server, organizationID, "default")

	request := createAgentRequest(organizationID, "alpha")
	request.EnvironmentId = environmentID
	created, err := server.CreateAgent(ctx, request)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if created.GetAgent().GetEnvironmentId() != environmentID {
		t.Fatalf("expected environment %s on the created agent, got %q", environmentID, created.GetAgent().GetEnvironmentId())
	}

	agentID := created.GetAgent().GetMeta().GetId()
	fetched, err := server.GetAgent(ctx, &agentsv1.GetAgentRequest{Id: agentID})
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if fetched.GetAgent().GetEnvironmentId() != environmentID {
		t.Fatalf("expected environment %s on the fetched agent, got %q", environmentID, fetched.GetAgent().GetEnvironmentId())
	}

	listed, err := server.ListAgents(ctx, &agentsv1.ListAgentsRequest{OrganizationId: organizationID.String()})
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(listed.GetAgents()) != 1 {
		t.Fatalf("expected one agent, got %d", len(listed.GetAgents()))
	}
	if listed.GetAgents()[0].GetEnvironmentId() != environmentID {
		t.Fatalf("expected environment %s on the listed agent, got %q", environmentID, listed.GetAgents()[0].GetEnvironmentId())
	}
}

// The reference is optional: agents created before environments existed have
// none, and one can still be created without naming an environment.
func TestCreateAgentWithoutAnEnvironmentLeavesItEmpty(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()

	created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha"))
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if created.GetAgent().GetEnvironmentId() != "" {
		t.Fatalf("expected no environment on the created agent, got %q", created.GetAgent().GetEnvironmentId())
	}

	fetched, err := server.GetAgent(ctx, &agentsv1.GetAgentRequest{Id: created.GetAgent().GetMeta().GetId()})
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if fetched.GetAgent().GetEnvironmentId() != "" {
		t.Fatalf("expected no environment on the fetched agent, got %q", fetched.GetAgent().GetEnvironmentId())
	}
}

// An environment belongs to one organization, and an agent naming another
// organization's would reach across tenants. The composite foreign key refuses
// it, but the caller is told which field was wrong instead of being handed a
// constraint violation.
func TestAgentRefusesAnEnvironmentFromAnotherOrganization(t *testing.T) {
	tests := []struct {
		name string
		// refuse names an environment belonging to otherOrganizationID on an
		// agent in organizationID, and reports the error the RPC returned.
		refuse func(ctx context.Context, t *testing.T, server *Server, organizationID uuid.UUID, environmentID string) error
	}{
		{
			name: "create",
			refuse: func(ctx context.Context, t *testing.T, server *Server, organizationID uuid.UUID, environmentID string) error {
				request := createAgentRequest(organizationID, "alpha")
				request.EnvironmentId = environmentID
				_, err := server.CreateAgent(ctx, request)
				return err
			},
		},
		{
			name: "update",
			refuse: func(ctx context.Context, t *testing.T, server *Server, organizationID uuid.UUID, environmentID string) error {
				t.Helper()
				created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha"))
				if err != nil {
					t.Fatalf("create agent: %v", err)
				}
				_, err = server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{
					Id:            created.GetAgent().GetMeta().GetId(),
					EnvironmentId: ptr(environmentID),
				})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := identityContext(uuid.New())
			server := agentEnvironmentServer(ctx, t)
			organizationID := uuid.New()
			otherOrganizationID := uuid.New()
			environmentID := createTestEnvironment(ctx, t, server, otherOrganizationID, "default")

			err := test.refuse(ctx, t, server, organizationID, environmentID)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("expected invalid argument, got %v", err)
			}
			if got := status.Convert(err).Message(); got != "environment_id: environment belongs to another organization" {
				t.Fatalf("expected the environment_id field to be named, got %q", got)
			}
		})
	}
}

// An agent may move to an environment it did not name at creation, and may give
// one up again — an agent with none is the state every agent was in before
// environments existed.
func TestUpdateAgentSetsAndClearsTheEnvironmentReference(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()
	environmentID := createTestEnvironment(ctx, t, server, organizationID, "default")

	created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha"))
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agentID := created.GetAgent().GetMeta().GetId()

	updated, err := server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{
		Id:            agentID,
		EnvironmentId: ptr(environmentID),
	})
	if err != nil {
		t.Fatalf("update agent: %v", err)
	}
	if updated.GetAgent().GetEnvironmentId() != environmentID {
		t.Fatalf("expected environment %s after the update, got %q", environmentID, updated.GetAgent().GetEnvironmentId())
	}

	fetched, err := server.GetAgent(ctx, &agentsv1.GetAgentRequest{Id: agentID})
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if fetched.GetAgent().GetEnvironmentId() != environmentID {
		t.Fatalf("expected environment %s to be stored, got %q", environmentID, fetched.GetAgent().GetEnvironmentId())
	}

	cleared, err := server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{
		Id:            agentID,
		EnvironmentId: ptr(""),
	})
	if err != nil {
		t.Fatalf("clear environment: %v", err)
	}
	if cleared.GetAgent().GetEnvironmentId() != "" {
		t.Fatalf("expected no environment after clearing, got %q", cleared.GetAgent().GetEnvironmentId())
	}

	refetched, err := server.GetAgent(ctx, &agentsv1.GetAgentRequest{Id: agentID})
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if refetched.GetAgent().GetEnvironmentId() != "" {
		t.Fatalf("expected the cleared environment to be stored, got %q", refetched.GetAgent().GetEnvironmentId())
	}
}

// An environment_id naming no environment at all is the caller's mistake, not
// an internal failure.
func TestAgentRefusesAnUnknownEnvironment(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()

	request := createAgentRequest(organizationID, "alpha")
	request.EnvironmentId = uuid.NewString()
	if _, err := server.CreateAgent(ctx, request); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestCreateAgentRejectsAMalformedEnvironment(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)

	request := createAgentRequest(uuid.New(), "alpha")
	request.EnvironmentId = "not-a-uuid"
	if _, err := server.CreateAgent(ctx, request); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

// environment_id alone is enough of an update; it used to be possible to send
// only fields the request rejected as empty.
func TestUpdateAgentAcceptsTheEnvironmentAsTheOnlyField(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()
	environmentID := createTestEnvironment(ctx, t, server, organizationID, "default")

	created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha"))
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{
		Id:            created.GetAgent().GetMeta().GetId(),
		EnvironmentId: ptr(environmentID),
	}); err != nil {
		t.Fatalf("update agent: %v", err)
	}
}
