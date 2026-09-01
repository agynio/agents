package server

import (
	"context"
	"log"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// teardownPageSize drains the paginated list endpoints while gathering the
// tuples to remove.
const teardownPageSize int32 = 200

// tupleDeleteBatchSize is OpenFGA's per-Write limit. An organization holds far
// more than that across its agents, instances, sandboxes, and environments, so
// the deletes go out in batches rather than one call that would be rejected
// whole.
const tupleDeleteBatchSize = 100

// DeleteOrganizationResources removes the organization's sandboxes, agent
// instances, agents and their sub-resources, and environments. It is internal:
// Istio settles who may call it, so there is no permission check and no caller
// identity to check against. Step 1 of the organization teardown -- first,
// because almost everything else in the organization is either referenced by
// these or refuses to be deleted while they exist.
//
// Tuples come off before the rows: a step interrupted after the rows are gone
// would have nothing left to enumerate the tuples from, and they would outlive
// the organization unreachable.
//
// Idempotent by construction: a retried step gathers nothing and deletes
// nothing.
func (s *Server) DeleteOrganizationResources(ctx context.Context, req *agentsv1.DeleteOrganizationResourcesRequest) (*agentsv1.DeleteOrganizationResourcesResponse, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}

	deletes, nicknamed, err := s.organizationTeardownTuples(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	for start := 0; start < len(deletes); start += tupleDeleteBatchSize {
		end := min(start+tupleDeleteBatchSize, len(deletes))
		if err := s.writeAuthorization(ctx, nil, deletes[start:end]); err != nil {
			return nil, status.Errorf(codes.Internal, "authorization delete failed: %v", err)
		}
	}

	// Each agent that took a nickname gives it back before Identity's own step
	// removes what is left. Best-effort: a nickname that outlives its agent is
	// a stale row in a table this teardown reaches again at step 8, not a
	// reason to stall step 1.
	for _, agentID := range nicknamed {
		if err := s.removeAgentNickname(ctx, agentID, organizationID); err != nil {
			log.Printf("agents: remove nickname during organization teardown (agent=%s org=%s): %v", agentID, organizationID, err)
		}
	}

	if err := s.store.DeleteOrganizationResources(ctx, organizationID); err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.DeleteOrganizationResourcesResponse{}, nil
}

// organizationTeardownTuples gathers every authorization tuple the
// organization's agent resources hold, and the agents whose nickname has to be
// given back.
func (s *Server) organizationTeardownTuples(ctx context.Context, organizationID uuid.UUID) ([]*authorizationv1.TupleKey, []uuid.UUID, error) {
	deletes := []*authorizationv1.TupleKey{}
	nicknamed := []uuid.UUID{}

	sandboxes, err := s.listAllSandboxes(ctx, organizationID)
	if err != nil {
		return nil, nil, err
	}
	for _, sandbox := range sandboxes {
		deletes = append(deletes,
			sandboxOrganizationTuple(sandbox.Meta.ID, organizationID),
			sandboxIdentityOrganizationMembershipTuple(sandbox.Meta.ID, organizationID),
			sandboxOwnerTuple(sandbox.Meta.ID, sandbox.OwnerID),
			sandboxHolderIdentityTuple(sandbox.Meta.ID),
			sandboxEnvironmentTuple(sandbox.Meta.ID, sandbox.EnvironmentID),
		)
	}

	instances, err := s.listAllAgentInstances(ctx, organizationID)
	if err != nil {
		return nil, nil, err
	}
	for _, instance := range instances {
		deletes = append(deletes,
			agentInstanceClassTuple(instance.Meta.ID, instance.AgentID),
			agentInstanceOrganizationTuple(instance.Meta.ID, organizationID),
			agentInstanceIdentityOrganizationMembershipTuple(instance.Meta.ID, organizationID),
			agentInstanceIdentityTuple(instance.Meta.ID, instance.AgentID),
		)
	}

	agents, err := s.listAllAgents(ctx, organizationID)
	if err != nil {
		return nil, nil, err
	}
	for _, agent := range agents {
		deletes = append(deletes,
			agentOrganizationTuple(agent.Meta.ID, organizationID),
			agentIdentityOrganizationMembershipTuple(agent.Meta.ID, organizationID),
		)
		if agent.Availability == store.AgentAvailabilityInternal {
			deletes = append(deletes, agentInternalAccessTuple(agent.Meta.ID, organizationID))
		}
		if agent.EnvironmentID != nil {
			deletes = append(deletes, agentEnvironmentTuple(agent.Meta.ID, *agent.EnvironmentID))
		}
		roles, err := s.store.ListAgentRoles(ctx, agent.Meta.ID)
		if err != nil {
			return nil, nil, toStatusError(err)
		}
		for _, role := range roles {
			deletes = append(deletes, agentRoleTuple(agent.Meta.ID, role.IdentityID, role.Role))
		}
		if agent.Nickname != "" {
			nicknamed = append(nicknamed, agent.Meta.ID)
		}
	}

	environments, err := s.listAllEnvironments(ctx, organizationID)
	if err != nil {
		return nil, nil, err
	}
	for _, environment := range environments {
		deletes = append(deletes, environmentOrganizationTuple(environment.Meta.ID, organizationID))
		if environment.Availability == store.EnvironmentAvailabilityInternal {
			deletes = append(deletes, environmentInternalAccessTuple(environment.Meta.ID, organizationID))
		}
		roles, err := s.store.ListEnvironmentRoles(ctx, environment.Meta.ID)
		if err != nil {
			return nil, nil, toStatusError(err)
		}
		for _, role := range roles {
			deletes = append(deletes, environmentRoleTuple(environment.Meta.ID, role.IdentityID, role.Role))
		}
	}

	return deletes, nicknamed, nil
}

func (s *Server) listAllSandboxes(ctx context.Context, organizationID uuid.UUID) ([]store.Sandbox, error) {
	all := []store.Sandbox{}
	var cursor *store.PageCursor
	for {
		result, err := s.store.ListSandboxes(ctx, &organizationID, store.SandboxFilter{}, teardownPageSize, cursor)
		if err != nil {
			return nil, toStatusError(err)
		}
		all = append(all, result.Sandboxes...)
		if result.NextCursor == nil {
			return all, nil
		}
		cursor = result.NextCursor
	}
}

func (s *Server) listAllAgentInstances(ctx context.Context, organizationID uuid.UUID) ([]store.AgentInstance, error) {
	all := []store.AgentInstance{}
	var cursor *store.PageCursor
	for {
		result, err := s.store.ListAgentInstances(ctx, store.AgentInstanceFilter{OrganizationID: &organizationID}, teardownPageSize, cursor)
		if err != nil {
			return nil, toStatusError(err)
		}
		all = append(all, result.Instances...)
		if result.NextCursor == nil {
			return all, nil
		}
		cursor = result.NextCursor
	}
}

func (s *Server) listAllAgents(ctx context.Context, organizationID uuid.UUID) ([]store.Agent, error) {
	all := []store.Agent{}
	var cursor *store.PageCursor
	for {
		result, err := s.store.ListAgents(ctx, &organizationID, store.AgentFilter{}, teardownPageSize, cursor)
		if err != nil {
			return nil, toStatusError(err)
		}
		all = append(all, result.Agents...)
		if result.NextCursor == nil {
			return all, nil
		}
		cursor = result.NextCursor
	}
}

func (s *Server) listAllEnvironments(ctx context.Context, organizationID uuid.UUID) ([]store.Environment, error) {
	all := []store.Environment{}
	var cursor *store.PageCursor
	for {
		result, err := s.store.ListEnvironments(ctx, organizationID, store.EnvironmentFilter{}, teardownPageSize, cursor)
		if err != nil {
			return nil, toStatusError(err)
		}
		all = append(all, result.Environments...)
		if result.NextCursor == nil {
			return all, nil
		}
		cursor = result.NextCursor
	}
}
