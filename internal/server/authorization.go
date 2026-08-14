package server

import (
	"context"
	"errors"
	"strings"

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
	// The platform's own identity, and the relation that settles its claim. The
	// type is metadata and proves nothing on its own.
	platformIdentityType = "platform"
	clusterAdminRelation = "admin"
	clusterObject        = "cluster:global"
	agentPrefix         = "agent:"
	agentInstancePrefix = "agent_instance:"
	sandboxPrefix       = "sandbox:"
	environmentPrefix   = "environment:"
)

var errNoStore = errors.New("store is not configured")

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

// requireEnvironmentConfigRead authorizes reading what an environment contains,
// as distinct from its metadata: the volumes, servers, scripts and variables a
// workload there carries. An internal caller holds no tuples and is served, as
// the Orchestrator reads all of it while assembling workloads.
func (s *Server) requireEnvironmentConfigRead(ctx context.Context, environmentID uuid.UUID) error {
	identityID, hasIdentity, err := optionalIdentityUUIDFromContext(ctx)
	if err != nil {
		return err
	}
	if !hasIdentity {
		return nil
	}
	return s.requireEnvironmentRelation(ctx, identityID, environmentID, "can_read_config")
}

// requireEnvironmentWrite authorizes changing an environment or anything it
// owns. These RPCs have no internal callers, so an identity is required.
func (s *Server) requireEnvironmentWrite(ctx context.Context, environment store.Environment) error {
	identityID, err := identityUUIDFromContext(ctx)
	if err != nil {
		return err
	}
	return s.requireEnvironmentRelation(ctx, identityID, environment.Meta.ID, "can_edit_config")
}

// requireEnvironmentUse gates running a workload in an environment. A shell in
// a sandbox there reaches the environment's secrets, egress credentials and
// volume contents, so this is a grant rather than a consequence of visibility.
func (s *Server) requireEnvironmentUse(ctx context.Context, environmentID uuid.UUID) error {
	identityID, hasIdentity, err := optionalIdentityUUIDFromContext(ctx)
	if err != nil {
		return err
	}
	if !hasIdentity {
		return nil
	}
	return s.requireEnvironmentRelation(ctx, identityID, environmentID, "can_use")
}

func environmentOrganizationTuple(environmentID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     organizationPrefix + organizationID.String(),
		Relation: "org",
		Object:   environmentPrefix + environmentID.String(),
	}
}

func environmentInternalAccessTuple(environmentID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     organizationPrefix + organizationID.String(),
		Relation: "internal_access",
		Object:   environmentPrefix + environmentID.String(),
	}
}

func environmentRoleTuple(environmentID uuid.UUID, identityID uuid.UUID, role store.EnvironmentRole) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     identityPrefix + identityID.String(),
		Relation: string(role),
		Object:   environmentPrefix + environmentID.String(),
	}
}

func (s *Server) addEnvironmentAuthorization(ctx context.Context, environmentID, organizationID, creatorID uuid.UUID, availability store.EnvironmentAvailability) error {
	writes := []*authorizationv1.TupleKey{
		environmentOrganizationTuple(environmentID, organizationID),
		environmentRoleTuple(environmentID, creatorID, store.EnvironmentRoleOwner),
	}
	if availability == store.EnvironmentAvailabilityInternal {
		writes = append(writes, environmentInternalAccessTuple(environmentID, organizationID))
	}
	return s.writeAuthorization(ctx, writes, nil)
}

// addEnvironmentBaseAuthorization writes what an environment needs to resolve
// at all, without an owner. Used by the backfill, where no creator is recorded.
//
// OpenFGA refuses to write a tuple that already exists, and refuses the whole
// batch when one of them does, so each is written on its own and an existing one
// counts as success. Writing them together would mean an environment holding the
// org tuple but not internal_access could never gain the second.
func (s *Server) addEnvironmentBaseAuthorization(ctx context.Context, environmentID, organizationID uuid.UUID, availability store.EnvironmentAvailability) error {
	tuples := []*authorizationv1.TupleKey{environmentOrganizationTuple(environmentID, organizationID)}
	if availability == store.EnvironmentAvailabilityInternal {
		tuples = append(tuples, environmentInternalAccessTuple(environmentID, organizationID))
	}
	for _, tuple := range tuples {
		if err := s.writeAuthorization(ctx, []*authorizationv1.TupleKey{tuple}, nil); err != nil {
			if isTupleAlreadyExists(err) {
				continue
			}
			return err
		}
	}
	return nil
}

