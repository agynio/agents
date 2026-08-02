package server

import (
	"strings"
	"testing"

	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The Orchestrator pauses instances the platform can no longer run -- the
// runner is gone, the volume is lost, start retries are spent -- and reaches
// Agents over the mesh with no identity of its own. Held to can_manage it was
// refused every one of those, because the identity it did present was the
// instance's, and an instance is not an owner of its own class.
func TestPauseServesTheInternalCallerWithoutAnIdentity(t *testing.T) {
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	if err := server.requireManageInstanceUnlessInternal(t.Context(), uuid.New()); err != nil {
		t.Fatalf("expected the internal caller to be served, got %v", err)
	}
	if len(authz.checks) != 0 {
		t.Fatalf("expected no authorization checks, got %d", len(authz.checks))
	}
}

// Presenting an identity makes the caller a principal, and a deliberate pause
// is still an owner's to make.
func TestPauseRefusesAnIdentityWithoutCanManage(t *testing.T) {
	instanceID := uuid.New()
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{allowedObjects: map[string]bool{}}
	server := &Server{authz: authz}

	err := server.requireManageInstanceUnlessInternal(identityContext(identityID), instanceID)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected an identified caller without can_manage to be refused, got %v", err)
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{{
		User:     identityPrefix + identityID.String(),
		Relation: "can_manage",
		Object:   agentInstancePrefix + instanceID.String(),
	}})
}

func TestPauseServesAnIdentityWithCanManage(t *testing.T) {
	instanceID := uuid.New()
	identityID := uuid.New()
	authz := &recordingAuthorizationWriter{}
	server := &Server{authz: authz}

	if err := server.requireManageInstanceUnlessInternal(identityContext(identityID), instanceID); err != nil {
		t.Fatalf("expected an owner to be served, got %v", err)
	}
	assertChecks(t, authz.checks, []*authorizationv1.TupleKey{{
		User:     identityPrefix + identityID.String(),
		Relation: "can_manage",
		Object:   agentInstancePrefix + instanceID.String(),
	}})
}

// A malformed identity is a broken caller, not an internal one: falling through
// to the unidentified path would turn a typo into an authorization bypass.
func TestPauseRejectsAMalformedIdentity(t *testing.T) {
	server := &Server{authz: &recordingAuthorizationWriter{}}

	err := server.requireManageInstanceUnlessInternal(malformedIdentityContext(), uuid.New())
	if err == nil || !strings.Contains(err.Error(), "identity_id") {
		t.Fatalf("expected a malformed identity to be rejected, got %v", err)
	}
}
