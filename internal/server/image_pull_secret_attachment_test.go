package server

import (
	"context"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/google/uuid"
)

// An image pull secret attachment names a registry credential belonging to one
// organization, and ListImagePullSecretAttachments used to authorize nothing at
// all: any caller who could reach the RPC could read any organization's
// attachments by naming an id from it.
func TestImagePullSecretAttachmentListFilterRefusesAnotherOrganizationsAttachments(t *testing.T) {
	callerOrganizationID := uuid.New()
	organizationID := uuid.New()
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{
		organizationPrefix + callerOrganizationID.String(): true,
	}}
	server := &Server{authz: authz}

	filter, err := server.imagePullSecretAttachmentListFilter(identityContext(identityID), &agentsv1.ListImagePullSecretAttachmentsRequest{
		OrganizationId: organizationID.String(),
	})
	if err == nil {
		t.Fatal("expected a caller from another organization to be refused")
	}
	if filter.OrganizationID != nil || filter.ImagePullSecretID != nil || filter.AgentID != nil || filter.McpID != nil || filter.EnvironmentID != nil {
		t.Fatalf("expected no filter to be returned on refusal, got %#v", filter)
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
		organizationRelationTuple(identityID, organizationID, "member"),
	})
}

// Naming an id from another organization must not smuggle a read past the
// check: the organization is what is authorized, and the targets only narrow
// the result within it.
func TestImagePullSecretAttachmentListFilterRefusesAnotherOrganizationsAttachmentsWhenNarrowedByTarget(t *testing.T) {
	callerOrganizationID := uuid.New()
	organizationID := uuid.New()
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{
		organizationPrefix + callerOrganizationID.String(): true,
	}}
	server := &Server{authz: authz}

	if _, err := server.imagePullSecretAttachmentListFilter(identityContext(identityID), &agentsv1.ListImagePullSecretAttachmentsRequest{
		OrganizationId: organizationID.String(),
		EnvironmentId:  uuid.NewString(),
	}); err == nil {
		t.Fatal("expected a caller from another organization to be refused")
	}
}

// Listing an organization's attachments without narrowing them is a legitimate
// read once the organization is authorized; it used to be refused outright
// because the table carried no organization to scope it by.
func TestImagePullSecretAttachmentListFilterScopesAnUnfilteredListToTheOrganization(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	organizationID := uuid.New()
	identityID := uuid.New()

	filter, err := server.imagePullSecretAttachmentListFilter(identityContext(identityID), &agentsv1.ListImagePullSecretAttachmentsRequest{
		OrganizationId: organizationID.String(),
	})
	if err != nil {
		t.Fatalf("image pull secret attachment list filter: %v", err)
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
func TestImagePullSecretAttachmentListFilterRequiresAnOrganizationFromAnIdentifiedCaller(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	if _, err := server.imagePullSecretAttachmentListFilter(identityContext(uuid.New()), &agentsv1.ListImagePullSecretAttachmentsRequest{}); err == nil {
		t.Fatal("expected a request without an organization to be rejected")
	}
	if len(authz.checks) != 0 {
		t.Fatalf("expected no authorization check, got %d", len(authz.checks))
	}
}

// The Agents Orchestrator lists an environment's image pull secret attachments
// while assembling a sandbox workload, immediately after listing its envs. It
// reaches the RPC over the mesh and carries no identity by design, holding no
// tuples any check could pass.
func TestImagePullSecretAttachmentListFilterServesTheInternalCallerWithoutAnIdentity(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	environmentID := uuid.New()

	filter, err := server.imagePullSecretAttachmentListFilter(context.Background(), &agentsv1.ListImagePullSecretAttachmentsRequest{
		EnvironmentId: environmentID.String(),
	})
	if err != nil {
		t.Fatalf("image pull secret attachment list filter: %v", err)
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
func TestImagePullSecretAttachmentListFilterScopesTheInternalCallerToANamedOrganization(t *testing.T) {
	server := &Server{authz: &recordingAuthorizationWriter{}}
	organizationID := uuid.New()

	filter, err := server.imagePullSecretAttachmentListFilter(context.Background(), &agentsv1.ListImagePullSecretAttachmentsRequest{
		OrganizationId: organizationID.String(),
	})
	if err != nil {
		t.Fatalf("image pull secret attachment list filter: %v", err)
	}
	if filter.OrganizationID == nil || *filter.OrganizationID != organizationID {
		t.Fatalf("expected organization filter %s, got %v", organizationID, filter.OrganizationID)
	}
}

func TestImagePullSecretAttachmentListFilterNarrowsByEveryTarget(t *testing.T) {
	server := &Server{authz: &recordingAuthorizationWriter{}}
	organizationID := uuid.New()
	imagePullSecretID := uuid.New()
	agentID := uuid.New()
	mcpID := uuid.New()
	environmentID := uuid.New()

	filter, err := server.imagePullSecretAttachmentListFilter(identityContext(uuid.New()), &agentsv1.ListImagePullSecretAttachmentsRequest{
		OrganizationId:    organizationID.String(),
		ImagePullSecretId: imagePullSecretID.String(),
		AgentId:           agentID.String(),
		McpId:             mcpID.String(),
		EnvironmentId:     environmentID.String(),
	})
	if err != nil {
		t.Fatalf("image pull secret attachment list filter: %v", err)
	}
	for name, pair := range map[string][2]*uuid.UUID{
		"organization":      {filter.OrganizationID, &organizationID},
		"image pull secret": {filter.ImagePullSecretID, &imagePullSecretID},
		"agent":             {filter.AgentID, &agentID},
		"mcp":               {filter.McpID, &mcpID},
		"environment":       {filter.EnvironmentID, &environmentID},
	} {
		if pair[0] == nil || *pair[0] != *pair[1] {
			t.Fatalf("expected %s filter %s, got %v", name, pair[1], pair[0])
		}
	}
}

func TestImagePullSecretAttachmentListFilterRejectsAMalformedOrganization(t *testing.T) {
	server := &Server{authz: &recordingAuthorizationWriter{}}

	if _, err := server.imagePullSecretAttachmentListFilter(identityContext(uuid.New()), &agentsv1.ListImagePullSecretAttachmentsRequest{
		OrganizationId: "not-a-uuid",
	}); err == nil {
		t.Fatal("expected a malformed organization to be rejected")
	}
}
