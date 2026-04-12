package server

import (
	"context"

	identityv1 "github.com/agynio/agents/.gen/go/agynio/api/identity/v1"
	"google.golang.org/grpc"
)

type IdentityWriter interface {
	RegisterIdentity(ctx context.Context, req *identityv1.RegisterIdentityRequest, opts ...grpc.CallOption) (*identityv1.RegisterIdentityResponse, error)
	SetNickname(ctx context.Context, req *identityv1.SetNicknameRequest, opts ...grpc.CallOption) (*identityv1.SetNicknameResponse, error)
	RemoveNickname(ctx context.Context, req *identityv1.RemoveNicknameRequest, opts ...grpc.CallOption) (*identityv1.RemoveNicknameResponse, error)
}

type identityWriter struct {
	client identityv1.IdentityServiceClient
}

func NewIdentityWriter(conn grpc.ClientConnInterface) IdentityWriter {
	if conn == nil {
		panic("identity connection is required")
	}
	return &identityWriter{client: identityv1.NewIdentityServiceClient(conn)}
}

func (w *identityWriter) RegisterIdentity(ctx context.Context, req *identityv1.RegisterIdentityRequest, opts ...grpc.CallOption) (*identityv1.RegisterIdentityResponse, error) {
	return w.client.RegisterIdentity(ctx, req, opts...)
}

func (w *identityWriter) SetNickname(ctx context.Context, req *identityv1.SetNicknameRequest, opts ...grpc.CallOption) (*identityv1.SetNicknameResponse, error) {
	return w.client.SetNickname(ctx, req, opts...)
}

func (w *identityWriter) RemoveNickname(ctx context.Context, req *identityv1.RemoveNicknameRequest, opts ...grpc.CallOption) (*identityv1.RemoveNicknameResponse, error) {
	return w.client.RemoveNickname(ctx, req, opts...)
}
