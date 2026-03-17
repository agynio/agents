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
	config, err := marshalConfig(req.GetConfig())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "config: %v", err)
	}
	agent, err := s.store.CreateAgent(ctx, store.AgentInput{
		Title:       req.GetTitle(),
		Description: req.GetDescription(),
		Config:      config,
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	protoAgent, err := toProtoAgent(agent)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode agent: %v", err)
	}
	return &teamsv1.CreateAgentResponse{Agent: protoAgent}, nil
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
	protoAgent, err := toProtoAgent(agent)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode agent: %v", err)
	}
	return &teamsv1.GetAgentResponse{Agent: protoAgent}, nil
}

func (s *Server) UpdateAgent(ctx context.Context, req *teamsv1.UpdateAgentRequest) (*teamsv1.UpdateAgentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Title == nil && req.Description == nil && req.Config == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	update := store.AgentUpdate{}
	if req.Title != nil {
		value := req.GetTitle()
		update.Title = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}
	if req.Config != nil {
		config, err := marshalConfig(req.GetConfig())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "config: %v", err)
		}
		update.Config = &config
	}

	agent, err := s.store.UpdateAgent(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoAgent, err := toProtoAgent(agent)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode agent: %v", err)
	}
	return &teamsv1.UpdateAgentResponse{Agent: protoAgent}, nil
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

	result, err := s.store.ListAgents(ctx, store.AgentFilter{Query: req.GetQuery()}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}

	agents, nextToken, err := mapListResult(result.Agents, result.NextCursor, toProtoAgent)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode agent: %v", err)
	}
	return &teamsv1.ListAgentsResponse{Agents: agents, NextPageToken: nextToken}, nil
}

func (s *Server) CreateTool(ctx context.Context, req *teamsv1.CreateToolRequest) (*teamsv1.CreateToolResponse, error) {
	if req.GetType() == teamsv1.ToolType_TOOL_TYPE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "type must be specified")
	}
	config, err := marshalConfig(req.GetConfig())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "config: %v", err)
	}
	toolType, err := toolTypeName(req.GetType())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "type: %v", err)
	}
	tool, err := s.store.CreateTool(ctx, store.ToolInput{
		Type:        toolType,
		Name:        req.GetName(),
		Description: req.GetDescription(),
		Config:      config,
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	protoTool, err := toProtoTool(tool)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode tool: %v", err)
	}
	return &teamsv1.CreateToolResponse{Tool: protoTool}, nil
}

func (s *Server) GetTool(ctx context.Context, req *teamsv1.GetToolRequest) (*teamsv1.GetToolResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	tool, err := s.store.GetTool(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoTool, err := toProtoTool(tool)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode tool: %v", err)
	}
	return &teamsv1.GetToolResponse{Tool: protoTool}, nil
}

func (s *Server) UpdateTool(ctx context.Context, req *teamsv1.UpdateToolRequest) (*teamsv1.UpdateToolResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Name == nil && req.Description == nil && req.Config == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	update := store.ToolUpdate{}
	if req.Name != nil {
		value := req.GetName()
		update.Name = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}
	if req.Config != nil {
		config, err := marshalConfig(req.GetConfig())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "config: %v", err)
		}
		update.Config = &config
	}

	tool, err := s.store.UpdateTool(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoTool, err := toProtoTool(tool)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode tool: %v", err)
	}
	return &teamsv1.UpdateToolResponse{Tool: protoTool}, nil
}

func (s *Server) DeleteTool(ctx context.Context, req *teamsv1.DeleteToolRequest) (*teamsv1.DeleteToolResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteTool(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteToolResponse{}, nil
}

func (s *Server) ListTools(ctx context.Context, req *teamsv1.ListToolsRequest) (*teamsv1.ListToolsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	filter := store.ToolFilter{}
	if req.GetType() != teamsv1.ToolType_TOOL_TYPE_UNSPECIFIED {
		name, err := toolTypeName(req.GetType())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "type: %v", err)
		}
		filter.Type = &name
	}

	result, err := s.store.ListTools(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}

	tools, nextToken, err := mapListResult(result.Tools, result.NextCursor, toProtoTool)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode tool: %v", err)
	}
	return &teamsv1.ListToolsResponse{Tools: tools, NextPageToken: nextToken}, nil
}

