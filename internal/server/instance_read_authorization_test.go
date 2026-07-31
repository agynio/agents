package server

import (
	"strings"
	"testing"

	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
)

// GetInstance authorized nothing: any caller who knew an id read the instance,
// including its organization and the agent behind it. It now decides on the
// same terms ListInstances does, and these cover that decision. The RPC itself
// loads from a concrete store these tests do not stand up, so what is exercised
// is the check it delegates to, which takes the loaded instance as a value.
func instanceFor(organizationID uuid.UUID) store.AgentInstance {
	return store.AgentInstance{
		Meta:           store.EntityMeta{ID: uuid.New()},
		AgentID:        uuid.New(),
		OrganizationID: organizationID,
	}
}

func TestInstanceReadRefusesAnotherOrganization(t *testing.T) {
	callerOrganizationID := uuid.New()
	instance := instanceFor(uuid.New())
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{
		organizationPrefix + callerOrganizationID.String(): true,
	}}
	server := &Server{authz: authz}

	err := server.requireInstanceReadAccess(identityContext(identityID), instance)
	if err == nil {
		t.Fatal("expected a caller from another organization to be refused")
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
		organizationRelationTuple(identityID, instance.OrganizationID, "member"),
	})
}

func TestInstanceReadServesAMemberOfTheOrganization(t *testing.T) {
	instance := instanceFor(uuid.New())
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	if err := server.requireInstanceReadAccess(identityContext(identityID), instance); err != nil {
		t.Fatalf("expected a member to be served, got %v", err)
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
		organizationRelationTuple(identityID, instance.OrganizationID, "member"),
	})
}

// Threads and the Agents Orchestrator resolve instances over the mesh rather
// than the Gateway and carry no identity by design: they hold no tuples, so a
// check could only refuse them. Breaking this stops agents from sending.
func TestInstanceReadServesTheInternalCallerWithoutAnIdentity(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	if err := server.requireInstanceReadAccess(t.Context(), instanceFor(uuid.New())); err != nil {
		t.Fatalf("expected the internal caller to be served, got %v", err)
	}
	if len(authz.checks) != 0 {
		t.Fatalf("expected no authorization checks, got %d", len(authz.checks))
	}
}

// An instance is not a member of its organization, so membership alone would
// deny it its own record.
func TestInstanceReadServesTheInstanceItself(t *testing.T) {
	instance := instanceFor(uuid.New())
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	if err := server.requireInstanceReadAccess(identityContext(instance.Meta.ID), instance); err != nil {
		t.Fatalf("expected the instance to read itself, got %v", err)
	}
	if len(authz.checks) != 0 {
		t.Fatalf("expected no authorization checks, got %d", len(authz.checks))
	}
}

func TestInstanceReadRejectsAMalformedIdentity(t *testing.T) {
	server := &Server{authz: &recordingAuthorizationWriter{}}

	err := server.requireInstanceReadAccess(malformedIdentityContext(), instanceFor(uuid.New()))
	if err == nil || !strings.Contains(err.Error(), "identity_id") {
		t.Fatalf("expected a malformed identity to be rejected, got %v", err)
	}
}
