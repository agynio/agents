//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const listPageSize int32 = 50

func TestAgentsServiceE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ctx = withTestIdentity(ctx)

	conn, err := grpc.DialContext(ctx, agentsAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	client := agentsv1.NewAgentsServiceClient(conn)

	t.Run("Agents", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp1, err := client.CreateAgent(ctx, &agentsv1.CreateAgentRequest{
			OrganizationId: testOrganizationID,
			Name:           "Agent Alpha " + testID,
			Role:           "engineer",
			Model:          "test-model",
			Description:    "First agent " + testID,
			Configuration:  "config-alpha",
			Image:          "agent-image:latest",
			InitImage:      "ghcr.io/agynio/agent-init-codex:v1.0.0",
			Resources:      baseResources(),
		})
		require.NoError(t, err)
		agentID1 := agentResp1.Agent.Meta.Id
		require.Equal(t, "ghcr.io/agynio/agent-init-codex:v1.0.0", agentResp1.Agent.InitImage)

		agentResp2, err := client.CreateAgent(ctx, &agentsv1.CreateAgentRequest{
			OrganizationId: testOrganizationID,
			Name:           "Agent Beta " + testID,
			Role:           "analyst",
			Model:          "test-model",
			Description:    "Second agent " + testID,
			Configuration:  "config-beta",
			Image:          "agent-image:latest",
			Resources:      baseResources(),
		})
		require.NoError(t, err)
		agentID2 := agentResp2.Agent.Meta.Id

		updatedAgentResp, err := client.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{
			Id:        agentID1,
			Name:      proto.String("Agent Alpha Updated " + testID),
			InitImage: proto.String("ghcr.io/agynio/agent-init-codex:v1.0.1"),
		})
		require.NoError(t, err)
		require.Equal(t, "Agent Alpha Updated "+testID, updatedAgentResp.Agent.Name)
		require.Equal(t, "ghcr.io/agynio/agent-init-codex:v1.0.1", updatedAgentResp.Agent.InitImage)

		listAgentsResp1, err := client.ListAgents(ctx, &agentsv1.ListAgentsRequest{OrganizationId: testOrganizationID, PageSize: 1})
		require.NoError(t, err)
		require.NotEmpty(t, listAgentsResp1.Agents)
		require.NotEmpty(t, listAgentsResp1.NextPageToken)

		listAgents := listAgents(ctx, t, client)
		require.True(t, hasID(listAgents, agentID1))
		require.True(t, hasID(listAgents, agentID2))

		_, err = client.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: agentID2})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: agentID1})
		require.NoError(t, err)
	})

	t.Run("Volumes", func(t *testing.T) {
		testID := uuid.NewString()
		volumeResp, err := client.CreateVolume(ctx, &agentsv1.CreateVolumeRequest{
			OrganizationId: testOrganizationID,
			Persistent:     true,
			MountPath:      "/data/" + testID,
			Size:           "1Gi",
			Description:    "Volume " + testID,
		})
		require.NoError(t, err)
		volumeID := volumeResp.Volume.Meta.Id

		updatedVolumeResp, err := client.UpdateVolume(ctx, &agentsv1.UpdateVolumeRequest{
			Id:          volumeID,
			Description: proto.String("Volume Updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "Volume Updated "+testID, updatedVolumeResp.Volume.Description)

		volumes := listVolumes(ctx, t, client)
		require.True(t, hasID(volumes, volumeID))

		_, err = client.DeleteVolume(ctx, &agentsv1.DeleteVolumeRequest{Id: volumeID})
		require.NoError(t, err)
	})

	t.Run("Mcps", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &agentsv1.CreateAgentRequest{
			OrganizationId: testOrganizationID,
			Name:           "Mcp Agent " + testID,
			Role:           "agent",
			Model:          "test-model",
			Description:    "Mcp agent " + testID,
			Configuration:  "config-mcp",
			Image:          "agent-image:latest",
			Resources:      baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		mcpResp, err := client.CreateMcp(ctx, &agentsv1.CreateMcpRequest{
			AgentId:     agentID,
			Image:       "mcp-image:latest",
			Command:     "mcp --run",
			Resources:   baseResources(),
			Description: "Mcp " + testID,
		})
		require.NoError(t, err)
		mcpID := mcpResp.Mcp.Meta.Id

		updatedMcpResp, err := client.UpdateMcp(ctx, &agentsv1.UpdateMcpRequest{
			Id:          mcpID,
			Description: proto.String("Mcp Updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "Mcp Updated "+testID, updatedMcpResp.Mcp.Description)

		mcps := listMcpsByAgent(ctx, t, client, agentID)
		require.True(t, hasID(mcps, mcpID))

		_, err = client.DeleteMcp(ctx, &agentsv1.DeleteMcpRequest{Id: mcpID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("Skills", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &agentsv1.CreateAgentRequest{
			OrganizationId: testOrganizationID,
			Name:           "Skill Agent " + testID,
			Role:           "agent",
			Model:          "test-model",
			Description:    "Skill agent " + testID,
			Configuration:  "config-skill",
			Image:          "agent-image:latest",
			Resources:      baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		skillResp, err := client.CreateSkill(ctx, &agentsv1.CreateSkillRequest{
			AgentId:     agentID,
			Name:        "Skill " + testID,
			Body:        "skill body",
			Description: "Skill description",
		})
		require.NoError(t, err)
		skillID := skillResp.Skill.Meta.Id

		updatedSkillResp, err := client.UpdateSkill(ctx, &agentsv1.UpdateSkillRequest{
			Id:   skillID,
			Body: proto.String("updated body"),
		})
		require.NoError(t, err)
		require.Equal(t, "updated body", updatedSkillResp.Skill.Body)

		skills := listSkillsByAgent(ctx, t, client, agentID)
		require.True(t, hasID(skills, skillID))

		_, err = client.DeleteSkill(ctx, &agentsv1.DeleteSkillRequest{Id: skillID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("Hooks", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &agentsv1.CreateAgentRequest{
			OrganizationId: testOrganizationID,
			Name:           "Hook Agent " + testID,
			Role:           "agent",
			Model:          "test-model",
			Description:    "Hook agent " + testID,
			Configuration:  "config-hook",
			Image:          "agent-image:latest",
			Resources:      baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		hookResp, err := client.CreateHook(ctx, &agentsv1.CreateHookRequest{
			AgentId:     agentID,
			Event:       "on_start",
			Function:    "handleStart",
			Image:       "hook-image:latest",
			Resources:   baseResources(),
			Description: "Hook " + testID,
		})
		require.NoError(t, err)
		hookID := hookResp.Hook.Meta.Id

		updatedHookResp, err := client.UpdateHook(ctx, &agentsv1.UpdateHookRequest{
			Id:    hookID,
			Event: proto.String("on_stop"),
		})
		require.NoError(t, err)
		require.Equal(t, "on_stop", updatedHookResp.Hook.Event)

		hooks := listHooksByAgent(ctx, t, client, agentID)
		require.True(t, hasID(hooks, hookID))

		_, err = client.DeleteHook(ctx, &agentsv1.DeleteHookRequest{Id: hookID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("Envs", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &agentsv1.CreateAgentRequest{
			OrganizationId: testOrganizationID,
			Name:           "Env Agent " + testID,
			Role:           "agent",
			Model:          "test-model",
			Description:    "Env agent " + testID,
			Configuration:  "config-env",
			Image:          "agent-image:latest",
			Resources:      baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		mcpResp, err := client.CreateMcp(ctx, &agentsv1.CreateMcpRequest{
			AgentId:     agentID,
			Image:       "mcp-image:latest",
			Command:     "mcp --env",
			Resources:   baseResources(),
			Description: "Env mcp " + testID,
		})
		require.NoError(t, err)
		mcpID := mcpResp.Mcp.Meta.Id

		hookResp, err := client.CreateHook(ctx, &agentsv1.CreateHookRequest{
			AgentId:     agentID,
			Event:       "env_event",
			Function:    "envHandler",
			Image:       "hook-image:latest",
			Resources:   baseResources(),
			Description: "Env hook " + testID,
		})
		require.NoError(t, err)
		hookID := hookResp.Hook.Meta.Id

		envResp, err := client.CreateEnv(ctx, &agentsv1.CreateEnvRequest{
			Name:        "ENV_VAR",
			Description: "Env " + testID,
			Target:      &agentsv1.CreateEnvRequest_AgentId{AgentId: agentID},
			Source:      &agentsv1.CreateEnvRequest_Value{Value: "value"},
		})
		require.NoError(t, err)
		envID := envResp.Env.Meta.Id

		envMcpResp, err := client.CreateEnv(ctx, &agentsv1.CreateEnvRequest{
			Name:        "MCP_ENV",
			Description: "Env mcp " + testID,
			Target:      &agentsv1.CreateEnvRequest_McpId{McpId: mcpID},
			Source:      &agentsv1.CreateEnvRequest_Value{Value: "mcp-value"},
		})
		require.NoError(t, err)
		envMcpID := envMcpResp.Env.Meta.Id

		envHookResp, err := client.CreateEnv(ctx, &agentsv1.CreateEnvRequest{
			Name:        "HOOK_ENV",
			Description: "Env hook " + testID,
			Target:      &agentsv1.CreateEnvRequest_HookId{HookId: hookID},
			Source:      &agentsv1.CreateEnvRequest_Value{Value: "hook-value"},
		})
		require.NoError(t, err)
		envHookID := envHookResp.Env.Meta.Id

		secretID := uuid.NewString()
		updatedEnvResp, err := client.UpdateEnv(ctx, &agentsv1.UpdateEnvRequest{
			Id:       envID,
			SecretId: proto.String(secretID),
		})
		require.NoError(t, err)
		require.Equal(t, secretID, updatedEnvResp.Env.GetSecretId())

		envsByAgent := listEnvs(ctx, t, client, &agentsv1.ListEnvsRequest{AgentId: agentID})
		require.True(t, hasID(envsByAgent, envID))
		envsByMcp := listEnvs(ctx, t, client, &agentsv1.ListEnvsRequest{McpId: mcpID})
		require.True(t, hasID(envsByMcp, envMcpID))
		envsByHook := listEnvs(ctx, t, client, &agentsv1.ListEnvsRequest{HookId: hookID})
		require.True(t, hasID(envsByHook, envHookID))

		_, err = client.DeleteEnv(ctx, &agentsv1.DeleteEnvRequest{Id: envHookID})
		require.NoError(t, err)
		_, err = client.DeleteEnv(ctx, &agentsv1.DeleteEnvRequest{Id: envMcpID})
		require.NoError(t, err)
		_, err = client.DeleteEnv(ctx, &agentsv1.DeleteEnvRequest{Id: envID})
		require.NoError(t, err)
		_, err = client.DeleteHook(ctx, &agentsv1.DeleteHookRequest{Id: hookID})
		require.NoError(t, err)
		_, err = client.DeleteMcp(ctx, &agentsv1.DeleteMcpRequest{Id: mcpID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("InitScripts", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &agentsv1.CreateAgentRequest{
			OrganizationId: testOrganizationID,
			Name:           "Init Agent " + testID,
			Role:           "agent",
			Model:          "test-model",
			Description:    "Init agent " + testID,
			Configuration:  "config-init",
			Image:          "agent-image:latest",
			Resources:      baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		mcpResp, err := client.CreateMcp(ctx, &agentsv1.CreateMcpRequest{
			AgentId:     agentID,
			Image:       "mcp-image:latest",
			Command:     "mcp --init",
			Resources:   baseResources(),
			Description: "Init mcp " + testID,
		})
		require.NoError(t, err)
		mcpID := mcpResp.Mcp.Meta.Id

		hookResp, err := client.CreateHook(ctx, &agentsv1.CreateHookRequest{
			AgentId:     agentID,
			Event:       "init_event",
			Function:    "initHandler",
			Image:       "hook-image:latest",
			Resources:   baseResources(),
			Description: "Init hook " + testID,
		})
		require.NoError(t, err)
		hookID := hookResp.Hook.Meta.Id

		initResp, err := client.CreateInitScript(ctx, &agentsv1.CreateInitScriptRequest{
			Script:      "echo init",
			Description: "Init script " + testID,
			Target:      &agentsv1.CreateInitScriptRequest_AgentId{AgentId: agentID},
		})
		require.NoError(t, err)
		initID := initResp.InitScript.Meta.Id

		mcpInitResp, err := client.CreateInitScript(ctx, &agentsv1.CreateInitScriptRequest{
			Script:      "echo mcp init",
			Description: "Init script mcp " + testID,
			Target:      &agentsv1.CreateInitScriptRequest_McpId{McpId: mcpID},
		})
		require.NoError(t, err)
		mcpInitID := mcpInitResp.InitScript.Meta.Id

		hookInitResp, err := client.CreateInitScript(ctx, &agentsv1.CreateInitScriptRequest{
			Script:      "echo hook init",
			Description: "Init script hook " + testID,
			Target:      &agentsv1.CreateInitScriptRequest_HookId{HookId: hookID},
		})
		require.NoError(t, err)
		hookInitID := hookInitResp.InitScript.Meta.Id

		updatedInitResp, err := client.UpdateInitScript(ctx, &agentsv1.UpdateInitScriptRequest{
			Id:          initID,
			Description: proto.String("Init script updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "Init script updated "+testID, updatedInitResp.InitScript.Description)

		scriptsByAgent := listInitScripts(ctx, t, client, &agentsv1.ListInitScriptsRequest{AgentId: agentID})
		require.True(t, hasID(scriptsByAgent, initID))
		scriptsByMcp := listInitScripts(ctx, t, client, &agentsv1.ListInitScriptsRequest{McpId: mcpID})
		require.True(t, hasID(scriptsByMcp, mcpInitID))
		scriptsByHook := listInitScripts(ctx, t, client, &agentsv1.ListInitScriptsRequest{HookId: hookID})
		require.True(t, hasID(scriptsByHook, hookInitID))

		_, err = client.DeleteInitScript(ctx, &agentsv1.DeleteInitScriptRequest{Id: hookInitID})
		require.NoError(t, err)
		_, err = client.DeleteInitScript(ctx, &agentsv1.DeleteInitScriptRequest{Id: mcpInitID})
		require.NoError(t, err)
		_, err = client.DeleteInitScript(ctx, &agentsv1.DeleteInitScriptRequest{Id: initID})
		require.NoError(t, err)
		_, err = client.DeleteHook(ctx, &agentsv1.DeleteHookRequest{Id: hookID})
		require.NoError(t, err)
		_, err = client.DeleteMcp(ctx, &agentsv1.DeleteMcpRequest{Id: mcpID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("VolumeAttachments", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &agentsv1.CreateAgentRequest{
			OrganizationId: testOrganizationID,
			Name:           "Attachment Agent " + testID,
			Role:           "agent",
			Model:          "test-model",
			Description:    "Attachment agent " + testID,
			Configuration:  "config-attachment",
			Image:          "agent-image:latest",
			Resources:      baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		mcpResp, err := client.CreateMcp(ctx, &agentsv1.CreateMcpRequest{
			AgentId:     agentID,
			Image:       "mcp-image:latest",
			Command:     "mcp --attach",
			Resources:   baseResources(),
			Description: "Attachment mcp " + testID,
		})
		require.NoError(t, err)
		mcpID := mcpResp.Mcp.Meta.Id

		hookResp, err := client.CreateHook(ctx, &agentsv1.CreateHookRequest{
			AgentId:     agentID,
			Event:       "attach_event",
			Function:    "attachHandler",
			Image:       "hook-image:latest",
			Resources:   baseResources(),
			Description: "Attachment hook " + testID,
		})
		require.NoError(t, err)
		hookID := hookResp.Hook.Meta.Id

		volumeResp, err := client.CreateVolume(ctx, &agentsv1.CreateVolumeRequest{
			OrganizationId: testOrganizationID,
			Persistent:     false,
			MountPath:      "/vol/" + testID,
			Size:           "2Gi",
			Description:    "Attachment volume " + testID,
		})
		require.NoError(t, err)
		volumeID := volumeResp.Volume.Meta.Id

		attachmentResp, err := client.CreateVolumeAttachment(ctx, &agentsv1.CreateVolumeAttachmentRequest{
			VolumeId: volumeID,
			Target:   &agentsv1.CreateVolumeAttachmentRequest_AgentId{AgentId: agentID},
		})
		require.NoError(t, err)
		attachmentID := attachmentResp.VolumeAttachment.Meta.Id

		mcpAttachmentResp, err := client.CreateVolumeAttachment(ctx, &agentsv1.CreateVolumeAttachmentRequest{
			VolumeId: volumeID,
			Target:   &agentsv1.CreateVolumeAttachmentRequest_McpId{McpId: mcpID},
		})
		require.NoError(t, err)
		mcpAttachmentID := mcpAttachmentResp.VolumeAttachment.Meta.Id

		hookAttachmentResp, err := client.CreateVolumeAttachment(ctx, &agentsv1.CreateVolumeAttachmentRequest{
			VolumeId: volumeID,
			Target:   &agentsv1.CreateVolumeAttachmentRequest_HookId{HookId: hookID},
		})
		require.NoError(t, err)
		hookAttachmentID := hookAttachmentResp.VolumeAttachment.Meta.Id

		_, err = client.CreateVolumeAttachment(ctx, &agentsv1.CreateVolumeAttachmentRequest{
			VolumeId: volumeID,
			Target:   &agentsv1.CreateVolumeAttachmentRequest_AgentId{AgentId: agentID},
		})
		requireStatusCode(t, err, codes.AlreadyExists)

		getAttachmentResp, err := client.GetVolumeAttachment(ctx, &agentsv1.GetVolumeAttachmentRequest{Id: attachmentID})
		require.NoError(t, err)
		require.Equal(t, volumeID, getAttachmentResp.VolumeAttachment.VolumeId)
		require.Equal(t, agentID, getAttachmentResp.VolumeAttachment.GetAgentId())

		getMcpAttachmentResp, err := client.GetVolumeAttachment(ctx, &agentsv1.GetVolumeAttachmentRequest{Id: mcpAttachmentID})
		require.NoError(t, err)
		require.Equal(t, volumeID, getMcpAttachmentResp.VolumeAttachment.VolumeId)
		require.Equal(t, mcpID, getMcpAttachmentResp.VolumeAttachment.GetMcpId())

		getHookAttachmentResp, err := client.GetVolumeAttachment(ctx, &agentsv1.GetVolumeAttachmentRequest{Id: hookAttachmentID})
		require.NoError(t, err)
		require.Equal(t, volumeID, getHookAttachmentResp.VolumeAttachment.VolumeId)
		require.Equal(t, hookID, getHookAttachmentResp.VolumeAttachment.GetHookId())

		attachments := listVolumeAttachments(ctx, t, client, &agentsv1.ListVolumeAttachmentsRequest{VolumeId: volumeID})
		require.True(t, hasID(attachments, attachmentID))
		require.True(t, hasID(attachments, mcpAttachmentID))
		require.True(t, hasID(attachments, hookAttachmentID))

		_, err = client.DeleteVolumeAttachment(ctx, &agentsv1.DeleteVolumeAttachmentRequest{Id: hookAttachmentID})
		require.NoError(t, err)
		_, err = client.DeleteVolumeAttachment(ctx, &agentsv1.DeleteVolumeAttachmentRequest{Id: mcpAttachmentID})
		require.NoError(t, err)
		_, err = client.DeleteVolumeAttachment(ctx, &agentsv1.DeleteVolumeAttachmentRequest{Id: attachmentID})
		require.NoError(t, err)
		_, err = client.DeleteVolume(ctx, &agentsv1.DeleteVolumeRequest{Id: volumeID})
		require.NoError(t, err)
		_, err = client.DeleteHook(ctx, &agentsv1.DeleteHookRequest{Id: hookID})
		require.NoError(t, err)
		_, err = client.DeleteMcp(ctx, &agentsv1.DeleteMcpRequest{Id: mcpID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("NegativePaths", func(t *testing.T) {
		_, err := client.GetAgent(ctx, &agentsv1.GetAgentRequest{Id: uuid.NewString()})
		requireStatusCode(t, err, codes.NotFound)

		_, err = client.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{Id: uuid.NewString()})
		requireStatusCode(t, err, codes.InvalidArgument)

		_, err = client.CreateMcp(ctx, &agentsv1.CreateMcpRequest{
			AgentId:     uuid.NewString(),
			Image:       "mcp-image:latest",
			Command:     "mcp",
			Resources:   baseResources(),
			Description: "negative",
		})
		requireStatusCode(t, err, codes.FailedPrecondition)

		agentResp, err := client.CreateAgent(ctx, &agentsv1.CreateAgentRequest{
			OrganizationId: testOrganizationID,
			Name:           "Negative Agent",
			Role:           "agent",
			Model:          "test-model",
			Description:    "negative",
			Configuration:  "config-negative",
			Image:          "agent-image:latest",
			Resources:      baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		mcpResp, err := client.CreateMcp(ctx, &agentsv1.CreateMcpRequest{
			AgentId:     agentID,
			Image:       "mcp-image:latest",
			Command:     "mcp",
			Resources:   baseResources(),
			Description: "negative",
		})
		require.NoError(t, err)
		mcpID := mcpResp.Mcp.Meta.Id

		_, err = client.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: agentID})
		requireStatusCode(t, err, codes.FailedPrecondition)

		envResp, err := client.CreateEnv(ctx, &agentsv1.CreateEnvRequest{
			Name:        "NEGATIVE_ENV",
			Description: "negative",
			Target:      &agentsv1.CreateEnvRequest_AgentId{AgentId: agentID},
			Source:      &agentsv1.CreateEnvRequest_Value{Value: "value"},
		})
		require.NoError(t, err)
		envID := envResp.Env.Meta.Id

		_, err = client.UpdateEnv(ctx, &agentsv1.UpdateEnvRequest{
			Id:       envID,
			Value:    proto.String("value"),
			SecretId: proto.String(uuid.NewString()),
		})
		requireStatusCode(t, err, codes.InvalidArgument)

		_, err = client.DeleteEnv(ctx, &agentsv1.DeleteEnvRequest{Id: envID})
		require.NoError(t, err)
		_, err = client.DeleteMcp(ctx, &agentsv1.DeleteMcpRequest{Id: mcpID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &agentsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})
}

func baseResources() *agentsv1.ComputeResources {
	return &agentsv1.ComputeResources{
		RequestsCpu:    "100m",
		RequestsMemory: "128Mi",
		LimitsCpu:      "200m",
		LimitsMemory:   "256Mi",
	}
}

type metaGetter interface {
	GetMeta() *agentsv1.EntityMeta
}

func listPaged[T any](t *testing.T, resource string, fetch func(pageToken string) ([]T, string, error)) []T {
	t.Helper()
	var items []T
	pageToken := ""
	for i := 0; i < 20; i++ {
		pageItems, nextPageToken, err := fetch(pageToken)
		require.NoError(t, err)
		items = append(items, pageItems...)
		if nextPageToken == "" {
			return items
		}
		pageToken = nextPageToken
	}
	t.Fatalf("%s pagination exceeded", resource)
	return nil
}

func listAgents(ctx context.Context, t *testing.T, client agentsv1.AgentsServiceClient) []*agentsv1.Agent {
	return listPaged(t, "agent", func(pageToken string) ([]*agentsv1.Agent, string, error) {
		resp, err := client.ListAgents(ctx, &agentsv1.ListAgentsRequest{OrganizationId: testOrganizationID, PageSize: listPageSize, PageToken: pageToken})
		if err != nil {
			return nil, "", err
		}
		return resp.Agents, resp.NextPageToken, nil
	})
}

func listVolumes(ctx context.Context, t *testing.T, client agentsv1.AgentsServiceClient) []*agentsv1.Volume {
	return listPaged(t, "volume", func(pageToken string) ([]*agentsv1.Volume, string, error) {
		resp, err := client.ListVolumes(ctx, &agentsv1.ListVolumesRequest{OrganizationId: testOrganizationID, PageSize: listPageSize, PageToken: pageToken})
		if err != nil {
			return nil, "", err
		}
		return resp.Volumes, resp.NextPageToken, nil
	})
}

func listMcpsByAgent(ctx context.Context, t *testing.T, client agentsv1.AgentsServiceClient, agentID string) []*agentsv1.Mcp {
	return listPaged(t, "mcp", func(pageToken string) ([]*agentsv1.Mcp, string, error) {
		resp, err := client.ListMcps(ctx, &agentsv1.ListMcpsRequest{AgentId: agentID, PageSize: listPageSize, PageToken: pageToken})
		if err != nil {
			return nil, "", err
		}
		return resp.Mcps, resp.NextPageToken, nil
	})
}

func listSkillsByAgent(ctx context.Context, t *testing.T, client agentsv1.AgentsServiceClient, agentID string) []*agentsv1.Skill {
	return listPaged(t, "skill", func(pageToken string) ([]*agentsv1.Skill, string, error) {
		resp, err := client.ListSkills(ctx, &agentsv1.ListSkillsRequest{AgentId: agentID, PageSize: listPageSize, PageToken: pageToken})
		if err != nil {
			return nil, "", err
		}
		return resp.Skills, resp.NextPageToken, nil
	})
}

func listHooksByAgent(ctx context.Context, t *testing.T, client agentsv1.AgentsServiceClient, agentID string) []*agentsv1.Hook {
	return listPaged(t, "hook", func(pageToken string) ([]*agentsv1.Hook, string, error) {
		resp, err := client.ListHooks(ctx, &agentsv1.ListHooksRequest{AgentId: agentID, PageSize: listPageSize, PageToken: pageToken})
		if err != nil {
			return nil, "", err
		}
		return resp.Hooks, resp.NextPageToken, nil
	})
}

func listEnvs(ctx context.Context, t *testing.T, client agentsv1.AgentsServiceClient, request *agentsv1.ListEnvsRequest) []*agentsv1.Env {
	baseRequest := *request
	return listPaged(t, "env", func(pageToken string) ([]*agentsv1.Env, string, error) {
		req := baseRequest
		req.PageSize = listPageSize
		req.PageToken = pageToken
		resp, err := client.ListEnvs(ctx, &req)
		if err != nil {
			return nil, "", err
		}
		return resp.Envs, resp.NextPageToken, nil
	})
}

func listInitScripts(ctx context.Context, t *testing.T, client agentsv1.AgentsServiceClient, request *agentsv1.ListInitScriptsRequest) []*agentsv1.InitScript {
	baseRequest := *request
	return listPaged(t, "init script", func(pageToken string) ([]*agentsv1.InitScript, string, error) {
		req := baseRequest
		req.PageSize = listPageSize
		req.PageToken = pageToken
		resp, err := client.ListInitScripts(ctx, &req)
		if err != nil {
			return nil, "", err
		}
		return resp.InitScripts, resp.NextPageToken, nil
	})
}

func listVolumeAttachments(ctx context.Context, t *testing.T, client agentsv1.AgentsServiceClient, request *agentsv1.ListVolumeAttachmentsRequest) []*agentsv1.VolumeAttachment {
	baseRequest := *request
	return listPaged(t, "volume attachment", func(pageToken string) ([]*agentsv1.VolumeAttachment, string, error) {
		req := baseRequest
		req.PageSize = listPageSize
		req.PageToken = pageToken
		resp, err := client.ListVolumeAttachments(ctx, &req)
		if err != nil {
			return nil, "", err
		}
		return resp.VolumeAttachments, resp.NextPageToken, nil
	})
}

func hasID[T metaGetter](items []T, id string) bool {
	for _, item := range items {
		if item.GetMeta().GetId() == id {
			return true
		}
	}
	return false
}

func requireStatusCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	statusErr, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, code, statusErr.Code())
}