func (s *Server) CreateMcpServer(ctx context.Context, req *teamsv1.CreateMcpServerRequest) (*teamsv1.CreateMcpServerResponse, error) {
	config, err := marshalConfig(req.GetConfig())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "config: %v", err)
	}
	server, err := s.store.CreateMcpServer(ctx, store.McpServerInput{
		Title:       req.GetTitle(),
		Description: req.GetDescription(),
		Config:      config,
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	protoServer, err := toProtoMcpServer(server)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode mcp_server: %v", err)
	}
	return &teamsv1.CreateMcpServerResponse{McpServer: protoServer}, nil
}

func (s *Server) GetMcpServer(ctx context.Context, req *teamsv1.GetMcpServerRequest) (*teamsv1.GetMcpServerResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	server, err := s.store.GetMcpServer(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoServer, err := toProtoMcpServer(server)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode mcp_server: %v", err)
	}
	return &teamsv1.GetMcpServerResponse{McpServer: protoServer}, nil
}

func (s *Server) UpdateMcpServer(ctx context.Context, req *teamsv1.UpdateMcpServerRequest) (*teamsv1.UpdateMcpServerResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Title == nil && req.Description == nil && req.Config == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	update := store.McpServerUpdate{}
	if req.Title != nil {
		value := req.GetTitle()
		update.Title = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}
	if req.Config != nil {
		config, err := marshalConfig(req.GetConfig())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "config: %v", err)
		}
		update.Config = &config
	}

	server, err := s.store.UpdateMcpServer(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoServer, err := toProtoMcpServer(server)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode mcp_server: %v", err)
	}
	return &teamsv1.UpdateMcpServerResponse{McpServer: protoServer}, nil
}

func (s *Server) DeleteMcpServer(ctx context.Context, req *teamsv1.DeleteMcpServerRequest) (*teamsv1.DeleteMcpServerResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteMcpServer(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteMcpServerResponse{}, nil
}

func (s *Server) ListMcpServers(ctx context.Context, req *teamsv1.ListMcpServersRequest) (*teamsv1.ListMcpServersResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	result, err := s.store.ListMcpServers(ctx, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}

	servers, nextToken, err := mapListResult(result.McpServers, result.NextCursor, toProtoMcpServer)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode mcp_server: %v", err)
	}
	return &teamsv1.ListMcpServersResponse{McpServers: servers, NextPageToken: nextToken}, nil
}

func (s *Server) CreateWorkspaceConfiguration(ctx context.Context, req *teamsv1.CreateWorkspaceConfigurationRequest) (*teamsv1.CreateWorkspaceConfigurationResponse, error) {
	config, err := marshalConfig(req.GetConfig())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "config: %v", err)
	}
	workspace, err := s.store.CreateWorkspaceConfiguration(ctx, store.WorkspaceConfigurationInput{
		Title:       req.GetTitle(),
		Description: req.GetDescription(),
		Config:      config,
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	protoWorkspace, err := toProtoWorkspaceConfiguration(workspace)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode workspace_configuration: %v", err)
	}
	return &teamsv1.CreateWorkspaceConfigurationResponse{WorkspaceConfiguration: protoWorkspace}, nil
}

func (s *Server) GetWorkspaceConfiguration(ctx context.Context, req *teamsv1.GetWorkspaceConfigurationRequest) (*teamsv1.GetWorkspaceConfigurationResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	workspace, err := s.store.GetWorkspaceConfiguration(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoWorkspace, err := toProtoWorkspaceConfiguration(workspace)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode workspace_configuration: %v", err)
	}
	return &teamsv1.GetWorkspaceConfigurationResponse{WorkspaceConfiguration: protoWorkspace}, nil
}

