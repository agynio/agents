package server

import (
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The reference checks answer for another service validating an x-agyn-ref
// before it is persisted. They run against AGENTS_TEST_DATABASE_URL and are
// skipped when it is unset.

func TestResolveAgentExists(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()
	environmentID := createTestEnvironment(ctx, t, server, organizationID, "default")

	created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha", environmentID))
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agentID := created.GetAgent().GetMeta().GetId()

	resp, err := server.ResolveAgentExists(ctx, &agentsv1.ResolveAgentExistsRequest{
		AgentId:        agentID,
		OrganizationId: organizationID.String(),
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !resp.GetExists() {
		t.Fatal("an agent in the organization did not resolve")
	}

	// Another organization answers exactly as a nonexistent agent does: the
	// difference is not something the caller is entitled to learn.
	resp, err = server.ResolveAgentExists(ctx, &agentsv1.ResolveAgentExistsRequest{
		AgentId:        agentID,
		OrganizationId: uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("resolve for another organization: %v", err)
	}
	if resp.GetExists() {
		t.Fatal("an agent resolved for an organization it does not belong to")
	}

	resp, err = server.ResolveAgentExists(ctx, &agentsv1.ResolveAgentExistsRequest{
		AgentId:        uuid.NewString(),
		OrganizationId: organizationID.String(),
	})
	if err != nil {
		t.Fatalf("resolve unknown agent: %v", err)
	}
	if resp.GetExists() {
		t.Fatal("an unknown agent resolved")
	}

	if _, err := server.ResolveAgentExists(ctx, &agentsv1.ResolveAgentExistsRequest{
		AgentId:        "not-a-uuid",
		OrganizationId: organizationID.String(),
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument for a malformed id, got %v", err)
	}
}

func TestResolveEnvironmentExists(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()
	environmentID := createTestEnvironment(ctx, t, server, organizationID, "default")

	resp, err := server.ResolveEnvironmentExists(ctx, &agentsv1.ResolveEnvironmentExistsRequest{
		EnvironmentId:  environmentID,
		OrganizationId: organizationID.String(),
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !resp.GetExists() {
		t.Fatal("an environment in the organization did not resolve")
	}

	resp, err = server.ResolveEnvironmentExists(ctx, &agentsv1.ResolveEnvironmentExistsRequest{
		EnvironmentId:  environmentID,
		OrganizationId: uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("resolve for another organization: %v", err)
	}
	if resp.GetExists() {
		t.Fatal("an environment resolved for an organization it does not belong to")
	}
}
