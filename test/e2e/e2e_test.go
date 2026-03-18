//go:build e2e
// +build e2e

package e2e

import (
	"context"
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
		agentResp1, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Name:          "Agent Alpha " + testID,
			Role:          "engineer",
			Model:         uuid.NewString(),
			Description:   "First agent " + testID,
			Configuration: "config-alpha",
			Image:         "agent-image:latest",
			Resources:     baseResources(),
		})
		require.NoError(t, err)
		agentID1 := agentResp1.Agent.Meta.Id

		agentResp2, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Name:          "Agent Beta " + testID,
			Role:          "analyst",
			Model:         uuid.NewString(),
			Description:   "Second agent " + testID,
			Configuration: "config-beta",
			Image:         "agent-image:latest",
			Resources:     baseResources(),
		})
		require.NoError(t, err)
		agentID2 := agentResp2.Agent.Meta.Id

		updatedAgentResp, err := client.UpdateAgent(ctx, &teamsv1.UpdateAgentRequest{
			Id:   agentID1,
			Name: proto.String("Agent Alpha Updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "Agent Alpha Updated "+testID, updatedAgentResp.Agent.Name)

		listAgentsResp1, err := client.ListAgents(ctx, &teamsv1.ListAgentsRequest{PageSize: 1})
		require.NoError(t, err)
		require.NotEmpty(t, listAgentsResp1.Agents)
		require.NotEmpty(t, listAgentsResp1.NextPageToken)

		listAgents := listAgents(ctx, t, client)
		require.True(t, hasAgentID(listAgents, agentID1))
		require.True(t, hasAgentID(listAgents, agentID2))

		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID2})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID1})
		require.NoError(t, err)
	})

	t.Run("Volumes", func(t *testing.T) {
		testID := uuid.NewString()
		volumeResp, err := client.CreateVolume(ctx, &teamsv1.CreateVolumeRequest{
			Persistent:  true,
			MountPath:   "/data/" + testID,
			Size:        "1Gi",
			Description: "Volume " + testID,
		})
		require.NoError(t, err)
		volumeID := volumeResp.Volume.Meta.Id

		updatedVolumeResp, err := client.UpdateVolume(ctx, &teamsv1.UpdateVolumeRequest{
			Id:          volumeID,
			Description: proto.String("Volume Updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "Volume Updated "+testID, updatedVolumeResp.Volume.Description)

		volumes := listVolumes(ctx, t, client)
		require.True(t, hasVolumeID(volumes, volumeID))

		_, err = client.DeleteVolume(ctx, &teamsv1.DeleteVolumeRequest{Id: volumeID})
		require.NoError(t, err)
	})

	t.Run("Mcps", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Name:          "Mcp Agent " + testID,
			Role:          "agent",
			Model:         uuid.NewString(),
			Description:   "Mcp agent " + testID,
			Configuration: "config-mcp",
			Image:         "agent-image:latest",
			Resources:     baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		mcpResp, err := client.CreateMcp(ctx, &teamsv1.CreateMcpRequest{
			AgentId:     agentID,
			Image:       "mcp-image:latest",
			Command:     "mcp --run",
			Resources:   baseResources(),
			Description: "Mcp " + testID,
		})
		require.NoError(t, err)
		mcpID := mcpResp.Mcp.Meta.Id

		updatedMcpResp, err := client.UpdateMcp(ctx, &teamsv1.UpdateMcpRequest{
			Id:          mcpID,
			Description: proto.String("Mcp Updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "Mcp Updated "+testID, updatedMcpResp.Mcp.Description)

		mcps := listMcpsByAgent(ctx, t, client, agentID)
		require.True(t, hasMcpID(mcps, mcpID))

		_, err = client.DeleteMcp(ctx, &teamsv1.DeleteMcpRequest{Id: mcpID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("Skills", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Name:          "Skill Agent " + testID,
			Role:          "agent",
			Model:         uuid.NewString(),
			Description:   "Skill agent " + testID,
			Configuration: "config-skill",
			Image:         "agent-image:latest",
			Resources:     baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		skillResp, err := client.CreateSkill(ctx, &teamsv1.CreateSkillRequest{
			AgentId:     agentID,
			Name:        "Skill " + testID,
			Body:        "skill body",
			Description: "Skill description",
		})
		require.NoError(t, err)
		skillID := skillResp.Skill.Meta.Id

		updatedSkillResp, err := client.UpdateSkill(ctx, &teamsv1.UpdateSkillRequest{
			Id:   skillID,
			Body: proto.String("updated body"),
		})
		require.NoError(t, err)
		require.Equal(t, "updated body", updatedSkillResp.Skill.Body)

		skills := listSkillsByAgent(ctx, t, client, agentID)
		require.True(t, hasSkillID(skills, skillID))

		_, err = client.DeleteSkill(ctx, &teamsv1.DeleteSkillRequest{Id: skillID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("Hooks", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Name:          "Hook Agent " + testID,
			Role:          "agent",
			Model:         uuid.NewString(),
			Description:   "Hook agent " + testID,
			Configuration: "config-hook",
			Image:         "agent-image:latest",
			Resources:     baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		hookResp, err := client.CreateHook(ctx, &teamsv1.CreateHookRequest{
			AgentId:     agentID,
			Event:       "on_start",
			Function:    "handleStart",
			Image:       "hook-image:latest",
			Resources:   baseResources(),
			Description: "Hook " + testID,
		})
		require.NoError(t, err)
		hookID := hookResp.Hook.Meta.Id

		updatedHookResp, err := client.UpdateHook(ctx, &teamsv1.UpdateHookRequest{
			Id:    hookID,
			Event: proto.String("on_stop"),
		})
		require.NoError(t, err)
		require.Equal(t, "on_stop", updatedHookResp.Hook.Event)

		hooks := listHooksByAgent(ctx, t, client, agentID)
		require.True(t, hasHookID(hooks, hookID))

		_, err = client.DeleteHook(ctx, &teamsv1.DeleteHookRequest{Id: hookID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("Envs", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Name:          "Env Agent " + testID,
			Role:          "agent",
			Model:         uuid.NewString(),
			Description:   "Env agent " + testID,
			Configuration: "config-env",
			Image:         "agent-image:latest",
			Resources:     baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		envResp, err := client.CreateEnv(ctx, &teamsv1.CreateEnvRequest{
			Name:        "ENV_VAR",
			Description: "Env " + testID,
			Target:      &teamsv1.CreateEnvRequest_AgentId{AgentId: agentID},
			Source:      &teamsv1.CreateEnvRequest_Value{Value: "value"},
		})
		require.NoError(t, err)
		envID := envResp.Env.Meta.Id

		secretID := uuid.NewString()
		updatedEnvResp, err := client.UpdateEnv(ctx, &teamsv1.UpdateEnvRequest{
			Id:       envID,
			SecretId: proto.String(secretID),
		})
		require.NoError(t, err)
		require.Equal(t, secretID, updatedEnvResp.Env.GetSecretId())

		envs := listEnvsByAgent(ctx, t, client, agentID)
		require.True(t, hasEnvID(envs, envID))

		_, err = client.DeleteEnv(ctx, &teamsv1.DeleteEnvRequest{Id: envID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("InitScripts", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Name:          "Init Agent " + testID,
			Role:          "agent",
			Model:         uuid.NewString(),
			Description:   "Init agent " + testID,
			Configuration: "config-init",
			Image:         "agent-image:latest",
			Resources:     baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		initResp, err := client.CreateInitScript(ctx, &teamsv1.CreateInitScriptRequest{
			Script:      "echo init",
			Description: "Init script " + testID,
			Target:      &teamsv1.CreateInitScriptRequest_AgentId{AgentId: agentID},
		})
		require.NoError(t, err)
		initID := initResp.InitScript.Meta.Id

		updatedInitResp, err := client.UpdateInitScript(ctx, &teamsv1.UpdateInitScriptRequest{
			Id:          initID,
			Description: proto.String("Init script updated " + testID),
		})
		require.NoError(t, err)
		require.Equal(t, "Init script updated "+testID, updatedInitResp.InitScript.Description)

		scripts := listInitScriptsByAgent(ctx, t, client, agentID)
		require.True(t, hasInitScriptID(scripts, initID))

		_, err = client.DeleteInitScript(ctx, &teamsv1.DeleteInitScriptRequest{Id: initID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("VolumeAttachments", func(t *testing.T) {
		testID := uuid.NewString()
		agentResp, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Name:          "Attachment Agent " + testID,
			Role:          "agent",
			Model:         uuid.NewString(),
			Description:   "Attachment agent " + testID,
			Configuration: "config-attachment",
			Image:         "agent-image:latest",
			Resources:     baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		volumeResp, err := client.CreateVolume(ctx, &teamsv1.CreateVolumeRequest{
			Persistent:  false,
			MountPath:   "/vol/" + testID,
			Size:        "2Gi",
			Description: "Attachment volume " + testID,
		})
		require.NoError(t, err)
		volumeID := volumeResp.Volume.Meta.Id

		attachmentResp, err := client.CreateVolumeAttachment(ctx, &teamsv1.CreateVolumeAttachmentRequest{
			VolumeId: volumeID,
			Target:   &teamsv1.CreateVolumeAttachmentRequest_AgentId{AgentId: agentID},
		})
		require.NoError(t, err)
		attachmentID := attachmentResp.VolumeAttachment.Meta.Id

		_, err = client.CreateVolumeAttachment(ctx, &teamsv1.CreateVolumeAttachmentRequest{
			VolumeId: volumeID,
			Target:   &teamsv1.CreateVolumeAttachmentRequest_AgentId{AgentId: agentID},
		})
		requireStatusCode(t, err, codes.AlreadyExists)

		getAttachmentResp, err := client.GetVolumeAttachment(ctx, &teamsv1.GetVolumeAttachmentRequest{Id: attachmentID})
		require.NoError(t, err)
		require.Equal(t, volumeID, getAttachmentResp.VolumeAttachment.VolumeId)
		require.Equal(t, agentID, getAttachmentResp.VolumeAttachment.GetAgentId())

		attachments := listVolumeAttachmentsByVolume(ctx, t, client, volumeID)
		require.True(t, hasVolumeAttachmentID(attachments, attachmentID))

		_, err = client.DeleteVolumeAttachment(ctx, &teamsv1.DeleteVolumeAttachmentRequest{Id: attachmentID})
		require.NoError(t, err)
		_, err = client.DeleteVolume(ctx, &teamsv1.DeleteVolumeRequest{Id: volumeID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})

	t.Run("NegativePaths", func(t *testing.T) {
		_, err := client.GetAgent(ctx, &teamsv1.GetAgentRequest{Id: uuid.NewString()})
		requireStatusCode(t, err, codes.NotFound)

		_, err = client.UpdateAgent(ctx, &teamsv1.UpdateAgentRequest{Id: uuid.NewString()})
		requireStatusCode(t, err, codes.InvalidArgument)

		agentResp, err := client.CreateAgent(ctx, &teamsv1.CreateAgentRequest{
			Name:          "Negative Agent",
			Role:          "agent",
			Model:         uuid.NewString(),
			Description:   "negative",
			Configuration: "config-negative",
			Image:         "agent-image:latest",
			Resources:     baseResources(),
		})
		require.NoError(t, err)
		agentID := agentResp.Agent.Meta.Id

		mcpResp, err := client.CreateMcp(ctx, &teamsv1.CreateMcpRequest{
			AgentId:     agentID,
			Image:       "mcp-image:latest",
			Command:     "mcp",
			Resources:   baseResources(),
			Description: "negative",
		})
		require.NoError(t, err)
		mcpID := mcpResp.Mcp.Meta.Id

		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID})
		requireStatusCode(t, err, codes.FailedPrecondition)

		envResp, err := client.CreateEnv(ctx, &teamsv1.CreateEnvRequest{
			Name:        "NEGATIVE_ENV",
			Description: "negative",
			Target:      &teamsv1.CreateEnvRequest_AgentId{AgentId: agentID},
			Source:      &teamsv1.CreateEnvRequest_Value{Value: "value"},
		})
		require.NoError(t, err)
		envID := envResp.Env.Meta.Id

		_, err = client.UpdateEnv(ctx, &teamsv1.UpdateEnvRequest{
			Id:       envID,
			Value:    proto.String("value"),
			SecretId: proto.String(uuid.NewString()),
		})
		requireStatusCode(t, err, codes.InvalidArgument)

		_, err = client.DeleteEnv(ctx, &teamsv1.DeleteEnvRequest{Id: envID})
		require.NoError(t, err)
		_, err = client.DeleteMcp(ctx, &teamsv1.DeleteMcpRequest{Id: mcpID})
		require.NoError(t, err)
		_, err = client.DeleteAgent(ctx, &teamsv1.DeleteAgentRequest{Id: agentID})
		require.NoError(t, err)
	})
}

func baseResources() *teamsv1.ComputeResources {
	return &teamsv1.ComputeResources{
		RequestsCpu:    "100m",
		RequestsMemory: "128Mi",
		LimitsCpu:      "200m",
		LimitsMemory:   "256Mi",
	}
}

func listAgents(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient) []*teamsv1.Agent {
	t.Helper()
	var agents []*teamsv1.Agent
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListAgents(ctx, &teamsv1.ListAgentsRequest{PageSize: listPageSize, PageToken: pageToken})
		require.NoError(t, err)
		agents = append(agents, resp.Agents...)
		if resp.NextPageToken == "" {
			return agents
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("agent pagination exceeded")
	return nil
}

func listVolumes(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient) []*teamsv1.Volume {
	t.Helper()
	var volumes []*teamsv1.Volume
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListVolumes(ctx, &teamsv1.ListVolumesRequest{PageSize: listPageSize, PageToken: pageToken})
		require.NoError(t, err)
		volumes = append(volumes, resp.Volumes...)
		if resp.NextPageToken == "" {
			return volumes
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("volume pagination exceeded")
	return nil
}

func listMcpsByAgent(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, agentID string) []*teamsv1.Mcp {
	t.Helper()
	var mcps []*teamsv1.Mcp
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListMcps(ctx, &teamsv1.ListMcpsRequest{AgentId: agentID, PageSize: listPageSize, PageToken: pageToken})
		require.NoError(t, err)
		mcps = append(mcps, resp.Mcps...)
		if resp.NextPageToken == "" {
			return mcps
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("mcp pagination exceeded")
	return nil
}

func listSkillsByAgent(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, agentID string) []*teamsv1.Skill {
	t.Helper()
	var skills []*teamsv1.Skill
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListSkills(ctx, &teamsv1.ListSkillsRequest{AgentId: agentID, PageSize: listPageSize, PageToken: pageToken})
		require.NoError(t, err)
		skills = append(skills, resp.Skills...)
		if resp.NextPageToken == "" {
			return skills
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("skill pagination exceeded")
	return nil
}

func listHooksByAgent(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, agentID string) []*teamsv1.Hook {
	t.Helper()
	var hooks []*teamsv1.Hook
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListHooks(ctx, &teamsv1.ListHooksRequest{AgentId: agentID, PageSize: listPageSize, PageToken: pageToken})
		require.NoError(t, err)
		hooks = append(hooks, resp.Hooks...)
		if resp.NextPageToken == "" {
			return hooks
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("hook pagination exceeded")
	return nil
}

func listEnvsByAgent(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, agentID string) []*teamsv1.Env {
	t.Helper()
	var envs []*teamsv1.Env
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListEnvs(ctx, &teamsv1.ListEnvsRequest{AgentId: agentID, PageSize: listPageSize, PageToken: pageToken})
		require.NoError(t, err)
		envs = append(envs, resp.Envs...)
		if resp.NextPageToken == "" {
			return envs
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("env pagination exceeded")
	return nil
}

func listInitScriptsByAgent(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, agentID string) []*teamsv1.InitScript {
	t.Helper()
	var scripts []*teamsv1.InitScript
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListInitScripts(ctx, &teamsv1.ListInitScriptsRequest{AgentId: agentID, PageSize: listPageSize, PageToken: pageToken})
		require.NoError(t, err)
		scripts = append(scripts, resp.InitScripts...)
		if resp.NextPageToken == "" {
			return scripts
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("init script pagination exceeded")
	return nil
}

func listVolumeAttachmentsByVolume(ctx context.Context, t *testing.T, client teamsv1.TeamsServiceClient, volumeID string) []*teamsv1.VolumeAttachment {
	t.Helper()
	var attachments []*teamsv1.VolumeAttachment
	pageToken := ""
	for i := 0; i < 20; i++ {
		resp, err := client.ListVolumeAttachments(ctx, &teamsv1.ListVolumeAttachmentsRequest{VolumeId: volumeID, PageSize: listPageSize, PageToken: pageToken})
		require.NoError(t, err)
		attachments = append(attachments, resp.VolumeAttachments...)
		if resp.NextPageToken == "" {
			return attachments
		}
		pageToken = resp.NextPageToken
	}
	t.Fatalf("volume attachment pagination exceeded")
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

func hasVolumeID(volumes []*teamsv1.Volume, id string) bool {
	for _, volume := range volumes {
		if volume.GetMeta().GetId() == id {
			return true
		}
	}
	return false
}

func hasMcpID(mcps []*teamsv1.Mcp, id string) bool {
	for _, mcp := range mcps {
		if mcp.GetMeta().GetId() == id {
			return true
		}
	}
	return false
}

func hasSkillID(skills []*teamsv1.Skill, id string) bool {
	for _, skill := range skills {
		if skill.GetMeta().GetId() == id {
			return true
		}
	}
	return false
}

func hasHookID(hooks []*teamsv1.Hook, id string) bool {
	for _, hook := range hooks {
		if hook.GetMeta().GetId() == id {
			return true
		}
	}
	return false
}

func hasEnvID(envs []*teamsv1.Env, id string) bool {
	for _, env := range envs {
		if env.GetMeta().GetId() == id {
			return true
		}
	}
	return false
}

func hasInitScriptID(scripts []*teamsv1.InitScript, id string) bool {
	for _, script := range scripts {
		if script.GetMeta().GetId() == id {
			return true
		}
	}
	return false
}

func hasVolumeAttachmentID(attachments []*teamsv1.VolumeAttachment, id string) bool {
	for _, attachment := range attachments {
		if attachment.GetMeta().GetId() == id {
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
