package server

import (
	"context"

	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	identityPrefix     = "identity:"
	organizationPrefix = "organization:"
	agentPrefix        = "agent:"
	sandboxPrefix      = "sandbox:"
)

type AuthorizationWriter interface {
	Check(ctx context.Context, req *authorizationv1.CheckRequest, opts ...grpc.CallOption) (*authorizationv1.CheckResponse, error)
	Write(ctx context.Context, req *authorizationv1.WriteRequest, opts ...grpc.CallOption) (*authorizationv1.WriteResponse, error)
}

func (s *Server) requireOrganizationMember(ctx context.Context, identityID uuid.UUID, organizationID uuid.UUID) error {
	response, err := s.authz.Check(ctx, &authorizationv1.CheckRequest{
		TupleKey: &authorizationv1.TupleKey{
			User:     identityPrefix + identityID.String(),
			Relation: "member",
			Object:   organizationPrefix + organizationID.String(),
		},
	})
	if err != nil {
		return err
	}
	if !response.GetAllowed() {
		return status.Error(codes.InvalidArgument, "identity is not a member of the agent organization")
	}
	return nil
}

func (s *Server) requireOrganizationRelation(ctx context.Context, identityID uuid.UUID, organizationID uuid.UUID, relation string) error {
	response, err := s.authz.Check(ctx, &authorizationv1.CheckRequest{
		TupleKey: &authorizationv1.TupleKey{
			User:     identityPrefix + identityID.String(),
			Relation: relation,
			Object:   organizationPrefix + organizationID.String(),
		},
	})
	if err != nil {
		return err
	}
	if !response.GetAllowed() {
		return status.Errorf(codes.PermissionDenied, "identity lacks %s on organization", relation)
	}
	return nil
}

func (s *Server) requireSandboxRelation(ctx context.Context, identityID uuid.UUID, sandboxID uuid.UUID, relation string) error {
	response, err := s.authz.Check(ctx, &authorizationv1.CheckRequest{
		TupleKey: &authorizationv1.TupleKey{
			User:     identityPrefix + identityID.String(),
			Relation: relation,
			Object:   sandboxPrefix + sandboxID.String(),
		},
	})
	if err != nil {
		return err
	}
	if !response.GetAllowed() {
		return status.Errorf(codes.PermissionDenied, "identity lacks %s on sandbox", relation)
	}
	return nil
}

func (s *Server) addAgentMembership(ctx context.Context, agentID uuid.UUID, organizationID uuid.UUID) error {
	return s.writeAuthorization(ctx,
		[]*authorizationv1.TupleKey{agentIdentityOrganizationMembershipTuple(agentID, organizationID)},
		nil,
	)
}

func (s *Server) removeAgentMembership(ctx context.Context, agentID uuid.UUID, organizationID uuid.UUID) error {
	return s.writeAuthorization(ctx,
		nil,
		[]*authorizationv1.TupleKey{agentIdentityOrganizationMembershipTuple(agentID, organizationID)},
	)
}

func (s *Server) addAgentAuthorization(ctx context.Context, agentID uuid.UUID, organizationID uuid.UUID, creatorID uuid.UUID, availability store.AgentAvailability) error {
	writes := []*authorizationv1.TupleKey{
		agentOrganizationTuple(agentID, organizationID),
		agentIdentityOrganizationMembershipTuple(agentID, organizationID),
		agentRoleTuple(agentID, creatorID, store.AgentRoleOwner),
	}
	if availability == store.AgentAvailabilityInternal {
		writes = append(writes, agentInternalAccessTuple(agentID, organizationID))
	}
	return s.writeAuthorization(ctx, writes, nil)
}

func (s *Server) removeAgentAuthorization(ctx context.Context, agentID uuid.UUID, organizationID uuid.UUID, roles []store.AgentRoleAssignment, availability store.AgentAvailability) error {
	deletes := []*authorizationv1.TupleKey{
		agentOrganizationTuple(agentID, organizationID),
		agentIdentityOrganizationMembershipTuple(agentID, organizationID),
	}
	if availability == store.AgentAvailabilityInternal {
		deletes = append(deletes, agentInternalAccessTuple(agentID, organizationID))
	}
	for _, role := range roles {
		deletes = append(deletes, agentRoleTuple(agentID, role.IdentityID, role.Role))
	}
	return s.writeAuthorization(ctx, nil, deletes)
}

