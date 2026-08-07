package server

import (
	"context"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
)

// `user` is the role that matters most: it opens an interactive shell onto the
// environment's secrets, egress credentials and volume contents, without any
// configuration access.
func TestEnvironmentRoleTuplesFollowTheGrant(t *testing.T) {
	environmentID := uuid.New()
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	if err := server.updateEnvironmentRoleAuthorization(context.Background(), environmentID, identityID, nil, store.EnvironmentRoleUser); err != nil {
		t.Fatalf("grant: %v", err)
	}
	request := singleWriteRequest(t, authz)
	assertTuples(t, request.GetWrites(), []*authorizationv1.TupleKey{
		{User: identityPrefix + identityID.String(), Relation: "user", Object: environmentPrefix + environmentID.String()},
	})
	if len(request.GetDeletes()) != 0 {
		t.Fatalf("expected no deletes on a first grant, got %d", len(request.GetDeletes()))
	}
}

// Changing a role replaces the tuple rather than adding a second one, or the
// identity would hold both.
func TestEnvironmentRoleChangeReplacesThePreviousTuple(t *testing.T) {
	environmentID := uuid.New()
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}
	previous := store.EnvironmentRoleUser

	if err := server.updateEnvironmentRoleAuthorization(context.Background(), environmentID, identityID, &previous, store.EnvironmentRoleMaintainer); err != nil {
		t.Fatalf("change: %v", err)
	}
	request := singleWriteRequest(t, authz)
	assertTuples(t, request.GetWrites(), []*authorizationv1.TupleKey{
		{User: identityPrefix + identityID.String(), Relation: "maintainer", Object: environmentPrefix + environmentID.String()},
	})
	assertTuples(t, request.GetDeletes(), []*authorizationv1.TupleKey{
		{User: identityPrefix + identityID.String(), Relation: "user", Object: environmentPrefix + environmentID.String()},
	})
}

// availability drives internal_access exactly as it does on agents: internal
// resolves every org member to can_use, private leaves only role holders.
func TestEnvironmentAvailabilityMovesInternalAccess(t *testing.T) {
	environmentID := uuid.New()
	organizationID := uuid.New()
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	if err := server.updateEnvironmentAvailabilityAuthorization(context.Background(), environmentID, organizationID,
		store.EnvironmentAvailabilityInternal, store.EnvironmentAvailabilityPrivate); err != nil {
		t.Fatalf("narrow: %v", err)
	}
	request := singleWriteRequest(t, authz)
	assertTuples(t, request.GetDeletes(), []*authorizationv1.TupleKey{
		{User: organizationPrefix + organizationID.String(), Relation: "internal_access", Object: environmentPrefix + environmentID.String()},
	})
}

func TestEnvironmentRoleFromProtoRejectsUnspecified(t *testing.T) {
	if _, err := environmentRoleFromProto(agentsv1.EnvironmentRole_ENVIRONMENT_ROLE_UNSPECIFIED); err == nil {
		t.Fatal("expected an unspecified role to be refused")
	}
	for _, role := range []agentsv1.EnvironmentRole{
		agentsv1.EnvironmentRole_ENVIRONMENT_ROLE_OWNER,
		agentsv1.EnvironmentRole_ENVIRONMENT_ROLE_MAINTAINER,
		agentsv1.EnvironmentRole_ENVIRONMENT_ROLE_USER,
	} {
		if _, err := environmentRoleFromProto(role); err != nil {
			t.Fatalf("role %v: %v", role, err)
		}
	}
}
