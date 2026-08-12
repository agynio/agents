package server

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	identityv1 "github.com/agynio/agents/.gen/go/agynio/api/identity/v1"
	notificationsv1 "github.com/agynio/agents/.gen/go/agynio/api/notifications/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Server struct {
	agentsv1.UnimplementedAgentsServiceServer
	store         *store.Store
	authz         AuthorizationWriter
	identity      IdentityWriter
	notifications notificationsv1.NotificationsServiceClient
	// Optional: without it, image references are stored unvalidated and the
	// orchestrator resolves them again at workload start.
	images ImagesClient
	// Required by CreateSandbox, which reads the organization's sandbox
	// lifecycle bounds rather than assuming a platform-wide number.
	organizations OrganizationsClient
}

func (s *Server) WithOrganizations(client OrganizationsClient) {
	s.organizations = client
}

const (
	maxMcpNameLength   = 63
	defaultIdleTimeout = "5m"
)

var mcpNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func New(store *store.Store, authz AuthorizationWriter, identity IdentityWriter, notifications notificationsv1.NotificationsServiceClient) *Server {
	if store == nil {
		panic("store is required")
	}
	if authz == nil {
		panic("authorization client is required")
	}
	if identity == nil {
		panic("identity client is required")
	}
	if notifications == nil {
		panic("notifications client is required")
	}
	return &Server{store: store, authz: authz, identity: identity, notifications: notifications}
}

// WithImages enables catalog validation on write.
func (s *Server) WithImages(images ImagesClient) *Server {
	s.images = images
	return s
}

func (s *Server) registerAgentIdentity(ctx context.Context, agentID uuid.UUID) error {
	_, err := s.identity.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{
		IdentityId:   agentID.String(),
		IdentityType: identityv1.IdentityType_IDENTITY_TYPE_AGENT,
	})
	return err
}

func metadataValueFromIncomingContext(ctx context.Context, key string) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	values := md.Get(key)
	if len(values) == 0 || values[0] == "" {
		return "", false
	}
	return values[0], true
}

func identityIDFromContext(ctx context.Context) (string, error) {
	identityID, ok := metadataValueFromIncomingContext(ctx, "x-identity-id")
	if !ok {
		return "", status.Error(codes.Unauthenticated, "identity not available: x-identity-id not found in metadata")
	}
	return identityID, nil
}

func identityOutgoingContext(ctx context.Context) (context.Context, error) {
	identityID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return metadata.AppendToOutgoingContext(ctx, "x-identity-id", identityID), nil
}

func (s *Server) setAgentNickname(ctx context.Context, agentID uuid.UUID, organizationID uuid.UUID, nickname string) error {
	identityCtx, err := identityOutgoingContext(ctx)
	if err != nil {
		return err
	}
	_, err = s.identity.SetNickname(identityCtx, &identityv1.SetNicknameRequest{
		OrganizationId: organizationID.String(),
		IdentityId:     agentID.String(),
		Nickname:       nickname,
	})
	return err
}

func (s *Server) removeAgentNickname(ctx context.Context, agentID uuid.UUID, organizationID uuid.UUID) error {
	identityCtx, err := identityOutgoingContext(ctx)
	if err != nil {
		return err
	}
	_, err = s.identity.RemoveNickname(identityCtx, &identityv1.RemoveNicknameRequest{
		OrganizationId: organizationID.String(),
		IdentityId:     agentID.String(),
	})
	return err
}

// environmentInOrganization resolves an environment_id and refuses one naming an
// environment in another organization. The composite foreign key on agents
// refuses that too, but the violation would reach the caller as an opaque
// internal error rather than naming the field that was wrong.
func (s *Server) environmentInOrganization(ctx context.Context, value string, organizationID uuid.UUID) (uuid.UUID, error) {
	environmentID, err := parseUUID(value)
	if err != nil {
		return uuid.UUID{}, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
	}
	environment, err := s.store.GetEnvironment(ctx, environmentID)
	if err != nil {
		var notFound *store.NotFoundError
		if errors.As(err, &notFound) {
			return uuid.UUID{}, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
		}
		return uuid.UUID{}, toStatusError(err)
	}
	if environment.OrganizationID != organizationID {
		return uuid.UUID{}, status.Error(codes.InvalidArgument, "environment_id: environment belongs to another organization")
	}
	// Pointing an agent at an environment runs its workloads there, reaching the
	// same secrets, egress credentials and volume contents a sandbox would, so
	// it takes the same grant.
	if err := s.requireEnvironmentUse(ctx, environmentID); err != nil {
		return uuid.UUID{}, err
	}
	return environmentID, nil
}

func (s *Server) CreateAgent(ctx context.Context, req *agentsv1.CreateAgentRequest) (*agentsv1.CreateAgentResponse, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	// init_image is required only for the deprecated inline path. An agent
	// running an environment takes its agent CLI from that environment's agent
	// runtime image instead.
	if req.GetInitImage() == "" && req.GetEnvironmentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "init_image is required when no environment is named")
	}
	availability, err := agentAvailabilityFromProto(req.GetAvailability())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "availability: %v", err)
	}
	creatorIDValue, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	creatorID, err := parseUUID(creatorIDValue)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	idleTimeout := defaultIdleTimeout
	if req.IdleTimeout != nil {
		idleTimeout = req.GetIdleTimeout()
	}
	if err := validateDurationString(idleTimeout); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "idle_timeout: %v", err)
	}
	// Unset rather than defaulted: no limit is the honest answer for an agent
	// whose author did not name one, and it is what every agent predating the
	// column already behaves as.
	var instanceIdleTTL *string
	if req.InstanceIdleTtl != nil {
		value := req.GetInstanceIdleTtl()
		if err := validateDurationString(value); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "instance_idle_ttl: %v", err)
		}
		instanceIdleTTL = &value
	}
	// Optional: an agent may name no environment and run from the deprecated
	// inline image and resources instead.
	var environmentID *uuid.UUID
	// An agent without an environment predates modes, so it is a platform one.
	llmMode := store.LLMModePlatform
	if req.GetEnvironmentId() != "" {
		resolved, err := s.environmentInOrganization(ctx, req.GetEnvironmentId(), organizationID)
		if err != nil {
			return nil, err
		}
		// An agent needs an agent CLI to run. An environment that names a
		// catalog workspace image but no agent runtime is workspace-only:
		// usable by a sandbox, not by an agent. Environments still on the
		// free-form image carry their CLI in the agent's init_image, so they
		// are exempt until that field goes.
		environment, err := s.store.GetEnvironment(ctx, resolved)
		if err != nil {
			return nil, toStatusError(err)
		}
		if environment.WorkspaceImageID != nil && environment.AgentRuntimeImageID == nil {
			return nil, status.Errorf(codes.FailedPrecondition,
				"environment %s names no agent runtime image, so it has no agent CLI to run", environment.Name)
		}
		environmentID = &resolved
		llmMode = environment.LLMMode
	}
	// The mode decides which of the two model references is legal, so a
	// mismatch fails when someone configures it rather than when it runs.
	modelID, modelName, err := resolveAgentModel(llmMode, req.GetModel(), req.GetModelName())
	if err != nil {
		return nil, err
	}
	nickname := req.GetNickname()
	defaultThread, err := agentDefaultThreadFromProto(req.GetDefaultThread())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "default_thread: %v", err)
	}
	finalMessage, err := agentFinalMessageFromProto(req.GetFinalMessage())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "final_message: %v", err)
	}
	resources := toStoreComputeResources(req.GetResources())
	agent, err := s.store.CreateAgent(ctx, organizationID, store.AgentInput{
		Name:            req.GetName(),
		Nickname:        nickname,
		Role:            req.GetRole(),
		Model:           modelID,
		ModelName:       modelName,
		Description:     req.GetDescription(),
		Configuration:   req.GetConfiguration(),
		Image:           req.GetImage(),
		InitImage:       req.GetInitImage(),
		IdleTimeout:     &idleTimeout,
		Capabilities:    append([]string(nil), req.GetCapabilities()...),
		Availability:    availability,
		Resources:       resources,
		EnvironmentID:   environmentID,
		DefaultThread:   defaultThread,
		FinalMessage:    finalMessage,
		InstanceIdleTTL: instanceIdleTTL,
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	creatorRole := store.AgentRoleAssignment{AgentID: agent.Meta.ID, IdentityID: creatorID, Role: store.AgentRoleOwner}
	if _, err := s.store.UpsertAgentRole(ctx, creatorRole); err != nil {
		rollbackErr := s.store.DeleteAgent(ctx, agent.Meta.ID)
		if rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "store role failed: %v; rollback failed: %v", err, rollbackErr)
		}
		return nil, status.Errorf(codes.Internal, "store role failed: %v", err)
	}
	if err := s.addAgentAuthorization(ctx, agent.Meta.ID, agent.OrganizationID, creatorID, availability); err != nil {
		rollbackErr := s.store.DeleteAgent(ctx, agent.Meta.ID)
		if rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "authorization write failed: %v; rollback failed: %v", err, rollbackErr)
		}
		return nil, status.Errorf(codes.Internal, "authorization write failed: %v", err)
	}
	if err := s.registerAgentIdentity(ctx, agent.Meta.ID); err != nil {
		rollbackErr := errors.Join(
			s.removeAgentAuthorization(ctx, agent.Meta.ID, agent.OrganizationID, []store.AgentRoleAssignment{creatorRole}, availability),
			s.store.DeleteAgent(ctx, agent.Meta.ID),
		)
		if rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "register identity: %v; rollback: %v", err, rollbackErr)
		}
		return nil, status.Errorf(codes.Internal, "register identity: %v", err)
	}
	if nickname != "" {
		if err := s.setAgentNickname(ctx, agent.Meta.ID, agent.OrganizationID, nickname); err != nil {
			// Identity records are not deletable; best-effort cleanup removes the nickname.
			cleanupErr := s.removeAgentNickname(ctx, agent.Meta.ID, agent.OrganizationID)
			if cleanupErr != nil && status.Code(cleanupErr) == codes.NotFound {
				cleanupErr = nil
			}
			rollbackErr := errors.Join(
				cleanupErr,
				s.removeAgentAuthorization(ctx, agent.Meta.ID, agent.OrganizationID, []store.AgentRoleAssignment{creatorRole}, availability),
				s.store.DeleteAgent(ctx, agent.Meta.ID),
			)
			if rollbackErr != nil {
				return nil, status.Errorf(codes.Internal, "set nickname: %v; rollback: %v", err, rollbackErr)
			}
			return nil, status.Errorf(codes.Internal, "set nickname: %v", err)
		}
	}
	s.publishAgentUpdated(ctx, agent.Meta.ID, agent.OrganizationID)
	return &agentsv1.CreateAgentResponse{Agent: toProtoAgent(agent)}, nil
}

