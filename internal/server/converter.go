package server

import (
	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/agynio/agents/internal/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoEntityMeta(meta store.EntityMeta) *agentsv1.EntityMeta {
	return &agentsv1.EntityMeta{
		Id:        meta.ID.String(),
		CreatedAt: timestamppb.New(meta.CreatedAt),
		UpdatedAt: timestamppb.New(meta.UpdatedAt),
	}
}

func toProtoComputeResources(resources store.ComputeResources) *agentsv1.ComputeResources {
	return &agentsv1.ComputeResources{
		RequestsCpu:    resources.RequestsCPU,
		RequestsMemory: resources.RequestsMemory,
		LimitsCpu:      resources.LimitsCPU,
		LimitsMemory:   resources.LimitsMemory,
	}
}

func toProtoAgent(agent store.Agent) *agentsv1.Agent {
	// TODO: Populate OrganizationId once included in Agent response proto.
	return &agentsv1.Agent{
		Meta:          toProtoEntityMeta(agent.Meta),
		Name:          agent.Name,
		Role:          agent.Role,
		Model:         agent.Model.String(),
		Description:   agent.Description,
		Configuration: agent.Configuration,
		Image:         agent.Image,
		InitImage:     agent.InitImage,
		Resources:     toProtoComputeResources(agent.Resources),
	}
}

func toProtoVolume(volume store.Volume) *agentsv1.Volume {
	// TODO: Populate OrganizationId once included in Volume response proto.
	return &agentsv1.Volume{
		Meta:        toProtoEntityMeta(volume.Meta),
		Persistent:  volume.Persistent,
		MountPath:   volume.MountPath,
		Size:        volume.Size,
		Description: volume.Description,
	}
}

func toProtoVolumeAttachment(attachment store.VolumeAttachment) *agentsv1.VolumeAttachment {
	protoAttachment := &agentsv1.VolumeAttachment{
		Meta:     toProtoEntityMeta(attachment.Meta),
		VolumeId: attachment.VolumeID.String(),
	}
	if attachment.AgentID != nil {
		protoAttachment.Target = &agentsv1.VolumeAttachment_AgentId{AgentId: attachment.AgentID.String()}
		return protoAttachment
	}
	if attachment.McpID != nil {
		protoAttachment.Target = &agentsv1.VolumeAttachment_McpId{McpId: attachment.McpID.String()}
		return protoAttachment
	}
	if attachment.HookID != nil {
		protoAttachment.Target = &agentsv1.VolumeAttachment_HookId{HookId: attachment.HookID.String()}
		return protoAttachment
	}
	panic("volume attachment missing target")
}

func toProtoMcp(mcp store.Mcp) *agentsv1.Mcp {
	return &agentsv1.Mcp{
		Meta:        toProtoEntityMeta(mcp.Meta),
		AgentId:     mcp.AgentID.String(),
		Image:       mcp.Image,
		Command:     mcp.Command,
		Resources:   toProtoComputeResources(mcp.Resources),
		Description: mcp.Description,
	}
}

func toProtoSkill(skill store.Skill) *agentsv1.Skill {
	return &agentsv1.Skill{
		Meta:        toProtoEntityMeta(skill.Meta),
		AgentId:     skill.AgentID.String(),
		Name:        skill.Name,
		Body:        skill.Body,
		Description: skill.Description,
	}
}

func toProtoHook(hook store.Hook) *agentsv1.Hook {
	return &agentsv1.Hook{
		Meta:        toProtoEntityMeta(hook.Meta),
		AgentId:     hook.AgentID.String(),
		Event:       hook.Event,
		Function:    hook.Function,
		Image:       hook.Image,
		Resources:   toProtoComputeResources(hook.Resources),
		Description: hook.Description,
	}
}

func toProtoEnv(env store.Env) *agentsv1.Env {
	protoEnv := &agentsv1.Env{
		Meta:        toProtoEntityMeta(env.Meta),
		Name:        env.Name,
		Description: env.Description,
	}
	if env.AgentID != nil {
		protoEnv.Target = &agentsv1.Env_AgentId{AgentId: env.AgentID.String()}
	} else if env.McpID != nil {
		protoEnv.Target = &agentsv1.Env_McpId{McpId: env.McpID.String()}
	} else if env.HookID != nil {
		protoEnv.Target = &agentsv1.Env_HookId{HookId: env.HookID.String()}
	} else {
		panic("env missing target")
	}

	if env.Value != nil {
		protoEnv.Source = &agentsv1.Env_Value{Value: *env.Value}
		return protoEnv
	}
	if env.SecretID != nil {
		protoEnv.Source = &agentsv1.Env_SecretId{SecretId: env.SecretID.String()}
		return protoEnv
	}
	panic("env missing source")
}

func toProtoInitScript(script store.InitScript) *agentsv1.InitScript {
	protoScript := &agentsv1.InitScript{
		Meta:        toProtoEntityMeta(script.Meta),
		Script:      script.Script,
		Description: script.Description,
	}
	if script.AgentID != nil {
		protoScript.Target = &agentsv1.InitScript_AgentId{AgentId: script.AgentID.String()}
		return protoScript
	}
	if script.McpID != nil {
		protoScript.Target = &agentsv1.InitScript_McpId{McpId: script.McpID.String()}
		return protoScript
	}
	if script.HookID != nil {
		protoScript.Target = &agentsv1.InitScript_HookId{HookId: script.HookID.String()}
		return protoScript
	}
	panic("init script missing target")
}
