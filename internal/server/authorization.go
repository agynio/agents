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
	identityPrefix      = "identity:"
	organizationPrefix  = "organization:"
	agentPrefix         = "agent:"
	agentInstancePrefix = "agent_instance:"
	sandboxPrefix       = "sandbox:"
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

// organizationListScope settles which organization a list RPC reads, and
// authorizes it.
//
// An internal caller reaches these RPCs over the mesh rather than the Gateway
// and carries no identity by design — the Agents Orchestrator holds no OpenFGA
// tuples, so a check could only ever refuse it — and reads whatever scope it
// asked for, including none at all. A caller that does present an identity is a
// user request: it names the organization it is reading and must be a member of
// it, so it cannot reach the unscoped internal path by leaving the organization
// out. A malformed identity is still rejected.
//
// The returned scope is nil only for an internal caller that named no
// organization.
func (s *Server) organizationListScope(ctx context.Context, organizationID string) (*uuid.UUID, error) {
	identityID, hasIdentity, err := optionalIdentityUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if !hasIdentity && organizationID == "" {
		return nil, nil
	}
	parsed, err := parseUUID(organizationID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	if hasIdentity {
		if err := s.requireOrganizationMember(ctx, identityID, parsed); err != nil {
			return nil, err
		}
	}
	return &parsed, nil
}

// requireOrganizationListAccess authorizes a list RPC that always names the
// organization it reads, on the same terms as organizationListScope: an
// identified caller must be a member of that organization, an internal caller
// holds no tuples and is served.
func (s *Server) requireOrganizationListAccess(ctx context.Context, organizationID uuid.UUID) error {
	identityID, hasIdentity, err := optionalIdentityUUIDFromContext(ctx)
	if err != nil {
		return err
	}
	if !hasIdentity {
		return nil
	}
	return s.requireOrganizationMember(ctx, identityID, organizationID)
}

// requireEnvironmentRead authorizes reading one environment. An identified
// caller must be a member of its organization; an internal caller holds no
// tuples and is served, as the Orchestrator reads environments while assembling
// workloads.
func (s *Server) requireEnvironmentRead(ctx context.Context, environment store.Environment) error {
	return s.requireOrganizationListAccess(ctx, environment.OrganizationID)
}

// requireEnvironmentWrite authorizes changing an environment or anything it
// owns. These RPCs have no internal callers, so an identity is required.
func (s *Server) requireEnvironmentWrite(ctx context.Context, environment store.Environment) error {
	identityID, err := identityUUIDFromContext(ctx)
	if err != nil {
		return err
	}
	return s.requireOrganizationMember(ctx, identityID, environment.OrganizationID)
}

func (s *Server) requireOrganizationRelation(ctx context.Context, identityID uuid.UUID, organizationID uuid.UUID, relation string) error {
	return s.requireAllowed(ctx, identityID, relation, organizationPrefix+organizationID.String(), status.Errorf(codes.PermissionDenied, "identity lacks %s on organization", relation))
}

// requireOwnSandboxListing authorizes listing the caller's own sandboxes.
//
// Membership is the ordinary route. A cluster admin is not a member of every
// organization it administers, though, and refusing it here while it may list
// every sandbox in the same organization is incoherent — the narrower request
// would be denied and the broader one allowed. So holding can_list_sandboxes
// also satisfies this.
func (s *Server) requireOwnSandboxListing(ctx context.Context, identityID uuid.UUID, organizationID uuid.UUID) error {
	err := s.requireOrganizationMember(ctx, identityID, organizationID)
	if err == nil {
		return nil
	}
	if status.Code(err) != codes.InvalidArgument {
		return err
	}
	if listErr := s.requireOrganizationRelation(ctx, identityID, organizationID, "can_list_sandboxes"); listErr != nil {
		return err
	}
	return nil
}

func (s *Server) requireSandboxRelation(ctx context.Context, identityID uuid.UUID, sandboxID uuid.UUID, relation string) error {
	return s.requireAllowed(ctx, identityID, relation, sandboxPrefix+sandboxID.String(), status.Errorf(codes.PermissionDenied, "identity lacks %s on sandbox", relation))
}

func (s *Server) requireAgentCanInitiate(ctx context.Context, identityID uuid.UUID, agentID uuid.UUID) error {
	return s.requireAllowed(ctx, identityID, "can_initiate", agentPrefix+agentID.String(), status.Error(codes.PermissionDenied, "identity cannot initiate this agent"))
}

func (s *Server) requireAgentInstanceCanManage(ctx context.Context, identityID uuid.UUID, agentInstanceID uuid.UUID) error {
	return s.requireAllowed(ctx, identityID, "can_manage", agentInstancePrefix+agentInstanceID.String(), status.Error(codes.PermissionDenied, "identity cannot manage this agent instance"))
}

func (s *Server) requireAgentInstanceCanWriteInbox(ctx context.Context, identityID uuid.UUID, agentInstanceID uuid.UUID) error {
	return s.requireAllowed(ctx, identityID, "can_write_inbox", agentInstancePrefix+agentInstanceID.String(), status.Error(codes.PermissionDenied, "identity cannot write this agent instance inbox"))
}

func (s *Server) requireAllowed(ctx context.Context, identityID uuid.UUID, relation string, object string, denied error) error {
	response, err := s.authz.Check(ctx, &authorizationv1.CheckRequest{
		TupleKey: &authorizationv1.TupleKey{
			User:     identityPrefix + identityID.String(),
			Relation: relation,
			Object:   object,
		},
	})
	if err != nil {
		return err
	}
	if !response.GetAllowed() {
		return denied
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

func (s *Server) addAgentInstanceAuthorization(ctx context.Context, instance store.AgentInstance) error {
	return s.writeAuthorization(ctx, []*authorizationv1.TupleKey{
		agentInstanceClassTuple(instance.Meta.ID, instance.AgentID),
		agentInstanceOrganizationTuple(instance.Meta.ID, instance.OrganizationID),
		agentInstanceIdentityOrganizationMembershipTuple(instance.Meta.ID, instance.OrganizationID),
	}, nil)
}

func (s *Server) removeAgentInstanceAuthorization(ctx context.Context, instance store.AgentInstance) error {
	return s.writeAuthorization(ctx, nil, []*authorizationv1.TupleKey{
		agentInstanceClassTuple(instance.Meta.ID, instance.AgentID),
		agentInstanceOrganizationTuple(instance.Meta.ID, instance.OrganizationID),
		agentInstanceIdentityOrganizationMembershipTuple(instance.Meta.ID, instance.OrganizationID),
	})
}

func (s *Server) addSandboxAuthorization(ctx context.Context, sandboxID uuid.UUID, organizationID uuid.UUID, ownerID uuid.UUID) error {
	return s.writeAuthorization(ctx, []*authorizationv1.TupleKey{
		sandboxOrganizationTuple(sandboxID, organizationID),
		sandboxOwnerTuple(sandboxID, ownerID),
		sandboxIdentityOrganizationMembershipTuple(sandboxID, organizationID),
	}, nil)
}

func (s *Server) removeSandboxAuthorization(ctx context.Context, sandboxID uuid.UUID, organizationID uuid.UUID, ownerID uuid.UUID) error {
	return s.writeAuthorization(ctx, nil, []*authorizationv1.TupleKey{
		sandboxOrganizationTuple(sandboxID, organizationID),
		sandboxOwnerTuple(sandboxID, ownerID),
		sandboxIdentityOrganizationMembershipTuple(sandboxID, organizationID),
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

func agentInstanceClassTuple(instanceID uuid.UUID, agentID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     agentPrefix + agentID.String(),
		Relation: "class",
		Object:   agentInstancePrefix + instanceID.String(),
	}
}

func agentInstanceOrganizationTuple(instanceID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     organizationPrefix + organizationID.String(),
		Relation: "org",
		Object:   agentInstancePrefix + instanceID.String(),
	}
}

func agentInstanceIdentityOrganizationMembershipTuple(instanceID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     identityPrefix + instanceID.String(),
		Relation: "member",
		Object:   organizationPrefix + organizationID.String(),
	}
}

func sandboxOrganizationTuple(sandboxID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     organizationPrefix + organizationID.String(),
		Relation: "org",
		Object:   sandboxPrefix + sandboxID.String(),
	}
}

// sandboxIdentityOrganizationMembershipTuple makes the sandbox workload a
// member of its organization, the same way an agent workload is one.
//
// The workload authenticates as its own sandbox id, and without this it holds
// no relation at all: every service it dials would refuse it, because the
// checks they run — member on the organization — have nothing to resolve. The
// identity type is not what earns the access; the tuple is, exactly as for
// every other identity.
func sandboxIdentityOrganizationMembershipTuple(sandboxID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     identityPrefix + sandboxID.String(),
		Relation: "member",
		Object:   organizationPrefix + organizationID.String(),
	}
}

func sandboxOwnerTuple(sandboxID uuid.UUID, ownerID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     identityPrefix + ownerID.String(),
		Relation: "owner",
		Object:   sandboxPrefix + sandboxID.String(),
	}
}
