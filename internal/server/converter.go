package server

import (
	teamsv1 "github.com/agynio/teams/.gen/go/agynio/api/teams/v1"
	"github.com/agynio/teams/internal/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoEntityMeta(meta store.EntityMeta) *teamsv1.EntityMeta {
	return &teamsv1.EntityMeta{
		Id:        meta.ID.String(),
		CreatedAt: timestamppb.New(meta.CreatedAt),
		UpdatedAt: timestamppb.New(meta.UpdatedAt),
	}
}

func toProtoComputeResources(resources store.ComputeResources) *teamsv1.ComputeResources {
	return &teamsv1.ComputeResources{
		RequestsCpu:    resources.RequestsCPU,
		RequestsMemory: resources.RequestsMemory,
		LimitsCpu:      resources.LimitsCPU,
		LimitsMemory:   resources.LimitsMemory,
	}
}

func toProtoAgent(agent store.Agent) *teamsv1.Agent {
	return &teamsv1.Agent{
		Meta:          toProtoEntityMeta(agent.Meta),
		Name:          agent.Name,
		Role:          agent.Role,
		Model:         agent.Model.String(),
		Description:   agent.Description,
		Configuration: agent.Configuration,
		Image:         agent.Image,
		Resources:     toProtoComputeResources(agent.Resources),
	}
}

func toProtoVolume(volume store.Volume) *teamsv1.Volume {
	return &teamsv1.Volume{
		Meta:        toProtoEntityMeta(volume.Meta),
		Persistent:  volume.Persistent,
		MountPath:   volume.MountPath,
		Size:        volume.Size,
		Description: volume.Description,
	}
}

func toProtoVolumeAttachment(attachment store.VolumeAttachment) *teamsv1.VolumeAttachment {
	protoAttachment := &teamsv1.VolumeAttachment{
		Meta:     toProtoEntityMeta(attachment.Meta),
		VolumeId: attachment.VolumeID.String(),
	}
	if attachment.AgentID != nil {
		protoAttachment.Target = &teamsv1.VolumeAttachment_AgentId{AgentId: attachment.AgentID.String()}
		return protoAttachment
	}
	if attachment.McpID != nil {
		protoAttachment.Target = &teamsv1.VolumeAttachment_McpId{McpId: attachment.McpID.String()}
		return protoAttachment
	}
	if attachment.HookID != nil {
		protoAttachment.Target = &teamsv1.VolumeAttachment_HookId{HookId: attachment.HookID.String()}
		return protoAttachment
	}
	panic("volume attachment missing target")
}

func toProtoMcp(mcp store.Mcp) *teamsv1.Mcp {
	return &teamsv1.Mcp{
		Meta:        toProtoEntityMeta(mcp.Meta),
		AgentId:     mcp.AgentID.String(),
		Image:       mcp.Image,
		Command:     mcp.Command,
		Resources:   toProtoComputeResources(mcp.Resources),
		Description: mcp.Description,
	}
}

func toProtoSkill(skill store.Skill) *teamsv1.Skill {
	return &teamsv1.Skill{
		Meta:        toProtoEntityMeta(skill.Meta),
		AgentId:     skill.AgentID.String(),
		Name:        skill.Name,
		Body:        skill.Body,
		Description: skill.Description,
	}
}

func toProtoHook(hook store.Hook) *teamsv1.Hook {
	return &teamsv1.Hook{
		Meta:        toProtoEntityMeta(hook.Meta),
		AgentId:     hook.AgentID.String(),
		Event:       hook.Event,
		Function:    hook.Function,
		Image:       hook.Image,
		Resources:   toProtoComputeResources(hook.Resources),
		Description: hook.Description,
	}
}

func toProtoEnv(env store.Env) *teamsv1.Env {
	protoEnv := &teamsv1.Env{
		Meta:        toProtoEntityMeta(env.Meta),
		Name:        env.Name,
		Description: env.Description,
	}
	if env.AgentID != nil {
		protoEnv.Target = &teamsv1.Env_AgentId{AgentId: env.AgentID.String()}
	} else if env.McpID != nil {
		protoEnv.Target = &teamsv1.Env_McpId{McpId: env.McpID.String()}
	} else if env.HookID != nil {
		protoEnv.Target = &teamsv1.Env_HookId{HookId: env.HookID.String()}
	} else {
		panic("env missing target")
	}

	if env.Value != nil {
		protoEnv.Source = &teamsv1.Env_Value{Value: *env.Value}
		return protoEnv
	}
	if env.SecretID != nil {
		protoEnv.Source = &teamsv1.Env_SecretId{SecretId: env.SecretID.String()}
		return protoEnv
	}
	panic("env missing source")
}

func toProtoInitScript(script store.InitScript) *teamsv1.InitScript {
	protoScript := &teamsv1.InitScript{
		Meta:        toProtoEntityMeta(script.Meta),
		Script:      script.Script,
		Description: script.Description,
	}
	if script.AgentID != nil {
		protoScript.Target = &teamsv1.InitScript_AgentId{AgentId: script.AgentID.String()}
		return protoScript
	}
	if script.McpID != nil {
		protoScript.Target = &teamsv1.InitScript_McpId{McpId: script.McpID.String()}
		return protoScript
	}
	if script.HookID != nil {
		protoScript.Target = &teamsv1.InitScript_HookId{HookId: script.HookID.String()}
		return protoScript
	}
	panic("init script missing target")
}
