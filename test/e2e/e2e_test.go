//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	teamsv1 "github.com/agynio/teams/.gen/go/agynio/api/teams/v1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

const listPageSize int32 = 50

func TestTeamsServiceE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	conn, err := grpc.DialContext(ctx, teamsAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	client := teamsv1.NewTeamsServiceClient(conn)

	t.Run("Agents", func(t *testing.T) {
		testID := uuid.NewString()
		agentConfig1 := baseAgentConfig("alpha-"+testID, "engineer")
		agentResp1, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Title:       "Agent Alpha " + testID,
			Description: "First agent " + testID,
			Config:      agentConfig1,
		})
		require.NoError(t, err)
		agentID1 := agentResp1.Agent.Meta.Id

		agentConfig2 := proto.Clone(agentConfig1).(*teamsv1.AgentConfig)
		agentConfig2.Name = "beta-" + testID
		agentConfig2.Role = "analyst"
		agentResp2, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Title:       "Agent Beta " + testID,
			Description: "Second agent " + testID,
			Config:      agentConfig2,
		})
		require.NoError(t, err)
		agentID2 := agentResp2.Agent.Meta.Id

		updatedAgentResp, err := client.UpdateAgent(ctx, &teamsv1.UpdateAgentRequest{
			Id:    agentID1,
			Title: proto.String("Agent Alpha Updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "Agent Alpha Updated "+testID, updatedAgentResp.Agent.Title)

		listAgentsResp1, err := client.ListAgents(ctx, &teamsv1.ListAgentsRequest{PageSize: 1, Query: testID})
		require.NoError(t, err)
		require.NotEmpty(t, listAgentsResp1.Agents)
		require.NotEmpty(t, listAgentsResp1.NextPageToken)

		listAgentsResp2, err := client.ListAgents(ctx, &teamsv1.ListAgentsRequest{PageToken: listAgentsResp1.NextPageToken, Query: testID})
		require.NoError(t, err)
		require.NotEmpty(t, listAgentsResp2.Agents)

		searchAgents := listAgentsByQuery(ctx, t, client, testID)
		require.True(t, hasAgentID(searchAgents, agentID1))
		require.True(t, hasAgentID(searchAgents, agentID2))

		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID2})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID1})
		require.NoError(t, err)
	})

	t.Run("Tools", func(t *testing.T) {
		testID := uuid.NewString()
		toolConfig1, err := structpb.NewStruct(map[string]any{"scope": "local", "id": testID})
		require.NoError(t, err)
		toolResp1, err := client.CreateTool(ctx, &teamsv1.CreateToolRequest{
			Type:        teamsv1.ToolType_TOOL_TYPE_MEMORY,
			Name:        "memory-" + testID,
			Description: "memory tool " + testID,
			Config:      toolConfig1,
		})
		require.NoError(t, err)
		toolID1 := toolResp1.Tool.Meta.Id

		toolConfig2, err := structpb.NewStruct(map[string]any{"mode": "auto", "id": testID})
		require.NoError(t, err)
		toolResp2, err := client.CreateTool(ctx, &teamsv1.CreateToolRequest{
			Type:        teamsv1.ToolType_TOOL_TYPE_MANAGE,
			Name:        "manage-" + testID,
			Description: "manage tool " + testID,
			Config:      toolConfig2,
		})
		require.NoError(t, err)
		toolID2 := toolResp2.Tool.Meta.Id

		updateToolResp, err := client.UpdateTool(ctx, &teamsv1.UpdateToolRequest{
			Id:          toolID1,
			Description: proto.String("memory tool updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "memory tool updated "+testID, updateToolResp.Tool.Description)

		requireToolListed(ctx, t, client, toolID1, teamsv1.ToolType_TOOL_TYPE_MEMORY)

		_, err = client.DeleteTool(ctx, &teamsv1.DeleteToolRequest{Id: toolID2})
		require.NoError(t, err)
		_, err = client.DeleteTool(ctx, &teamsv1.DeleteToolRequest{Id: toolID1})
		require.NoError(t, err)
	})

	t.Run("McpServers", func(t *testing.T) {
		testID := uuid.NewString()
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
			Title:       "MCP Server " + testID,
			Description: "MCP server " + testID,
			Config:      mcpConfig,
		})
		require.NoError(t, err)
		mcpID := mcpResp.McpServer.Meta.Id

		updatedMcpResp, err := client.UpdateMcpServer(ctx, &teamsv1.UpdateMcpServerRequest{
			Id:    mcpID,
			Title: proto.String("MCP Server Updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "MCP Server Updated "+testID, updatedMcpResp.McpServer.Title)

		requireMcpServerListed(ctx, t, client, mcpID)

		_, err = client.DeleteMcpServer(ctx, &teamsv1.DeleteMcpServerRequest{Id: mcpID})
		require.NoError(t, err)
	})

	t.Run("WorkspaceConfigurations", func(t *testing.T) {
		testID := uuid.NewString()
		nixConfig, err := structpb.NewStruct(map[string]any{"shell": "bash", "id": testID})
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
			Title:       "Workspace " + testID,
			Description: "Workspace config " + testID,
			Config:      workspaceConfig,
		})
		require.NoError(t, err)
		workspaceID := workspaceResp.WorkspaceConfiguration.Meta.Id

		updatedWorkspaceResp, err := client.UpdateWorkspaceConfiguration(ctx, &teamsv1.UpdateWorkspaceConfigurationRequest{
			Id:          workspaceID,
			Description: proto.String("Workspace config updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "Workspace config updated "+testID, updatedWorkspaceResp.WorkspaceConfiguration.Description)

		requireWorkspaceListed(ctx, t, client, workspaceID)

		_, err = client.DeleteWorkspaceConfiguration(ctx, &teamsv1.DeleteWorkspaceConfigurationRequest{Id: workspaceID})
		require.NoError(t, err)
	})

	t.Run("MemoryBuckets", func(t *testing.T) {
		testID := uuid.NewString()
		memoryConfig := &teamsv1.MemoryBucketConfig{Scope: teamsv1.MemoryBucketScope_MEMORY_BUCKET_SCOPE_GLOBAL, CollectionPrefix: "mem-" + testID}
		memoryResp, err := client.CreateMemoryBucket(ctx, &teamsv1.CreateMemoryBucketRequest{
			Title:       "Memory Bucket " + testID,
			Description: "Memory bucket " + testID,
			Config:      memoryConfig,
		})
		require.NoError(t, err)
		memoryID := memoryResp.MemoryBucket.Meta.Id

		updatedMemoryResp, err := client.UpdateMemoryBucket(ctx, &teamsv1.UpdateMemoryBucketRequest{
			Id:    memoryID,
			Title: proto.String("Memory Bucket Updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "Memory Bucket Updated "+testID, updatedMemoryResp.MemoryBucket.Title)

		requireMemoryBucketListed(ctx, t, client, memoryID)

		_, err = client.DeleteMemoryBucket(ctx, &teamsv1.DeleteMemoryBucketRequest{Id: memoryID})
		require.NoError(t, err)
	})

	t.Run("Variables", func(t *testing.T) {
		testID := uuid.NewString()
		variableResp1, err := client.CreateVariable(ctx, &teamsv1.CreateVariableRequest{
			Key:         fmt.Sprintf("API_KEY_%s", testID),
			Value:       "secret",
			Description: "Primary API key",
		})
		require.NoError(t, err)
		variableID1 := variableResp1.Variable.Meta.Id

		variableResp2, err := client.CreateVariable(ctx, &teamsv1.CreateVariableRequest{
			Key:         fmt.Sprintf("ENV_%s", testID),
			Value:       "prod",
			Description: "Environment",
		})
		require.NoError(t, err)
		variableID2 := variableResp2.Variable.Meta.Id

		getVariableResp, err := client.GetVariable(ctx, &teamsv1.GetVariableRequest{Id: variableID1})
		require.NoError(t, err)
		require.Equal(t, variableID1, getVariableResp.Variable.Meta.Id)
		require.Equal(t, fmt.Sprintf("API_KEY_%s", testID), getVariableResp.Variable.Key)

		updatedVariableResp, err := client.UpdateVariable(ctx, &teamsv1.UpdateVariableRequest{
			Id:    variableID1,
			Value: proto.String("secret-updated"),
		})
		require.NoError(t, err)
		require.Equal(t, "secret-updated", updatedVariableResp.Variable.Value)

		listVariables := listVariablesByQuery(ctx, t, client, testID)
		require.True(t, hasVariableID(listVariables, variableID1))
		require.True(t, hasVariableID(listVariables, variableID2))

		resolveVariableResp, err := client.ResolveVariable(ctx, &teamsv1.ResolveVariableRequest{Key: fmt.Sprintf("API_KEY_%s", testID)})
		require.NoError(t, err)
		require.True(t, resolveVariableResp.Found)
		require.Equal(t, "secret-updated", resolveVariableResp.Value)

		missingVariableResp, err := client.ResolveVariable(ctx, &teamsv1.ResolveVariableRequest{Key: "MISSING_" + testID})
		require.NoError(t, err)
		require.False(t, missingVariableResp.Found)
		require.Empty(t, missingVariableResp.Value)

		_, err = client.DeleteVariable(ctx, &teamsv1.DeleteVariableRequest{Id: variableID2})
		require.NoError(t, err)
		_, err = client.DeleteVariable(ctx, &teamsv1.DeleteVariableRequest{Id: variableID1})
		require.NoError(t, err)
	})

	t.Run("Attachments", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Title:       "Agent Alpha " + testID,
			Description: "First agent " + testID,
			Config:      baseAgentConfig("alpha-"+testID, "engineer"),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		toolResp, err := client.CreateTool(ctx, &teamsv1.CreateToolRequest{
			Type:        teamsv1.ToolType_TOOL_TYPE_MEMORY,
			Name:        "memory-" + testID,
			Description: "memory tool " + testID,
			Config:      &structpb.Struct{},
		})
		require.NoError(t, err)
		toolID := toolResp.Tool.Meta.Id

		attachmentResp, err := client.CreateAttachment(ctx, &teamsv1.CreateAttachmentRequest{
			Kind:     teamsv1.AttachmentKind_ATTACHMENT_KIND_AGENT_TOOL,
			SourceId: agentID,
			TargetId: toolID,
		})
		require.NoError(t, err)
		attachmentID := attachmentResp.Attachment.Meta.Id

		getAttachmentResp, err := client.GetAttachment(ctx, &teamsv1.GetAttachmentRequest{Id: attachmentID})
		require.NoError(t, err)
		require.Equal(t, attachmentID, getAttachmentResp.Attachment.Meta.Id)
		require.Equal(t, agentID, getAttachmentResp.Attachment.SourceId)
		require.Equal(t, toolID, getAttachmentResp.Attachment.TargetId)

		requireAttachmentListed(ctx, t, client, attachmentID, agentID)

		_, err = client.DeleteAttachment(ctx, &teamsv1.DeleteAttachmentRequest{Id: attachmentID})
		require.NoError(t, err)

		listAttachmentAfterDelete, err := client.ListAttachments(ctx, &teamsv1.ListAttachmentsRequest{
			SourceType: teamsv1.EntityType_ENTITY_TYPE_AGENT,
			SourceId:   agentID,
			PageSize:   listPageSize,
		})
		require.NoError(t, err)
		require.Len(t, listAttachmentAfterDelete.Attachments, 0)

		_, err = client.DeleteTool(ctx, &teamsv1.DeleteToolRequest{Id: toolID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("NegativePaths", func(t *testing.T) {
		testID := uuid.NewString()
		_, err := client.GetAgent(ctx, &teamsv1.GetAgentRequest{Id: uuid.NewString()})
		requireStatusCode(t, err, codes.NotFound)

		_, err = client.UpdateAgent(ctx, &teamsv1.UpdateAgentRequest{Id: uuid.NewString()})
		requireStatusCode(t, err, codes.InvalidArgument)

		agentResp, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Title:       "Agent Alpha " + testID,
			Description: "First agent " + testID,
			Config:      baseAgentConfig("alpha-"+testID, "engineer"),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		toolResp, err := client.CreateTool(ctx, &teamsv1.CreateToolRequest{
			Type:        teamsv1.ToolType_TOOL_TYPE_MEMORY,
			Name:        "memory-" + testID,
			Description: "memory tool " + testID,
			Config:      &structpb.Struct{},
		})
		require.NoError(t, err)
		toolID := toolResp.Tool.Meta.Id

		variableKey := fmt.Sprintf("DUPLICATE_KEY_%s", testID)
		variableResp, err := client.CreateVariable(ctx, &teamsv1.CreateVariableRequest{
			Key:         variableKey,
			Value:       "first",
			Description: "first value",
		})
		require.NoError(t, err)
		variableID := variableResp.Variable.Meta.Id

		_, err = client.CreateVariable(ctx, &teamsv1.CreateVariableRequest{
			Key:         variableKey,
			Value:       "second",
			Description: "second value",
		})
		requireStatusCode(t, err, codes.AlreadyExists)

		attachmentResp, err := client.CreateAttachment(ctx, &teamsv1.CreateAttachmentRequest{
			Kind:     teamsv1.AttachmentKind_ATTACHMENT_KIND_AGENT_TOOL,
			SourceId: agentID,
			TargetId: toolID,
		})
		require.NoError(t, err)
		attachmentID := attachmentResp.Attachment.Meta.Id

		_, err = client.CreateAttachment(ctx, &teamsv1.CreateAttachmentRequest{
			Kind:     teamsv1.AttachmentKind_ATTACHMENT_KIND_AGENT_TOOL,
			SourceId: agentID,
			TargetId: toolID,
		})
		requireStatusCode(t, err, codes.AlreadyExists)

		_, err = client.DeleteAttachment(ctx, &teamsv1.DeleteAttachmentRequest{Id: attachmentID})
		require.NoError(t, err)
		_, err = client.DeleteVariable(ctx, &teamsv1.DeleteVariableRequest{Id: variableID})
		require.NoError(t, err)
		_, err = client.DeleteTool(ctx, &teamsv1.DeleteToolRequest{Id: toolID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
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

func listAgentsByQuery(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, query string) []*teamsv1.Agent {
	t.Helper()
	var agents []*teamsv1.Agent
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListAgents(ctx, &teamsv1.ListAgentsRequest{
			Query:     query,
			PageSize:  listPageSize,
			PageToken: pageToken,
		})
		require.NoError(t, err)
		agents = append(agents, resp.Agents...)
		if resp.NextPageToken == "" {
			return agents
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("agent pagination exceeded for query %q", query)
	return nil
}

func hasAgentID(agents []*teamsv1.Agent, id string) bool {
	for _, agent := range agents {
		if agent.GetMeta().GetId() == id {
			return true
		}
	}
	return false
}

func requireToolListed(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, toolID string, toolType teamsv1.ToolType) {
	t.Helper()
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListTools(ctx, &teamsv1.ListToolsRequest{
			Type:      toolType,
			PageSize:  listPageSize,
			PageToken: pageToken,
		})
		require.NoError(t, err)
		for _, tool := range resp.Tools {
			if tool.GetMeta().GetId() == toolID {
				return
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("expected tool %s to be listed", toolID)
}

func requireMcpServerListed(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, mcpID string) {
	t.Helper()
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListMcpServers(ctx, &teamsv1.ListMcpServersRequest{
			PageSize:  listPageSize,
			PageToken: pageToken,
		})
		require.NoError(t, err)
		for _, server := range resp.McpServers {
			if server.GetMeta().GetId() == mcpID {
				return
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("expected MCP server %s to be listed", mcpID)
}

func requireWorkspaceListed(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, workspaceID string) {
	t.Helper()
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListWorkspaceConfigurations(ctx, &teamsv1.ListWorkspaceConfigurationsRequest{
			PageSize:  listPageSize,
			PageToken: pageToken,
		})
		require.NoError(t, err)
		for _, workspace := range resp.WorkspaceConfigurations {
			if workspace.GetMeta().GetId() == workspaceID {
				return
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("expected workspace configuration %s to be listed", workspaceID)
}

func requireMemoryBucketListed(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, memoryID string) {
	t.Helper()
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListMemoryBuckets(ctx, &teamsv1.ListMemoryBucketsRequest{
			PageSize:  listPageSize,
			PageToken: pageToken,
		})
		require.NoError(t, err)
		for _, bucket := range resp.MemoryBuckets {
			if bucket.GetMeta().GetId() == memoryID {
				return
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("expected memory bucket %s to be listed", memoryID)
}

func listVariablesByQuery(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, query string) []*teamsv1.Variable {
	t.Helper()
	var variables []*teamsv1.Variable
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListVariables(ctx, &teamsv1.ListVariablesRequest{
			Query:     query,
			PageSize:  listPageSize,
			PageToken: pageToken,
		})
		require.NoError(t, err)
		variables = append(variables, resp.Variables...)
		if resp.NextPageToken == "" {
			return variables
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("variable pagination exceeded for query %q", query)
	return nil
}

func hasVariableID(variables []*teamsv1.Variable, id string) bool {
	for _, variable := range variables {
		if variable.GetMeta().GetId() == id {
			return true
		}
	}
	return false
}

func requireAttachmentListed(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, attachmentID, agentID string) {
	t.Helper()
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListAttachments(ctx, &teamsv1.ListAttachmentsRequest{
			SourceType: teamsv1.EntityType_ENTITY_TYPE_AGENT,
			SourceId:   agentID,
			PageSize:   listPageSize,
			PageToken:  pageToken,
		})
		require.NoError(t, err)
		for _, attachment := range resp.Attachments {
			if attachment.GetMeta().GetId() == attachmentID {
				return
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("expected attachment %s to be listed", attachmentID)
}

func requireStatusCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	statusErr, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, code, statusErr.Code())
}