// isTupleAlreadyExists reports the write OpenFGA refuses because the tuple is
// already there, which for a repair pass is the desired end state.
func isTupleAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "already exists") ||
		strings.Contains(message, "write_failed_due_to_invalid_input")
}

func (s *Server) removeEnvironmentAuthorization(ctx context.Context, environmentID, organizationID uuid.UUID, roles []store.EnvironmentRoleAssignment, availability store.EnvironmentAvailability) error {
	deletes := []*authorizationv1.TupleKey{environmentOrganizationTuple(environmentID, organizationID)}
	if availability == store.EnvironmentAvailabilityInternal {
		deletes = append(deletes, environmentInternalAccessTuple(environmentID, organizationID))
	}
	for _, role := range roles {
		deletes = append(deletes, environmentRoleTuple(environmentID, role.IdentityID, role.Role))
	}
	return s.writeAuthorization(ctx, nil, deletes)
}

func (s *Server) updateEnvironmentAvailabilityAuthorization(ctx context.Context, environmentID, organizationID uuid.UUID, previous, next store.EnvironmentAvailability) error {
	if previous == next {
		return nil
	}
	tuple := environmentInternalAccessTuple(environmentID, organizationID)
	if next == store.EnvironmentAvailabilityInternal {
		return s.writeAuthorization(ctx, []*authorizationv1.TupleKey{tuple}, nil)
	}
	return s.writeAuthorization(ctx, nil, []*authorizationv1.TupleKey{tuple})
}

func (s *Server) updateEnvironmentRoleAuthorization(ctx context.Context, environmentID, identityID uuid.UUID, previous *store.EnvironmentRole, next store.EnvironmentRole) error {
	var deletes []*authorizationv1.TupleKey
	if previous != nil {
		if *previous == next {
			return nil
		}
		deletes = append(deletes, environmentRoleTuple(environmentID, identityID, *previous))
	}
	return s.writeAuthorization(ctx, []*authorizationv1.TupleKey{environmentRoleTuple(environmentID, identityID, next)}, deletes)
}

func (s *Server) removeEnvironmentRoleAuthorization(ctx context.Context, environmentID, identityID uuid.UUID, role store.EnvironmentRole) error {
	return s.writeAuthorization(ctx, nil, []*authorizationv1.TupleKey{environmentRoleTuple(environmentID, identityID, role)})
}