func (s *Server) UpdateWorkspaceConfiguration(ctx context.Context, req *teamsv1.UpdateWorkspaceConfigurationRequest) (*teamsv1.UpdateWorkspaceConfigurationResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Title == nil && req.Description == nil && req.Config == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	update := store.WorkspaceConfigurationUpdate{}
	if req.Title != nil {
		value := req.GetTitle()
		update.Title = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}
	if req.Config != nil {
		config, err := marshalConfig(req.GetConfig())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "config: %v", err)
		}
		update.Config = &config
	}

	workspace, err := s.store.UpdateWorkspaceConfiguration(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoWorkspace, err := toProtoWorkspaceConfiguration(workspace)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode workspace_configuration: %v", err)
	}
	return &teamsv1.UpdateWorkspaceConfigurationResponse{WorkspaceConfiguration: protoWorkspace}, nil
}

func (s *Server) DeleteWorkspaceConfiguration(ctx context.Context, req *teamsv1.DeleteWorkspaceConfigurationRequest) (*teamsv1.DeleteWorkspaceConfigurationResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteWorkspaceConfiguration(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteWorkspaceConfigurationResponse{}, nil
}

func (s *Server) ListWorkspaceConfigurations(ctx context.Context, req *teamsv1.ListWorkspaceConfigurationsRequest) (*teamsv1.ListWorkspaceConfigurationsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	result, err := s.store.ListWorkspaceConfigurations(ctx, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}

	workspaces, nextToken, err := mapListResult(result.WorkspaceConfigurations, result.NextCursor, toProtoWorkspaceConfiguration)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode workspace_configuration: %v", err)
	}
	return &teamsv1.ListWorkspaceConfigurationsResponse{WorkspaceConfigurations: workspaces, NextPageToken: nextToken}, nil
}

func (s *Server) CreateMemoryBucket(ctx context.Context, req *teamsv1.CreateMemoryBucketRequest) (*teamsv1.CreateMemoryBucketResponse, error) {
	config, err := marshalConfig(req.GetConfig())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "config: %v", err)
	}
	bucket, err := s.store.CreateMemoryBucket(ctx, store.MemoryBucketInput{
		Title:       req.GetTitle(),
		Description: req.GetDescription(),
		Config:      config,
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	protoBucket, err := toProtoMemoryBucket(bucket)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode memory_bucket: %v", err)
	}
	return &teamsv1.CreateMemoryBucketResponse{MemoryBucket: protoBucket}, nil
}

func (s *Server) GetMemoryBucket(ctx context.Context, req *teamsv1.GetMemoryBucketRequest) (*teamsv1.GetMemoryBucketResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	bucket, err := s.store.GetMemoryBucket(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoBucket, err := toProtoMemoryBucket(bucket)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode memory_bucket: %v", err)
	}
	return &teamsv1.GetMemoryBucketResponse{MemoryBucket: protoBucket}, nil
}

func (s *Server) UpdateMemoryBucket(ctx context.Context, req *teamsv1.UpdateMemoryBucketRequest) (*teamsv1.UpdateMemoryBucketResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Title == nil && req.Description == nil && req.Config == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	update := store.MemoryBucketUpdate{}
	if req.Title != nil {
		value := req.GetTitle()
		update.Title = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}
	if req.Config != nil {
		config, err := marshalConfig(req.GetConfig())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "config: %v", err)
		}
		update.Config = &config
	}

	bucket, err := s.store.UpdateMemoryBucket(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoBucket, err := toProtoMemoryBucket(bucket)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode memory_bucket: %v", err)
	}
	return &teamsv1.UpdateMemoryBucketResponse{MemoryBucket: protoBucket}, nil
}