func (s *Server) GetAgent(ctx context.Context, req *agentsv1.GetAgentRequest) (*agentsv1.GetAgentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	agent, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.GetAgentResponse{Agent: toProtoAgent(agent)}, nil
}

func (s *Server) ResolveAgentIdentity(ctx context.Context, req *agentsv1.ResolveAgentIdentityRequest) (*agentsv1.ResolveAgentIdentityResponse, error) {
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	agent, err := s.store.GetAgent(ctx, identityID)
	if err == nil {
		return &agentsv1.ResolveAgentIdentityResponse{
			AgentId:        agent.Meta.ID.String(),
			OrganizationId: agent.OrganizationID.String(),
		}, nil
	}
	var notFound *store.NotFoundError
	if !errors.As(err, &notFound) {
		return nil, toStatusError(err)
	}
	instance, err := s.store.GetAgentInstance(ctx, identityID)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.ResolveAgentIdentityResponse{
		AgentId:         instance.AgentID.String(),
		OrganizationId:  instance.OrganizationID.String(),
		AgentInstanceId: protoString(instance.Meta.ID.String()),
	}, nil
}

func (s *Server) UpdateAgent(ctx context.Context, req *agentsv1.UpdateAgentRequest) (*agentsv1.UpdateAgentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	// NOTE: proto3 repeated fields do not track presence on the wire. A nil
	// slice indicates the caller did not set capabilities; when provided, the
	// list replaces existing capabilities.
	capabilitiesProvided := req.Capabilities != nil
	availabilityProvided := req.Availability != nil
	environmentProvided := req.EnvironmentId != nil
	// Either model reference, or a change of environment, re-decides the pair,
	// so all three route through the same validation below.
	modelProvided := req.Model != nil || req.ModelName != nil
	if req.Name == nil && req.Nickname == nil && req.Role == nil && req.Model == nil && req.ModelName == nil && req.Description == nil && req.Configuration == nil && req.Image == nil && req.InitImage == nil && req.IdleTimeout == nil && req.Resources == nil && !capabilitiesProvided && !availabilityProvided && !environmentProvided && req.DefaultThread == nil && req.FinalMessage == nil && req.InstanceIdleTtl == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}
	if req.InitImage != nil && req.GetInitImage() == "" {
		return nil, status.Error(codes.InvalidArgument, "init_image must not be empty")
	}

	nicknameProvided := req.Nickname != nil
	var previousAgent store.Agent
	var nicknameValue string
	var nicknameUpdateNeeded bool
	// An environment is checked against the agent's organization, which only the
	// stored agent names.
	if nicknameProvided || availabilityProvided || environmentProvided || modelProvided {
		previousAgent, err = s.store.GetAgent(ctx, id)
		if err != nil {
			return nil, toStatusError(err)
		}
	}
	if nicknameProvided {
		nicknameValue = req.GetNickname()
		nicknameUpdateNeeded = nicknameValue != previousAgent.Nickname
	}

	update := store.AgentUpdate{}
	if req.Name != nil {
		value := req.GetName()
		update.Name = &value
	}
	if req.Nickname != nil {
		value := nicknameValue
		update.Nickname = &value
	}
	if req.Role != nil {
		value := req.GetRole()
		update.Role = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}
	if req.Configuration != nil {
		value := req.GetConfiguration()
		update.Configuration = &value
	}
	if req.Image != nil {
		value := req.GetImage()
		update.Image = &value
	}
	if req.InitImage != nil {
		value := req.GetInitImage()
		update.InitImage = &value
	}
	if req.IdleTimeout != nil {
		value := req.GetIdleTimeout()
		if err := validateDurationString(value); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "idle_timeout: %v", err)
		}
		update.IdleTimeout = &value
	}
	if capabilitiesProvided {
		value := append([]string(nil), req.GetCapabilities()...)
		update.Capabilities = &value
	}
	if availabilityProvided {
		value, err := agentAvailabilityFromProto(req.GetAvailability())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "availability: %v", err)
		}
		update.Availability = &value
	}
	if req.Resources != nil {
		resources := toStoreComputeResources(req.GetResources())
		update.Resources = &resources
	}
	if environmentProvided {
		// An empty environment_id clears the reference rather than naming an
		// environment, leaving the agent as one created before they existed.
		if req.GetEnvironmentId() == "" {
			update.ClearEnvironmentID = true
		} else {
			environmentID, err := s.environmentInOrganization(ctx, req.GetEnvironmentId(), previousAgent.OrganizationID)
			if err != nil {
				return nil, err
			}
			update.EnvironmentID = &environmentID
		}
	}

	if modelProvided || environmentProvided {
		modelID, modelName, err := s.resolveUpdatedAgentModel(ctx, req, previousAgent, update)
		if err != nil {
			return nil, err
		}
		update.Model = &modelID
		update.ModelName = &modelName
	}

	if req.DefaultThread != nil {
		value, err := agentDefaultThreadFromProto(req.GetDefaultThread())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "default_thread: %v", err)
		}
		update.DefaultThread = &value
	}
	if req.InstanceIdleTtl != nil {
		value := req.GetInstanceIdleTtl()
		// An empty string clears it -- "this agent no longer expires its
		// instances" has to be expressible, and validateDurationString would
		// reject "" as a duration.
		if value != "" {
			if err := validateDurationString(value); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "instance_idle_ttl: %v", err)
			}
		}
		update.InstanceIdleTTL = &value
	}
	if req.FinalMessage != nil {
		value, err := agentFinalMessageFromProto(req.GetFinalMessage())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "final_message: %v", err)
		}
		update.FinalMessage = &value
	}

	agent, err := s.store.UpdateAgent(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	if nicknameProvided && nicknameUpdateNeeded {
		var nicknameErr error
		if nicknameValue == "" {
			nicknameErr = s.removeAgentNickname(ctx, agent.Meta.ID, agent.OrganizationID)
		} else {
			nicknameErr = s.setAgentNickname(ctx, agent.Meta.ID, agent.OrganizationID, nicknameValue)
		}
		if nicknameErr != nil {
			rollbackNickname := previousAgent.Nickname
			_, rollbackErr := s.store.UpdateAgent(ctx, id, store.AgentUpdate{Nickname: &rollbackNickname})
			if rollbackErr != nil {
				return nil, status.Errorf(codes.Internal, "update nickname: %v; rollback: %v", nicknameErr, rollbackErr)
			}
			return nil, status.Errorf(codes.Internal, "update nickname: %v", nicknameErr)
		}
	}
	if availabilityProvided {
		if err := s.updateAgentAvailabilityAuthorization(ctx, agent.Meta.ID, agent.OrganizationID, previousAgent.Availability, agent.Availability); err != nil {
			_, rollbackErr := s.store.UpdateAgent(ctx, id, store.AgentUpdate{Availability: &previousAgent.Availability})
			if rollbackErr != nil {
				return nil, status.Errorf(codes.Internal, "update availability authorization: %v; rollback: %v", err, rollbackErr)
			}
			return nil, status.Errorf(codes.Internal, "update availability authorization: %v", err)
		}
	}
	s.publishAgentUpdated(ctx, agent.Meta.ID, agent.OrganizationID)
	return &agentsv1.UpdateAgentResponse{Agent: toProtoAgent(agent)}, nil
}

