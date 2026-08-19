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

// createTestEnvironment builds one an agent can run in: it names an agent
// runtime image, which is where the agent CLI comes from.
func createTestEnvironment(ctx context.Context, t *testing.T, server *Server, organizationID uuid.UUID, name string) string {
	t.Helper()
	return createEnvironmentWithRuntime(ctx, t, server, organizationID, name, true)
}

// createWorkspaceOnlyEnvironment builds one only a sandbox can use: a sandbox
// brings its own tooling, so it needs no agent runtime image.
func createWorkspaceOnlyEnvironment(ctx context.Context, t *testing.T, server *Server, organizationID uuid.UUID, name string) string {
	t.Helper()
	return createEnvironmentWithRuntime(ctx, t, server, organizationID, name, false)
}

func createEnvironmentWithRuntime(ctx context.Context, t *testing.T, server *Server, organizationID uuid.UUID, name string, withRuntime bool) string {
	t.Helper()
	request := &agentsv1.CreateEnvironmentRequest{
		OrganizationId: organizationID.String(),
		RunnerId:       uuid.NewString(),
		Name:           name,
		Image:          "ghcr.io/agynio/environment:latest",
		Flavor:         "small",
		Availability:   agentsv1.EnvironmentAvailability_ENVIRONMENT_AVAILABILITY_INTERNAL,
	}
	if withRuntime {
		request.AgentRuntimeImageId = uuid.NewString()
		request.AgentRuntimeImageTag = "1.0.0"
	}
	response, err := server.CreateEnvironment(ctx, request)
	if err != nil {
		t.Fatalf("create environment: %v", err)
	}
	return response.GetEnvironment().GetMeta().GetId()
}

func createAgentRequest(organizationID uuid.UUID, name, environmentID string) *agentsv1.CreateAgentRequest {
	return &agentsv1.CreateAgentRequest{
		OrganizationId: organizationID.String(),
		Name:           name,
		Model:          uuid.NewString(),
		EnvironmentId:  environmentID,
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

	created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha", environmentID))
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

// An agent takes its CLI from its environment's agent runtime image, so an agent
// with no environment has no CLI to run. The Orchestrator refuses to assemble
// one, on every cycle, forever -- so the refusal belongs here, where the caller
// hears it.
func TestCreateAgentRequiresAnEnvironment(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)

	_, err := server.CreateAgent(ctx, createAgentRequest(uuid.New(), "alpha", ""))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

// A workspace-only environment is one a sandbox uses. It names no agent runtime
// image, so it leaves an agent in exactly the state above.
func TestAgentRefusesAWorkspaceOnlyEnvironment(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()
	runnable := createTestEnvironment(ctx, t, server, organizationID, "runnable")
	workspaceOnly := createWorkspaceOnlyEnvironment(ctx, t, server, organizationID, "sandboxes")

	_, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha", workspaceOnly))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected failed precondition on create, got %v", err)
	}

	created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "beta", runnable))
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	_, err = server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{
		Id:            created.GetAgent().GetMeta().GetId(),
		EnvironmentId: ptr(workspaceOnly),
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected failed precondition on update, got %v", err)
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
				_, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha", environmentID))
				return err
			},
		},
		{
			name: "update",
			refuse: func(ctx context.Context, t *testing.T, server *Server, organizationID uuid.UUID, environmentID string) error {
				t.Helper()
				own := createTestEnvironment(ctx, t, server, organizationID, "own")
				created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha", own))
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

// An agent may move to an environment it did not name at creation. It may not
// give one up: an empty environment_id used to clear the reference, and now
// leaves the agent in the state CreateAgent refuses to produce.
func TestUpdateAgentMovesTheEnvironmentAndRefusesToClearIt(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()
	first := createTestEnvironment(ctx, t, server, organizationID, "first")
	second := createTestEnvironment(ctx, t, server, organizationID, "second")

	created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha", first))
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agentID := created.GetAgent().GetMeta().GetId()

	updated, err := server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{
		Id:            agentID,
		EnvironmentId: ptr(second),
	})
	if err != nil {
		t.Fatalf("update agent: %v", err)
	}
	if updated.GetAgent().GetEnvironmentId() != second {
		t.Fatalf("expected environment %s after the update, got %q", second, updated.GetAgent().GetEnvironmentId())
	}

	fetched, err := server.GetAgent(ctx, &agentsv1.GetAgentRequest{Id: agentID})
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if fetched.GetAgent().GetEnvironmentId() != second {
		t.Fatalf("expected environment %s to be stored, got %q", second, fetched.GetAgent().GetEnvironmentId())
	}

	if _, err := server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{
		Id:            agentID,
		EnvironmentId: ptr(""),
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected clearing to be refused, got %v", err)
	}

	refetched, err := server.GetAgent(ctx, &agentsv1.GetAgentRequest{Id: agentID})
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if refetched.GetAgent().GetEnvironmentId() != second {
		t.Fatalf("expected environment %s to survive the refusal, got %q", second, refetched.GetAgent().GetEnvironmentId())
	}
}

// An environment_id naming no environment at all is the caller's mistake, not
// an internal failure.
func TestAgentRefusesAnUnknownEnvironment(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()

	request := createAgentRequest(organizationID, "alpha", uuid.NewString())
	if _, err := server.CreateAgent(ctx, request); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestCreateAgentRejectsAMalformedEnvironment(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)

	request := createAgentRequest(uuid.New(), "alpha", "not-a-uuid")
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
	other := createTestEnvironment(ctx, t, server, organizationID, "other")

	created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha", other))
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
