package server

import (
	"context"
	"strings"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/google/uuid"
)

// These RPCs used to authorize nothing at all: each took an organization id (or
// an id from which one resolves) and returned rows, so knowing an id was the
// permission. What follows checks the decision each one now makes.
//
// The Server's store is a concrete type these tests do not stand up, so every
// case that is meant to survive authorization carries a page token that cannot
// be decoded. The request then stops on the token, one step past the check and
// short of the database, and what is asserted is the authorization outcome: how
// many checks were made, against which object, and whether the request lived
// through them. A refusal never gets that far and needs no such device.
const undecodablePageToken = "not-a-page-token"

// listRPC is one of the list RPCs under test, reduced to what authorization
// sees: the organization named in the request, and the call itself.
type listRPC struct {
	name string
	call func(server *Server, ctx context.Context, organizationID string) error
}

func listRPCs() []listRPC {
	return []listRPC{
		{
			name: "ListAgents",
			call: func(server *Server, ctx context.Context, organizationID string) error {
				_, err := server.ListAgents(ctx, &agentsv1.ListAgentsRequest{
					OrganizationId: organizationID,
					PageToken:      undecodablePageToken,
				})
				return err
			},
		},
		{
			name: "ListVolumes",
			call: func(server *Server, ctx context.Context, organizationID string) error {
				_, err := server.ListVolumes(ctx, &agentsv1.ListVolumesRequest{
					OrganizationId: organizationID,
					PageToken:      undecodablePageToken,
				})
				return err
			},
		},
		{
			name: "ListEnvironments",
			call: func(server *Server, ctx context.Context, organizationID string) error {
				_, err := server.ListEnvironments(ctx, &agentsv1.ListEnvironmentsRequest{
					OrganizationId: organizationID,
					PageToken:      undecodablePageToken,
				})
				return err
			},
		},
		{
			name: "ListInstances",
			call: func(server *Server, ctx context.Context, organizationID string) error {
				_, err := server.ListInstances(ctx, &agentsv1.ListInstancesRequest{
					OrganizationId: organizationID,
					PageToken:      undecodablePageToken,
				})
				return err
			},
		},
	}
}

// A caller holding tuples in one organization must not be able to read another
// one's rows by naming its id.
func TestListRPCsRefuseAnotherOrganization(t *testing.T) {
	for _, rpc := range listRPCs() {
		t.Run(rpc.name, func(t *testing.T) {
			callerOrganizationID := uuid.New()
			organizationID := uuid.New()
			identityID := uuid.New()
			authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{
				organizationPrefix + callerOrganizationID.String(): true,
			}}
			server := &Server{authz: authz}

			err := rpc.call(server, identityContext(identityID), organizationID.String())
			if err == nil {
				t.Fatal("expected a caller from another organization to be refused")
			}
			if strings.Contains(err.Error(), "page_token") {
				t.Fatalf("expected the refusal to come before anything else, got %v", err)
			}
			assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
				organizationRelationTuple(identityID, organizationID, "member"),
			})
		})
	}
}

// A member of the organization reads it, and is checked for exactly that.
func TestListRPCsServeAMemberOfTheOrganization(t *testing.T) {
	for _, rpc := range listRPCs() {
		t.Run(rpc.name, func(t *testing.T) {
			organizationID := uuid.New()
			identityID := uuid.New()
			authz := &recordingAuthorizationWriter{}
			server := &Server{authz: authz}

			err := rpc.call(server, identityContext(identityID), organizationID.String())
			if err == nil || !strings.Contains(err.Error(), "page_token") {
				t.Fatalf("expected the request to survive authorization and stop on the page token, got %v", err)
			}
			assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
				organizationRelationTuple(identityID, organizationID, "member"),
			})
		})
	}
}

// The Agents Orchestrator reaches these RPCs over the mesh rather than the
// Gateway and carries no identity by design: it holds no OpenFGA tuples, so a
// check could only ever refuse it. Breaking this path stops sandboxes and
// agents from starting.
func TestListRPCsServeTheInternalCallerWithoutAnIdentity(t *testing.T) {
	for _, rpc := range listRPCs() {
		t.Run(rpc.name, func(t *testing.T) {
			authz := &recordingAuthorizationWriter{}
			server := &Server{authz: authz}

			err := rpc.call(server, context.Background(), uuid.NewString())
			if err == nil || !strings.Contains(err.Error(), "page_token") {
				t.Fatalf("expected the request to survive authorization and stop on the page token, got %v", err)
			}
			if len(authz.checks) != 0 {
				t.Fatalf("expected no authorization check, got %d", len(authz.checks))
			}
		})
	}
}

// A malformed identity is not an internal caller. It is rejected rather than
// waved through as one.
func TestListRPCsRejectAMalformedIdentity(t *testing.T) {
	for _, rpc := range listRPCs() {
		t.Run(rpc.name, func(t *testing.T) {
			authz := &recordingAuthorizationWriter{}
			server := &Server{authz: authz}

			if err := rpc.call(server, malformedIdentityContext(), uuid.NewString()); err == nil {
				t.Fatal("expected a malformed identity to be rejected")
			}
			if len(authz.checks) != 0 {
				t.Fatalf("expected no authorization check, got %d", len(authz.checks))
			}
		})
	}
}

// ListAgents and ListInstances leave organization_id optional so the
// Orchestrator can read across organizations. A caller presenting an identity
// must not reach that unscoped path by omitting it.
func TestListAgentsRequiresAnOrganizationFromAnIdentifiedCaller(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	if _, err := server.ListAgents(identityContext(uuid.New()), &agentsv1.ListAgentsRequest{}); err == nil {
		t.Fatal("expected a request without an organization to be rejected")
	}
	if len(authz.checks) != 0 {
		t.Fatalf("expected no authorization check, got %d", len(authz.checks))
	}
}

func TestListInstancesRequiresAnOrganizationFromAnIdentifiedCaller(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	if _, err := server.ListInstances(identityContext(uuid.New()), &agentsv1.ListInstancesRequest{}); err == nil {
		t.Fatal("expected a request without an organization to be rejected")
	}
	if len(authz.checks) != 0 {
		t.Fatalf("expected no authorization check, got %d", len(authz.checks))
	}
}

// The Orchestrator's desired-state query names no organization and reads every
// one of them.
func TestListInstancesLeavesTheInternalCallerUnscoped(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	scope, err := server.organizationListScope(context.Background(), "")
	if err != nil {
		t.Fatalf("organization list scope: %v", err)
	}
	if scope != nil {
		t.Fatalf("expected an unscoped read, got organization %s", scope)
	}
	if len(authz.checks) != 0 {
		t.Fatalf("expected no authorization check, got %d", len(authz.checks))
	}
}

func TestOrganizationListScopeRejectsAMalformedOrganization(t *testing.T) {
	server := &Server{authz: &recordingAuthorizationWriter{}}

	if _, err := server.organizationListScope(identityContext(uuid.New()), "not-a-uuid"); err == nil {
		t.Fatal("expected a malformed organization to be rejected")
	}
}
