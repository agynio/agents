package server

import (
	"context"
	"errors"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/agynio/agents/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ResolveAgentExists reports whether an agent exists in an organization.
//
// It is a dedicated existence check rather than a GetAgent because the caller
// is another service validating a reference on behalf of an admin whose own
// read permissions on the agent are beside the point, and because it needs one
// boolean rather than a record it would then have to be trusted not to leak.
// Internal only: no OpenFGA check, reached over Istio.
//
// The organization is part of the question rather than a second call, so the
// check and the scoping cannot disagree. An agent in another organization
// answers false, exactly as a nonexistent one does.
func (s *Server) ResolveAgentExists(ctx context.Context, req *agentsv1.ResolveAgentExistsRequest) (*agentsv1.ResolveAgentExistsResponse, error) {
	id, err := parseUUID(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
	}
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	agent, err := s.store.GetAgent(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return &agentsv1.ResolveAgentExistsResponse{Exists: false}, nil
		}
		return nil, toStatusError(err)
	}
	return &agentsv1.ResolveAgentExistsResponse{Exists: agent.OrganizationID == organizationID}, nil
}

// ResolveEnvironmentExists is the environment counterpart of
// ResolveAgentExists, with the same contract.
func (s *Server) ResolveEnvironmentExists(ctx context.Context, req *agentsv1.ResolveEnvironmentExistsRequest) (*agentsv1.ResolveEnvironmentExistsResponse, error) {
	id, err := parseUUID(req.GetEnvironmentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
	}
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	environment, err := s.store.GetEnvironment(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return &agentsv1.ResolveEnvironmentExistsResponse{Exists: false}, nil
		}
		return nil, toStatusError(err)
	}
	return &agentsv1.ResolveEnvironmentExistsResponse{Exists: environment.OrganizationID == organizationID}, nil
}

func isNotFound(err error) bool {
	var notFound *store.NotFoundError
	return errors.As(err, &notFound)
}
