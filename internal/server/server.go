package server

import (
	"context"
	"errors"
	"fmt"

	teamsv1 "github.com/agynio/teams/.gen/go/agynio/api/teams/v1"
	"github.com/agynio/teams/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	teamsv1.UnimplementedTeamsServiceServer
	store *store.Store
}

func New(store *store.Store) *Server {
	return &Server{store: store}
}

func (s *Server) CreateAgent(ctx context.Context, req *teamsv1.CreateAgentRequest) (*teamsv1.CreateAgentResponse, error) {
	modelID, err := parseUUID(req.GetModel())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "model: %v", err)
	}
	resources := toStoreComputeResources(req.GetResources())
	agent, err := s.store.CreateAgent(ctx, store.AgentInput{
		Name:          req.GetName(),
		Role:          req.GetRole(),
		Model:         modelID,
		Description:   req.GetDescription(),
		Configuration: req.GetConfiguration(),
		Image:         req.GetImage(),
		Resources:     resources,
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.CreateAgentResponse{Agent: toProtoAgent(agent)}, nil
}

func (s *Server) GetAgent(ctx context.Context, req *teamsv1.GetAgentRequest) (*teamsv1.GetAgentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	agent, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.GetAgentResponse{Agent: toProtoAgent(agent)}, nil
}

func (s *Server) UpdateAgent(ctx context.Context, req *teamsv1.UpdateAgentRequest) (*teamsv1.UpdateAgentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Name == nil && req.Role == nil && req.Model == nil && req.Description == nil && req.Configuration == nil && req.Image == nil && req.Resources == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
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
	if req.Resources != nil {
		resources := toStoreComputeResources(req.GetResources())
		update.Resources = &resources
	}

	agent, err := s.store.UpdateAgent(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.UpdateAgentResponse{Agent: toProtoAgent(agent)}, nil
}

func (s *Server) DeleteAgent(ctx context.Context, req *teamsv1.DeleteAgentRequest) (*teamsv1.DeleteAgentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteAgent(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteAgentResponse{}, nil
}

func (s *Server) ListAgents(ctx context.Context, req *teamsv1.ListAgentsRequest) (*teamsv1.ListAgentsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	result, err := s.store.ListAgents(ctx, store.AgentFilter{}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	agents, nextToken := mapListResult(result.Agents, result.NextCursor, toProtoAgent)
	return &teamsv1.ListAgentsResponse{Agents: agents, NextPageToken: nextToken}, nil
}

func (s *Server) CreateVolume(ctx context.Context, req *teamsv1.CreateVolumeRequest) (*teamsv1.CreateVolumeResponse, error) {
	volume, err := s.store.CreateVolume(ctx, store.VolumeInput{
		Persistent:  req.GetPersistent(),
		MountPath:   req.GetMountPath(),
		Size:        req.GetSize(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.CreateVolumeResponse{Volume: toProtoVolume(volume)}, nil
}

func (s *Server) GetVolume(ctx context.Context, req *teamsv1.GetVolumeRequest) (*teamsv1.GetVolumeResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	volume, err := s.store.GetVolume(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.GetVolumeResponse{Volume: toProtoVolume(volume)}, nil
}

func (s *Server) UpdateVolume(ctx context.Context, req *teamsv1.UpdateVolumeRequest) (*teamsv1.UpdateVolumeResponse, error) {
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
	return &teamsv1.UpdateVolumeResponse{Volume: toProtoVolume(volume)}, nil
}

func (s *Server) DeleteVolume(ctx context.Context, req *teamsv1.DeleteVolumeRequest) (*teamsv1.DeleteVolumeResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteVolume(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteVolumeResponse{}, nil
}

func (s *Server) ListVolumes(ctx context.Context, req *teamsv1.ListVolumesRequest) (*teamsv1.ListVolumesResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	result, err := s.store.ListVolumes(ctx, store.VolumeFilter{}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	volumes, nextToken := mapListResult(result.Volumes, result.NextCursor, toProtoVolume)
	return &teamsv1.ListVolumesResponse{Volumes: volumes, NextPageToken: nextToken}, nil
}

func (s *Server) CreateVolumeAttachment(ctx context.Context, req *teamsv1.CreateVolumeAttachmentRequest) (*teamsv1.CreateVolumeAttachmentResponse, error) {
	volumeID, err := parseUUID(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "volume_id: %v", err)
	}

	input := store.VolumeAttachmentInput{VolumeID: volumeID}
	switch target := req.GetTarget().(type) {
	case *teamsv1.CreateVolumeAttachmentRequest_AgentId:
		id, err := parseUUID(target.AgentId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		input.AgentID = &id
	case *teamsv1.CreateVolumeAttachmentRequest_McpId:
		id, err := parseUUID(target.McpId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		input.McpID = &id
	case *teamsv1.CreateVolumeAttachmentRequest_HookId:
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
	return &teamsv1.CreateVolumeAttachmentResponse{VolumeAttachment: toProtoVolumeAttachment(attachment)}, nil
}

func (s *Server) GetVolumeAttachment(ctx context.Context, req *teamsv1.GetVolumeAttachmentRequest) (*teamsv1.GetVolumeAttachmentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	attachment, err := s.store.GetVolumeAttachment(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.GetVolumeAttachmentResponse{VolumeAttachment: toProtoVolumeAttachment(attachment)}, nil
}

func (s *Server) DeleteVolumeAttachment(ctx context.Context, req *teamsv1.DeleteVolumeAttachmentRequest) (*teamsv1.DeleteVolumeAttachmentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteVolumeAttachment(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteVolumeAttachmentResponse{}, nil
}

func (s *Server) ListVolumeAttachments(ctx context.Context, req *teamsv1.ListVolumeAttachmentsRequest) (*teamsv1.ListVolumeAttachmentsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	filter := store.VolumeAttachmentFilter{}
	if req.GetVolumeId() != "" {
		volumeID, err := parseUUID(req.GetVolumeId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "volume_id: %v", err)
		}
		filter.VolumeID = &volumeID
	}
	if req.GetAgentId() != "" {
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		filter.AgentID = &agentID
	}
	if req.GetMcpId() != "" {
		mcpID, err := parseUUID(req.GetMcpId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		filter.McpID = &mcpID
	}
	if req.GetHookId() != "" {
		hookID, err := parseUUID(req.GetHookId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		filter.HookID = &hookID
	}

	result, err := s.store.ListVolumeAttachments(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	attachments, nextToken := mapListResult(result.VolumeAttachments, result.NextCursor, toProtoVolumeAttachment)
	return &teamsv1.ListVolumeAttachmentsResponse{VolumeAttachments: attachments, NextPageToken: nextToken}, nil
}

func (s *Server) CreateMcp(ctx context.Context, req *teamsv1.CreateMcpRequest) (*teamsv1.CreateMcpResponse, error) {
	agentID, err := parseUUID(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
	}
	resources := toStoreComputeResources(req.GetResources())
	mcp, err := s.store.CreateMcp(ctx, store.McpInput{
		AgentID:     agentID,
		Image:       req.GetImage(),
		Command:     req.GetCommand(),
		Resources:   resources,
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.CreateMcpResponse{Mcp: toProtoMcp(mcp)}, nil
}

func (s *Server) GetMcp(ctx context.Context, req *teamsv1.GetMcpRequest) (*teamsv1.GetMcpResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	mcp, err := s.store.GetMcp(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.GetMcpResponse{Mcp: toProtoMcp(mcp)}, nil
}

func (s *Server) UpdateMcp(ctx context.Context, req *teamsv1.UpdateMcpRequest) (*teamsv1.UpdateMcpResponse, error) {
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
	return &teamsv1.UpdateMcpResponse{Mcp: toProtoMcp(mcp)}, nil
}

func (s *Server) DeleteMcp(ctx context.Context, req *teamsv1.DeleteMcpRequest) (*teamsv1.DeleteMcpResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteMcp(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteMcpResponse{}, nil
}

func (s *Server) ListMcps(ctx context.Context, req *teamsv1.ListMcpsRequest) (*teamsv1.ListMcpsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	filter := store.McpFilter{}
	if req.GetAgentId() != "" {
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		filter.AgentID = &agentID
	}

	result, err := s.store.ListMcps(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	mcps, nextToken := mapListResult(result.Mcps, result.NextCursor, toProtoMcp)
	return &teamsv1.ListMcpsResponse{Mcps: mcps, NextPageToken: nextToken}, nil
}

func (s *Server) CreateSkill(ctx context.Context, req *teamsv1.CreateSkillRequest) (*teamsv1.CreateSkillResponse, error) {
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
	return &teamsv1.CreateSkillResponse{Skill: toProtoSkill(skill)}, nil
}

func (s *Server) GetSkill(ctx context.Context, req *teamsv1.GetSkillRequest) (*teamsv1.GetSkillResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	skill, err := s.store.GetSkill(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.GetSkillResponse{Skill: toProtoSkill(skill)}, nil
}

func (s *Server) UpdateSkill(ctx context.Context, req *teamsv1.UpdateSkillRequest) (*teamsv1.UpdateSkillResponse, error) {
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
	return &teamsv1.UpdateSkillResponse{Skill: toProtoSkill(skill)}, nil
}

func (s *Server) DeleteSkill(ctx context.Context, req *teamsv1.DeleteSkillRequest) (*teamsv1.DeleteSkillResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteSkill(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteSkillResponse{}, nil
}

func (s *Server) ListSkills(ctx context.Context, req *teamsv1.ListSkillsRequest) (*teamsv1.ListSkillsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	filter := store.SkillFilter{}
	if req.GetAgentId() != "" {
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		filter.AgentID = &agentID
	}

	result, err := s.store.ListSkills(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	skills, nextToken := mapListResult(result.Skills, result.NextCursor, toProtoSkill)
	return &teamsv1.ListSkillsResponse{Skills: skills, NextPageToken: nextToken}, nil
}

func (s *Server) CreateHook(ctx context.Context, req *teamsv1.CreateHookRequest) (*teamsv1.CreateHookResponse, error) {
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
	return &teamsv1.CreateHookResponse{Hook: toProtoHook(hook)}, nil
}

func (s *Server) GetHook(ctx context.Context, req *teamsv1.GetHookRequest) (*teamsv1.GetHookResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	hook, err := s.store.GetHook(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.GetHookResponse{Hook: toProtoHook(hook)}, nil
}

func (s *Server) UpdateHook(ctx context.Context, req *teamsv1.UpdateHookRequest) (*teamsv1.UpdateHookResponse, error) {
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
	return &teamsv1.UpdateHookResponse{Hook: toProtoHook(hook)}, nil
}

func (s *Server) DeleteHook(ctx context.Context, req *teamsv1.DeleteHookRequest) (*teamsv1.DeleteHookResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteHook(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteHookResponse{}, nil
}

func (s *Server) ListHooks(ctx context.Context, req *teamsv1.ListHooksRequest) (*teamsv1.ListHooksResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	filter := store.HookFilter{}
	if req.GetAgentId() != "" {
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		filter.AgentID = &agentID
	}

	result, err := s.store.ListHooks(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	hooks, nextToken := mapListResult(result.Hooks, result.NextCursor, toProtoHook)
	return &teamsv1.ListHooksResponse{Hooks: hooks, NextPageToken: nextToken}, nil
}

func (s *Server) CreateEnv(ctx context.Context, req *teamsv1.CreateEnvRequest) (*teamsv1.CreateEnvResponse, error) {
	input := store.EnvInput{
		Name:        req.GetName(),
		Description: req.GetDescription(),
	}

	switch target := req.GetTarget().(type) {
	case *teamsv1.CreateEnvRequest_AgentId:
		id, err := parseUUID(target.AgentId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		input.AgentID = &id
	case *teamsv1.CreateEnvRequest_McpId:
		id, err := parseUUID(target.McpId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		input.McpID = &id
	case *teamsv1.CreateEnvRequest_HookId:
		id, err := parseUUID(target.HookId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		input.HookID = &id
	default:
		return nil, status.Error(codes.InvalidArgument, "target must be specified")
	}

	switch source := req.GetSource().(type) {
	case *teamsv1.CreateEnvRequest_Value:
		value := source.Value
		input.Value = &value
	case *teamsv1.CreateEnvRequest_SecretId:
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
	return &teamsv1.CreateEnvResponse{Env: toProtoEnv(env)}, nil
}

func (s *Server) GetEnv(ctx context.Context, req *teamsv1.GetEnvRequest) (*teamsv1.GetEnvResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	env, err := s.store.GetEnv(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.GetEnvResponse{Env: toProtoEnv(env)}, nil
}

func (s *Server) UpdateEnv(ctx context.Context, req *teamsv1.UpdateEnvRequest) (*teamsv1.UpdateEnvResponse, error) {
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
	return &teamsv1.UpdateEnvResponse{Env: toProtoEnv(env)}, nil
}

func (s *Server) DeleteEnv(ctx context.Context, req *teamsv1.DeleteEnvRequest) (*teamsv1.DeleteEnvResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteEnv(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteEnvResponse{}, nil
}

func (s *Server) ListEnvs(ctx context.Context, req *teamsv1.ListEnvsRequest) (*teamsv1.ListEnvsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	filter := store.EnvFilter{}
	if req.GetAgentId() != "" {
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		filter.AgentID = &agentID
	}
	if req.GetMcpId() != "" {
		mcpID, err := parseUUID(req.GetMcpId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		filter.McpID = &mcpID
	}
	if req.GetHookId() != "" {
		hookID, err := parseUUID(req.GetHookId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		filter.HookID = &hookID
	}

	result, err := s.store.ListEnvs(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	envs, nextToken := mapListResult(result.Envs, result.NextCursor, toProtoEnv)
	return &teamsv1.ListEnvsResponse{Envs: envs, NextPageToken: nextToken}, nil
}

func (s *Server) CreateInitScript(ctx context.Context, req *teamsv1.CreateInitScriptRequest) (*teamsv1.CreateInitScriptResponse, error) {
	input := store.InitScriptInput{
		Script:      req.GetScript(),
		Description: req.GetDescription(),
	}

	switch target := req.GetTarget().(type) {
	case *teamsv1.CreateInitScriptRequest_AgentId:
		id, err := parseUUID(target.AgentId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		input.AgentID = &id
	case *teamsv1.CreateInitScriptRequest_McpId:
		id, err := parseUUID(target.McpId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		input.McpID = &id
	case *teamsv1.CreateInitScriptRequest_HookId:
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
	return &teamsv1.CreateInitScriptResponse{InitScript: toProtoInitScript(script)}, nil
}

func (s *Server) GetInitScript(ctx context.Context, req *teamsv1.GetInitScriptRequest) (*teamsv1.GetInitScriptResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	script, err := s.store.GetInitScript(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.GetInitScriptResponse{InitScript: toProtoInitScript(script)}, nil
}

func (s *Server) UpdateInitScript(ctx context.Context, req *teamsv1.UpdateInitScriptRequest) (*teamsv1.UpdateInitScriptResponse, error) {
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
	return &teamsv1.UpdateInitScriptResponse{InitScript: toProtoInitScript(script)}, nil
}

func (s *Server) DeleteInitScript(ctx context.Context, req *teamsv1.DeleteInitScriptRequest) (*teamsv1.DeleteInitScriptResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteInitScript(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteInitScriptResponse{}, nil
}

func (s *Server) ListInitScripts(ctx context.Context, req *teamsv1.ListInitScriptsRequest) (*teamsv1.ListInitScriptsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	filter := store.InitScriptFilter{}
	if req.GetAgentId() != "" {
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		filter.AgentID = &agentID
	}
	if req.GetMcpId() != "" {
		mcpID, err := parseUUID(req.GetMcpId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		filter.McpID = &mcpID
	}
	if req.GetHookId() != "" {
		hookID, err := parseUUID(req.GetHookId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hook_id: %v", err)
		}
		filter.HookID = &hookID
	}

	result, err := s.store.ListInitScripts(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	scripts, nextToken := mapListResult(result.InitScripts, result.NextCursor, toProtoInitScript)
	return &teamsv1.ListInitScriptsResponse{InitScripts: scripts, NextPageToken: nextToken}, nil
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

func toStoreComputeResources(resources *teamsv1.ComputeResources) store.ComputeResources {
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