func (s *Server) requireEnvironmentRelation(ctx context.Context, identityID uuid.UUID, environmentID uuid.UUID, relation string) error {
	return s.requireAllowed(ctx, identityID, relation, environmentPrefix+environmentID.String(),
		status.Errorf(codes.PermissionDenied, "identity lacks %s on environment", relation))
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

// addAgentInstanceAuthorization also grants the running instance the config it
// was started from: agynd reads its agent's skills and its environment's init
// scripts as the instance's own identity, and without these it is refused both
// and the workload exits before serving.
//
// The agent's environment tuple is written here rather than at agent creation so
// the grant repairs itself: an agent that predates the relation gains it the
// next time it runs. That makes a repeat write the normal case, and OpenFGA
// refuses a batch when any member already exists, so they go one at a time.
func (s *Server) addAgentInstanceAuthorization(ctx context.Context, instance store.AgentInstance) error {
	tuples := []*authorizationv1.TupleKey{
		agentInstanceClassTuple(instance.Meta.ID, instance.AgentID),
		agentInstanceOrganizationTuple(instance.Meta.ID, instance.OrganizationID),
		agentInstanceIdentityOrganizationMembershipTuple(instance.Meta.ID, instance.OrganizationID),
		agentInstanceIdentityTuple(instance.Meta.ID, instance.AgentID),
	}
	// Null on agents written before environments existed, which carry their own
	// image and so have no environment config to read.
	if agent, err := s.agentForAuthorization(ctx, instance.AgentID); err == nil && agent.EnvironmentID != nil {
		tuples = append(tuples, agentEnvironmentTuple(instance.AgentID, *agent.EnvironmentID))
	}
	for _, tuple := range tuples {
		if err := s.writeAuthorization(ctx, []*authorizationv1.TupleKey{tuple}, nil); err != nil {
			if isTupleAlreadyExists(err) {
				continue
			}
			return err
		}
	}
	return nil
}

// The agent's environment tuple is deliberately left: it belongs to the agent,
// which outlives this instance and has other instances still reading it.
func (s *Server) removeAgentInstanceAuthorization(ctx context.Context, instance store.AgentInstance) error {
	return s.writeAuthorization(ctx, nil, []*authorizationv1.TupleKey{
		agentInstanceClassTuple(instance.Meta.ID, instance.AgentID),
		agentInstanceOrganizationTuple(instance.Meta.ID, instance.OrganizationID),
		agentInstanceIdentityOrganizationMembershipTuple(instance.Meta.ID, instance.OrganizationID),
		agentInstanceIdentityTuple(instance.Meta.ID, instance.AgentID),
	})
}

func (s *Server) addSandboxAuthorization(ctx context.Context, sandboxID uuid.UUID, organizationID uuid.UUID, ownerID uuid.UUID, environmentID uuid.UUID) error {
	return s.writeAuthorization(ctx, []*authorizationv1.TupleKey{
		sandboxOrganizationTuple(sandboxID, organizationID),
		sandboxOwnerTuple(sandboxID, ownerID),
		sandboxIdentityOrganizationMembershipTuple(sandboxID, organizationID),
		sandboxHolderIdentityTuple(sandboxID),
		sandboxEnvironmentTuple(sandboxID, environmentID),
	}, nil)
}

func (s *Server) removeSandboxAuthorization(ctx context.Context, sandboxID uuid.UUID, organizationID uuid.UUID, ownerID uuid.UUID, environmentID uuid.UUID) error {
	return s.writeAuthorization(ctx, nil, []*authorizationv1.TupleKey{
		sandboxOrganizationTuple(sandboxID, organizationID),
		sandboxOwnerTuple(sandboxID, ownerID),
		sandboxIdentityOrganizationMembershipTuple(sandboxID, organizationID),
		sandboxHolderIdentityTuple(sandboxID),
		sandboxEnvironmentTuple(sandboxID, environmentID),
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

// agentForAuthorization reads the agent an instance belongs to, and tolerates a
// Server built without a store so the tuple writes stay unit-testable.
func (s *Server) agentForAuthorization(ctx context.Context, agentID uuid.UUID) (store.Agent, error) {
	if s.store == nil {
		return store.Agent{}, errNoStore
	}
	return s.store.GetAgent(ctx, agentID)
}

// agentInstanceIdentityTuple names the identity a running instance
// authenticates as, which is how it reads the config it was started from.
func agentInstanceIdentityTuple(instanceID uuid.UUID, agentID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     identityPrefix + instanceID.String(),
		Relation: "instance",
		Object:   agentPrefix + agentID.String(),
	}
}

// agentEnvironmentTuple reaches the environment's config from the agents it
// supplies, so an instance reads the init scripts its workload carries.
func agentEnvironmentTuple(agentID uuid.UUID, environmentID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     agentPrefix + agentID.String(),
		Relation: "agent",
		Object:   environmentPrefix + environmentID.String(),
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

// sandboxHolderIdentityTuple names the identity the sandbox workload
// authenticates as, which is how it reads the environment it was started from.
// Distinct from the owner: the workload holds this for its whole life, and the
// person who started it is not who is asking.
func sandboxHolderIdentityTuple(sandboxID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     identityPrefix + sandboxID.String(),
		Relation: "holder",
		Object:   sandboxPrefix + sandboxID.String(),
	}
}

// sandboxEnvironmentTuple reaches the environment's config from the sandboxes
// started in it, so a holder reads the init scripts it must run. The mirror of
// agentEnvironmentTuple, for the workload that carries no agent.
func sandboxEnvironmentTuple(sandboxID uuid.UUID, environmentID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{
		User:     sandboxPrefix + sandboxID.String(),
		Relation: "sandbox",
		Object:   environmentPrefix + environmentID.String(),
	}
}

func identityTypeFromContext(ctx context.Context) string {
	identityType, ok := metadataValueFromIncomingContext(ctx, "x-identity-type")
	if !ok {
		return ""
	}
	return strings.TrimSpace(identityType)
}