func (s *Server) DeleteAgent(ctx context.Context, req *agentsv1.DeleteAgentRequest) (*agentsv1.DeleteAgentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	agent, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	hasInstances, err := s.store.HasNonTerminatedAgentInstances(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if hasInstances {
		return nil, status.Error(codes.FailedPrecondition, "agent has non-terminated instances")
	}
	roles, err := s.store.ListAgentRoles(ctx, agent.Meta.ID)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.removeAgentAuthorization(ctx, agent.Meta.ID, agent.OrganizationID, roles, agent.Availability); err != nil {
		return nil, status.Errorf(codes.Internal, "authorization delete failed: %v", err)
	}
	removedNickname := false
	if agent.Nickname != "" {
		if err := s.removeAgentNickname(ctx, agent.Meta.ID, agent.OrganizationID); err != nil {
			rollbackErr := s.restoreAgentAuthorization(ctx, agent, roles)
			if rollbackErr != nil {
				return nil, status.Errorf(codes.Internal, "remove nickname: %v; rollback: %v", err, rollbackErr)
			}
			return nil, status.Errorf(codes.Internal, "remove nickname: %v", err)
		}
		removedNickname = true
	}
	if err := s.store.DeleteAgent(ctx, id); err != nil {
		rollbackErr := s.restoreAgentAuthorization(ctx, agent, roles)
		if removedNickname {
			rollbackErr = errors.Join(rollbackErr, s.setAgentNickname(ctx, agent.Meta.ID, agent.OrganizationID, agent.Nickname))
		}
		if rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "agent delete failed: %v; rollback failed: %v", err, rollbackErr)
		}
		return nil, toStatusError(err)
	}
	s.publishAgentUpdated(ctx, agent.Meta.ID, agent.OrganizationID)
	return &agentsv1.DeleteAgentResponse{}, nil
}

func (s *Server) restoreAgentAuthorization(ctx context.Context, agent store.Agent, roles []store.AgentRoleAssignment) error {
	writes := []*authorizationv1.TupleKey{
		agentOrganizationTuple(agent.Meta.ID, agent.OrganizationID),
		agentIdentityOrganizationMembershipTuple(agent.Meta.ID, agent.OrganizationID),
	}
	if agent.Availability == store.AgentAvailabilityInternal {
		writes = append(writes, agentInternalAccessTuple(agent.Meta.ID, agent.OrganizationID))
	}
	for _, role := range roles {
		writes = append(writes, agentRoleTuple(agent.Meta.ID, role.IdentityID, role.Role))
	}
	return s.writeAuthorization(ctx, writes, nil)
}

func (s *Server) SetAgentRole(ctx context.Context, req *agentsv1.SetAgentRoleRequest) (*agentsv1.SetAgentRoleResponse, error) {
	agentID, err := parseUUID(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
	}
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	role, err := agentRoleFromProto(req.GetRole())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "role: %v", err)
	}
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireOrganizationMember(ctx, identityID, agent.OrganizationID); err != nil {
		return nil, status.Errorf(status.Code(err), "organization membership check failed: %v", err)
	}
	previousAssignment, err := s.store.GetAgentRole(ctx, agentID, identityID)
	var previousRole *store.AgentRole
	if err != nil {
		var notFound *store.NotFoundError
		if !errors.As(err, &notFound) {
			return nil, toStatusError(err)
		}
	} else {
		previousRole = &previousAssignment.Role
	}
	assignment := store.AgentRoleAssignment{AgentID: agentID, IdentityID: identityID, Role: role}
	assignment, err = s.store.UpsertAgentRole(ctx, assignment)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.updateAgentRoleAuthorization(ctx, agent.Meta.ID, identityID, previousRole, role); err != nil {
		if rollbackErr := s.restoreAgentRoleAssignment(ctx, previousRole, assignment); rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "authorization write failed: %v; rollback failed: %v", err, rollbackErr)
		}
		return nil, status.Errorf(codes.Internal, "authorization write failed: %v", err)
	}
	return &agentsv1.SetAgentRoleResponse{Assignment: toProtoAgentRoleAssignment(assignment)}, nil
}

func (s *Server) restoreAgentRoleAssignment(ctx context.Context, previousRole *store.AgentRole, next store.AgentRoleAssignment) error {
	if previousRole != nil {
		_, err := s.store.UpsertAgentRole(ctx, store.AgentRoleAssignment{AgentID: next.AgentID, IdentityID: next.IdentityID, Role: *previousRole})
		return err
	}
	return s.store.DeleteAgentRoleIfExists(ctx, next.AgentID, next.IdentityID)
}

func (s *Server) RemoveAgentRole(ctx context.Context, req *agentsv1.RemoveAgentRoleRequest) (*agentsv1.RemoveAgentRoleResponse, error) {
	agentID, err := parseUUID(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
	}
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	assignment, err := s.store.DeleteAgentRole(ctx, agentID, identityID)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.removeAgentRoleAuthorization(ctx, agentID, identityID, assignment.Role); err != nil {
		if _, rollbackErr := s.store.UpsertAgentRole(ctx, assignment); rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "authorization delete failed: %v; rollback failed: %v", err, rollbackErr)
		}
		return nil, status.Errorf(codes.Internal, "authorization delete failed: %v", err)
	}
	return &agentsv1.RemoveAgentRoleResponse{}, nil
}

func (s *Server) ListAgentRoles(ctx context.Context, req *agentsv1.ListAgentRolesRequest) (*agentsv1.ListAgentRolesResponse, error) {
	agentID, err := parseUUID(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
	}
	assignments, err := s.store.ListAgentRoles(ctx, agentID)
	if err != nil {
		return nil, toStatusError(err)
	}
	response := &agentsv1.ListAgentRolesResponse{Assignments: make([]*agentsv1.AgentRoleAssignment, len(assignments))}
	for i, assignment := range assignments {
		response.Assignments[i] = toProtoAgentRoleAssignment(assignment)
	}
	return response, nil
}

func (s *Server) ListMyAgentRoles(ctx context.Context, req *agentsv1.ListMyAgentRolesRequest) (*agentsv1.ListMyAgentRolesResponse, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	identityIDValue, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	identityID, err := parseUUID(identityIDValue)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	assignments, err := s.store.ListIdentityAgentRoles(ctx, organizationID, identityID)
	if err != nil {
		return nil, toStatusError(err)
	}
	response := &agentsv1.ListMyAgentRolesResponse{Assignments: make([]*agentsv1.AgentRoleAssignment, len(assignments))}
	for i, assignment := range assignments {
		response.Assignments[i] = toProtoAgentRoleAssignment(assignment)
	}
	return response, nil
}

