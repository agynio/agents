package store

import (
	"time"

	"github.com/google/uuid"
)

type JSONData []byte

type EntityMeta struct {
	ID        uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Agent struct {
	Meta        EntityMeta
	Title       string
	Description string
	Config      JSONData
}

type Tool struct {
	Meta        EntityMeta
	Type        string
	Name        string
	Description string
	Config      JSONData
}

type McpServer struct {
	Meta        EntityMeta
	Title       string
	Description string
	Config      JSONData
}

type WorkspaceConfiguration struct {
	Meta        EntityMeta
	Title       string
	Description string
	Config      JSONData
}

type MemoryBucket struct {
	Meta        EntityMeta
	Title       string
	Description string
	Config      JSONData
}

type Variable struct {
	Meta        EntityMeta
	Key         string
	Value       string
	Description string
}

type Attachment struct {
	Meta       EntityMeta
	Kind       string
	SourceType string
	SourceID   uuid.UUID
	TargetType string
	TargetID   uuid.UUID
}

type AgentInput struct {
	Title       string
	Description string
	Config      JSONData
}

type AgentUpdate struct {
	Title       *string
	Description *string
	Config      *JSONData
}

type ToolInput struct {
	Type        string
	Name        string
	Description string
	Config      JSONData
}

type ToolUpdate struct {
	Name        *string
	Description *string
	Config      *JSONData
}

type McpServerInput struct {
	Title       string
	Description string
	Config      JSONData
}

type McpServerUpdate struct {
	Title       *string
	Description *string
	Config      *JSONData
}

type WorkspaceConfigurationInput struct {
	Title       string
	Description string
	Config      JSONData
}

type WorkspaceConfigurationUpdate struct {
	Title       *string
	Description *string
	Config      *JSONData
}

type MemoryBucketInput struct {
	Title       string
	Description string
	Config      JSONData
}

type MemoryBucketUpdate struct {
	Title       *string
	Description *string
	Config      *JSONData
}

type VariableInput struct {
	Key         string
	Value       string
	Description string
}

type VariableUpdate struct {
	Key         *string
	Value       *string
	Description *string
}

type AttachmentInput struct {
	Kind       string
	SourceType string
	SourceID   uuid.UUID
	TargetType string
	TargetID   uuid.UUID
}

type AgentFilter struct {
	Query string
}

type ToolFilter struct {
	Type *string
}

type VariableFilter struct {
	Query string
}

type AttachmentFilter struct {
	Kind       *string
	SourceType *string
	SourceID   *uuid.UUID
	TargetType *string
	TargetID   *uuid.UUID
}

type PageCursor struct {
	AfterID uuid.UUID
}

type AgentListResult struct {
	Agents     []Agent
	NextCursor *PageCursor
}

type ToolListResult struct {
	Tools      []Tool
	NextCursor *PageCursor
}

type McpServerListResult struct {
	McpServers []McpServer
	NextCursor *PageCursor
}

type WorkspaceConfigurationListResult struct {
	WorkspaceConfigurations []WorkspaceConfiguration
	NextCursor              *PageCursor
}

type MemoryBucketListResult struct {
	MemoryBuckets []MemoryBucket
	NextCursor    *PageCursor
}

type VariableListResult struct {
	Variables  []Variable
	NextCursor *PageCursor
}

type AttachmentListResult struct {
	Attachments []Attachment
	NextCursor  *PageCursor
}