func (s *Server) updateAgentAvailabilityAuthorization(ctx context.Context, agentID uuid.UUID, organizationID uuid.UUID, previous, next store.AgentAvailability) error {
	if previous == next {
		return nil
	}
	tuple := agentInternalAccessTuple(agentID, organizationID)
	if next == store.AgentAvailabilityInternal {
		return s.writeAuthorization(ctx, []*authorizationv1.TupleKey{tuple}, nil)
	}
	return s.writeAuthorization(ctx, nil, []*authorizationv1.TupleKey{tuple})
}

func (s *Server) updateAgentRoleAuthorization(ctx context.Context, agentID uuid.UUID, identityID uuid.UUID, previous *store.AgentRole, next store.AgentRole) error {
	writes := []*authorizationv1.TupleKey{agentRoleTuple(agentID, identityID, next)}
	var deletes []*authorizationv1.TupleKey
	if previous != nil && *previous != next {
		deletes = []*authorizationv1.TupleKey{agentRoleTuple(agentID, identityID, *previous)}
	}
	return s.writeAuthorization(ctx, writes, deletes)
}

func (s *Server) removeAgentRoleAuthorization(ctx context.Context, agentID uuid.UUID, identityID uuid.UUID, role store.AgentRole) error {
	return s.writeAuthorization(ctx, nil, []*authorizationv1.TupleKey{agentRoleTuple(agentID, identityID, role)})
}

func (s *Server) addSandboxAuthorization(ctx context.Context, sandboxID uuid.UUID, organizationID uuid.UUID, ownerID uuid.UUID) error {
	return s.writeAuthorization(ctx, []*authorizationv1.TupleKey{
		sandboxOrganizationTuple(sandboxID, organizationID),
		sandboxOwnerTuple(sandboxID, ownerID),
	}, nil)
}

func (s *Server) removeSandboxAuthorization(ctx context.Context, sandboxID uuid.UUID, organizationID uuid.UUID, ownerID uuid.UUID) error {
	return s.writeAuthorization(ctx, nil, []*authorizationv1.TupleKey{
		sandboxOrganizationTuple(sandboxID, organizationID),
		sandboxOwnerTuple(sandboxID, ownerID),
	})
}

func (s *Server) writeAuthorization(ctx context.Context, writes []*authorizationv1.TupleKey, deletes []*authorizationv1.TupleKey) error {
	_, err := s.authz.Write(ctx, &authorizationv1.WriteRequest{
		Writes:  writes,
		Deletes: deletes,
	})
	return err
}

func agentOrganizationTuple(agentID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     organizationPrefix + organizationID.String(),
		Relation: "org",
		Object:   agentPrefix + agentID.String(),
	}
}

func agentIdentityOrganizationMembershipTuple(agentID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     identityPrefix + agentID.String(),
		Relation: "member",
		Object:   organizationPrefix + organizationID.String(),
	}
}

func agentInternalAccessTuple(agentID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     organizationPrefix + organizationID.String(),
		Relation: "internal_access",
		Object:   agentPrefix + agentID.String(),
	}
}

func agentRoleTuple(agentID uuid.UUID, identityID uuid.UUID, role store.AgentRole) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     identityPrefix + identityID.String(),
		Relation: string(role),
		Object:   agentPrefix + agentID.String(),
	}
}

func sandboxOrganizationTuple(sandboxID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     organizationPrefix + organizationID.String(),
		Relation: "org",
		Object:   sandboxPrefix + sandboxID.String(),
	}
}

func sandboxOwnerTuple(sandboxID uuid.UUID, ownerID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     identityPrefix + ownerID.String(),
		Relation: "owner",
		Object:   sandboxPrefix + sandboxID.String(),
	}
}
