package server

import (
	"context"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

func identityContext(identityID uuid.UUID) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-identity-id", identityID.String()))
}

// An env names a value or a secret belonging to one organization, and ListEnvs
// used to authorize nothing at all: any caller who could reach the RPC could
// read any organization's envs by naming an id from it.
func TestEnvListFilterRefusesAnotherOrganizationsEnvs(t *testing.T) {
	callerOrganizationID := uuid.New()
	organizationID := uuid.New()
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{
		organizationPrefix + callerOrganizationID.String(): true,
	}}
	server := &Server{authz: authz}

	filter, err := server.envListFilter(identityContext(identityID), &agentsv1.ListEnvsRequest{
		OrganizationId: organizationID.String(),
	})
	if err == nil {
		t.Fatal("expected a caller from another organization to be refused")
	}
	if filter.OrganizationID != nil || filter.AgentID != nil || filter.McpID != nil || filter.HookID != nil || filter.EnvironmentID != nil {
		t.Fatalf("expected no filter to be returned on refusal, got %#v", filter)
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
		organizationRelationTuple(identityID, organizationID, "member"),
	})
}

// Naming an id from another organization must not smuggle a read past the
// check: the organization is what is authorized, and the targets only narrow
// the result within it.
func TestEnvListFilterRefusesAnotherOrganizationsEnvsWhenNarrowedByTarget(t *testing.T) {
	callerOrganizationID := uuid.New()
	organizationID := uuid.New()
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{
		organizationPrefix + callerOrganizationID.String(): true,
	}}
	server := &Server{authz: authz}

	if _, err := server.envListFilter(identityContext(identityID), &agentsv1.ListEnvsRequest{
		OrganizationId: organizationID.String(),
		AgentId:        uuid.NewString(),
	}); err == nil {
		t.Fatal("expected a caller from another organization to be refused")
	}
}

// Listing an organization's envs without narrowing them is a legitimate read
// once the organization is authorized; it used to be refused outright because
// the table carried no organization to scope it by.
func TestEnvListFilterScopesAnUnfilteredListToTheOrganization(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	organizationID := uuid.New()
	identityID := uuid.New()

	filter, err := server.envListFilter(identityContext(identityID), &agentsv1.ListEnvsRequest{
		OrganizationId: organizationID.String(),
	})
	if err != nil {
		t.Fatalf("env list filter: %v", err)
	}
	if filter.OrganizationID == nil || *filter.OrganizationID != organizationID {
		t.Fatalf("expected organization filter %s, got %v", organizationID, filter.OrganizationID)
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
		organizationRelationTuple(identityID, organizationID, "member"),
	})
}

// A caller that presents an identity is a user request and cannot fall through
// to the unscoped internal path by leaving the organization out.
func TestEnvListFilterRequiresAnOrganizationFromAnIdentifiedCaller(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	if _, err := server.envListFilter(identityContext(uuid.New()), &agentsv1.ListEnvsRequest{}); err == nil {
		t.Fatal("expected a request without an organization to be rejected")
	}
	if len(authz.checks) != 0 {
		t.Fatalf("expected no authorization check, got %d", len(authz.checks))
	}
}

// The Agents Orchestrator lists an environment's envs while assembling a
// sandbox workload. It reaches the RPC over the mesh and carries no identity by
// design, holding no tuples any check could pass.
func TestEnvListFilterServesTheInternalCallerWithoutAnIdentity(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	environmentID := uuid.New()

	filter, err := server.envListFilter(context.Background(), &agentsv1.ListEnvsRequest{
		EnvironmentId: environmentID.String(),
	})
	if err != nil {
		t.Fatalf("env list filter: %v", err)
	}
	if filter.EnvironmentID == nil || *filter.EnvironmentID != environmentID {
		t.Fatalf("expected environment filter %s, got %v", environmentID, filter.EnvironmentID)
	}
	if filter.OrganizationID != nil {
		t.Fatalf("expected no organization filter, got %s", filter.OrganizationID)
	}
	if len(authz.checks) != 0 {
		t.Fatalf("expected no authorization check, got %d", len(authz.checks))
	}
}

// An internal caller that does name an organization is still held to it.
func TestEnvListFilterScopesTheInternalCallerToANamedOrganization(t *testing.T) {
	server := &Server{authz: &recordingAuthorizationWriter{}}
	organizationID := uuid.New()

	filter, err := server.envListFilter(context.Background(), &agentsv1.ListEnvsRequest{
		OrganizationId: organizationID.String(),
	})
	if err != nil {
		t.Fatalf("env list filter: %v", err)
	}
	if filter.OrganizationID == nil || *filter.OrganizationID != organizationID {
		t.Fatalf("expected organization filter %s, got %v", organizationID, filter.OrganizationID)
	}
}

func TestEnvListFilterNarrowsByEveryTarget(t *testing.T) {
	server := &Server{authz: &recordingAuthorizationWriter{}}
	organizationID := uuid.New()
	agentID := uuid.New()
	mcpID := uuid.New()
	hookID := uuid.New()
	environmentID := uuid.New()

	filter, err := server.envListFilter(identityContext(uuid.New()), &agentsv1.ListEnvsRequest{
		OrganizationId: organizationID.String(),
		AgentId:        agentID.String(),
		McpId:          mcpID.String(),
		HookId:         hookID.String(),
		EnvironmentId:  environmentID.String(),
	})
	if err != nil {
		t.Fatalf("env list filter: %v", err)
	}
	for name, pair := range map[string][2]*uuid.UUID{
		"organization": {filter.OrganizationID, &organizationID},
		"agent":        {filter.AgentID, &agentID},
		"mcp":          {filter.McpID, &mcpID},
		"hook":         {filter.HookID, &hookID},
		"environment":  {filter.EnvironmentID, &environmentID},
	} {
		if pair[0] == nil || *pair[0] != *pair[1] {
			t.Fatalf("expected %s filter %s, got %v", name, pair[1], pair[0])
		}
	}
}

func TestEnvListFilterRejectsAMalformedOrganization(t *testing.T) {
	server := &Server{authz: &recordingAuthorizationWriter{}}

	if _, err := server.envListFilter(identityContext(uuid.New()), &agentsv1.ListEnvsRequest{
		OrganizationId: "not-a-uuid",
	}); err == nil {
		t.Fatal("expected a malformed organization to be rejected")
	}
}
