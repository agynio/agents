package server

import (
	"context"
	"errors"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	identityv1 "github.com/agynio/agents/.gen/go/agynio/api/identity/v1"
	"google.golang.org/grpc"
)

type nicknameIdentityWriter struct {
	noopIdentityWriter
	entries []*identityv1.NicknameEntry
	err     error
	request *identityv1.BatchGetNicknamesRequest
}

func (w *nicknameIdentityWriter) BatchGetNicknames(_ context.Context, req *identityv1.BatchGetNicknamesRequest, _ ...grpc.CallOption) (*identityv1.BatchGetNicknamesResponse, error) {
	w.request = req
	if w.err != nil {
		return nil, w.err
	}
	return &identityv1.BatchGetNicknamesResponse{Entries: w.entries}, nil
}

func TestSenderHandlesAreAttached(t *testing.T) {
	suffix := "a1b2"
	identity := &nicknameIdentityWriter{entries: []*identityv1.NicknameEntry{
		{IdentityId: "sender-1", Nickname: "rowan"},
		{IdentityId: "sender-2", Nickname: "casey", InstanceSuffix: &suffix},
	}}
	server := &Server{identity: identity}
	items := []*agentsv1.InboxItem{
		{SenderId: "sender-1"},
		{SenderId: "sender-2"},
		{SenderId: "sender-3"},
	}

	server.applySenderHandles(items, identity.entries)

	if got := items[0].GetSenderHandle(); got != "rowan" {
		t.Fatalf("expected rowan, got %q", got)
	}
	if got := items[1].GetSenderHandle(); got != "casey#a1b2" {
		t.Fatalf("expected casey#a1b2, got %q", got)
	}
	if items[2].SenderHandle != nil {
		t.Fatalf("expected no handle for an unknown sender, got %q", items[2].GetSenderHandle())
	}
}

func TestSenderHandlesSkipEmptyNicknames(t *testing.T) {
	server := &Server{identity: &nicknameIdentityWriter{}}
	items := []*agentsv1.InboxItem{{SenderId: "sender-1"}}

	server.applySenderHandles(items, []*identityv1.NicknameEntry{{IdentityId: "sender-1", Nickname: ""}})

	if items[0].SenderHandle != nil {
		t.Fatalf("expected no handle, got %q", items[0].GetSenderHandle())
	}
}

func TestSenderHandleLookupFailureLeavesItemsIntact(t *testing.T) {
	identity := &nicknameIdentityWriter{err: errors.New("identity is down")}
	server := &Server{identity: identity}
	items := []*agentsv1.InboxItem{{SenderId: "sender-1", Body: "hello"}}

	handles, err := server.senderHandles(context.Background(), "org-1", items)
	if err == nil {
		t.Fatal("expected the lookup error to be reported")
	}
	if handles != nil {
		t.Fatalf("expected no entries, got %v", handles)
	}
	if items[0].GetBody() != "hello" {
		t.Fatal("expected the item to be untouched")
	}
}

func TestSenderHandlesRequestUniqueSenders(t *testing.T) {
	identity := &nicknameIdentityWriter{}
	server := &Server{identity: identity}
	items := []*agentsv1.InboxItem{
		{SenderId: "sender-1"},
		{SenderId: "sender-1"},
		{SenderId: ""},
		{SenderId: "sender-2"},
	}

	if _, err := server.senderHandles(context.Background(), "org-1", items); err != nil {
		t.Fatalf("sender handles: %v", err)
	}
	if got := identity.request.GetIdentityIds(); len(got) != 2 {
		t.Fatalf("expected two unique senders, got %v", got)
	}
}
