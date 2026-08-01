package server

import (
	"context"
	"testing"

	identityv1 "github.com/agynio/agents/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type recordingIdentityWriter struct {
	noopIdentityWriter
	caller string
}

func (r *recordingIdentityWriter) SetNickname(ctx context.Context, _ *identityv1.SetNicknameRequest, _ ...grpc.CallOption) (*identityv1.SetNicknameResponse, error) {
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if values := md.Get("x-identity-id"); len(values) == 1 {
			r.caller = values[0]
		}
	}
	return &identityv1.SetNicknameResponse{}, nil
}

// Identity lets a caller name itself with plain organization membership, and
// name anyone else only with can_manage_members. An instance is created from a
// thread, so forwarding the caller would present an ordinary participant --
// who has neither -- and no instance could ever be named.
func TestSetAgentInstanceNicknameNamesItself(t *testing.T) {
	identity := &recordingIdentityWriter{}
	server := New(&store.Store{}, noopAuthorizationWriter{}, identity, noopNotificationsClient{})

	instance := store.AgentInstance{
		Meta:           store.EntityMeta{ID: uuid.New()},
		OrganizationID: uuid.New(),
		Nickname:       "helper",
		Suffix:         "a1b2c3d4",
	}

	// A caller is present and is deliberately someone else: the instance still
	// has to be the one named.
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-identity-id", uuid.NewString()))
	if err := server.setAgentInstanceNickname(ctx, instance); err != nil {
		t.Fatalf("setAgentInstanceNickname failed: %v", err)
	}
	if identity.caller != instance.Meta.ID.String() {
		t.Fatalf("expected the instance %s to name itself, got %q", instance.Meta.ID, identity.caller)
	}
}

// The Orchestrator reaches instance creation over the mesh with no caller at
// all, which used to fail before the nickname call was even made.
func TestSetAgentInstanceNicknameWithoutACaller(t *testing.T) {
	identity := &recordingIdentityWriter{}
	server := New(&store.Store{}, noopAuthorizationWriter{}, identity, noopNotificationsClient{})

	instance := store.AgentInstance{
		Meta:           store.EntityMeta{ID: uuid.New()},
		OrganizationID: uuid.New(),
		Nickname:       "helper",
		Suffix:         "a1b2c3d4",
	}
	if err := server.setAgentInstanceNickname(context.Background(), instance); err != nil {
		t.Fatalf("setAgentInstanceNickname failed: %v", err)
	}
	if identity.caller != instance.Meta.ID.String() {
		t.Fatalf("expected the instance %s to name itself, got %q", instance.Meta.ID, identity.caller)
	}
}
