package server

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	identityv1 "github.com/agynio/agents/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	agentsv1.UnimplementedAgentsServiceServer
	store    *store.Store
	authz    AuthorizationWriter
	identity IdentityWriter
}

const maxMcpNameLength = 63

var mcpNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func New(store *store.Store, authz AuthorizationWriter, identity IdentityWriter) *Server {
	if store == nil {
		panic("store is required")
	}
	if authz == nil {
		panic("authorization client is required")
	}
	if identity == nil {
		panic("identity client is required")
	}
	return &Server{store: store, authz: authz, identity: identity}
}

func (s *Server) registerAgentIdentity(ctx context.Context, agentID uuid.UUID) error {
	_, err := s.identity.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{
		IdentityId:   agentID.String(),
		IdentityType: identityv1.IdentityType_IDENTITY_TYPE_AGENT,
	})
	return err
}

func (s *Server) CreateAgent(ctx context.Context, req *agentsv1.CreateAgentRequest) (*agentsv1.CreateAgentResponse, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	modelID, err := parseUUID(req.GetModel())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "model: %v", err)
	}
	if req.GetInitImage() == "" {
		return nil, status.Error(codes.InvalidArgument, "init_image is required")
	}
	resources := toStoreComputeResources(req.GetResources())
	agent, err := s.store.CreateAgent(ctx, organizationID, store.AgentInput{
		Name:          req.GetName(),
		Role:          req.GetRole(),
		Model:         modelID,
		Description:   req.GetDescription(),
		Configuration: req.GetConfiguration(),
		Image:         req.GetImage(),
		InitImage:     req.GetInitImage(),
		Resources:     resources,
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.addAgentMembership(ctx, agent.Meta.ID, agent.OrganizationID); err != nil {
		rollbackErr := s.store.DeleteAgent(ctx, agent.Meta.ID)
		if rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "authorization write failed: %v; rollback failed: %v", err, rollbackErr)
		}
		return nil, status.Errorf(codes.Internal, "authorization write failed: %v", err)
	}
	if err := s.registerAgentIdentity(ctx, agent.Meta.ID); err != nil {
		rollbackErr := errors.Join(
			s.removeAgentMembership(ctx, agent.Meta.ID, agent.OrganizationID),
			s.store.DeleteAgent(ctx, agent.Meta.ID),
		)
		if rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "register identity: %v; rollback: %v", err, rollbackErr)
		}
		return nil, status.Errorf(codes.Internal, "register identity: %v", err)
	}
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

func (s *Server) UpdateAgent(ctx context.Context, req *agentsv1.UpdateAgentRequest) (*agentsv1.UpdateAgentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Name == nil && req.Role == nil && req.Model == nil && req.Description == nil && req.Configuration == nil && req.Image == nil && req.InitImage == nil && req.Resources == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}
	if req.InitImage != nil && req.GetInitImage() == "" {
		return nil, status.Error(codes.InvalidArgument, "init_image must not be empty")
	}

	update := store.AgentUpdate{}
	if req.Name != nil {
		value := req.GetName()
		update.Name = &value
	}
	if req.Role != nil {
		value := req.GetRole()
		update.Role = &value
	}
	if req.Model != nil {
		modelID, err := parseUUID(req.GetModel())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "model: %v", err)
		}
		update.Model = &modelID
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
	if req.Resources != nil {
		resources := toStoreComputeResources(req.GetResources())
		update.Resources = &resources
	}

	agent, err := s.store.UpdateAgent(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
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
	if err := s.removeAgentMembership(ctx, agent.Meta.ID, agent.OrganizationID); err != nil {
		return nil, status.Errorf(codes.Internal, "authorization delete failed: %v", err)
	}
	if err := s.store.DeleteAgent(ctx, id); err != nil {
		rollbackErr := s.addAgentMembership(ctx, agent.Meta.ID, agent.OrganizationID)
		if rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "agent delete failed: %v; authorization rollback failed: %v", err, rollbackErr)
		}
		return nil, toStatusError(err)
	}
	return &agentsv1.DeleteAgentResponse{}, nil
}

