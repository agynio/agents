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
		DefaultThread:  agentDefaultThreadToProto(agent.DefaultThread),
		FinalMessage:   agentFinalMessageToProto(agent.FinalMessage),
	}
	if agent.InstanceIdleTTL != nil {
		protoAgent.InstanceIdleTtl = agent.InstanceIdleTTL
	}
	if agent.IdleTimeout != nil {
		protoAgent.IdleTimeout = agent.IdleTimeout
	}
	// Empty on agents created before environments existed, which run from the
	// deprecated inline image and resources above.
	if agent.EnvironmentID != nil {
		protoAgent.EnvironmentId = agent.EnvironmentID.String()
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
	protoVolume := &agentsv1.Volume{
		Meta:       toProtoEntityMeta(volume.Meta),
		Name:       volume.Name,
		MountPath:  volume.MountPath,
		Persistent: volume.Persistent,
	}
	if volume.Size != nil {
		protoVolume.Size = *volume.Size
	}
	protoVolume.StorageClass = volume.StorageClass
	protoVolume.Ttl = volume.TTL
	if volume.EnvironmentID != nil {
		protoVolume.Target = &agentsv1.Volume_EnvironmentId{EnvironmentId: volume.EnvironmentID.String()}
	} else if volume.McpID != nil {
		protoVolume.Target = &agentsv1.Volume_McpId{McpId: volume.McpID.String()}
	}
	return protoVolume
}

func toProtoEnvironment(environment store.Environment) *agentsv1.Environment {
	proto := &agentsv1.Environment{
		Meta:           toProtoEntityMeta(environment.Meta),
		OrganizationId: environment.OrganizationID.String(),
		Name:           environment.Name,
		Image:          environment.Image,
		Flavor:         environment.Flavor,
		FlavorName:     environment.FlavorName,
	}
	if environment.RunnerID != nil {
		proto.RunnerId = environment.RunnerID.String()
	}
	// Deprecated, and null for environments created since placement moved to
	// runner plus flavor name.
	if environment.FlavorID != nil {
		proto.FlavorId = environment.FlavorID.String()
	}
	if environment.WorkspaceImageID != nil {
		proto.WorkspaceImageId = environment.WorkspaceImageID.String()
		proto.WorkspaceImageTag = environment.WorkspaceImageTag
	}
	if environment.AgentRuntimeImageID != nil {
		proto.AgentRuntimeImageId = environment.AgentRuntimeImageID.String()
		proto.AgentRuntimeImageTag = environment.AgentRuntimeImageTag
	}
	return proto
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
	protoMcp := &agentsv1.Mcp{
		Meta:          toProtoEntityMeta(mcp.Meta),
		Name:          mcp.Name,
		Image:         mcp.Image,
		Command:       mcp.Command,
		Resources:     toProtoComputeResources(mcp.Resources),
		Description:   mcp.Description,
		ImageTag:      mcp.ImageTag,
		SharedVolumes: mcp.SharedVolumes,
	}
	if mcp.ImageID != nil {
		protoMcp.ImageId = mcp.ImageID.String()
	}
	if mcp.EnvironmentID != nil {
		protoMcp.Target = &agentsv1.Mcp_EnvironmentId{EnvironmentId: mcp.EnvironmentID.String()}
	} else if mcp.AgentID != nil {
		protoMcp.Target = &agentsv1.Mcp_AgentId{AgentId: mcp.AgentID.String()}
	}
	return protoMcp
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
	} else if env.EnvironmentID != nil {
		protoEnv.Target = &agentsv1.Env_EnvironmentId{EnvironmentId: env.EnvironmentID.String()}
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
	if script.EnvironmentID != nil {
		protoScript.Target = &agentsv1.InitScript_EnvironmentId{EnvironmentId: script.EnvironmentID.String()}
		return protoScript
	}
	if script.McpID != nil {
		protoScript.Target = &agentsv1.InitScript_McpId{McpId: script.McpID.String()}
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
	if instance.DefaultThreadID != nil {
		protoInstance.DefaultThreadId = protoString(instance.DefaultThreadID.String())
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
