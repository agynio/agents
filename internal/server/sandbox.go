package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	imagesv1 "github.com/agynio/agents/.gen/go/agynio/api/images/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	maxSandboxNameLength      = 63
	defaultSandboxIdleTimeout = "30m"
	defaultSandboxTTL         = "72h"
	maxGeneratedNameAttempts  = 8
)

var (
	sandboxNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)
	sandboxAdjectives  = []string{"brave", "calm", "eager", "gentle", "lucky", "nimble", "quiet", "rapid"}
	sandboxNouns       = []string{"badger", "falcon", "otter", "panda", "raven", "tiger", "yak", "zebra"}
)

func (s *Server) CreateEnvironment(ctx context.Context, req *agentsv1.CreateEnvironmentRequest) (*agentsv1.CreateEnvironmentResponse, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	runnerID, err := parseUUID(req.GetRunnerId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "runner_id: %v", err)
	}
	if err := validateSandboxName(req.GetName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "name: %v", err)
	}
	workspace, err := parseImageReference(req.GetWorkspaceImageId(), req.GetWorkspaceImageTag(), "workspace_image")
	if err != nil {
		return nil, err
	}
	agentRuntime, err := parseImageReference(req.GetAgentRuntimeImageId(), req.GetAgentRuntimeImageTag(), "agent_runtime_image")
	if err != nil {
		return nil, err
	}
	// Either a catalog workspace image or the free-form one, until the
	// free-form field goes. An environment with neither names nothing to run.
	if workspace == nil && req.GetImage() == "" {
		return nil, status.Error(codes.InvalidArgument, "workspace_image_id or image is required")
	}
	if workspace != nil {
		if err := s.validateImageReference(ctx, *workspace, organizationID, imagesv1.ImageType_IMAGE_TYPE_WORKSPACE, "workspace_image"); err != nil {
			return nil, err
		}
	}
	if agentRuntime != nil {
		if err := s.validateImageReference(ctx, *agentRuntime, organizationID, imagesv1.ImageType_IMAGE_TYPE_AGENT_RUNTIME, "agent_runtime_image"); err != nil {
			return nil, err
		}
	}

	// The flavor name is deliberately not checked against the runner's catalog:
	// it is resolved at workload start so an environment and the runner
	// configuration naming its flavor can be applied in either order.
	input := store.EnvironmentInput{
		Name:     req.GetName(),
		Image:    req.GetImage(),
		RunnerID: &runnerID,
		Flavor:   req.GetFlavor(),
	}
	if workspace != nil {
		input.WorkspaceImageID = &workspace.ImageID
		input.WorkspaceImageTag = workspace.Tag
	}
	if agentRuntime != nil {
		input.AgentRuntimeImageID = &agentRuntime.ImageID
		input.AgentRuntimeImageTag = agentRuntime.Tag
	}
	environment, err := s.store.CreateEnvironment(ctx, organizationID, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.CreateEnvironmentResponse{Environment: toProtoEnvironment(environment)}, nil
}

func (s *Server) GetEnvironment(ctx context.Context, req *agentsv1.GetEnvironmentRequest) (*agentsv1.GetEnvironmentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	environment, err := s.store.GetEnvironment(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.GetEnvironmentResponse{Environment: toProtoEnvironment(environment)}, nil
}

func (s *Server) UpdateEnvironment(ctx context.Context, req *agentsv1.UpdateEnvironmentRequest) (*agentsv1.UpdateEnvironmentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Name == nil && req.Image == nil && req.RunnerId == nil && req.Flavor == nil &&
		req.WorkspaceImageId == nil && req.WorkspaceImageTag == nil &&
		req.AgentRuntimeImageId == nil && req.AgentRuntimeImageTag == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}
	update := store.EnvironmentUpdate{}
	if req.Name != nil {
		name := req.GetName()
		if err := validateSandboxName(name); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "name: %v", err)
		}
		update.Name = &name
	}
	if req.RunnerId != nil {
		runnerID, err := parseUUID(req.GetRunnerId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "runner_id: %v", err)
		}
		update.RunnerID = &runnerID
	}
	if req.Flavor != nil {
		flavor := req.GetFlavor()
		update.Flavor = &flavor
	}
	if err := s.applyEnvironmentImageUpdate(ctx, req, id, &update); err != nil {
		return nil, err
	}
	if req.Image != nil {
		image := req.GetImage()
		if image == "" {
			return nil, status.Error(codes.InvalidArgument, "image is required")
		}
		update.Image = &image
	}
	environment, err := s.store.UpdateEnvironment(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.UpdateEnvironmentResponse{Environment: toProtoEnvironment(environment)}, nil
}

