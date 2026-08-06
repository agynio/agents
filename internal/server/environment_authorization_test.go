package server

import (
	"context"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The environment RPCs used to authorize nothing at all: Create took an
// organization id and Get, Update and Delete took a bare environment id, so
// holding an id was the permission and it reached across organizations. What
// follows checks the decision each one now makes.
//
// Create refuses before touching the store, so these cases need no database. A
// caller meant to survive the check carries an unparseable runner id and stops
// on it — one step past the authorization and short of the store. Get, Update
// and Delete resolve the organization from the record itself and so are covered
// by the database-backed cases in environment_authorization_db_test.go.

func TestCreateEnvironmentRefusesWithoutIdentity(t *testing.T) {
	server := &Server{authz: &recordingAuthorizationWriter{}}

	_, err := server.CreateEnvironment(context.Background(), &agentsv1.CreateEnvironmentRequest{
		OrganizationId: uuid.NewString(),
		RunnerId:       uuid.NewString(),
		Name:           "alpha",
		Image:          "ghcr.io/agynio/environment:latest",
	})
	if err == nil {
		t.Fatal("CreateEnvironment accepted a caller carrying no identity")
	}
}

func TestCreateEnvironmentRefusesANonMember(t *testing.T) {
	organizationID := uuid.New()
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{}}
	server := &Server{authz: authz}

	_, err := server.CreateEnvironment(identityContext(identityID), &agentsv1.CreateEnvironmentRequest{
		OrganizationId: organizationID.String(),
		RunnerId:       uuid.NewString(),
		Name:           "alpha",
		Image:          "ghcr.io/agynio/environment:latest",
	})
	if err == nil {
		t.Fatal("CreateEnvironment accepted an identity holding nothing on the organization")
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
		{User: identityPrefix + identityID.String(), Relation: "member", Object: organizationPrefix + organizationID.String()},
	})
}

func TestCreateEnvironmentChecksMembershipBeforeTheStore(t *testing.T) {
	organizationID := uuid.New()
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	// The runner id is unparseable, so a caller that survives the membership
	// check stops there rather than reaching the store this server has none of.
	_, err := server.CreateEnvironment(identityContext(identityID), &agentsv1.CreateEnvironmentRequest{
		OrganizationId: organizationID.String(),
		RunnerId:       "not-a-uuid",
		Name:           "alpha",
		Image:          "ghcr.io/agynio/environment:latest",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected the request to reach runner_id validation, got %v", err)
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
		{User: identityPrefix + identityID.String(), Relation: "member", Object: organizationPrefix + organizationID.String()},
	})
}

// An internal caller reaches the service over the mesh carrying no identity and
// holds no tuples, so a check could only refuse it. Reads are served on those
// terms — the Orchestrator resolves an environment while assembling every
// workload — and writes are not, which is why Create requires an identity.
func TestRequireEnvironmentReadServesAnInternalCaller(t *testing.T) {
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{}}
	server := &Server{authz: authz}

	if err := server.requireEnvironmentRead(context.Background(), store.Environment{OrganizationID: uuid.New()}); err != nil {
		t.Fatalf("expected an identity-less caller to be served, got %v", err)
	}
	if len(authz.checks) != 0 {
		t.Fatalf("expected no authorization check, got %d", len(authz.checks))
	}
}

func TestRequireEnvironmentReadChecksAnIdentifiedCaller(t *testing.T) {
	organizationID := uuid.New()
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{}}
	server := &Server{authz: authz}

	err := server.requireEnvironmentRead(identityContext(identityID), store.Environment{OrganizationID: organizationID})
	if err == nil {
		t.Fatal("expected a non-member to be refused")
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{
		{User: identityPrefix + identityID.String(), Relation: "member", Object: organizationPrefix + organizationID.String()},
	})
}

func TestRequireEnvironmentWriteRefusesAnInternalCaller(t *testing.T) {
	server := &Server{authz: &recordingAuthorizationWriter{}}

	if err := server.requireEnvironmentWrite(context.Background(), store.Environment{OrganizationID: uuid.New()}); err == nil {
		t.Fatal("expected an identity-less caller to be refused a write")
	}
}
