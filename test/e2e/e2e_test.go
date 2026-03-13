package e2e

import (
	"context"
	"net"
	"testing"
	"time"

	teamsv1 "github.com/agynio/teams/gen/go/agynio/api/teams/v1"
	"github.com/agynio/teams/internal/db"
	"github.com/agynio/teams/internal/server"
	"github.com/agynio/teams/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestTeamsServiceE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	require.NoError(t, db.ApplyMigrations(ctx, pool))

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	teamsv1.RegisterTeamsServiceServer(grpcServer, server.New(store.New(pool)))

	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.GracefulStop()

	conn, err := grpc.DialContext(ctx, lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	require.NoError(t, err)
	defer conn.Close()

	client := teamsv1.NewTeamsServiceClient(conn)

	t.Run("Agents", func(t *testing.T) {
		resetDatabase(ctx, t, pool)

		agentConfig1 := baseAgentConfig("alpha", "engineer")
		agentResp1, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Title:       "Agent Alpha",
			Description: "First agent",
			Config:      agentConfig1,
		})
		require.NoError(t, err)
		agentID1 := agentResp1.Agent.Meta.Id

		agentConfig2 := proto.Clone(agentConfig1).(*teamsv1.AgentConfig)
		agentConfig2.Name = "beta"
		agentConfig2.Role = "analyst"
		agentResp2, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Title:       "Agent Beta",
			Description: "Second agent",
			Config:      agentConfig2,
		})
		require.NoError(t, err)
		agentID2 := agentResp2.Agent.Meta.Id

		updatedAgentResp, err := client.UpdateAgent(ctx, &teamsv1.UpdateAgentRequest{
			Id:    agentID1,
			Title: proto.String("Agent Alpha Updated"),
		})
		require.NoError(t, err)
		require.Equal(t, "Agent Alpha Updated", updatedAgentResp.Agent.Title)

		listAgentsResp1, err := client.ListAgents(ctx, &teamsv1.ListAgentsRequest{PageSize: 1})
		require.NoError(t, err)
		require.Len(t, listAgentsResp1.Agents, 1)
		require.NotEmpty(t, listAgentsResp1.NextPageToken)

		listAgentsResp2, err := client.ListAgents(ctx, &teamsv1.ListAgentsRequest{PageToken: listAgentsResp1.NextPageToken})
		require.NoError(t, err)
		require.Len(t, listAgentsResp2.Agents, 1)
		require.Empty(t, listAgentsResp2.NextPageToken)

		searchAgentsResp, err := client.ListAgents(ctx, &teamsv1.ListAgentsRequest{Query: "Alpha"})
		require.NoError(t, err)
		require.Len(t, searchAgentsResp.Agents, 1)
		require.Equal(t, agentID1, searchAgentsResp.Agents[0].Meta.Id)

		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID2})
		require.NoError(t, err)
	})

	t.Run("Tools", func(t *testing.T) {
		resetDatabase(ctx, t, pool)

		toolConfig1, err := structpb.NewStruct(map[string]any{"scope": "local"})
		require.NoError(t, err)
		toolResp1, err := client.CreateTool(ctx, &teamsv1.CreateToolRequest{
			Type:        teamsv1.ToolType_TOOL_TYPE_MEMORY,
			Name:        "memory",
			Description: "memory tool",
			Config:      toolConfig1,
		})
		require.NoError(t, err)
		toolID1 := toolResp1.Tool.Meta.Id

		toolConfig2, err := structpb.NewStruct(map[string]any{"mode": "auto"})
		require.NoError(t, err)
		_, err = client.CreateTool(ctx, &teamsv1.CreateToolRequest{
			Type:        teamsv1.ToolType_TOOL_TYPE_MANAGE,
			Name:        "manage",
			Description: "manage tool",
			Config:      toolConfig2,
		})
		require.NoError(t, err)

		updateToolResp, err := client.UpdateTool(ctx, &teamsv1.UpdateToolRequest{
			Id:          toolID1,
			Description: proto.String("memory tool updated"),
		})
		require.NoError(t, err)
		require.Equal(t, "memory tool updated", updateToolResp.Tool.Description)

		listToolsResp, err := client.ListTools(ctx, &teamsv1.ListToolsRequest{Type: teamsv1.ToolType_TOOL_TYPE_MEMORY})
		require.NoError(t, err)
		require.Len(t, listToolsResp.Tools, 1)
		require.Equal(t, toolID1, listToolsResp.Tools[0].Meta.Id)
	})

	t.Run("McpServers", func(t *testing.T) {
		resetDatabase(ctx, t, pool)

		mcpConfig := &teamsv1.McpServerConfig{
			Namespace:           "default",
			Command:             "mcp",
			Workdir:             "/srv",
			Env:                 []*teamsv1.McpEnvItem{{Name: "API_KEY", Value: "token"}},
			RequestTimeoutMs:    1000,
			StartupTimeoutMs:    2000,
			HeartbeatIntervalMs: 5000,
			StaleTimeoutMs:      10000,
			Restart:             &teamsv1.McpServerRestartConfig{MaxAttempts: 3, BackoffMs: 250},
		}

		mcpResp, err := client.CreateMcpServer(ctx, &teamsv1.CreateMcpServerRequest{
			Title:       "MCP Server",
			Description: "MCP server",
			Config:      mcpConfig,
		})
		require.NoError(t, err)
		mcpID := mcpResp.McpServer.Meta.Id

		updatedMcpResp, err := client.UpdateMcpServer(ctx, &teamsv1.UpdateMcpServerRequest{
			Id:    mcpID,
			Title: proto.String("MCP Server Updated"),
		})
		require.NoError(t, err)
		require.Equal(t, "MCP Server Updated", updatedMcpResp.McpServer.Title)

		listMcpResp, err := client.ListMcpServers(ctx, &teamsv1.ListMcpServersRequest{PageSize: 1})
		require.NoError(t, err)
		require.Len(t, listMcpResp.McpServers, 1)
	})

	t.Run("WorkspaceConfigurations", func(t *testing.T) {
		resetDatabase(ctx, t, pool)

		nixConfig, err := structpb.NewStruct(map[string]any{"shell": "bash"})
		require.NoError(t, err)
		workspaceConfig := &teamsv1.WorkspaceConfig{
			Image:         "ubuntu:latest",
			Env:           []*teamsv1.WorkspaceEnvItem{{Name: "PATH", Value: "/usr/bin"}},
			InitialScript: "echo ready",
			CpuLimit:      "1",
			MemoryLimit:   "512Mi",
			Platform:      teamsv1.WorkspacePlatform_WORKSPACE_PLATFORM_LINUX_AMD64,
			EnableDind:    true,
			TtlSeconds:    3600,
			Nix:           nixConfig,
			Volumes:       &teamsv1.WorkspaceVolumeConfig{Enabled: true, MountPath: "/workspace"},
		}
		workspaceResp, err := client.CreateWorkspaceConfiguration(ctx, &teamsv1.CreateWorkspaceConfigurationRequest{
			Title:       "Workspace",
			Description: "Workspace config",
			Config:      workspaceConfig,
		})
		require.NoError(t, err)
		workspaceID := workspaceResp.WorkspaceConfiguration.Meta.Id

		updatedWorkspaceResp, err := client.UpdateWorkspaceConfiguration(ctx, &teamsv1.UpdateWorkspaceConfigurationRequest{
			Id:          workspaceID,
			Description: proto.String("Workspace config updated"),
		})
		require.NoError(t, err)
		require.Equal(t, "Workspace config updated", updatedWorkspaceResp.WorkspaceConfiguration.Description)

		listWorkspaceResp, err := client.ListWorkspaceConfigurations(ctx, &teamsv1.ListWorkspaceConfigurationsRequest{PageSize: 1})
		require.NoError(t, err)
		require.Len(t, listWorkspaceResp.WorkspaceConfigurations, 1)
	})

	t.Run("MemoryBuckets", func(t *testing.T) {
		resetDatabase(ctx, t, pool)

		memoryConfig := &teamsv1.MemoryBucketConfig{Scope: teamsv1.MemoryBucketScope_MEMORY_BUCKET_SCOPE_GLOBAL, CollectionPrefix: "mem"}
		memoryResp, err := client.CreateMemoryBucket(ctx, &teamsv1.CreateMemoryBucketRequest{
			Title:       "Memory Bucket",
			Description: "Memory bucket",
			Config:      memoryConfig,
		})
		require.NoError(t, err)
		memoryID := memoryResp.MemoryBucket.Meta.Id

		updatedMemoryResp, err := client.UpdateMemoryBucket(ctx, &teamsv1.UpdateMemoryBucketRequest{
			Id:    memoryID,
			Title: proto.String("Memory Bucket Updated"),
		})
		require.NoError(t, err)
		require.Equal(t, "Memory Bucket Updated", updatedMemoryResp.MemoryBucket.Title)

		listMemoryResp, err := client.ListMemoryBuckets(ctx, &teamsv1.ListMemoryBucketsRequest{PageSize: 1})
		require.NoError(t, err)
		require.Len(t, listMemoryResp.MemoryBuckets, 1)
	})

	t.Run("Variables", func(t *testing.T) {
		resetDatabase(ctx, t, pool)

		variableResp1, err := client.CreateVariable(ctx, &teamsv1.CreateVariableRequest{
			Key:         "API_KEY",
			Value:       "secret",
			Description: "Primary API key",
		})
		require.NoError(t, err)
		variableID1 := variableResp1.Variable.Meta.Id

		variableResp2, err := client.CreateVariable(ctx, &teamsv1.CreateVariableRequest{
			Key:         "ENV",
			Value:       "prod",
			Description: "Environment",
		})
		require.NoError(t, err)
		variableID2 := variableResp2.Variable.Meta.Id

		getVariableResp, err := client.GetVariable(ctx, &teamsv1.GetVariableRequest{Id: variableID1})
		require.NoError(t, err)
		require.Equal(t, variableID1, getVariableResp.Variable.Meta.Id)
		require.Equal(t, "API_KEY", getVariableResp.Variable.Key)

		updatedVariableResp, err := client.UpdateVariable(ctx, &teamsv1.UpdateVariableRequest{
			Id:    variableID1,
			Value: proto.String("secret-updated"),
		})
		require.NoError(t, err)
		require.Equal(t, "secret-updated", updatedVariableResp.Variable.Value)

		listVariablesResp1, err := client.ListVariables(ctx, &teamsv1.ListVariablesRequest{PageSize: 1})
		require.NoError(t, err)
		require.Len(t, listVariablesResp1.Variables, 1)
		require.NotEmpty(t, listVariablesResp1.NextPageToken)

		listVariablesResp2, err := client.ListVariables(ctx, &teamsv1.ListVariablesRequest{PageToken: listVariablesResp1.NextPageToken})
		require.NoError(t, err)
		require.Len(t, listVariablesResp2.Variables, 1)
		require.Empty(t, listVariablesResp2.NextPageToken)

		searchVariablesResp, err := client.ListVariables(ctx, &teamsv1.ListVariablesRequest{Query: "API"})
		require.NoError(t, err)
		require.Len(t, searchVariablesResp.Variables, 1)
		require.Equal(t, variableID1, searchVariablesResp.Variables[0].Meta.Id)

		resolveVariableResp, err := client.ResolveVariable(ctx, &teamsv1.ResolveVariableRequest{Key: "API_KEY"})
		require.NoError(t, err)
		require.True(t, resolveVariableResp.Found)
		require.Equal(t, "secret-updated", resolveVariableResp.Value)

		missingVariableResp, err := client.ResolveVariable(ctx, &teamsv1.ResolveVariableRequest{Key: "MISSING"})
		require.NoError(t, err)
		require.False(t, missingVariableResp.Found)
		require.Empty(t, missingVariableResp.Value)

		_, err = client.DeleteVariable(ctx, &teamsv1.DeleteVariableRequest{Id: variableID2})
		require.NoError(t, err)
	})

	t.Run("Attachments", func(t *testing.T) {
		resetDatabase(ctx, t, pool)

		agentResp, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Title:       "Agent Alpha",
			Description: "First agent",
			Config:      baseAgentConfig("alpha", "engineer"),
		})
		require.NoError(t, err)
		toolResp, err := client.CreateTool(ctx, &teamsv1.CreateToolRequest{
			Type:        teamsv1.ToolType_TOOL_TYPE_MEMORY,
			Name:        "memory",
			Description: "memory tool",
			Config:      &structpb.Struct{},
		})
		require.NoError(t, err)

		attachmentResp, err := client.CreateAttachment(ctx, &teamsv1.CreateAttachmentRequest{
			Kind:     teamsv1.AttachmentKind_ATTACHMENT_KIND_AGENT_TOOL,
			SourceId: agentResp.Agent.Meta.Id,
			TargetId: toolResp.Tool.Meta.Id,
		})
		require.NoError(t, err)
		attachmentID := attachmentResp.Attachment.Meta.Id

		getAttachmentResp, err := client.GetAttachment(ctx, &teamsv1.GetAttachmentRequest{Id: attachmentID})
		require.NoError(t, err)
		require.Equal(t, attachmentID, getAttachmentResp.Attachment.Meta.Id)
		require.Equal(t, agentResp.Agent.Meta.Id, getAttachmentResp.Attachment.SourceId)
		require.Equal(t, toolResp.Tool.Meta.Id, getAttachmentResp.Attachment.TargetId)

		listAttachmentResp, err := client.ListAttachments(ctx, &teamsv1.ListAttachmentsRequest{
			SourceType: teamsv1.EntityType_ENTITY_TYPE_AGENT,
			SourceId:   agentResp.Agent.Meta.Id,
			PageSize:   1,
		})
		require.NoError(t, err)
		require.Len(t, listAttachmentResp.Attachments, 1)
		require.Equal(t, attachmentID, listAttachmentResp.Attachments[0].Meta.Id)

		_, err = client.DeleteAttachment(ctx, &teamsv1.DeleteAttachmentRequest{Id: attachmentID})
		require.NoError(t, err)

		listAttachmentAfterDelete, err := client.ListAttachments(ctx, &teamsv1.ListAttachmentsRequest{SourceId: agentResp.Agent.Meta.Id})
		require.NoError(t, err)
		require.Len(t, listAttachmentAfterDelete.Attachments, 0)
	})

	t.Run("NegativePaths", func(t *testing.T) {
		resetDatabase(ctx, t, pool)

		_, err := client.GetAgent(ctx, &teamsv1.GetAgentRequest{Id: uuid.NewString()})
		requireStatusCode(t, err, codes.NotFound)

		_, err = client.UpdateAgent(ctx, &teamsv1.UpdateAgentRequest{Id: uuid.NewString()})
		requireStatusCode(t, err, codes.InvalidArgument)

		agentResp, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Title:       "Agent Alpha",
			Description: "First agent",
			Config:      baseAgentConfig("alpha", "engineer"),
		})
		require.NoError(t, err)
		toolResp, err := client.CreateTool(ctx, &teamsv1.CreateToolRequest{
			Type:        teamsv1.ToolType_TOOL_TYPE_MEMORY,
			Name:        "memory",
			Description: "memory tool",
			Config:      &structpb.Struct{},
		})
		require.NoError(t, err)

		_, err = client.CreateVariable(ctx, &teamsv1.CreateVariableRequest{
			Key:         "DUPLICATE_KEY",
			Value:       "first",
			Description: "first value",
		})
		require.NoError(t, err)

		_, err = client.CreateVariable(ctx, &teamsv1.CreateVariableRequest{
			Key:         "DUPLICATE_KEY",
			Value:       "second",
			Description: "second value",
		})
		requireStatusCode(t, err, codes.AlreadyExists)

		_, err = client.CreateAttachment(ctx, &teamsv1.CreateAttachmentRequest{
			Kind:     teamsv1.AttachmentKind_ATTACHMENT_KIND_AGENT_TOOL,
			SourceId: agentResp.Agent.Meta.Id,
			TargetId: toolResp.Tool.Meta.Id,
		})
		require.NoError(t, err)

		_, err = client.CreateAttachment(ctx, &teamsv1.CreateAttachmentRequest{
			Kind:     teamsv1.AttachmentKind_ATTACHMENT_KIND_AGENT_TOOL,
			SourceId: agentResp.Agent.Meta.Id,
			TargetId: toolResp.Tool.Meta.Id,
		})
		requireStatusCode(t, err, codes.AlreadyExists)
	})
}

func baseAgentConfig(name, role string) *teamsv1.AgentConfig {
	return &teamsv1.AgentConfig{
		Model:                     "gpt-4",
		SystemPrompt:              "system",
		DebounceMs:                100,
		WhenBusy:                  teamsv1.AgentWhenBusy_AGENT_WHEN_BUSY_WAIT,
		ProcessBuffer:             teamsv1.AgentProcessBuffer_AGENT_PROCESS_BUFFER_ALL_TOGETHER,
		SendFinalResponseToThread: true,
		SummarizationKeepTokens:   50,
		SummarizationMaxTokens:    500,
		RestrictOutput:            false,
		RestrictionMessage:        "",
		RestrictionMaxInjections:  2,
		Name:                      name,
		Role:                      role,
	}
}

func resetDatabase(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `TRUNCATE attachments, agents, tools, mcp_servers, workspace_configurations, memory_buckets, variables RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
}

func requireStatusCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	statusErr, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, code, statusErr.Code())
}