func (s *Server) DeleteEnvironment(ctx context.Context, req *agentsv1.DeleteEnvironmentRequest) (*agentsv1.DeleteEnvironmentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteEnvironment(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.DeleteEnvironmentResponse{}, nil
}

func (s *Server) ListEnvironments(ctx context.Context, req *agentsv1.ListEnvironmentsRequest) (*agentsv1.ListEnvironmentsResponse, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	if err := s.requireOrganizationListAccess(ctx, organizationID); err != nil {
		return nil, err
	}
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	result, err := s.store.ListEnvironments(ctx, organizationID, store.EnvironmentFilter{}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	environments, nextToken := mapListResult(result.Environments, result.NextCursor, toProtoEnvironment)
	return &agentsv1.ListEnvironmentsResponse{Environments: environments, NextPageToken: nextToken}, nil
}

func (s *Server) CreateSandbox(ctx context.Context, req *agentsv1.CreateSandboxRequest) (*agentsv1.CreateSandboxResponse, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	environmentID, err := parseUUID(req.GetEnvironmentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "environment_id: %v", err)
	}
	var requestedName *string
	if req.Name != nil {
		name := req.GetName()
		if err := validateSandboxName(name); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "name: %v", err)
		}
		requestedName = &name
	}
	if err := validateDurationString(defaultSandboxIdleTimeout); err != nil {
		return nil, status.Errorf(codes.Internal, "default sandbox idle_timeout: %v", err)
	}
	if err := validateDurationString(defaultSandboxTTL); err != nil {
		return nil, status.Errorf(codes.Internal, "default sandbox ttl: %v", err)
	}
	ownerID, err := identityUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requireOrganizationRelation(ctx, ownerID, organizationID, "can_create_sandbox"); err != nil {
		return nil, err
	}

	var sandbox store.Sandbox
	if requestedName != nil {
		sandbox, err = s.store.CreateSandbox(ctx, organizationID, store.SandboxInput{
			Name:          *requestedName,
			EnvironmentID: environmentID,
			OwnerID:       ownerID,
			Status:        store.SandboxStatusStarting,
			IdleTimeout:   defaultSandboxIdleTimeout,
			TTL:           defaultSandboxTTL,
		})
	} else {
		sandbox, err = s.createSandboxWithGeneratedName(ctx, organizationID, environmentID, ownerID)
	}
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.addSandboxAuthorization(ctx, sandbox.Meta.ID, sandbox.OrganizationID, sandbox.OwnerID); err != nil {
		if rollbackErr := s.store.DeleteSandboxRecord(ctx, sandbox.Meta.ID); rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "authorization failed: %v; rollback failed: %v", err, rollbackErr)
		}
		return nil, err
	}
	s.publishSandboxUpdated(ctx, sandbox)
	return &agentsv1.CreateSandboxResponse{Sandbox: toProtoSandbox(sandbox)}, nil
}

func (s *Server) GetSandbox(ctx context.Context, req *agentsv1.GetSandboxRequest) (*agentsv1.GetSandboxResponse, error) {
	sandbox, err := s.getSandboxFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	// The Runners service resolves a sandbox-owned workload's owner through this
	// record, and the Orchestrator reads it while reconciling. Both reach the
	// service over the mesh carrying no identity, by design — they hold no
	// OpenFGA tuples. A caller that does present an identity is a user request
	// and is checked as one.
	identityID, hasIdentity, err := optionalIdentityUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if hasIdentity && !sandboxReadableWithoutCheck(sandbox, identityID) {
		if err := s.requireSandboxRelation(ctx, identityID, sandbox.Meta.ID, "can_list_all"); err != nil {
			return nil, err
		}
	}
	return &agentsv1.GetSandboxResponse{Sandbox: toProtoSandbox(sandbox)}, nil
}

// sandboxReadableWithoutCheck covers the two callers that need no tuple: the
// owner, who holds one anyway, and the sandbox workload itself.
//
// A sandbox workload authenticates as its sandbox, and this is the record the
// platform services it dials — Gateway, LLM Proxy, Tracing — resolve it through
// to reach its organization and owner. It is not an organization member and
// holds no tuple of its own, so an OpenFGA check would refuse it and the
// resolution every one of those services depends on could not happen. Identity
// equality answers it instead, the same way an agent instance reads its own
// inbox.
func sandboxReadableWithoutCheck(sandbox store.Sandbox, identityID uuid.UUID) bool {
	return sandbox.OwnerID == identityID || sandbox.Meta.ID == identityID
}