func (s *Server) DeleteMemoryBucket(ctx context.Context, req *teamsv1.DeleteMemoryBucketRequest) (*teamsv1.DeleteMemoryBucketResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteMemoryBucket(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteMemoryBucketResponse{}, nil
}

func (s *Server) ListMemoryBuckets(ctx context.Context, req *teamsv1.ListMemoryBucketsRequest) (*teamsv1.ListMemoryBucketsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	result, err := s.store.ListMemoryBuckets(ctx, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}

	buckets, nextToken, err := mapListResult(result.MemoryBuckets, result.NextCursor, toProtoMemoryBucket)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode memory_bucket: %v", err)
	}
	return &teamsv1.ListMemoryBucketsResponse{MemoryBuckets: buckets, NextPageToken: nextToken}, nil
}

func (s *Server) CreateVariable(ctx context.Context, req *teamsv1.CreateVariableRequest) (*teamsv1.CreateVariableResponse, error) {
	variable, err := s.store.CreateVariable(ctx, store.VariableInput{
		Key:         req.GetKey(),
		Value:       req.GetValue(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	protoVariable, err := toProtoVariable(variable)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode variable: %v", err)
	}
	return &teamsv1.CreateVariableResponse{Variable: protoVariable}, nil
}

func (s *Server) GetVariable(ctx context.Context, req *teamsv1.GetVariableRequest) (*teamsv1.GetVariableResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	variable, err := s.store.GetVariable(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoVariable, err := toProtoVariable(variable)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode variable: %v", err)
	}
	return &teamsv1.GetVariableResponse{Variable: protoVariable}, nil
}

func (s *Server) UpdateVariable(ctx context.Context, req *teamsv1.UpdateVariableRequest) (*teamsv1.UpdateVariableResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Key == nil && req.Value == nil && req.Description == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	update := store.VariableUpdate{}
	if req.Key != nil {
		value := req.GetKey()
		update.Key = &value
	}
	if req.Value != nil {
		value := req.GetValue()
		update.Value = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		update.Description = &value
	}

	variable, err := s.store.UpdateVariable(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoVariable, err := toProtoVariable(variable)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode variable: %v", err)
	}
	return &teamsv1.UpdateVariableResponse{Variable: protoVariable}, nil
}

func (s *Server) DeleteVariable(ctx context.Context, req *teamsv1.DeleteVariableRequest) (*teamsv1.DeleteVariableResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteVariable(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteVariableResponse{}, nil
}

func (s *Server) ListVariables(ctx context.Context, req *teamsv1.ListVariablesRequest) (*teamsv1.ListVariablesResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	result, err := s.store.ListVariables(ctx, store.VariableFilter{Query: req.GetQuery()}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}

	variables, nextToken, err := mapListResult(result.Variables, result.NextCursor, toProtoVariable)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode variable: %v", err)
	}
	return &teamsv1.ListVariablesResponse{Variables: variables, NextPageToken: nextToken}, nil
}

func (s *Server) ResolveVariable(ctx context.Context, req *teamsv1.ResolveVariableRequest) (*teamsv1.ResolveVariableResponse, error) {
	key := req.GetKey()
	if key == "" {
		return nil, status.Error(codes.InvalidArgument, "key must be provided")
	}
	value, found, err := s.store.ResolveVariableByKey(ctx, key)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.ResolveVariableResponse{Value: value, Found: found}, nil
}

func (s *Server) CreateAttachment(ctx context.Context, req *teamsv1.CreateAttachmentRequest) (*teamsv1.CreateAttachmentResponse, error) {
	if req.GetKind() == teamsv1.AttachmentKind_ATTACHMENT_KIND_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "kind must be specified")
	}
	sourceID, err := parseUUID(req.GetSourceId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "source_id: %v", err)
	}
	targetID, err := parseUUID(req.GetTargetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "target_id: %v", err)
	}
	sourceType, targetType, err := attachmentRelation(req.GetKind())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "kind: %v", err)
	}
	kindName, err := attachmentKindName(req.GetKind())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "kind: %v", err)
	}
	sourceTypeName, err := entityTypeName(sourceType)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "source_type: %v", err)
	}
	targetTypeName, err := entityTypeName(targetType)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "target_type: %v", err)
	}

	attachment, err := s.store.CreateAttachment(ctx, store.AttachmentInput{
		Kind:       kindName,
		SourceType: sourceTypeName,
		SourceID:   sourceID,
		TargetType: targetTypeName,
		TargetID:   targetID,
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	protoAttachment, err := toProtoAttachment(attachment)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode attachment: %v", err)
	}
	return &teamsv1.CreateAttachmentResponse{Attachment: protoAttachment}, nil
}

