package server

import (
	"fmt"

	teamsv1 "github.com/agynio/teams/gen/go/agynio/api/teams/v1"
	"github.com/agynio/teams/internal/store"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	marshalOptions   = protojson.MarshalOptions{UseProtoNames: true}
	unmarshalOptions = protojson.UnmarshalOptions{}
)

func marshalConfig(msg proto.Message) (store.JSONData, error) {
	if msg == nil {
		return nil, fmt.Errorf("config is required")
	}
	data, err := marshalOptions.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return store.JSONData(data), nil
}

func unmarshalConfig(data store.JSONData, msg proto.Message) error {
	if len(data) == 0 {
		return fmt.Errorf("config is empty")
	}
	return unmarshalOptions.Unmarshal(data, msg)
}

func toolTypeName(toolType teamsv1.ToolType) (string, error) {
	if toolType == teamsv1.ToolType_TOOL_TYPE_UNSPECIFIED {
		return "", fmt.Errorf("tool_type must be specified")
	}
	name, ok := teamsv1.ToolType_name[int32(toolType)]
	if !ok {
		return "", fmt.Errorf("unknown tool_type %d", toolType)
	}
	return name, nil
}

func entityTypeName(entityType teamsv1.EntityType) (string, error) {
	if entityType == teamsv1.EntityType_ENTITY_TYPE_UNSPECIFIED {
		return "", fmt.Errorf("entity_type must be specified")
	}
	name, ok := teamsv1.EntityType_name[int32(entityType)]
	if !ok {
		return "", fmt.Errorf("unknown entity_type %d", entityType)
	}
	return name, nil
}

func attachmentKindName(kind teamsv1.AttachmentKind) (string, error) {
	if kind == teamsv1.AttachmentKind_ATTACHMENT_KIND_UNSPECIFIED {
		return "", fmt.Errorf("attachment kind must be specified")
	}
	name, ok := teamsv1.AttachmentKind_name[int32(kind)]
	if !ok {
		return "", fmt.Errorf("unknown attachment kind %d", kind)
	}
	return name, nil
}

func parseToolType(value string) (teamsv1.ToolType, error) {
	if value == "" {
		return teamsv1.ToolType_TOOL_TYPE_UNSPECIFIED, fmt.Errorf("tool type is empty")
	}
	raw, ok := teamsv1.ToolType_value[value]
	if !ok {
		return teamsv1.ToolType_TOOL_TYPE_UNSPECIFIED, fmt.Errorf("unknown tool type %q", value)
	}
	return teamsv1.ToolType(raw), nil
}

func parseEntityType(value string) (teamsv1.EntityType, error) {
	if value == "" {
		return teamsv1.EntityType_ENTITY_TYPE_UNSPECIFIED, fmt.Errorf("entity type is empty")
	}
	raw, ok := teamsv1.EntityType_value[value]
	if !ok {
		return teamsv1.EntityType_ENTITY_TYPE_UNSPECIFIED, fmt.Errorf("unknown entity type %q", value)
	}
	return teamsv1.EntityType(raw), nil
}

func parseAttachmentKind(value string) (teamsv1.AttachmentKind, error) {
	if value == "" {
		return teamsv1.AttachmentKind_ATTACHMENT_KIND_UNSPECIFIED, fmt.Errorf("attachment kind is empty")
	}
	raw, ok := teamsv1.AttachmentKind_value[value]
	if !ok {
		return teamsv1.AttachmentKind_ATTACHMENT_KIND_UNSPECIFIED, fmt.Errorf("unknown attachment kind %q", value)
	}
	return teamsv1.AttachmentKind(raw), nil
}

func attachmentRelation(kind teamsv1.AttachmentKind) (teamsv1.EntityType, teamsv1.EntityType, error) {
	switch kind {
	case teamsv1.AttachmentKind_ATTACHMENT_KIND_AGENT_TOOL:
		return teamsv1.EntityType_ENTITY_TYPE_AGENT, teamsv1.EntityType_ENTITY_TYPE_TOOL, nil
	case teamsv1.AttachmentKind_ATTACHMENT_KIND_AGENT_MEMORY_BUCKET:
		return teamsv1.EntityType_ENTITY_TYPE_AGENT, teamsv1.EntityType_ENTITY_TYPE_MEMORY_BUCKET, nil
	case teamsv1.AttachmentKind_ATTACHMENT_KIND_AGENT_WORKSPACE_CONFIGURATION:
		return teamsv1.EntityType_ENTITY_TYPE_AGENT, teamsv1.EntityType_ENTITY_TYPE_WORKSPACE_CONFIGURATION, nil
	case teamsv1.AttachmentKind_ATTACHMENT_KIND_AGENT_MCP_SERVER:
		return teamsv1.EntityType_ENTITY_TYPE_AGENT, teamsv1.EntityType_ENTITY_TYPE_MCP_SERVER, nil
	case teamsv1.AttachmentKind_ATTACHMENT_KIND_MCP_SERVER_WORKSPACE_CONFIGURATION:
		return teamsv1.EntityType_ENTITY_TYPE_MCP_SERVER, teamsv1.EntityType_ENTITY_TYPE_WORKSPACE_CONFIGURATION, nil
	default:
		panic(fmt.Sprintf("unhandled attachment kind: %d", kind))
	}
}

