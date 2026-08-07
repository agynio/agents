package server

import (
	"context"

	"errors"
	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/agynio/agents/internal/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) SetEnvironmentRole(ctx context.Context, req *agentsv1.SetEnvironmentRoleRequest) (*agentsv1.SetEnvironmentRoleResponse, error) {
	environmentID, err := parseUUID(req.GetEnvironmentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
	}
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	role, err := environmentRoleFromProto(req.GetRole())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "role: %v", err)
	}
	environment, err := s.store.GetEnvironment(ctx, environmentID)
	if err != nil {
		return nil, toStatusError(err)
	}
	callerID, err := identityUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requireEnvironmentRelation(ctx, callerID, environmentID, "can_manage_roles"); err != nil {
		return nil, err
	}
	// A role on an environment reaches its secrets and volume contents, so the
	// grantee must already belong to the organization that owns it.
	if err := s.requireOrganizationMember(ctx, identityID, environment.OrganizationID); err != nil {
		return nil, err
	}

	assignment := store.EnvironmentRoleAssignment{EnvironmentID: environmentID, IdentityID: identityID, Role: role}
	previous, err := s.store.UpsertEnvironmentRole(ctx, assignment)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.updateEnvironmentRoleAuthorization(ctx, environmentID, identityID, previous, role); err != nil {
		return nil, err
	}
	s.publishEnvironmentUpdated(ctx, environmentID, environment.OrganizationID)
	return &agentsv1.SetEnvironmentRoleResponse{Assignment: toProtoEnvironmentRoleAssignment(assignment)}, nil
}

func (s *Server) RemoveEnvironmentRole(ctx context.Context, req *agentsv1.RemoveEnvironmentRoleRequest) (*agentsv1.RemoveEnvironmentRoleResponse, error) {
	environmentID, err := parseUUID(req.GetEnvironmentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
	}
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	environment, err := s.store.GetEnvironment(ctx, environmentID)
	if err != nil {
		return nil, toStatusError(err)
	}
	callerID, err := identityUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requireEnvironmentRelation(ctx, callerID, environmentID, "can_manage_roles"); err != nil {
		return nil, err
	}
	role, err := s.store.DeleteEnvironmentRole(ctx, environmentID, identityID)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.removeEnvironmentRoleAuthorization(ctx, environmentID, identityID, role); err != nil {
		return nil, err
	}
	s.publishEnvironmentUpdated(ctx, environmentID, environment.OrganizationID)
	return &agentsv1.RemoveEnvironmentRoleResponse{}, nil
}

func (s *Server) ListEnvironmentRoles(ctx context.Context, req *agentsv1.ListEnvironmentRolesRequest) (*agentsv1.ListEnvironmentRolesResponse, error) {
	environmentID, err := parseUUID(req.GetEnvironmentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
	}
	if _, err := s.store.GetEnvironment(ctx, environmentID); err != nil {
		return nil, toStatusError(err)
	}
	callerID, err := identityUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requireEnvironmentRelation(ctx, callerID, environmentID, "can_manage_roles"); err != nil {
		return nil, err
	}
	assignments, err := s.store.ListEnvironmentRoles(ctx, environmentID)
	if err != nil {
		return nil, toStatusError(err)
	}
	response := &agentsv1.ListEnvironmentRolesResponse{Assignments: make([]*agentsv1.EnvironmentRoleAssignment, len(assignments))}
	for i, assignment := range assignments {
		response.Assignments[i] = toProtoEnvironmentRoleAssignment(assignment)
	}
	return response, nil
}

func environmentRoleFromProto(role agentsv1.EnvironmentRole) (store.EnvironmentRole, error) {
	switch role {
	case agentsv1.EnvironmentRole_ENVIRONMENT_ROLE_OWNER:
		return store.EnvironmentRoleOwner, nil
	case agentsv1.EnvironmentRole_ENVIRONMENT_ROLE_MAINTAINER:
		return store.EnvironmentRoleMaintainer, nil
	case agentsv1.EnvironmentRole_ENVIRONMENT_ROLE_USER:
		return store.EnvironmentRoleUser, nil
	default:
		return "", errUnknownEnvironmentRole
	}
}

func toProtoEnvironmentRoleAssignment(assignment store.EnvironmentRoleAssignment) *agentsv1.EnvironmentRoleAssignment {
	role := agentsv1.EnvironmentRole_ENVIRONMENT_ROLE_UNSPECIFIED
	switch assignment.Role {
	case store.EnvironmentRoleOwner:
		role = agentsv1.EnvironmentRole_ENVIRONMENT_ROLE_OWNER
	case store.EnvironmentRoleMaintainer:
		role = agentsv1.EnvironmentRole_ENVIRONMENT_ROLE_MAINTAINER
	case store.EnvironmentRoleUser:
		role = agentsv1.EnvironmentRole_ENVIRONMENT_ROLE_USER
	}
	return &agentsv1.EnvironmentRoleAssignment{
		EnvironmentId: assignment.EnvironmentID.String(),
		IdentityId:    assignment.IdentityID.String(),
		Role:          role,
	}
}

var errUnknownEnvironmentRole = errors.New("must be owner, maintainer or user")
