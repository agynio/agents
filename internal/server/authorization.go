package server

import (
	"context"

	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

const (
	identityPrefix     = "identity:"
	organizationPrefix = "organization:"
	memberRelation     = "member"
)

type AuthorizationWriter interface {
	Write(ctx context.Context, req *authorizationv1.WriteRequest, opts ...grpc.CallOption) (*authorizationv1.WriteResponse, error)
}

func (s *Server) addAgentMembership(ctx context.Context, agentID uuid.UUID, organizationID uuid.UUID) error {
	return s.writeAuthorization(ctx,
		[]*authorizationv1.TupleKey{authorizationTuple(agentID, organizationID)},
		nil,
	)
}

func (s *Server) removeAgentMembership(ctx context.Context, agentID uuid.UUID, organizationID uuid.UUID) error {
	return s.writeAuthorization(ctx,
		nil,
		[]*authorizationv1.TupleKey{authorizationTuple(agentID, organizationID)},
	)
}

func (s *Server) writeAuthorization(ctx context.Context, writes []*authorizationv1.TupleKey, deletes []*authorizationv1.TupleKey) error {
	_, err := s.authz.Write(ctx, &authorizationv1.WriteRequest{
		Writes:  writes,
		Deletes: deletes,
	})
	return err
}

func authorizationTuple(agentID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     identityPrefix + agentID.String(),
		Relation: memberRelation,
		Object:   organizationPrefix + organizationID.String(),
	}
}