func (s *Server) ListAgents(ctx context.Context, req *agentsv1.ListAgentsRequest) (*agentsv1.ListAgentsResponse, error) {
	var organizationID *uuid.UUID
	organizationValue := req.GetOrganizationId()
	if organizationValue != "" {
		parsedOrganizationID, err := parseUUID(organizationValue)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
		}
		organizationID = &parsedOrganizationID
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

func (s *Server) CreateVolume(ctx context.Context, req *agentsv1.CreateVolumeRequest) (*agentsv1.CreateVolumeResponse, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	volume, err := s.store.CreateVolume(ctx, organizationID, store.VolumeInput{
		Persistent:  req.GetPersistent(),
		MountPath:   req.GetMountPath(),
		Size:        req.GetSize(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
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
	return &agentsv1.GetVolumeResponse{Volume: toProtoVolume(volume)}, nil
}

func (s *Server) UpdateVolume(ctx context.Context, req *agentsv1.UpdateVolumeRequest) (*agentsv1.UpdateVolumeResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Persistent == nil && req.MountPath == nil && req.Size == nil && req.Description == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	update := store.VolumeUpdate{}
	if req.Persistent != nil {
		value := req.GetPersistent()
		update.Persistent = &value
	}
	if req.MountPath != nil {
		value := req.GetMountPath()
		update.MountPath = &value
	}
	if req.Size != nil {
		value := req.GetSize()
		update.Size = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}

	volume, err := s.store.UpdateVolume(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.UpdateVolumeResponse{Volume: toProtoVolume(volume)}, nil
}

func (s *Server) DeleteVolume(ctx context.Context, req *agentsv1.DeleteVolumeRequest) (*agentsv1.DeleteVolumeResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteVolume(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.DeleteVolumeResponse{}, nil
}

func (s *Server) ListVolumes(ctx context.Context, req *agentsv1.ListVolumesRequest) (*agentsv1.ListVolumesResponse, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	result, err := s.store.ListVolumes(ctx, organizationID, store.VolumeFilter{}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	volumes, nextToken := mapListResult(result.Volumes, result.NextCursor, toProtoVolume)
	return &agentsv1.ListVolumesResponse{Volumes: volumes, NextPageToken: nextToken}, nil
}

func (s *Server) CreateVolumeAttachment(ctx context.Context, req *agentsv1.CreateVolumeAttachmentRequest) (*agentsv1.CreateVolumeAttachmentResponse, error) {
	volumeID, err := parseUUID(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "volume_id: %v", err)
	}

	input := store.VolumeAttachmentInput{VolumeID: volumeID}
	switch target := req.GetTarget().(type) {
	case *agentsv1.CreateVolumeAttachmentRequest_AgentId:
		id, err := parseUUID(target.AgentId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		input.AgentID = &id
	case *agentsv1.CreateVolumeAttachmentRequest_McpId:
		id, err := parseUUID(target.McpId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		input.McpID = &id
	case *agentsv1.CreateVolumeAttachmentRequest_HookId:
		id, err := parseUUID(target.HookId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		input.HookID = &id
	default:
		return nil, status.Error(codes.InvalidArgument, "target must be specified")
	}

	attachment, err := s.store.CreateVolumeAttachment(ctx, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.CreateVolumeAttachmentResponse{VolumeAttachment: toProtoVolumeAttachment(attachment)}, nil
}

func (s *Server) GetVolumeAttachment(ctx context.Context, req *agentsv1.GetVolumeAttachmentRequest) (*agentsv1.GetVolumeAttachmentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	attachment, err := s.store.GetVolumeAttachment(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.GetVolumeAttachmentResponse{VolumeAttachment: toProtoVolumeAttachment(attachment)}, nil
}

func (s *Server) DeleteVolumeAttachment(ctx context.Context, req *agentsv1.DeleteVolumeAttachmentRequest) (*agentsv1.DeleteVolumeAttachmentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteVolumeAttachment(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.DeleteVolumeAttachmentResponse{}, nil
}

func (s *Server) ListVolumeAttachments(ctx context.Context, req *agentsv1.ListVolumeAttachmentsRequest) (*agentsv1.ListVolumeAttachmentsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	hasFilter := false
	filter := store.VolumeAttachmentFilter{}
	if req.GetVolumeId() != "" {
		hasFilter = true
		volumeID, err := parseUUID(req.GetVolumeId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "volume_id: %v", err)
		}
		filter.VolumeID = &volumeID
	}
	if req.GetAgentId() != "" {
		hasFilter = true
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		filter.AgentID = &agentID
	}
	if req.GetMcpId() != "" {
		hasFilter = true
		mcpID, err := parseUUID(req.GetMcpId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		filter.McpID = &mcpID
	}
	if req.GetHookId() != "" {
		hasFilter = true
		hookID, err := parseUUID(req.GetHookId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		filter.HookID = &hookID
	}
	if !hasFilter {
		return nil, status.Error(codes.InvalidArgument, "at least one filter must be provided")
	}

	result, err := s.store.ListVolumeAttachments(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	attachments, nextToken := mapListResult(result.VolumeAttachments, result.NextCursor, toProtoVolumeAttachment)
	return &agentsv1.ListVolumeAttachmentsResponse{VolumeAttachments: attachments, NextPageToken: nextToken}, nil
}

func (s *Server) CreateImagePullSecretAttachment(ctx context.Context, req *agentsv1.CreateImagePullSecretAttachmentRequest) (*agentsv1.CreateImagePullSecretAttachmentResponse, error) {
	imagePullSecretID, err := parseUUID(req.GetImagePullSecretId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "image_pull_secret_id: %v", err)
	}

	input := store.ImagePullSecretAttachmentInput{ImagePullSecretID: imagePullSecretID}
	switch target := req.GetTarget().(type) {
	case *agentsv1.CreateImagePullSecretAttachmentRequest_AgentId:
		id, err := parseUUID(target.AgentId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		input.AgentID = &id
	case *agentsv1.CreateImagePullSecretAttachmentRequest_McpId:
		id, err := parseUUID(target.McpId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		input.McpID = &id
	case *agentsv1.CreateImagePullSecretAttachmentRequest_HookId:
		id, err := parseUUID(target.HookId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		input.HookID = &id
	default:
		return nil, status.Error(codes.InvalidArgument, "target must be specified")
	}

	attachment, err := s.store.CreateImagePullSecretAttachment(ctx, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.CreateImagePullSecretAttachmentResponse{ImagePullSecretAttachment: toProtoImagePullSecretAttachment(attachment)}, nil
}

func (s *Server) GetImagePullSecretAttachment(ctx context.Context, req *agentsv1.GetImagePullSecretAttachmentRequest) (*agentsv1.GetImagePullSecretAttachmentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	attachment, err := s.store.GetImagePullSecretAttachment(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.GetImagePullSecretAttachmentResponse{ImagePullSecretAttachment: toProtoImagePullSecretAttachment(attachment)}, nil
}

func (s *Server) DeleteImagePullSecretAttachment(ctx context.Context, req *agentsv1.DeleteImagePullSecretAttachmentRequest) (*agentsv1.DeleteImagePullSecretAttachmentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteImagePullSecretAttachment(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.DeleteImagePullSecretAttachmentResponse{}, nil
}

func (s *Server) ListImagePullSecretAttachments(ctx context.Context, req *agentsv1.ListImagePullSecretAttachmentsRequest) (*agentsv1.ListImagePullSecretAttachmentsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	hasFilter := false
	filter := store.ImagePullSecretAttachmentFilter{}
	if req.GetImagePullSecretId() != "" {
		hasFilter = true
		imagePullSecretID, err := parseUUID(req.GetImagePullSecretId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "image_pull_secret_id: %v", err)
		}
		filter.ImagePullSecretID = &imagePullSecretID
	}
	if req.GetAgentId() != "" {
		hasFilter = true
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		filter.AgentID = &agentID
	}
	if req.GetMcpId() != "" {
		hasFilter = true
		mcpID, err := parseUUID(req.GetMcpId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		filter.McpID = &mcpID
	}
	if req.GetHookId() != "" {
		hasFilter = true
		hookID, err := parseUUID(req.GetHookId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		filter.HookID = &hookID
	}
	if !hasFilter {
		return nil, status.Error(codes.InvalidArgument, "at least one filter must be provided")
	}

	result, err := s.store.ListImagePullSecretAttachments(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	attachments, nextToken := mapListResult(result.ImagePullSecretAttachments, result.NextCursor, toProtoImagePullSecretAttachment)
	return &agentsv1.ListImagePullSecretAttachmentsResponse{ImagePullSecretAttachments: attachments, NextPageToken: nextToken}, nil
}

func (s *Server) CreateMcp(ctx context.Context, req *agentsv1.CreateMcpRequest) (*agentsv1.CreateMcpResponse, error) {
	agentID, err := parseUUID(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
	}
	name := req.GetName()
	if err := validateMcpName(name); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "name: %v", err)
	}
	resources := toStoreComputeResources(req.GetResources())
	mcp, err := s.store.CreateMcp(ctx, store.McpInput{
		AgentID:     agentID,
		Name:        name,
		Image:       req.GetImage(),
		Command:     req.GetCommand(),
		Resources:   resources,
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
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
	return &agentsv1.GetMcpResponse{Mcp: toProtoMcp(mcp)}, nil
}

func (s *Server) UpdateMcp(ctx context.Context, req *agentsv1.UpdateMcpRequest) (*agentsv1.UpdateMcpResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Image == nil && req.Command == nil && req.Resources == nil && req.Description == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	update := store.McpUpdate{}
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
	return &agentsv1.UpdateMcpResponse{Mcp: toProtoMcp(mcp)}, nil
}

func (s *Server) DeleteMcp(ctx context.Context, req *agentsv1.DeleteMcpRequest) (*agentsv1.DeleteMcpResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteMcp(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.DeleteMcpResponse{}, nil
}

func (s *Server) ListMcps(ctx context.Context, req *agentsv1.ListMcpsRequest) (*agentsv1.ListMcpsResponse, error) {
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
	filter := store.McpFilter{AgentID: &agentID}

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
	skill, err := s.store.CreateSkill(ctx, store.SkillInput{
		AgentID:     agentID,
		Name:        req.GetName(),
		Body:        req.GetBody(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
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
	return &agentsv1.UpdateSkillResponse{Skill: toProtoSkill(skill)}, nil
}

func (s *Server) DeleteSkill(ctx context.Context, req *agentsv1.DeleteSkillRequest) (*agentsv1.DeleteSkillResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteSkill(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
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
	filter := store.SkillFilter{AgentID: &agentID}

	result, err := s.store.ListSkills(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	skills, nextToken := mapListResult(result.Skills, result.NextCursor, toProtoSkill)
	return &agentsv1.ListSkillsResponse{Skills: skills, NextPageToken: nextToken}, nil
}

func (s *Server) CreateHook(ctx context.Context, req *agentsv1.CreateHookRequest) (*agentsv1.CreateHookResponse, error) {
	agentID, err := parseUUID(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
	}
	resources := toStoreComputeResources(req.GetResources())
	hook, err := s.store.CreateHook(ctx, store.HookInput{
		AgentID:     agentID,
		Event:       req.GetEvent(),
		Function:    req.GetFunction(),
		Image:       req.GetImage(),
		Resources:   resources,
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.CreateHookResponse{Hook: toProtoHook(hook)}, nil
}

func (s *Server) GetHook(ctx context.Context, req *agentsv1.GetHookRequest) (*agentsv1.GetHookResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	hook, err := s.store.GetHook(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.GetHookResponse{Hook: toProtoHook(hook)}, nil
}

func (s *Server) UpdateHook(ctx context.Context, req *agentsv1.UpdateHookRequest) (*agentsv1.UpdateHookResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Event == nil && req.Function == nil && req.Image == nil && req.Resources == nil && req.Description == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	update := store.HookUpdate{}
	if req.Event != nil {
		value := req.GetEvent()
		update.Event = &value
	}
	if req.Function != nil {
		value := req.GetFunction()
		update.Function = &value
	}
	if req.Image != nil {
		value := req.GetImage()
		update.Image = &value
	}
	if req.Resources != nil {
		resources := toStoreComputeResources(req.GetResources())
		update.Resources = &resources
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}

	hook, err := s.store.UpdateHook(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.UpdateHookResponse{Hook: toProtoHook(hook)}, nil
}

func (s *Server) DeleteHook(ctx context.Context, req *agentsv1.DeleteHookRequest) (*agentsv1.DeleteHookResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteHook(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.DeleteHookResponse{}, nil
}

func (s *Server) ListHooks(ctx context.Context, req *agentsv1.ListHooksRequest) (*agentsv1.ListHooksResponse, error) {
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
	filter := store.HookFilter{AgentID: &agentID}

	result, err := s.store.ListHooks(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	hooks, nextToken := mapListResult(result.Hooks, result.NextCursor, toProtoHook)
	return &agentsv1.ListHooksResponse{Hooks: hooks, NextPageToken: nextToken}, nil
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
	case *agentsv1.CreateEnvRequest_HookId:
		id, err := parseUUID(target.HookId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		input.HookID = &id
	default:
		return nil, status.Error(codes.InvalidArgument, "target must be specified")
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
	return &agentsv1.UpdateEnvResponse{Env: toProtoEnv(env)}, nil
}

func (s *Server) DeleteEnv(ctx context.Context, req *agentsv1.DeleteEnvRequest) (*agentsv1.DeleteEnvResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteEnv(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.DeleteEnvResponse{}, nil
}

func (s *Server) ListEnvs(ctx context.Context, req *agentsv1.ListEnvsRequest) (*agentsv1.ListEnvsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	hasFilter := false
	filter := store.EnvFilter{}
	if req.GetAgentId() != "" {
		hasFilter = true
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		filter.AgentID = &agentID
	}
	if req.GetMcpId() != "" {
		hasFilter = true
		mcpID, err := parseUUID(req.GetMcpId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		filter.McpID = &mcpID
	}
	if req.GetHookId() != "" {
		hasFilter = true
		hookID, err := parseUUID(req.GetHookId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		filter.HookID = &hookID
	}
	if !hasFilter {
		return nil, status.Error(codes.InvalidArgument, "at least one filter must be provided")
	}

	result, err := s.store.ListEnvs(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	envs, nextToken := mapListResult(result.Envs, result.NextCursor, toProtoEnv)
	return &agentsv1.ListEnvsResponse{Envs: envs, NextPageToken: nextToken}, nil
}

func (s *Server) CreateInitScript(ctx context.Context, req *agentsv1.CreateInitScriptRequest) (*agentsv1.CreateInitScriptResponse, error) {
	input := store.InitScriptInput{
		Script:      req.GetScript(),
		Description: req.GetDescription(),
	}

	switch target := req.GetTarget().(type) {
	case *agentsv1.CreateInitScriptRequest_AgentId:
		id, err := parseUUID(target.AgentId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		input.AgentID = &id
	case *agentsv1.CreateInitScriptRequest_McpId:
		id, err := parseUUID(target.McpId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		input.McpID = &id
	case *agentsv1.CreateInitScriptRequest_HookId:
		id, err := parseUUID(target.HookId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		input.HookID = &id
	default:
		return nil, status.Error(codes.InvalidArgument, "target must be specified")
	}

	script, err := s.store.CreateInitScript(ctx, input)
	if err != nil {
		return nil, toStatusError(err)
	}
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
	return &agentsv1.UpdateInitScriptResponse{InitScript: toProtoInitScript(script)}, nil
}

func (s *Server) DeleteInitScript(ctx context.Context, req *agentsv1.DeleteInitScriptRequest) (*agentsv1.DeleteInitScriptResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteInitScript(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.DeleteInitScriptResponse{}, nil
}

func (s *Server) ListInitScripts(ctx context.Context, req *agentsv1.ListInitScriptsRequest) (*agentsv1.ListInitScriptsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	hasFilter := false
	filter := store.InitScriptFilter{}
	if req.GetAgentId() != "" {
		hasFilter = true
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		filter.AgentID = &agentID
	}
	if req.GetMcpId() != "" {
		hasFilter = true
		mcpID, err := parseUUID(req.GetMcpId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		filter.McpID = &mcpID
	}
	if req.GetHookId() != "" {
		hasFilter = true
		hookID, err := parseUUID(req.GetHookId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		filter.HookID = &hookID
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
