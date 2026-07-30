package server

import (
	"context"
	"testing"
	"time"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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

func TestAddSandboxAuthorizationWritesOrgAndOwner(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	sandboxID := uuid.New()
	organizationID := uuid.New()
	ownerID := uuid.New()

	if err := server.addSandboxAuthorization(context.Background(), sandboxID, organizationID, ownerID); err != nil {
		t.Fatalf("add sandbox authorization: %v", err)
	}

	request := singleWriteRequest(t, authz)
	assertTuples(t, request.GetWrites(), []*authorizationv1.TupleKey{
		sandboxOrganizationTuple(sandboxID, organizationID),
		sandboxOwnerTuple(sandboxID, ownerID),
		sandboxIdentityOrganizationMembershipTuple(sandboxID, organizationID),
	})
	assertTuples(t, request.GetDeletes(), nil)
}

// The workload's access comes from this tuple, not from its identity type, so
// assert the literal user/relation/object rather than re-deriving it.
func TestSandboxIdentityBecomesOrganizationMember(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	sandboxID := uuid.New()
	organizationID := uuid.New()

	if err := server.addSandboxAuthorization(context.Background(), sandboxID, organizationID, uuid.New()); err != nil {
		t.Fatalf("add sandbox authorization: %v", err)
	}

	var found *authorizationv1.TupleKey
	for _, tuple := range singleWriteRequest(t, authz).GetWrites() {
		if tuple.GetRelation() == "member" {
			found = tuple
		}
	}
	if found == nil {
		t.Fatal("expected the sandbox workload to be written as an organization member")
	}
	if found.GetUser() != "identity:"+sandboxID.String() {
		t.Fatalf("expected the sandbox's own identity, got %q", found.GetUser())
	}
	if found.GetObject() != "organization:"+organizationID.String() {
		t.Fatalf("expected membership of the sandbox organization, got %q", found.GetObject())
	}
}

func TestRemoveSandboxAuthorizationDeletesOrgAndOwner(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	sandboxID := uuid.New()
	organizationID := uuid.New()
	ownerID := uuid.New()

	if err := server.removeSandboxAuthorization(context.Background(), sandboxID, organizationID, ownerID); err != nil {
		t.Fatalf("remove sandbox authorization: %v", err)
	}

	request := singleWriteRequest(t, authz)
	assertTuples(t, request.GetWrites(), nil)
	assertTuples(t, request.GetDeletes(), []*authorizationv1.TupleKey{
		sandboxOrganizationTuple(sandboxID, organizationID),
		sandboxOwnerTuple(sandboxID, ownerID),
		sandboxIdentityOrganizationMembershipTuple(sandboxID, organizationID),
	})
}

func TestSandboxReadableWithoutCheck(t *testing.T) {
	sandboxID := uuid.New()
	ownerID := uuid.New()
	sandbox := store.Sandbox{Meta: store.EntityMeta{ID: sandboxID}, OwnerID: ownerID}

	if !sandboxReadableWithoutCheck(sandbox, ownerID) {
		t.Fatalf("expected the owner to read the sandbox without a check")
	}
	// The sandbox workload authenticates as its sandbox and holds no tuple; the
	// platform services it dials resolve it through this record.
	if !sandboxReadableWithoutCheck(sandbox, sandboxID) {
		t.Fatalf("expected the sandbox workload to read its own record")
	}
	if sandboxReadableWithoutCheck(sandbox, uuid.New()) {
		t.Fatalf("expected any other identity to need a check")
	}
}

func TestSandboxListFilterDefaultsToCallerOwner(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	organizationID := uuid.New()
	identityID := uuid.New()

	filter, err := server.sandboxListFilter(context.Background(), &agentsv1.ListSandboxesRequest{}, organizationID, identityID)
	if err != nil {
		t.Fatalf("sandbox list filter: %v", err)
	}
	if filter.OwnerID == nil || *filter.OwnerID != identityID {
		t.Fatalf("expected default owner filter %s, got %v", identityID, filter.OwnerID)
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
		organizationRelationTuple(identityID, organizationID, "member"),
	})
}

func TestSandboxListFilterAllowsOrgWideListAll(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	organizationID := uuid.New()
	identityID := uuid.New()
	allOwners := ""

	filter, err := server.sandboxListFilter(context.Background(), &agentsv1.ListSandboxesRequest{OwnerId: &allOwners}, organizationID, identityID)
	if err != nil {
		t.Fatalf("sandbox list filter: %v", err)
	}
	if filter.OwnerID != nil {
		t.Fatalf("expected no owner filter for list-all, got %v", filter.OwnerID)
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
		organizationRelationTuple(identityID, organizationID, "can_list_sandboxes"),
	})
}

func TestSandboxListFilterRequiresListPermissionForOtherOwner(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	organizationID := uuid.New()
	identityID := uuid.New()
	otherOwnerID := uuid.NewString()

	filter, err := server.sandboxListFilter(context.Background(), &agentsv1.ListSandboxesRequest{OwnerId: &otherOwnerID}, organizationID, identityID)
	if err != nil {
		t.Fatalf("sandbox list filter: %v", err)
	}
	if filter.OwnerID == nil || filter.OwnerID.String() != otherOwnerID {
		t.Fatalf("expected owner filter %s, got %v", otherOwnerID, filter.OwnerID)
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
		organizationRelationTuple(identityID, organizationID, "can_list_sandboxes"),
	})
}

