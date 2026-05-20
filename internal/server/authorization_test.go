package server

import (
	"context"
	"testing"
	"time"

	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

func TestAddAgentAuthorizationWritesAgentOrganizationMembership(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	agentID := uuid.New()
	organizationID := uuid.New()
	creatorID := uuid.New()

	if err := server.addAgentAuthorization(context.Background(), agentID, organizationID, creatorID, store.AgentAvailabilityPrivate); err != nil {
		t.Fatalf("add agent authorization: %v", err)
	}

	request := singleWriteRequest(t, authz)
	assertTuples(t, request.GetWrites(), []*authorizationv1.TupleKey{
		agentOrganizationTuple(agentID, organizationID),
		agentIdentityOrganizationMembershipTuple(agentID, organizationID),
		agentRoleTuple(agentID, creatorID, store.AgentRoleOwner),
	})
	assertTuples(t, request.GetDeletes(), nil)
}

func TestRemoveAgentAuthorizationDeletesAgentOrganizationMembership(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	agentID := uuid.New()
	organizationID := uuid.New()
	ownerID := uuid.New()
	participantID := uuid.New()
	roles := []store.AgentRoleAssignment{
		{AgentID: agentID, IdentityID: ownerID, Role: store.AgentRoleOwner},
		{AgentID: agentID, IdentityID: participantID, Role: store.AgentRoleParticipant},
	}

	if err := server.removeAgentAuthorization(context.Background(), agentID, organizationID, roles, store.AgentAvailabilityPrivate); err != nil {
		t.Fatalf("remove agent authorization: %v", err)
	}

	request := singleWriteRequest(t, authz)
	assertTuples(t, request.GetWrites(), nil)
	assertTuples(t, request.GetDeletes(), []*authorizationv1.TupleKey{
		agentOrganizationTuple(agentID, organizationID),
		agentIdentityOrganizationMembershipTuple(agentID, organizationID),
		agentRoleTuple(agentID, ownerID, store.AgentRoleOwner),
		agentRoleTuple(agentID, participantID, store.AgentRoleParticipant),
	})
}

func TestRestoreAgentAuthorizationWritesAgentOrganizationMembership(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	agentID := uuid.New()
	organizationID := uuid.New()
	ownerID := uuid.New()
	agent := store.Agent{
		Meta: store.EntityMeta{
			ID:        agentID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		OrganizationID: organizationID,
		Availability:   store.AgentAvailabilityInternal,
	}
	roles := []store.AgentRoleAssignment{{AgentID: agentID, IdentityID: ownerID, Role: store.AgentRoleOwner}}

	if err := server.restoreAgentAuthorization(context.Background(), agent, roles); err != nil {
		t.Fatalf("restore agent authorization: %v", err)
	}

	request := singleWriteRequest(t, authz)
	assertTuples(t, request.GetWrites(), []*authorizationv1.TupleKey{
		agentOrganizationTuple(agentID, organizationID),
		agentIdentityOrganizationMembershipTuple(agentID, organizationID),
		agentInternalAccessTuple(agentID, organizationID),
		agentRoleTuple(agentID, ownerID, store.AgentRoleOwner),
	})
	assertTuples(t, request.GetDeletes(), nil)
}

type recordingAuthorizationWriter struct {
	writes []*authorizationv1.WriteRequest
}

func (w *recordingAuthorizationWriter) Check(context.Context, *authorizationv1.CheckRequest, ...grpc.CallOption) (*authorizationv1.CheckResponse, error) {
	return &authorizationv1.CheckResponse{Allowed: true}, nil
}

func (w *recordingAuthorizationWriter) Write(_ context.Context, req *authorizationv1.WriteRequest, _ ...grpc.CallOption) (*authorizationv1.WriteResponse, error) {
	w.writes = append(w.writes, req)
	return &authorizationv1.WriteResponse{}, nil
}

func singleWriteRequest(t *testing.T, authz *recordingAuthorizationWriter) *authorizationv1.WriteRequest {
	t.Helper()
	if len(authz.writes) != 1 {
		t.Fatalf("expected 1 authorization write, got %d", len(authz.writes))
	}
	return authz.writes[0]
}

func assertTuples(t *testing.T, actual []*authorizationv1.TupleKey, expected []*authorizationv1.TupleKey) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("expected %d tuples, got %d: %#v", len(expected), len(actual), actual)
	}
	for i := range expected {
		if actual[i].GetUser() != expected[i].GetUser() || actual[i].GetRelation() != expected[i].GetRelation() || actual[i].GetObject() != expected[i].GetObject() {
			t.Fatalf("tuple %d mismatch: expected user=%q relation=%q object=%q, got user=%q relation=%q object=%q",
				i,
				expected[i].GetUser(),
				expected[i].GetRelation(),
				expected[i].GetObject(),
				actual[i].GetUser(),
				actual[i].GetRelation(),
				actual[i].GetObject(),
			)
		}
	}
}