func (s *Server) GetAttachment(ctx context.Context, req *teamsv1.GetAttachmentRequest) (*teamsv1.GetAttachmentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	attachment, err := s.store.GetAttachment(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoAttachment, err := toProtoAttachment(attachment)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode attachment: %v", err)
	}
	return &teamsv1.GetAttachmentResponse{Attachment: protoAttachment}, nil
}

func (s *Server) DeleteAttachment(ctx context.Context, req *teamsv1.DeleteAttachmentRequest) (*teamsv1.DeleteAttachmentResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteAttachment(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &teamsv1.DeleteAttachmentResponse{}, nil
}

func (s *Server) ListAttachments(ctx context.Context, req *teamsv1.ListAttachmentsRequest) (*teamsv1.ListAttachmentsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	filter := store.AttachmentFilter{}
	if req.GetKind() != teamsv1.AttachmentKind_ATTACHMENT_KIND_UNSPECIFIED {
		kindName, err := attachmentKindName(req.GetKind())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "kind: %v", err)
		}
		filter.Kind = &kindName
	}
	if req.GetSourceType() != teamsv1.EntityType_ENTITY_TYPE_UNSPECIFIED {
		sourceName, err := entityTypeName(req.GetSourceType())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "source_type: %v", err)
		}
		filter.SourceType = &sourceName
	}
	if req.GetSourceId() != "" {
		id, err := parseUUID(req.GetSourceId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "source_id: %v", err)
		}
		filter.SourceID = &id
	}
	if req.GetTargetType() != teamsv1.EntityType_ENTITY_TYPE_UNSPECIFIED {
		targetName, err := entityTypeName(req.GetTargetType())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "target_type: %v", err)
		}
		filter.TargetType = &targetName
	}
	if req.GetTargetId() != "" {
		id, err := parseUUID(req.GetTargetId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "target_id: %v", err)
		}
		filter.TargetID = &id
	}

	result, err := s.store.ListAttachments(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}

	attachments, nextToken, err := mapListResult(result.Attachments, result.NextCursor, toProtoAttachment)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decode attachment: %v", err)
	}
	return &teamsv1.ListAttachmentsResponse{Attachments: attachments, NextPageToken: nextToken}, nil
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

func mapListResult[T any, P any](items []T, nextCursor *store.PageCursor, convert func(T) (P, error)) ([]P, string, error) {
	results := make([]P, len(items))
	for i, item := range items {
		converted, err := convert(item)
		if err != nil {
			return nil, "", err
		}
		results[i] = converted
	}
	if nextCursor == nil {
		return results, "", nil
	}
	return results, store.EncodePageToken(nextCursor.AfterID), nil
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

func toStatusError(err error) error {
	var notFound *store.NotFoundError
	if errors.As(err, &notFound) {
		return status.Error(codes.NotFound, notFound.Error())
	}
	var exists *store.AlreadyExistsError
	if errors.As(err, &exists) {
		return status.Error(codes.AlreadyExists, exists.Error())
	}
	return status.Errorf(codes.Internal, "internal error: %v", err)
}