func (s *Server) ListAgents(ctx context.Context, req *agentsv1.ListAgentsRequest) (*agentsv1.ListAgentsResponse, error) {
	organizationID, err := s.organizationListScope(ctx, req.GetOrganizationId())
	if err != nil {
		return nil, err
	}
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	result, err := s.store.ListAgents(ctx, organizationID, store.AgentFilter{}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	agents, nextToken := mapListResult(result.Agents, result.NextCursor, toProtoAgent)
	return &agentsv1.ListAgentsResponse{Agents: agents, NextPageToken: nextToken}, nil
}

// volumeTarget resolves the environment or MCP a volume names, and the
// organization both belong to. Authorization follows the target: a volume is
// gated by the environment that owns it, never by itself.
func (s *Server) volumeTarget(ctx context.Context, environmentIDRaw, mcpIDRaw string) (store.VolumeInput, uuid.UUID, error) {
	input := store.VolumeInput{}
	switch {
	case environmentIDRaw != "":
		environmentID, err := parseUUID(environmentIDRaw)
		if err != nil {
			return input, uuid.UUID{}, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
		}
		environment, err := s.store.GetEnvironment(ctx, environmentID)
		if err != nil {
			return input, uuid.UUID{}, toStatusError(err)
		}
		if err := s.requireConfigWrite(ctx, nil, nil, &environmentID); err != nil {
			return input, uuid.UUID{}, err
		}
		input.EnvironmentID = &environmentID
		return input, environment.OrganizationID, nil
	case mcpIDRaw != "":
		mcpID, err := parseUUID(mcpIDRaw)
		if err != nil {
			return input, uuid.UUID{}, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		mcp, err := s.store.GetMcp(ctx, mcpID)
		if err != nil {
			return input, uuid.UUID{}, toStatusError(err)
		}
		if err := s.requireConfigWrite(ctx, mcp.AgentID, nil, mcp.EnvironmentID); err != nil {
			return input, uuid.UUID{}, err
		}
		input.McpID = &mcpID
		return input, mcp.OrganizationID, nil
	default:
		return input, uuid.UUID{}, status.Error(codes.InvalidArgument, "environment_id or mcp_id is required")
	}
}

func (s *Server) CreateVolume(ctx context.Context, req *agentsv1.CreateVolumeRequest) (*agentsv1.CreateVolumeResponse, error) {
	input, organizationID, err := s.volumeTarget(ctx, req.GetEnvironmentId(), req.GetMcpId())
	if err != nil {
		return nil, err
	}
	if err := validateVolumeName(req.GetName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "name: %v", err)
	}
	if err := validateMountPath(req.GetMountPath()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "mount_path: %v", err)
	}
	// size is what makes a volume persistent: the two are biconditional, so a
	// separate flag could only ever contradict it.
	size, persistent, err := resolveVolumeSize(req.GetSize(), req.GetPersistent())
	if err != nil {
		return nil, err
	}
	input.Name = req.GetName()
	input.MountPath = req.GetMountPath()
	input.Persistent = persistent
	input.Size = size
	input.StorageClass = req.StorageClass
	input.TTL = req.Ttl
	volume, err := s.store.CreateVolume(ctx, organizationID, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishVolumeTargetUpdated(ctx, volume)
	return &agentsv1.CreateVolumeResponse{Volume: toProtoVolume(volume)}, nil
}

func (s *Server) GetVolume(ctx context.Context, req *agentsv1.GetVolumeRequest) (*agentsv1.GetVolumeResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	volume, err := s.store.GetVolume(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigRead(ctx, nil, volume.McpID, volume.EnvironmentID); err != nil {
		return nil, err
	}
	return &agentsv1.GetVolumeResponse{Volume: toProtoVolume(volume)}, nil
}

func (s *Server) UpdateVolume(ctx context.Context, req *agentsv1.UpdateVolumeRequest) (*agentsv1.UpdateVolumeResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Persistent == nil && req.MountPath == nil && req.Size == nil &&
		req.Name == nil && req.StorageClass == nil && req.Ttl == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}
	existing, err := s.store.GetVolume(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigWrite(ctx, nil, existing.McpID, existing.EnvironmentID); err != nil {
		return nil, err
	}

	update := store.VolumeUpdate{}
	if req.Name != nil {
		value := req.GetName()
		if err := validateVolumeName(value); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "name: %v", err)
		}
		update.Name = &value
	}
	if req.MountPath != nil {
		value := req.GetMountPath()
		if err := validateMountPath(value); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mount_path: %v", err)
		}
		update.MountPath = &value
	}
	if req.Size != nil || req.Persistent != nil {
		requestedSize := req.GetSize()
		if req.Size == nil && existing.Size != nil {
			requestedSize = *existing.Size
		}
		size, persistent, err := resolveVolumeSize(requestedSize, req.GetPersistent())
		if err != nil {
			return nil, err
		}
		update.Size = &size
		update.Persistent = &persistent
	}
	if req.StorageClass != nil {
		value := req.StorageClass
		update.StorageClass = &value
	}
	if req.Ttl != nil {
		value := req.Ttl
		update.TTL = &value
	}

	volume, err := s.store.UpdateVolume(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishVolumeTargetUpdated(ctx, volume)
	return &agentsv1.UpdateVolumeResponse{Volume: toProtoVolume(volume)}, nil
}