type recordingAuthorizationWriter struct {
	writes []*authorizationv1.WriteRequest
	checks []*authorizationv1.CheckRequest
	// allowedObjects, when set, are the only objects a check is allowed against,
	// which is how a caller holding tuples somewhere else is expressed. A nil map
	// allows every check.
	allowedObjects map[string]bool
}

func (w *recordingAuthorizationWriter) Check(_ context.Context, req *authorizationv1.CheckRequest, _ ...grpc.CallOption) (*authorizationv1.CheckResponse, error) {
	w.checks = append(w.checks, req)
	allowed := w.allowedObjects == nil || w.allowedObjects[req.GetTupleKey().GetObject()]
	return &authorizationv1.CheckResponse{Allowed: allowed}, nil
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

func assertChecks(t *testing.T, actual []*authorizationv1.CheckRequest, expected []*authorizationv1.TupleKey) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("expected %d checks, got %d: %#v", len(expected), len(actual), actual)
	}
	for i := range expected {
		actualTuple := actual[i].GetTupleKey()
		if actualTuple.GetUser() != expected[i].GetUser() || actualTuple.GetRelation() != expected[i].GetRelation() || actualTuple.GetObject() != expected[i].GetObject() {
			t.Fatalf("check %d mismatch: expected user=%q relation=%q object=%q, got user=%q relation=%q object=%q",
				i,
				expected[i].GetUser(),
				expected[i].GetRelation(),
				expected[i].GetObject(),
				actualTuple.GetUser(),
				actualTuple.GetRelation(),
				actualTuple.GetObject(),
			)
		}
	}
}

func organizationRelationTuple(identityID uuid.UUID, organizationID uuid.UUID, relation string) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     identityPrefix + identityID.String(),
		Relation: relation,
		Object:   organizationPrefix + organizationID.String(),
	}
}

func TestAddAgentInstanceAuthorizationWritesClassOrgMembership(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	instance := store.AgentInstance{
		Meta:           store.EntityMeta{ID: uuid.New()},
		AgentID:        uuid.New(),
		OrganizationID: uuid.New(),
	}

	if err := server.addAgentInstanceAuthorization(context.Background(), instance); err != nil {
		t.Fatalf("add agent instance authorization: %v", err)
	}

	request := singleWriteRequest(t, authz)
	assertTuples(t, request.GetWrites(), []*authorizationv1.TupleKey{
		agentInstanceClassTuple(instance.Meta.ID, instance.AgentID),
		agentInstanceOrganizationTuple(instance.Meta.ID, instance.OrganizationID),
		agentInstanceIdentityOrganizationMembershipTuple(instance.Meta.ID, instance.OrganizationID),
	})
	assertTuples(t, request.GetDeletes(), nil)
}

func TestRemoveAgentInstanceAuthorizationDeletesClassOrgMembership(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	instance := store.AgentInstance{
		Meta:           store.EntityMeta{ID: uuid.New()},
		AgentID:        uuid.New(),
		OrganizationID: uuid.New(),
	}

	if err := server.removeAgentInstanceAuthorization(context.Background(), instance); err != nil {
		t.Fatalf("remove agent instance authorization: %v", err)
	}

	request := singleWriteRequest(t, authz)
	assertTuples(t, request.GetWrites(), nil)
	assertTuples(t, request.GetDeletes(), []*authorizationv1.TupleKey{
		agentInstanceClassTuple(instance.Meta.ID, instance.AgentID),
		agentInstanceOrganizationTuple(instance.Meta.ID, instance.OrganizationID),
		agentInstanceIdentityOrganizationMembershipTuple(instance.Meta.ID, instance.OrganizationID),
	})
}

func TestOptionalIdentityAbsentForInternalCaller(t *testing.T) {
	// The orchestrator reaches ListSandboxes over the mesh without an identity;
	// absence must be reported rather than rejected, so the RPC can serve both
	// the internal reconcile path and Gateway-fronted user requests.
	id, ok, err := optionalIdentityUUIDFromContext(context.Background())
	if err != nil {
		t.Fatalf("optional identity: %v", err)
	}
	if ok {
		t.Fatalf("expected no identity, got %s", id)
	}
}

func TestOptionalIdentityRejectsMalformedValue(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-identity-id", "not-a-uuid"))

	if _, _, err := optionalIdentityUUIDFromContext(ctx); err == nil {
		t.Fatal("expected malformed identity to be rejected")
	}
}

func TestOptionalIdentityReturnsCallerIdentity(t *testing.T) {
	identityID := uuid.New()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-identity-id", identityID.String()))

	got, ok, err := optionalIdentityUUIDFromContext(ctx)
	if err != nil {
		t.Fatalf("optional identity: %v", err)
	}
	if !ok || got != identityID {
		t.Fatalf("expected identity %s, got %s (ok=%v)", identityID, got, ok)
	}
}

func TestGetSandboxServesTheInternalCallerWithoutAnIdentity(t *testing.T) {
	// The Runners service resolves a sandbox-owned workload's owner through
	// GetSandbox, and the Orchestrator reads it while reconciling. Neither
	// carries an identity, and demanding one stalled every sandbox at starting.
	_, hasIdentity, err := optionalIdentityUUIDFromContext(context.Background())
	if err != nil {
		t.Fatalf("optional identity: %v", err)
	}
	if hasIdentity {
		t.Fatal("expected no identity for an internal caller")
	}
}