func (s *Server) ListSandboxes(ctx context.Context, req *agentsv1.ListSandboxesRequest) (*agentsv1.ListSandboxesResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	// The Agents Orchestrator lists sandboxes to reconcile them and carries no
	// identity, by design — it holds no OpenFGA tuples and reaches this RPC over
	// the mesh rather than the Gateway. It may also name no organization, and
	// then reconciles every one: deriving that set any other way leaves a
	// sandbox in an organization it has not heard of never started. A caller
	// that does present an identity is a user request, must name an
	// organization, and is filtered and checked as one.
	identityID, hasIdentity, err := optionalIdentityUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var organizationID *uuid.UUID
	if raw := req.GetOrganizationId(); raw != "" {
		parsed, parseErr := parseUUID(raw)
		if parseErr != nil {
			return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", parseErr)
		}
		organizationID = &parsed
	} else if hasIdentity {
		return nil, status.Error(codes.InvalidArgument, "organization_id is required")
	}
	var filter store.SandboxFilter
	if hasIdentity {
		filter, err = s.sandboxListFilter(ctx, req, *organizationID, identityID)
		if err != nil {
			return nil, err
		}
	} else {
		filter = store.SandboxFilter{IncludeTerminated: req.GetIncludeTerminated()}
		if ownerID := req.GetOwnerId(); ownerID != "" {
			parsed, parseErr := parseUUID(ownerID)
			if parseErr != nil {
				return nil, status.Errorf(codes.InvalidArgument, "owner_id: %v", parseErr)
			}
			filter.OwnerID = &parsed
		}
	}
	result, err := s.store.ListSandboxes(ctx, organizationID, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	sandboxes, nextToken := mapListResult(result.Sandboxes, result.NextCursor, toProtoSandbox)
	return &agentsv1.ListSandboxesResponse{Sandboxes: sandboxes, NextPageToken: nextToken}, nil
}

func (s *Server) sandboxListFilter(ctx context.Context, req *agentsv1.ListSandboxesRequest, organizationID uuid.UUID, identityID uuid.UUID) (store.SandboxFilter, error) {
	filter := store.SandboxFilter{IncludeTerminated: req.GetIncludeTerminated()}
	if req.OwnerId == nil {
		filter.OwnerID = &identityID
		if err := s.requireOwnSandboxListing(ctx, identityID, organizationID); err != nil {
			return store.SandboxFilter{}, err
		}
		return filter, nil
	}

	if req.GetOwnerId() == "" {
		if err := s.requireOrganizationRelation(ctx, identityID, organizationID, "can_list_sandboxes"); err != nil {
			return store.SandboxFilter{}, err
		}
		return filter, nil
	}

	ownerID, err := parseUUID(req.GetOwnerId())
	if err != nil {
		return store.SandboxFilter{}, status.Errorf(codes.InvalidArgument, "owner_id: %v", err)
	}
	filter.OwnerID = &ownerID
	if ownerID == identityID {
		if err := s.requireOwnSandboxListing(ctx, identityID, organizationID); err != nil {
			return store.SandboxFilter{}, err
		}
		return filter, nil
	}
	if err := s.requireOrganizationRelation(ctx, identityID, organizationID, "can_list_sandboxes"); err != nil {
		return store.SandboxFilter{}, err
	}
	return filter, nil
}

func (s *Server) StopSandbox(ctx context.Context, req *agentsv1.StopSandboxRequest) (*agentsv1.StopSandboxResponse, error) {
	sandbox, err := s.updateSandboxStatusWithAuthorization(ctx, req.GetId(), "can_stop", store.SandboxStatusStopped)
	if err != nil {
		return nil, err
	}
	return &agentsv1.StopSandboxResponse{Sandbox: toProtoSandbox(sandbox)}, nil
}

func (s *Server) DeleteSandbox(ctx context.Context, req *agentsv1.DeleteSandboxRequest) (*agentsv1.DeleteSandboxResponse, error) {
	sandboxID, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	sandbox, err := s.store.GetSandbox(ctx, sandboxID)
	if err != nil {
		return nil, toStatusError(err)
	}
	identityID, err := identityUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requireSandboxRelation(ctx, identityID, sandbox.Meta.ID, "can_delete"); err != nil {
		return nil, err
	}
	terminated := store.SandboxStatusTerminated
	sandbox, err = s.store.UpdateSandbox(ctx, sandbox.Meta.ID, store.SandboxUpdate{Status: &terminated})
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishSandboxUpdated(ctx, sandbox)
	return &agentsv1.DeleteSandboxResponse{Sandbox: toProtoSandbox(sandbox)}, nil
}

func (s *Server) EnsureSandboxRunning(ctx context.Context, req *agentsv1.EnsureSandboxRunningRequest) (*agentsv1.EnsureSandboxRunningResponse, error) {
	sandboxID, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	sandbox, err := s.store.GetSandbox(ctx, sandboxID)
	if err != nil {
		return nil, toStatusError(err)
	}
	identityID, err := identityUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requireSandboxRelation(ctx, identityID, sandbox.Meta.ID, "can_connect"); err != nil {
		return nil, err
	}
	update, shouldUpdate, err := sandboxRestartOnConnectUpdate(sandbox.Status)
	if err != nil {
		return nil, err
	}
	if shouldUpdate {
		sandbox, err = s.store.UpdateSandbox(ctx, sandbox.Meta.ID, update)
		if err != nil {
			return nil, toStatusError(err)
		}
		s.publishSandboxUpdated(ctx, sandbox)
	}
	return &agentsv1.EnsureSandboxRunningResponse{Sandbox: toProtoSandbox(sandbox)}, nil
}

func (s *Server) UpdateSandboxRuntimeState(ctx context.Context, req *agentsv1.UpdateSandboxRuntimeStateRequest) (*agentsv1.UpdateSandboxRuntimeStateResponse, error) {
	sandboxID, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	update, err := sandboxRuntimeStateUpdateFromProto(req)
	if err != nil {
		return nil, err
	}
	sandbox, err := s.store.UpdateSandbox(ctx, sandboxID, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	return s.sandboxRuntimeStateUpdatedResponse(ctx, sandbox), nil
}

func (s *Server) sandboxRuntimeStateUpdatedResponse(ctx context.Context, sandbox store.Sandbox) *agentsv1.UpdateSandboxRuntimeStateResponse {
	s.publishSandboxUpdated(ctx, sandbox)
	return &agentsv1.UpdateSandboxRuntimeStateResponse{Sandbox: toProtoSandbox(sandbox)}
}

func sandboxRestartOnConnectUpdate(sandboxStatus store.SandboxStatus) (store.SandboxUpdate, bool, error) {
	switch sandboxStatus {
	case store.SandboxStatusRunning, store.SandboxStatusStarting:
		return store.SandboxUpdate{}, false, nil
	case store.SandboxStatusStopped, store.SandboxStatusFailed:
		starting := store.SandboxStatusStarting
		return store.SandboxUpdate{Status: &starting, ClearWorkloadID: true}, true, nil
	case store.SandboxStatusTerminated:
		return store.SandboxUpdate{}, false, status.Error(codes.FailedPrecondition, "terminated sandbox cannot be started")
	default:
		panic(fmt.Sprintf("unknown sandbox status %q", sandboxStatus))
	}
}

func (s *Server) UpdateSandboxLastSession(ctx context.Context, req *agentsv1.UpdateSandboxLastSessionRequest) (*agentsv1.UpdateSandboxLastSessionResponse, error) {
	sandboxID, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.GetLastSessionAt() == nil {
		return nil, status.Error(codes.InvalidArgument, "last_session_at is required")
	}
	lastSessionAt := req.GetLastSessionAt().AsTime()
	sandbox, err := s.store.UpdateSandbox(ctx, sandboxID, store.SandboxUpdate{LastSessionAt: &lastSessionAt})
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishSandboxUpdated(ctx, sandbox)
	return &agentsv1.UpdateSandboxLastSessionResponse{Sandbox: toProtoSandbox(sandbox)}, nil
}

func (s *Server) createSandboxWithGeneratedName(ctx context.Context, organizationID uuid.UUID, environmentID uuid.UUID, ownerID uuid.UUID) (store.Sandbox, error) {
	create := func(name string) (store.Sandbox, error) {
		return s.store.CreateSandbox(ctx, organizationID, store.SandboxInput{
			Name:          name,
			EnvironmentID: environmentID,
			OwnerID:       ownerID,
			Status:        store.SandboxStatusStarting,
			IdleTimeout:   defaultSandboxIdleTimeout,
			TTL:           defaultSandboxTTL,
		})
	}
	return createSandboxWithGeneratedName(maxGeneratedNameAttempts, generateSandboxName, create)
}

func createSandboxWithGeneratedName(attempts int, generate func() (string, error), create func(string) (store.Sandbox, error)) (store.Sandbox, error) {
	for range attempts {
		name, err := generate()
		if err != nil {
			return store.Sandbox{}, err
		}
		sandbox, err := create(name)
		if err == nil {
			return sandbox, nil
		}
		var exists *store.AlreadyExistsError
		if !errors.As(err, &exists) {
			return store.Sandbox{}, err
		}
	}
	return store.Sandbox{}, status.Error(codes.ResourceExhausted, "unable to generate a unique sandbox name")
}

func (s *Server) getSandboxFromRequest(ctx context.Context, req *agentsv1.GetSandboxRequest) (store.Sandbox, error) {
	switch ref := req.GetRef().(type) {
	case *agentsv1.GetSandboxRequest_Id:
		id, err := parseUUID(ref.Id)
		if err != nil {
			return store.Sandbox{}, status.Errorf(codes.InvalidArgument, "id: %v", err)
		}
		sandbox, err := s.store.GetSandbox(ctx, id)
		if err != nil {
			return store.Sandbox{}, toStatusError(err)
		}
		return sandbox, nil
	case *agentsv1.GetSandboxRequest_Name:
		if ref.Name == nil {
			return store.Sandbox{}, status.Error(codes.InvalidArgument, "name ref must be provided")
		}
		organizationID, err := parseUUID(ref.Name.GetOrganizationId())
		if err != nil {
			return store.Sandbox{}, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
		}
		if err := validateSandboxName(ref.Name.GetName()); err != nil {
			return store.Sandbox{}, status.Errorf(codes.InvalidArgument, "name: %v", err)
		}
		sandbox, err := s.store.GetSandboxByName(ctx, organizationID, ref.Name.GetName())
		if err != nil {
			return store.Sandbox{}, toStatusError(err)
		}
		return sandbox, nil
	default:
		return store.Sandbox{}, status.Error(codes.InvalidArgument, "id or name must be specified")
	}
}

func (s *Server) updateSandboxStatusWithAuthorization(ctx context.Context, id string, relation string, next store.SandboxStatus) (store.Sandbox, error) {
	sandboxID, err := parseUUID(id)
	if err != nil {
		return store.Sandbox{}, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	sandbox, err := s.store.GetSandbox(ctx, sandboxID)
	if err != nil {
		return store.Sandbox{}, toStatusError(err)
	}
	identityID, err := identityUUIDFromContext(ctx)
	if err != nil {
		return store.Sandbox{}, err
	}
	if err := s.requireSandboxRelation(ctx, identityID, sandbox.Meta.ID, relation); err != nil {
		return store.Sandbox{}, err
	}
	if sandbox.Status == store.SandboxStatusTerminated {
		return store.Sandbox{}, status.Error(codes.FailedPrecondition, "terminated sandbox cannot be updated")
	}
	updated, err := s.store.UpdateSandbox(ctx, sandbox.Meta.ID, store.SandboxUpdate{Status: &next})
	if err != nil {
		return store.Sandbox{}, toStatusError(err)
	}
	s.publishSandboxUpdated(ctx, updated)
	return updated, nil
}

// optionalIdentityUUIDFromContext reports the caller's identity when one is
// present. Absence means an internal caller reaching the service over the mesh
// rather than through the Gateway; a malformed identity is still an error.
func optionalIdentityUUIDFromContext(ctx context.Context) (uuid.UUID, bool, error) {
	identityID, ok := metadataValueFromIncomingContext(ctx, "x-identity-id")
	if !ok {
		return uuid.UUID{}, false, nil
	}
	id, err := parseUUID(identityID)
	if err != nil {
		return uuid.UUID{}, false, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	return id, true, nil
}

func identityUUIDFromContext(ctx context.Context) (uuid.UUID, error) {
	identityID, err := identityIDFromContext(ctx)
	if err != nil {
		return uuid.UUID{}, err
	}
	id, err := parseUUID(identityID)
	if err != nil {
		return uuid.UUID{}, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	return id, nil
}

func validateSandboxName(name string) error {
	if name == "" {
		return fmt.Errorf("value is empty")
	}
	if len(name) > maxSandboxNameLength {
		return fmt.Errorf("must be at most %d characters", maxSandboxNameLength)
	}
	if !sandboxNamePattern.MatchString(name) {
		return fmt.Errorf("must match %s", sandboxNamePattern.String())
	}
	return nil
}

func generateSandboxName() (string, error) {
	return generateSandboxNameWithReader(rand.Reader)
}

func generateSandboxNameWithReader(reader io.Reader) (string, error) {
	adjective, err := randomSandboxWord(reader, sandboxAdjectives)
	if err != nil {
		return "", err
	}
	noun, err := randomSandboxWord(reader, sandboxNouns)
	if err != nil {
		return "", err
	}
	suffix, err := randomHexSuffix(reader)
	if err != nil {
		return "", err
	}
	return adjective + "-" + noun + "-" + suffix, nil
}

func randomSandboxWord(reader io.Reader, words []string) (string, error) {
	index, err := rand.Int(reader, big.NewInt(int64(len(words))))
	if err != nil {
		return "", err
	}
	return words[index.Int64()], nil
}

func randomHexSuffix(reader io.Reader) (string, error) {
	bytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func sandboxStatusToProto(sandboxStatus store.SandboxStatus) agentsv1.SandboxStatus {
	switch sandboxStatus {
	case store.SandboxStatusStarting:
		return agentsv1.SandboxStatus_SANDBOX_STATUS_STARTING
	case store.SandboxStatusRunning:
		return agentsv1.SandboxStatus_SANDBOX_STATUS_RUNNING
	case store.SandboxStatusStopped:
		return agentsv1.SandboxStatus_SANDBOX_STATUS_STOPPED
	case store.SandboxStatusFailed:
		return agentsv1.SandboxStatus_SANDBOX_STATUS_FAILED
	case store.SandboxStatusTerminated:
		return agentsv1.SandboxStatus_SANDBOX_STATUS_TERMINATED
	default:
		panic(fmt.Sprintf("unknown sandbox status %q", sandboxStatus))
	}
}

func sandboxRuntimeStateUpdateFromProto(req *agentsv1.UpdateSandboxRuntimeStateRequest) (store.SandboxUpdate, error) {
	update := store.SandboxUpdate{}
	if req.Status != nil {
		statusValue, err := sandboxStatusFromProto(req.GetStatus())
		if err != nil {
			return store.SandboxUpdate{}, status.Errorf(codes.InvalidArgument, "status: %v", err)
		}
		update.Status = &statusValue
	}
	switch workload := req.GetWorkloadIdUpdate().(type) {
	case *agentsv1.UpdateSandboxRuntimeStateRequest_WorkloadId:
		workloadID, err := parseUUID(workload.WorkloadId)
		if err != nil {
			return store.SandboxUpdate{}, status.Errorf(codes.InvalidArgument, "workload_id: %v", err)
		}
		update.WorkloadID = &workloadID
	case *agentsv1.UpdateSandboxRuntimeStateRequest_ClearWorkloadId:
		if !workload.ClearWorkloadId {
			return store.SandboxUpdate{}, status.Error(codes.InvalidArgument, "clear_workload_id must be true when provided")
		}
		update.ClearWorkloadID = true
	case nil:
	default:
		return store.SandboxUpdate{}, status.Error(codes.InvalidArgument, "unknown workload_id update")
	}
	if update.Status == nil && update.WorkloadID == nil && !update.ClearWorkloadID {
		return store.SandboxUpdate{}, status.Error(codes.InvalidArgument, "at least one runtime state field must be provided")
	}
	return update, nil
}

func sandboxStatusFromProto(sandboxStatus agentsv1.SandboxStatus) (store.SandboxStatus, error) {
	switch sandboxStatus {
	case agentsv1.SandboxStatus_SANDBOX_STATUS_STARTING:
		return store.SandboxStatusStarting, nil
	case agentsv1.SandboxStatus_SANDBOX_STATUS_RUNNING:
		return store.SandboxStatusRunning, nil
	case agentsv1.SandboxStatus_SANDBOX_STATUS_STOPPED:
		return store.SandboxStatusStopped, nil
	case agentsv1.SandboxStatus_SANDBOX_STATUS_FAILED:
		return store.SandboxStatusFailed, nil
	case agentsv1.SandboxStatus_SANDBOX_STATUS_TERMINATED:
		return store.SandboxStatusTerminated, nil
	case agentsv1.SandboxStatus_SANDBOX_STATUS_UNSPECIFIED:
		return "", fmt.Errorf("must be starting, running, stopped, failed, or terminated")
	default:
		return "", fmt.Errorf("unknown value %d", sandboxStatus)
	}
}