func (s *Server) DeleteVolume(ctx context.Context, req *agentsv1.DeleteVolumeRequest) (*agentsv1.DeleteVolumeResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	// Read before the delete: the target names who to notify, and it is gone
	// with the row.
	volume, err := s.store.GetVolume(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigWrite(ctx, nil, volume.McpID, volume.EnvironmentID); err != nil {
		return nil, err
	}
	if err := s.store.DeleteVolume(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	s.publishVolumeTargetUpdated(ctx, volume)
	return &agentsv1.DeleteVolumeResponse{}, nil
}

func (s *Server) ListVolumes(ctx context.Context, req *agentsv1.ListVolumesRequest) (*agentsv1.ListVolumesResponse, error) {
	filter := store.VolumeFilter{}
	var organizationID uuid.UUID
	switch {
	case req.GetEnvironmentId() != "":
		environmentID, err := parseUUID(req.GetEnvironmentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
		}
		environment, err := s.store.GetEnvironment(ctx, environmentID)
		if err != nil {
			return nil, toStatusError(err)
		}
		if err := s.requireEnvironmentConfigRead(ctx, environmentID); err != nil {
			return nil, err
		}
		filter.EnvironmentID = &environmentID
		organizationID = environment.OrganizationID
	case req.GetMcpId() != "":
		mcpID, err := parseUUID(req.GetMcpId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		mcp, err := s.store.GetMcp(ctx, mcpID)
		if err != nil {
			return nil, toStatusError(err)
		}
		if err := s.requireConfigRead(ctx, mcp.AgentID, nil, mcp.EnvironmentID); err != nil {
			return nil, err
		}
		filter.McpID = &mcpID
		organizationID = mcp.OrganizationID
	default:
		return nil, status.Error(codes.InvalidArgument, "environment_id or mcp_id is required")
	}
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	result, err := s.store.ListVolumes(ctx, organizationID, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	volumes, nextToken := mapListResult(result.Volumes, result.NextCursor, toProtoVolume)
	return &agentsv1.ListVolumesResponse{Volumes: volumes, NextPageToken: nextToken}, nil
}

func (s *Server) CreateMcp(ctx context.Context, req *agentsv1.CreateMcpRequest) (*agentsv1.CreateMcpResponse, error) {
	name := req.GetName()
	if err := validateMcpName(name); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "name: %v", err)
	}
	input := store.McpInput{
		Name:          name,
		Image:         req.GetImage(),
		Command:       req.GetCommand(),
		Resources:     toStoreComputeResources(req.GetResources()),
		Description:   req.GetDescription(),
		SharedVolumes: req.GetSharedVolumes(),
	}
	var organizationID uuid.UUID
	switch {
	case req.GetEnvironmentId() != "":
		environmentID, err := parseUUID(req.GetEnvironmentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
		}
		environment, err := s.store.GetEnvironment(ctx, environmentID)
		if err != nil {
			return nil, toStatusError(err)
		}
		if err := s.requireConfigWrite(ctx, nil, nil, &environmentID); err != nil {
			return nil, err
		}
		input.EnvironmentID = &environmentID
		organizationID = environment.OrganizationID
	case req.GetAgentId() != "":
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		organizationID, err = s.organizationOfAgent(ctx, agentID)
		if err != nil {
			return nil, err
		}
		if err := s.requireConfigWrite(ctx, &agentID, nil, nil); err != nil {
			return nil, err
		}
		input.AgentID = &agentID
	default:
		return nil, status.Error(codes.InvalidArgument, "environment_id or agent_id is required")
	}
	// shared_volumes names environment volumes, which an MCP on an environment
	// resolves against its own; an agent-level one resolves at workload start.
	if len(input.SharedVolumes) > 0 && input.AgentID == nil && input.EnvironmentID == nil {
		return nil, status.Error(codes.InvalidArgument, "shared_volumes requires a target")
	}
	reference, err := parseImageReference(req.GetImageId(), req.GetImageTag(), "image")
	if err != nil {
		return nil, err
	}
	if reference != nil {
		// An MCP may run a purpose-built server image or a devcontainer, so
		// the type is not narrowed here; the catalog rejects an agent runtime.
		if err := s.validateMcpImage(ctx, *reference, organizationID); err != nil {
			return nil, err
		}
		input.ImageID = &reference.ImageID
		input.ImageTag = reference.Tag
	}
	mcp, err := s.store.CreateMcp(ctx, organizationID, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedForConfigTarget(ctx, mcp.AgentID, nil, mcp.EnvironmentID)
	return &agentsv1.CreateMcpResponse{Mcp: toProtoMcp(mcp)}, nil
}

func (s *Server) GetMcp(ctx context.Context, req *agentsv1.GetMcpRequest) (*agentsv1.GetMcpResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	mcp, err := s.store.GetMcp(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigRead(ctx, mcp.AgentID, nil, mcp.EnvironmentID); err != nil {
		return nil, err
	}
	return &agentsv1.GetMcpResponse{Mcp: toProtoMcp(mcp)}, nil
}

func (s *Server) UpdateMcp(ctx context.Context, req *agentsv1.UpdateMcpRequest) (*agentsv1.UpdateMcpResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Image == nil && req.Command == nil && req.Resources == nil && req.Description == nil &&
		req.ImageId == nil && req.ImageTag == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	existing, err := s.store.GetMcp(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigWrite(ctx, existing.AgentID, nil, existing.EnvironmentID); err != nil {
		return nil, err
	}

	update := store.McpUpdate{}
	if err := s.applyMcpImageUpdate(ctx, req, id, &update); err != nil {
		return nil, err
	}
	if req.Image != nil {
		value := req.GetImage()
		update.Image = &value
	}
	if req.Command != nil {
		value := req.GetCommand()
		update.Command = &value
	}
	if req.Resources != nil {
		resources := toStoreComputeResources(req.GetResources())
		update.Resources = &resources
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}

	mcp, err := s.store.UpdateMcp(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedForConfigTarget(ctx, mcp.AgentID, nil, mcp.EnvironmentID)
	return &agentsv1.UpdateMcpResponse{Mcp: toProtoMcp(mcp)}, nil
}

func (s *Server) DeleteMcp(ctx context.Context, req *agentsv1.DeleteMcpRequest) (*agentsv1.DeleteMcpResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	mcp, err := s.store.GetMcp(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigWrite(ctx, mcp.AgentID, nil, mcp.EnvironmentID); err != nil {
		return nil, err
	}
	if err := s.store.DeleteMcp(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedForConfigTarget(ctx, mcp.AgentID, nil, mcp.EnvironmentID)
	return &agentsv1.DeleteMcpResponse{}, nil
}

func (s *Server) ListMcps(ctx context.Context, req *agentsv1.ListMcpsRequest) (*agentsv1.ListMcpsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	filter := store.McpFilter{}
	switch {
	case req.GetEnvironmentId() != "":
		environmentID, err := parseUUID(req.GetEnvironmentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
		}
		if err := s.requireConfigRead(ctx, nil, nil, &environmentID); err != nil {
			return nil, err
		}
		filter.EnvironmentID = &environmentID
	case req.GetAgentId() != "":
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		if err := s.requireConfigRead(ctx, &agentID, nil, nil); err != nil {
			return nil, err
		}
		filter.AgentID = &agentID
	default:
		return nil, status.Error(codes.InvalidArgument, "agent_id or environment_id must be provided")
	}

	result, err := s.store.ListMcps(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	mcps, nextToken := mapListResult(result.Mcps, result.NextCursor, toProtoMcp)
	return &agentsv1.ListMcpsResponse{Mcps: mcps, NextPageToken: nextToken}, nil
}

func (s *Server) CreateSkill(ctx context.Context, req *agentsv1.CreateSkillRequest) (*agentsv1.CreateSkillResponse, error) {
	agentID, err := parseUUID(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
	}
	if err := s.requireConfigWrite(ctx, &agentID, nil, nil); err != nil {
		return nil, err
	}
	skill, err := s.store.CreateSkill(ctx, store.SkillInput{
		AgentID:     agentID,
		Name:        req.GetName(),
		Body:        req.GetBody(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedByID(ctx, skill.AgentID)
	return &agentsv1.CreateSkillResponse{Skill: toProtoSkill(skill)}, nil
}

func (s *Server) GetSkill(ctx context.Context, req *agentsv1.GetSkillRequest) (*agentsv1.GetSkillResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	skill, err := s.store.GetSkill(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigRead(ctx, &skill.AgentID, nil, nil); err != nil {
		return nil, err
	}
	return &agentsv1.GetSkillResponse{Skill: toProtoSkill(skill)}, nil
}

func (s *Server) UpdateSkill(ctx context.Context, req *agentsv1.UpdateSkillRequest) (*agentsv1.UpdateSkillResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Name == nil && req.Body == nil && req.Description == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	existing, err := s.store.GetSkill(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigWrite(ctx, &existing.AgentID, nil, nil); err != nil {
		return nil, err
	}

	update := store.SkillUpdate{}
	if req.Name != nil {
		value := req.GetName()
		update.Name = &value
	}
	if req.Body != nil {
		value := req.GetBody()
		update.Body = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}

	skill, err := s.store.UpdateSkill(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedByID(ctx, skill.AgentID)
	return &agentsv1.UpdateSkillResponse{Skill: toProtoSkill(skill)}, nil
}

func (s *Server) DeleteSkill(ctx context.Context, req *agentsv1.DeleteSkillRequest) (*agentsv1.DeleteSkillResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	skill, err := s.store.GetSkill(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigWrite(ctx, &skill.AgentID, nil, nil); err != nil {
		return nil, err
	}
	if err := s.store.DeleteSkill(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedByID(ctx, skill.AgentID)
	return &agentsv1.DeleteSkillResponse{}, nil
}

func (s *Server) ListSkills(ctx context.Context, req *agentsv1.ListSkillsRequest) (*agentsv1.ListSkillsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	if req.GetAgentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id must be provided")
	}
	agentID, err := parseUUID(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
	}
	if err := s.requireConfigRead(ctx, &agentID, nil, nil); err != nil {
		return nil, err
	}
	filter := store.SkillFilter{AgentID: &agentID}

	result, err := s.store.ListSkills(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	skills, nextToken := mapListResult(result.Skills, result.NextCursor, toProtoSkill)
	return &agentsv1.ListSkillsResponse{Skills: skills, NextPageToken: nextToken}, nil
}

func (s *Server) CreateEnv(ctx context.Context, req *agentsv1.CreateEnvRequest) (*agentsv1.CreateEnvResponse, error) {
	input := store.EnvInput{
		Name:        req.GetName(),
		Description: req.GetDescription(),
	}

	switch target := req.GetTarget().(type) {
	case *agentsv1.CreateEnvRequest_AgentId:
		id, err := parseUUID(target.AgentId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		input.AgentID = &id
	case *agentsv1.CreateEnvRequest_McpId:
		id, err := parseUUID(target.McpId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		input.McpID = &id
	case *agentsv1.CreateEnvRequest_EnvironmentId:
		id, err := parseUUID(target.EnvironmentId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
		}
		input.EnvironmentID = &id
	default:
		return nil, status.Error(codes.InvalidArgument, "target must be specified")
	}
	if err := s.requireConfigWrite(ctx, input.AgentID, input.McpID, input.EnvironmentID); err != nil {
		return nil, err
	}

	switch source := req.GetSource().(type) {
	case *agentsv1.CreateEnvRequest_Value:
		value := source.Value
		input.Value = &value
	case *agentsv1.CreateEnvRequest_SecretId:
		secretID, err := parseUUID(source.SecretId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "secret_id: %v", err)
		}
		input.SecretID = &secretID
	default:
		return nil, status.Error(codes.InvalidArgument, "source must be specified")
	}

	env, err := s.store.CreateEnv(ctx, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedForConfigTarget(ctx, env.AgentID, env.McpID, env.EnvironmentID)
	return &agentsv1.CreateEnvResponse{Env: toProtoEnv(env)}, nil
}

func (s *Server) GetEnv(ctx context.Context, req *agentsv1.GetEnvRequest) (*agentsv1.GetEnvResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	env, err := s.store.GetEnv(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigRead(ctx, env.AgentID, env.McpID, env.EnvironmentID); err != nil {
		return nil, err
	}
	return &agentsv1.GetEnvResponse{Env: toProtoEnv(env)}, nil
}

func (s *Server) UpdateEnv(ctx context.Context, req *agentsv1.UpdateEnvRequest) (*agentsv1.UpdateEnvResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Name == nil && req.Description == nil && req.Value == nil && req.SecretId == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}
	if req.Value != nil && req.SecretId != nil {
		return nil, status.Error(codes.InvalidArgument, "value and secret_id cannot both be set")
	}

	existing, err := s.store.GetEnv(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigWrite(ctx, existing.AgentID, existing.McpID, existing.EnvironmentID); err != nil {
		return nil, err
	}

	update := store.EnvUpdate{}
	if req.Name != nil {
		value := req.GetName()
		update.Name = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}
	if req.Value != nil {
		value := req.GetValue()
		update.Value = &value
	}
	if req.SecretId != nil {
		secretID, err := parseUUID(req.GetSecretId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "secret_id: %v", err)
		}
		update.SecretID = &secretID
	}

	env, err := s.store.UpdateEnv(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedForConfigTarget(ctx, env.AgentID, env.McpID, env.EnvironmentID)
	return &agentsv1.UpdateEnvResponse{Env: toProtoEnv(env)}, nil
}

func (s *Server) DeleteEnv(ctx context.Context, req *agentsv1.DeleteEnvRequest) (*agentsv1.DeleteEnvResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	env, err := s.store.GetEnv(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigWrite(ctx, env.AgentID, env.McpID, env.EnvironmentID); err != nil {
		return nil, err
	}
	if err := s.store.DeleteEnv(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedForConfigTarget(ctx, env.AgentID, env.McpID, env.EnvironmentID)
	return &agentsv1.DeleteEnvResponse{}, nil
}

func (s *Server) ListEnvs(ctx context.Context, req *agentsv1.ListEnvsRequest) (*agentsv1.ListEnvsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	filter, err := s.envListFilter(ctx, req)
	if err != nil {
		return nil, err
	}

	result, err := s.store.ListEnvs(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	envs, nextToken := mapListResult(result.Envs, result.NextCursor, toProtoEnv)
	return &agentsv1.ListEnvsResponse{Envs: envs, NextPageToken: nextToken}, nil
}

// envListFilter authorizes the read and narrows it.
//
// The Agents Orchestrator lists an environment's envs while assembling a
// sandbox workload and carries no identity, by design — it holds no OpenFGA
// tuples and reaches this RPC over the mesh rather than the Gateway. A caller
// that does present an identity is a user request: it names the organization it
// is reading and must be a member of it. The target ids only narrow the result
// further; an env is a reference to a secret, and which organization's secrets
// are being read is settled by the organization, not by them.
func (s *Server) envListFilter(ctx context.Context, req *agentsv1.ListEnvsRequest) (store.EnvFilter, error) {
	filter := store.EnvFilter{}
	if req.GetAgentId() != "" {
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return store.EnvFilter{}, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		if err := s.requireConfigRead(ctx, &agentID, nil, nil); err != nil {
			return store.EnvFilter{}, err
		}
		filter.AgentID = &agentID
	}
	if req.GetMcpId() != "" {
		mcpID, err := parseUUID(req.GetMcpId())
		if err != nil {
			return store.EnvFilter{}, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		if err := s.requireConfigRead(ctx, nil, &mcpID, nil); err != nil {
			return store.EnvFilter{}, err
		}
		filter.McpID = &mcpID
	}
	if req.GetEnvironmentId() != "" {
		environmentID, err := parseUUID(req.GetEnvironmentId())
		if err != nil {
			return store.EnvFilter{}, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
		}
		if err := s.requireConfigRead(ctx, nil, nil, &environmentID); err != nil {
			return store.EnvFilter{}, err
		}
		filter.EnvironmentID = &environmentID
	}

	organizationID, err := s.organizationListScope(ctx, req.GetOrganizationId())
	if err != nil {
		return store.EnvFilter{}, err
	}
	filter.OrganizationID = organizationID
	return filter, nil
}

func (s *Server) CreateInitScript(ctx context.Context, req *agentsv1.CreateInitScriptRequest) (*agentsv1.CreateInitScriptResponse, error) {
	input := store.InitScriptInput{
		Script:      req.GetScript(),
		Description: req.GetDescription(),
	}
	var organizationID uuid.UUID

	switch target := req.GetTarget().(type) {
	case *agentsv1.CreateInitScriptRequest_AgentId:
		id, err := parseUUID(target.AgentId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		organizationID, err = s.organizationOfAgent(ctx, id)
		if err != nil {
			return nil, err
		}
		input.AgentID = &id
	case *agentsv1.CreateInitScriptRequest_McpId:
		id, err := parseUUID(target.McpId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		mcp, err := s.store.GetMcp(ctx, id)
		if err != nil {
			return nil, toStatusError(err)
		}
		organizationID = mcp.OrganizationID
		input.McpID = &id
	case *agentsv1.CreateInitScriptRequest_EnvironmentId:
		id, err := parseUUID(target.EnvironmentId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
		}
		environment, err := s.store.GetEnvironment(ctx, id)
		if err != nil {
			return nil, toStatusError(err)
		}
		organizationID = environment.OrganizationID
		input.EnvironmentID = &id
	default:
		return nil, status.Error(codes.InvalidArgument, "target must be specified")
	}
	// An init script is a shell script the workload runs, so writing one is
	// authorized as any other configuration write on the parent.
	if err := s.requireConfigWrite(ctx, input.AgentID, input.McpID, input.EnvironmentID); err != nil {
		return nil, err
	}

	script, err := s.store.CreateInitScript(ctx, organizationID, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedForConfigTarget(ctx, script.AgentID, script.McpID, script.EnvironmentID)
	return &agentsv1.CreateInitScriptResponse{InitScript: toProtoInitScript(script)}, nil
}

func (s *Server) GetInitScript(ctx context.Context, req *agentsv1.GetInitScriptRequest) (*agentsv1.GetInitScriptResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	script, err := s.store.GetInitScript(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigRead(ctx, script.AgentID, script.McpID, script.EnvironmentID); err != nil {
		return nil, err
	}
	return &agentsv1.GetInitScriptResponse{InitScript: toProtoInitScript(script)}, nil
}

func (s *Server) UpdateInitScript(ctx context.Context, req *agentsv1.UpdateInitScriptRequest) (*agentsv1.UpdateInitScriptResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Script == nil && req.Description == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	existing, err := s.store.GetInitScript(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigWrite(ctx, existing.AgentID, existing.McpID, existing.EnvironmentID); err != nil {
		return nil, err
	}

	update := store.InitScriptUpdate{}
	if req.Script != nil {
		value := req.GetScript()
		update.Script = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}

	script, err := s.store.UpdateInitScript(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedForConfigTarget(ctx, script.AgentID, script.McpID, script.EnvironmentID)
	return &agentsv1.UpdateInitScriptResponse{InitScript: toProtoInitScript(script)}, nil
}

func (s *Server) DeleteInitScript(ctx context.Context, req *agentsv1.DeleteInitScriptRequest) (*agentsv1.DeleteInitScriptResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	script, err := s.store.GetInitScript(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireConfigWrite(ctx, script.AgentID, script.McpID, script.EnvironmentID); err != nil {
		return nil, err
	}
	if err := s.store.DeleteInitScript(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	s.publishAgentUpdatedForConfigTarget(ctx, script.AgentID, script.McpID, script.EnvironmentID)
	return &agentsv1.DeleteInitScriptResponse{}, nil
}

func (s *Server) ListInitScripts(ctx context.Context, req *agentsv1.ListInitScriptsRequest) (*agentsv1.ListInitScriptsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	hasFilter := false
	filter := store.InitScriptFilter{}
	if req.GetEnvironmentId() != "" {
		environmentID, err := parseUUID(req.GetEnvironmentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
		}
		if err := s.requireConfigRead(ctx, nil, nil, &environmentID); err != nil {
			return nil, err
		}
		filter.EnvironmentID = &environmentID
		hasFilter = true
	}
	if req.GetAgentId() != "" {
		hasFilter = true
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		if err := s.requireConfigRead(ctx, &agentID, nil, nil); err != nil {
			return nil, err
		}
		filter.AgentID = &agentID
	}
	if req.GetMcpId() != "" {
		hasFilter = true
		mcpID, err := parseUUID(req.GetMcpId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		if err := s.requireConfigRead(ctx, nil, &mcpID, nil); err != nil {
			return nil, err
		}
		filter.McpID = &mcpID
	}
	if !hasFilter {
		return nil, status.Error(codes.InvalidArgument, "at least one filter must be provided")
	}

	result, err := s.store.ListInitScripts(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	scripts, nextToken := mapListResult(result.InitScripts, result.NextCursor, toProtoInitScript)
	return &agentsv1.ListInitScriptsResponse{InitScripts: scripts, NextPageToken: nextToken}, nil
}

func decodePageCursor(token string) (*store.PageCursor, error) {
	if token == "" {
		return nil, nil
	}
	id, err := store.DecodePageToken(token)
	if err != nil {
		return nil, err
	}
	return &store.PageCursor{AfterID: id}, nil
}

func decodeInboxPageCursor(token string) (*store.InboxPageCursor, error) {
	if token == "" {
		return nil, nil
	}
	acceptedAt, id, err := store.DecodeInboxPageToken(token)
	if err != nil {
		return nil, err
	}
	return &store.InboxPageCursor{AfterAcceptedAt: acceptedAt, AfterID: id}, nil
}

func mapListResult[T any, P any](items []T, nextCursor *store.PageCursor, convert func(T) P) ([]P, string) {
	results := make([]P, len(items))
	for i, item := range items {
		results[i] = convert(item)
	}
	if nextCursor == nil {
		return results, ""
	}
	return results, store.EncodePageToken(nextCursor.AfterID)
}

func mapInboxListResult[T any, P any](items []T, nextCursor *store.InboxPageCursor, convert func(T) P) ([]P, string) {
	results := make([]P, len(items))
	for i, item := range items {
		results[i] = convert(item)
	}
	if nextCursor == nil {
		return results, ""
	}
	return results, store.EncodeInboxPageToken(nextCursor.AfterAcceptedAt, nextCursor.AfterID)
}

func parseUUID(value string) (uuid.UUID, error) {
	if value == "" {
		return uuid.UUID{}, fmt.Errorf("value is empty")
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, err
	}
	return id, nil
}

func validateMcpName(name string) error {
	if name == "" {
		return fmt.Errorf("value is empty")
	}
	if len(name) > maxMcpNameLength {
		return fmt.Errorf("must be at most %d characters", maxMcpNameLength)
	}
	if !mcpNamePattern.MatchString(name) {
		return fmt.Errorf("must match %s", mcpNamePattern.String())
	}
	return nil
}

func validateDurationString(value string) error {
	if value == "" {
		return fmt.Errorf("value is empty")
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	if duration <= 0 {
		return fmt.Errorf("must be a positive duration")
	}
	return nil
}

func toStoreComputeResources(resources *agentsv1.ComputeResources) store.ComputeResources {
	if resources == nil {
		return store.ComputeResources{}
	}
	return store.ComputeResources{
		RequestsCPU:    resources.GetRequestsCpu(),
		RequestsMemory: resources.GetRequestsMemory(),
		LimitsCPU:      resources.GetLimitsCpu(),
		LimitsMemory:   resources.GetLimitsMemory(),
	}
}

func environmentAvailabilityFromProto(availability agentsv1.EnvironmentAvailability) (store.EnvironmentAvailability, error) {
	switch availability {
	case agentsv1.EnvironmentAvailability_ENVIRONMENT_AVAILABILITY_INTERNAL:
		return store.EnvironmentAvailabilityInternal, nil
	case agentsv1.EnvironmentAvailability_ENVIRONMENT_AVAILABILITY_PRIVATE:
		return store.EnvironmentAvailabilityPrivate, nil
	case agentsv1.EnvironmentAvailability_ENVIRONMENT_AVAILABILITY_UNSPECIFIED:
		return "", fmt.Errorf("must be internal or private")
	default:
		return "", fmt.Errorf("unknown value %d", availability)
	}
}

func toProtoEnvironmentAvailability(availability store.EnvironmentAvailability) agentsv1.EnvironmentAvailability {
	if availability == store.EnvironmentAvailabilityPrivate {
		return agentsv1.EnvironmentAvailability_ENVIRONMENT_AVAILABILITY_PRIVATE
	}
	return agentsv1.EnvironmentAvailability_ENVIRONMENT_AVAILABILITY_INTERNAL
}

// Unspecified is platform: the mode is opt-in, and every environment written
// before it existed is a platform one.
// resolveUpdatedAgentModel validates the pair the agent will have after the
// update, not the fields the request happened to carry: repointing an agent at
// an environment in the other mode invalidates a reference the caller never
// mentioned.
func (s *Server) resolveUpdatedAgentModel(ctx context.Context, req *agentsv1.UpdateAgentRequest, previous store.Agent, update store.AgentUpdate) (uuid.UUID, string, error) {
	environmentID := previous.EnvironmentID
	if update.ClearEnvironmentID {
		environmentID = nil
	} else if update.EnvironmentID != nil {
		environmentID = update.EnvironmentID
	}

	// No environment means an agent predating them, which is a platform one.
	mode := store.LLMModePlatform
	if environmentID != nil {
		environment, err := s.store.GetEnvironment(ctx, *environmentID)
		if err != nil {
			return uuid.UUID{}, "", toStatusError(err)
		}
		mode = environment.LLMMode
	}

	model := previous.Model.String()
	if previous.Model == uuid.Nil {
		model = ""
	}
	if req.Model != nil {
		model = req.GetModel()
	}
	modelName := previous.ModelName
	if req.ModelName != nil {
		modelName = req.GetModelName()
	}
	// Switching mode without restating the reference: drop the one the target
	// mode rejects rather than refusing an otherwise valid repoint.
	if mode == store.LLMModeNative && req.Model == nil {
		model = ""
	}
	if mode == store.LLMModePlatform && req.ModelName == nil {
		modelName = ""
	}
	return resolveAgentModel(mode, model, modelName)
}

// resolveAgentModel enforces the two references as required-and-exclusive in
// each direction: platform mode owns a model namespace and native mode does
// not, so exactly one of them can mean anything.
func resolveAgentModel(mode store.LLMMode, model string, modelName string) (uuid.UUID, string, error) {
	model = strings.TrimSpace(model)
	modelName = strings.TrimSpace(modelName)
	if mode == store.LLMModeNative {
		if model != "" {
			return uuid.UUID{}, "", status.Error(codes.InvalidArgument,
				"model is rejected in a native environment, which has no platform model namespace; set model_name instead")
		}
		// Optional: unset leaves the CLI on its own default and its own picker.
		return uuid.UUID{}, modelName, nil
	}
	if modelName != "" {
		return uuid.UUID{}, "", status.Error(codes.InvalidArgument,
			"model_name is rejected in a platform environment; reference a Model with model instead")
	}
	modelID, err := parseUUID(model)
	if err != nil {
		return uuid.UUID{}, "", status.Errorf(codes.InvalidArgument, "model: %v", err)
	}
	return modelID, "", nil
}

func llmModeFromProto(mode agentsv1.LLMMode) (store.LLMMode, error) {
	switch mode {
	case agentsv1.LLMMode_LLM_MODE_UNSPECIFIED, agentsv1.LLMMode_LLM_MODE_PLATFORM:
		return store.LLMModePlatform, nil
	case agentsv1.LLMMode_LLM_MODE_NATIVE:
		return store.LLMModeNative, nil
	default:
		return "", fmt.Errorf("unknown value %d", mode)
	}
}

func toProtoLLMMode(mode store.LLMMode) agentsv1.LLMMode {
	if mode == store.LLMModeNative {
		return agentsv1.LLMMode_LLM_MODE_NATIVE
	}
	return agentsv1.LLMMode_LLM_MODE_PLATFORM
}

func agentAvailabilityFromProto(availability agentsv1.AgentAvailability) (store.AgentAvailability, error) {
	switch availability {
	case agentsv1.AgentAvailability_AGENT_AVAILABILITY_INTERNAL:
		return store.AgentAvailabilityInternal, nil
	case agentsv1.AgentAvailability_AGENT_AVAILABILITY_PRIVATE:
		return store.AgentAvailabilityPrivate, nil
	case agentsv1.AgentAvailability_AGENT_AVAILABILITY_UNSPECIFIED:
		return "", fmt.Errorf("must be internal or private")
	default:
		return "", fmt.Errorf("unknown value %d", availability)
	}
}

func agentAvailabilityToProto(availability store.AgentAvailability) agentsv1.AgentAvailability {
	switch availability {
	case store.AgentAvailabilityInternal:
		return agentsv1.AgentAvailability_AGENT_AVAILABILITY_INTERNAL
	case store.AgentAvailabilityPrivate:
		return agentsv1.AgentAvailability_AGENT_AVAILABILITY_PRIVATE
	default:
		panic(fmt.Sprintf("unknown agent availability %q", availability))
	}
}

// agentDefaultThreadFromProto maps the class policy. UNSPECIFIED means the
// caller did not choose, which is the documented default rather than an error:
// origin is the only inference that composes for delegation.
func agentDefaultThreadFromProto(value agentsv1.AgentDefaultThread) (store.AgentDefaultThread, error) {
	switch value {
	case agentsv1.AgentDefaultThread_AGENT_DEFAULT_THREAD_UNSPECIFIED,
		agentsv1.AgentDefaultThread_AGENT_DEFAULT_THREAD_ORIGIN:
		return store.AgentDefaultThreadOrigin, nil
	case agentsv1.AgentDefaultThread_AGENT_DEFAULT_THREAD_NONE:
		return store.AgentDefaultThreadNone, nil
	default:
		return "", fmt.Errorf("unknown value %d", value)
	}
}

func agentDefaultThreadToProto(value store.AgentDefaultThread) agentsv1.AgentDefaultThread {
	switch value {
	case store.AgentDefaultThreadNone:
		return agentsv1.AgentDefaultThread_AGENT_DEFAULT_THREAD_NONE
	default:
		return agentsv1.AgentDefaultThread_AGENT_DEFAULT_THREAD_ORIGIN
	}
}

// agentFinalMessageFromProto maps what becomes of a turn's final text.
// UNSPECIFIED is discard, so agents that already send explicitly do not start
// posting everything twice.
func agentFinalMessageFromProto(value agentsv1.AgentFinalMessage) (store.AgentFinalMessage, error) {
	switch value {
	case agentsv1.AgentFinalMessage_AGENT_FINAL_MESSAGE_UNSPECIFIED,
		agentsv1.AgentFinalMessage_AGENT_FINAL_MESSAGE_DISCARD:
		return store.AgentFinalMessageDiscard, nil
	case agentsv1.AgentFinalMessage_AGENT_FINAL_MESSAGE_DEFAULT_THREAD:
		return store.AgentFinalMessageDefaultThread, nil
	default:
		return "", fmt.Errorf("unknown value %d", value)
	}
}

func agentFinalMessageToProto(value store.AgentFinalMessage) agentsv1.AgentFinalMessage {
	switch value {
	case store.AgentFinalMessageDefaultThread:
		return agentsv1.AgentFinalMessage_AGENT_FINAL_MESSAGE_DEFAULT_THREAD
	default:
		return agentsv1.AgentFinalMessage_AGENT_FINAL_MESSAGE_DISCARD
	}
}

func agentRoleFromProto(role agentsv1.AgentRole) (store.AgentRole, error) {
	switch role {
	case agentsv1.AgentRole_AGENT_ROLE_OWNER:
		return store.AgentRoleOwner, nil
	case agentsv1.AgentRole_AGENT_ROLE_MAINTAINER:
		return store.AgentRoleMaintainer, nil
	case agentsv1.AgentRole_AGENT_ROLE_PARTICIPANT:
		return store.AgentRoleParticipant, nil
	case agentsv1.AgentRole_AGENT_ROLE_UNSPECIFIED:
		return "", fmt.Errorf("must be owner, maintainer, or participant")
	default:
		return "", fmt.Errorf("unknown value %d", role)
	}
}

func agentRoleToProto(role store.AgentRole) agentsv1.AgentRole {
	switch role {
	case store.AgentRoleOwner:
		return agentsv1.AgentRole_AGENT_ROLE_OWNER
	case store.AgentRoleMaintainer:
		return agentsv1.AgentRole_AGENT_ROLE_MAINTAINER
	case store.AgentRoleParticipant:
		return agentsv1.AgentRole_AGENT_ROLE_PARTICIPANT
	default:
		panic(fmt.Sprintf("unknown agent role %q", role))
	}
}

func agentInstanceStateFromProto(state agentsv1.AgentInstanceState) (store.AgentInstanceState, error) {
	switch state {
	case agentsv1.AgentInstanceState_AGENT_INSTANCE_STATE_ACTIVE:
		return store.AgentInstanceStateActive, nil
	case agentsv1.AgentInstanceState_AGENT_INSTANCE_STATE_PAUSED:
		return store.AgentInstanceStatePaused, nil
	case agentsv1.AgentInstanceState_AGENT_INSTANCE_STATE_TERMINATED:
		return store.AgentInstanceStateTerminated, nil
	case agentsv1.AgentInstanceState_AGENT_INSTANCE_STATE_UNSPECIFIED:
		return "", fmt.Errorf("must be active, paused, or terminated")
	default:
		return "", fmt.Errorf("unknown value %d", state)
	}
}

func agentInstanceStateToProto(state store.AgentInstanceState) agentsv1.AgentInstanceState {
	switch state {
	case store.AgentInstanceStateActive:
		return agentsv1.AgentInstanceState_AGENT_INSTANCE_STATE_ACTIVE
	case store.AgentInstanceStatePaused:
		return agentsv1.AgentInstanceState_AGENT_INSTANCE_STATE_PAUSED
	case store.AgentInstanceStateTerminated:
		return agentsv1.AgentInstanceState_AGENT_INSTANCE_STATE_TERMINATED
	default:
		panic(fmt.Sprintf("unknown agent instance state %q", state))
	}
}

func inboxItemSourceKindToProto(kind store.InboxItemSourceKind) agentsv1.InboxItemSourceKind {
	switch kind {
	case store.InboxItemSourceKindThread:
		return agentsv1.InboxItemSourceKind_INBOX_ITEM_SOURCE_KIND_THREAD
	case store.InboxItemSourceKindDirect:
		return agentsv1.InboxItemSourceKind_INBOX_ITEM_SOURCE_KIND_DIRECT
	default:
		panic(fmt.Sprintf("unknown inbox item source kind %q", kind))
	}
}

func protoString(value string) *string {
	return &value
}

func toStatusError(err error) error {
	var notFound *store.NotFoundError
	if errors.As(err, &notFound) {
		return status.Error(codes.NotFound, notFound.Error())
	}
	var exists *store.AlreadyExistsError
	if errors.As(err, &exists) {
		return status.Error(codes.AlreadyExists, exists.Error())
	}
	var foreignKey *store.ForeignKeyViolationError
	if errors.As(err, &foreignKey) {
		return status.Error(codes.FailedPrecondition, foreignKey.Error())
	}
	return status.Errorf(codes.Internal, "internal error: %v", err)
}
