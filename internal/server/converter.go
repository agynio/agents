package server

import (
	"github.com/google/uuid"

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
	protoAgent := &agentsv1.Agent{
		Meta:           toProtoEntityMeta(agent.Meta),
		OrganizationId: agent.OrganizationID.String(),
		Name:           agent.Name,
		Nickname:       agent.Nickname,
		Role:           agent.Role,
		Model:          agent.Model.String(),
		Description:    agent.Description,
		Configuration:  agent.Configuration,
		Image:          agent.Image,
		InitImage:      agent.InitImage,
		Capabilities:   append([]string(nil), agent.Capabilities...),
		Availability:   agentAvailabilityToProto(agent.Availability),
		Resources:      toProtoComputeResources(agent.Resources),
	}
	if agent.IdleTimeout != nil {
		protoAgent.IdleTimeout = agent.IdleTimeout
	}
	return protoAgent
}

func toProtoAgentRoleAssignment(assignment store.AgentRoleAssignment) *agentsv1.AgentRoleAssignment {
	return &agentsv1.AgentRoleAssignment{
		AgentId:    assignment.AgentID.String(),
		IdentityId: assignment.IdentityID.String(),
		Role:       agentRoleToProto(assignment.Role),
	}
}

func toProtoVolume(volume store.Volume) *agentsv1.Volume {
	// TODO: Populate OrganizationId once included in Volume response proto.
	protoVolume := &agentsv1.Volume{
		Meta:        toProtoEntityMeta(volume.Meta),
		Persistent:  volume.Persistent,
		MountPath:   volume.MountPath,
		Size:        volume.Size,
		Description: volume.Description,
	}
	if volume.TTL != nil {
		protoVolume.Ttl = volume.TTL
	}
	return protoVolume
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

func toProtoImagePullSecretAttachment(attachment store.ImagePullSecretAttachment) *agentsv1.ImagePullSecretAttachment {
	protoAttachment := &agentsv1.ImagePullSecretAttachment{
		Meta:              toProtoEntityMeta(attachment.Meta),
		ImagePullSecretId: attachment.ImagePullSecretID.String(),
	}
	if attachment.AgentID != nil {
		protoAttachment.Target = &agentsv1.ImagePullSecretAttachment_AgentId{AgentId: attachment.AgentID.String()}
		return protoAttachment
	}
	if attachment.McpID != nil {
		protoAttachment.Target = &agentsv1.ImagePullSecretAttachment_McpId{McpId: attachment.McpID.String()}
		return protoAttachment
	}
	if attachment.HookID != nil {
		protoAttachment.Target = &agentsv1.ImagePullSecretAttachment_HookId{HookId: attachment.HookID.String()}
		return protoAttachment
	}
	panic("image pull secret attachment missing target")
}

func toProtoEnvironment(environment store.Environment) *agentsv1.Environment {
	return &agentsv1.Environment{
		Meta:           toProtoEntityMeta(environment.Meta),
		OrganizationId: environment.OrganizationID.String(),
		Name:           environment.Name,
		FlavorId:       environment.FlavorID.String(),
		Image:          environment.Image,
		FlavorName:     environment.FlavorName,
	}
}

func toProtoSandbox(sandbox store.Sandbox) *agentsv1.Sandbox {
	protoSandbox := &agentsv1.Sandbox{
		Meta:            toProtoEntityMeta(sandbox.Meta),
		OrganizationId:  sandbox.OrganizationID.String(),
		Name:            sandbox.Name,
		EnvironmentId:   sandbox.EnvironmentID.String(),
		OwnerId:         sandbox.OwnerID.String(),
		Status:          sandboxStatusToProto(sandbox.Status),
		IdleTimeout:     sandbox.IdleTimeout,
		Ttl:             sandbox.TTL,
		EnvironmentName: sandbox.EnvironmentName,
	}
	if sandbox.LastSessionAt != nil {
		protoSandbox.LastSessionAt = timestamppb.New(*sandbox.LastSessionAt)
	}
	if sandbox.WorkloadID != nil {
		workloadID := sandbox.WorkloadID.String()
		protoSandbox.WorkloadId = &workloadID
	}
	return protoSandbox
}

func toProtoMcp(mcp store.Mcp) *agentsv1.Mcp {
	return &agentsv1.Mcp{
		Meta:        toProtoEntityMeta(mcp.Meta),
		AgentId:     mcp.AgentID.String(),
		Name:        mcp.Name,
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

func toProtoAgentInstance(instance store.AgentInstance) *agentsv1.AgentInstance {
	protoInstance := &agentsv1.AgentInstance{
		Meta:           toProtoEntityMeta(instance.Meta),
		AgentId:        instance.AgentID.String(),
		OrganizationId: instance.OrganizationID.String(),
		Suffix:         instance.Suffix,
		State:          agentInstanceStateToProto(instance.State),
		LastActivityAt: timestamppb.New(instance.LastActivityAt),
		Nickname:       instance.Nickname,
		Handle:         "@" + instance.Nickname + "#" + instance.Suffix,
	}
	if instance.Label != nil {
		protoInstance.Label = instance.Label
	}
	if instance.PauseReason != nil {
		protoInstance.PauseReason = instance.PauseReason
	}
	return protoInstance
}

func toProtoInboxItem(item store.InboxItem) *agentsv1.InboxItem {
	protoItem := &agentsv1.InboxItem{
		Id:              item.ID.String(),
		AgentInstanceId: item.AgentInstanceID.String(),
		SourceKind:      inboxItemSourceKindToProto(item.SourceKind),
		SenderId:        item.SenderID.String(),
		Body:            item.Body,
		FileIds:         uuidValuesToStrings(item.FileIDs),
		AcceptedAt:      timestamppb.New(item.AcceptedAt),
	}
	if item.ThreadID != nil {
		value := item.ThreadID.String()
		protoItem.ThreadId = &value
	}
	if item.MessageID != nil {
		value := item.MessageID.String()
		protoItem.MessageId = &value
	}
	if item.AckedAt != nil {
		protoItem.AckedAt = timestamppb.New(*item.AckedAt)
	}
	return protoItem
}

func uuidValuesToStrings(ids []uuid.UUID) []string {
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = id.String()
	}
	return values
}