func toProtoEntityMeta(meta store.EntityMeta) *teamsv1.EntityMeta {
	return &teamsv1.EntityMeta{
		Id:        meta.ID.String(),
		CreatedAt: timestamppb.New(meta.CreatedAt),
		UpdatedAt: timestamppb.New(meta.UpdatedAt),
	}
}

func toProtoAgent(agent store.Agent) (*teamsv1.Agent, error) {
	config := &teamsv1.AgentConfig{}
	if err := unmarshalConfig(agent.Config, config); err != nil {
		return nil, fmt.Errorf("agent config: %w", err)
	}
	return &teamsv1.Agent{
		Meta:        toProtoEntityMeta(agent.Meta),
		Title:       agent.Title,
		Description: agent.Description,
		Config:      config,
	}, nil
}

func toProtoTool(tool store.Tool) (*teamsv1.Tool, error) {
	config := &structpb.Struct{}
	if err := unmarshalConfig(tool.Config, config); err != nil {
		return nil, fmt.Errorf("tool config: %w", err)
	}
	toolType, err := parseToolType(tool.Type)
	if err != nil {
		return nil, err
	}
	return &teamsv1.Tool{
		Meta:        toProtoEntityMeta(tool.Meta),
		Type:        toolType,
		Name:        tool.Name,
		Description: tool.Description,
		Config:      config,
	}, nil
}

func toProtoMcpServer(server store.McpServer) (*teamsv1.McpServer, error) {
	config := &teamsv1.McpServerConfig{}
	if err := unmarshalConfig(server.Config, config); err != nil {
		return nil, fmt.Errorf("mcp server config: %w", err)
	}
	return &teamsv1.McpServer{
		Meta:        toProtoEntityMeta(server.Meta),
		Title:       server.Title,
		Description: server.Description,
		Config:      config,
	}, nil
}

func toProtoWorkspaceConfiguration(workspace store.WorkspaceConfiguration) (*teamsv1.WorkspaceConfiguration, error) {
	config := &teamsv1.WorkspaceConfig{}
	if err := unmarshalConfig(workspace.Config, config); err != nil {
		return nil, fmt.Errorf("workspace configuration config: %w", err)
	}
	return &teamsv1.WorkspaceConfiguration{
		Meta:        toProtoEntityMeta(workspace.Meta),
		Title:       workspace.Title,
		Description: workspace.Description,
		Config:      config,
	}, nil
}

func toProtoMemoryBucket(bucket store.MemoryBucket) (*teamsv1.MemoryBucket, error) {
	config := &teamsv1.MemoryBucketConfig{}
	if err := unmarshalConfig(bucket.Config, config); err != nil {
		return nil, fmt.Errorf("memory bucket config: %w", err)
	}
	return &teamsv1.MemoryBucket{
		Meta:        toProtoEntityMeta(bucket.Meta),
		Title:       bucket.Title,
		Description: bucket.Description,
		Config:      config,
	}, nil
}

func toProtoVariable(variable store.Variable) (*teamsv1.Variable, error) {
	return &teamsv1.Variable{
		Meta:        toProtoEntityMeta(variable.Meta),
		Key:         variable.Key,
		Value:       variable.Value,
		Description: variable.Description,
	}, nil
}

func toProtoAttachment(attachment store.Attachment) (*teamsv1.Attachment, error) {
	kind, err := parseAttachmentKind(attachment.Kind)
	if err != nil {
		return nil, err
	}
	sourceType, err := parseEntityType(attachment.SourceType)
	if err != nil {
		return nil, err
	}
	targetType, err := parseEntityType(attachment.TargetType)
	if err != nil {
		return nil, err
	}
	return &teamsv1.Attachment{
		Meta:       toProtoEntityMeta(attachment.Meta),
		Kind:       kind,
		SourceType: sourceType,
		SourceId:   attachment.SourceID.String(),
		TargetType: targetType,
		TargetId:   attachment.TargetID.String(),
	}, nil
}
