package server

import (
	"context"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// An agent was reachable by id alone: deleting one, and reading or rewriting who
// holds a role on it, checked nothing at all. Removing a role is the sharpest of
// them -- it revokes another identity's access to an agent the caller may hold
// nothing on.

func agentForAuthorizationTest(ctx context.Context, t *testing.T, server *Server, organizationID uuid.UUID, name string) string {
	t.Helper()
	created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, name))
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return created.GetAgent().GetMeta().GetId()
}

func TestAgentRPCsRefuseACallerWithoutTheAgent(t *testing.T) {
	ctx := identityContext(uuid.New())
	authz := &recordingAuthorizationWriter{}
	server, _ := environmentAuthorizationServer(ctx, t, authz)
	agentID := agentForAuthorizationTest(ctx, t, server, uuid.New(), "alpha")

	authz.allowedObjects = map[string]bool{}

	cases := []struct {
		name string
		call func() error
	}{
		{"DeleteAgent", func() error {
			_, err := server.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: agentID})
			return err
		}},
		{"ListAgentRoles", func() error {
			_, err := server.ListAgentRoles(ctx, &agentsv1.ListAgentRolesRequest{AgentId: agentID})
			return err
		}},
		{"RemoveAgentRole", func() error {
			_, err := server.RemoveAgentRole(ctx, &agentsv1.RemoveAgentRoleRequest{
				AgentId: agentID, IdentityId: uuid.NewString(),
			})
			return err
		}},
		{"SetAgentRole", func() error {
			_, err := server.SetAgentRole(ctx, &agentsv1.SetAgentRoleRequest{
				AgentId: agentID, IdentityId: uuid.NewString(), Role: agentsv1.AgentRole_AGENT_ROLE_MAINTAINER,
			})
			return err
		}},
		{"UpdateAgent", func() error {
			name := "renamed"
			_, err := server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{Id: agentID, Name: &name})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := status.Code(tc.call()); code != codes.PermissionDenied {
				t.Fatalf("expected PermissionDenied, got %v", code)
			}
		})
	}
}

// RemoveAgentRole reached the mutation before anything else, so a refusal has to
// leave the assignment in place rather than roll one back.
func TestRemoveAgentRoleRefusalLeavesTheRoleInPlace(t *testing.T) {
	ctx := identityContext(uuid.New())
	authz := &recordingAuthorizationWriter{}
	server, _ := environmentAuthorizationServer(ctx, t, authz)
	organizationID := uuid.New()
	agentID := agentForAuthorizationTest(ctx, t, server, organizationID, "alpha")
	granteeID := uuid.NewString()

	if _, err := server.SetAgentRole(ctx, &agentsv1.SetAgentRoleRequest{
		AgentId: agentID, IdentityId: granteeID, Role: agentsv1.AgentRole_AGENT_ROLE_MAINTAINER,
	}); err != nil {
		t.Fatalf("set agent role: %v", err)
	}

	authz.allowedObjects = map[string]bool{}
	if _, err := server.RemoveAgentRole(ctx, &agentsv1.RemoveAgentRoleRequest{
		AgentId: agentID, IdentityId: granteeID,
	}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", err)
	}

	authz.allowedObjects = nil
	roles, err := server.ListAgentRoles(ctx, &agentsv1.ListAgentRolesRequest{AgentId: agentID})
	if err != nil {
		t.Fatalf("list agent roles: %v", err)
	}
	// The creator's own owner role is written by CreateAgent, so the grantee's
	// is the one to look for rather than a count.
	found := false
	for _, assignment := range roles.GetAssignments() {
		if assignment.GetIdentityId() == granteeID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the refused removal to leave the grantee's role, got %d assignments", len(roles.GetAssignments()))
	}
}

// Creating an agent is an organization owner's, and the creator becomes its
// owner -- so nothing else gates who may mint one.
func TestCreateAgentRefusesANonOwner(t *testing.T) {
	ctx := identityContext(uuid.New())
	authz := &recordingAuthorizationWriter{deniedRelations: map[string]bool{"owner": true}}
	server, _ := environmentAuthorizationServer(ctx, t, authz)

	_, err := server.CreateAgent(ctx, createAgentRequest(uuid.New(), "alpha"))
	if code := status.Code(err); code != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", code)
	}
}

// Availability decides who may reach the agent, so it is gated with deletion
// rather than with the rest of the configuration.
func TestUpdateAgentAvailabilityNeedsCanDelete(t *testing.T) {
	ctx := identityContext(uuid.New())
	authz := &recordingAuthorizationWriter{}
	server, _ := environmentAuthorizationServer(ctx, t, authz)
	agentID := agentForAuthorizationTest(ctx, t, server, uuid.New(), "alpha")

	authz.deniedRelations = map[string]bool{"can_delete": true}
	availability := agentsv1.AgentAvailability_AGENT_AVAILABILITY_PRIVATE
	_, err := server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{Id: agentID, Availability: &availability})
	if code := status.Code(err); code != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", code)
	}

	// A configuration-only update is unaffected by the same denial.
	name := "renamed"
	if _, err := server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{Id: agentID, Name: &name}); err != nil {
		t.Fatalf("configuration update refused: %v", err)
	}
}

// NotFound has to precede PermissionDenied, or the refusal tells a caller that
// an agent it may not touch exists.
func TestAgentRPCsReportNotFoundBeforeDenial(t *testing.T) {
	ctx := identityContext(uuid.New())
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{}}
	server, _ := environmentAuthorizationServer(ctx, t, authz)
	missing := uuid.NewString()

	if _, err := server.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: missing}); status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	name := "renamed"
	if _, err := server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{Id: missing, Name: &name}); status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}
