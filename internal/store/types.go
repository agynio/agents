package store

import (
	"time"

	"github.com/google/uuid"
)

type EntityMeta struct {
	ID        uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ComputeResources struct {
	RequestsCPU    string
	RequestsMemory string
	LimitsCPU      string
	LimitsMemory   string
}

type Agent struct {
	Meta           EntityMeta
	OrganizationID uuid.UUID
	Name           string
	Role           string
	Model          uuid.UUID
	Description    string
	Configuration  string
	Image          string
	InitImage      string
	Resources      ComputeResources
}

type Volume struct {
	Meta           EntityMeta
	OrganizationID uuid.UUID
	Persistent     bool
	MountPath      string
	Size           string
	Description    string
}

type VolumeAttachment struct {
	Meta     EntityMeta
	VolumeID uuid.UUID
	AgentID  *uuid.UUID
	McpID    *uuid.UUID
	HookID   *uuid.UUID
}

type Mcp struct {
	Meta        EntityMeta
	AgentID     uuid.UUID
	Image       string
	Command     string
	Resources   ComputeResources
	Description string
}

type Skill struct {
	Meta        EntityMeta
	AgentID     uuid.UUID
	Name        string
	Body        string
	Description string
}

type Hook struct {
	Meta        EntityMeta
	AgentID     uuid.UUID
	Event       string
	Function    string
	Image       string
	Resources   ComputeResources
	Description string
}

type Env struct {
	Meta        EntityMeta
	Name        string
	Description string
	AgentID     *uuid.UUID
	McpID       *uuid.UUID
	HookID      *uuid.UUID
	Value       *string
	SecretID    *uuid.UUID
}

type InitScript struct {
	Meta        EntityMeta
	Script      string
	Description string
	AgentID     *uuid.UUID
	McpID       *uuid.UUID
	HookID      *uuid.UUID
}

type AgentInput struct {
	Name          string
	Role          string
	Model         uuid.UUID
	Description   string
	Configuration string
	Image         string
	InitImage     string
	Resources     ComputeResources
}

type AgentUpdate struct {
	Name          *string
	Role          *string
	Model         *uuid.UUID
	Description   *string
	Configuration *string
	Image         *string
	InitImage     *string
	Resources     *ComputeResources
}

type VolumeInput struct {
	Persistent  bool
	MountPath   string
	Size        string
	Description string
}

type VolumeUpdate struct {
	Persistent  *bool
	MountPath   *string
	Size        *string
	Description *string
}

type VolumeAttachmentInput struct {
	VolumeID uuid.UUID
	AgentID  *uuid.UUID
	McpID    *uuid.UUID
	HookID   *uuid.UUID
}

type McpInput struct {
	AgentID     uuid.UUID
	Image       string
	Command     string
	Resources   ComputeResources
	Description string
}

type McpUpdate struct {
	Image       *string
	Command     *string
	Resources   *ComputeResources
	Description *string
}

type SkillInput struct {
	AgentID     uuid.UUID
	Name        string
	Body        string
	Description string
}

type SkillUpdate struct {
	Name        *string
	Body        *string
	Description *string
}

type HookInput struct {
	AgentID     uuid.UUID
	Event       string
	Function    string
	Image       string
	Resources   ComputeResources
	Description string
}

type HookUpdate struct {
	Event       *string
	Function    *string
	Image       *string
	Resources   *ComputeResources
	Description *string
}

type EnvInput struct {
	Name        string
	Description string
	AgentID     *uuid.UUID
	McpID       *uuid.UUID
	HookID      *uuid.UUID
	Value       *string
	SecretID    *uuid.UUID
}

type EnvUpdate struct {
	Name        *string
	Description *string
	Value       *string
	SecretID    *uuid.UUID
}

type InitScriptInput struct {
	Script      string
	Description string
	AgentID     *uuid.UUID
	McpID       *uuid.UUID
	HookID      *uuid.UUID
}

type InitScriptUpdate struct {
	Script      *string
	Description *string
}

type AgentFilter struct{}

type VolumeFilter struct{}

type VolumeAttachmentFilter struct {
	VolumeID *uuid.UUID
	AgentID  *uuid.UUID
	McpID    *uuid.UUID
	HookID   *uuid.UUID
}

type McpFilter struct {
	AgentID *uuid.UUID
}

type SkillFilter struct {
	AgentID *uuid.UUID
}

type HookFilter struct {
	AgentID *uuid.UUID
}

type EnvFilter struct {
	AgentID *uuid.UUID
	McpID   *uuid.UUID
	HookID  *uuid.UUID
}

type InitScriptFilter struct {
	AgentID *uuid.UUID
	McpID   *uuid.UUID
	HookID  *uuid.UUID
}

type PageCursor struct {
	AfterID uuid.UUID
}

type AgentListResult struct {
	Agents     []Agent
	NextCursor *PageCursor
}

type VolumeListResult struct {
	Volumes    []Volume
	NextCursor *PageCursor
}

type VolumeAttachmentListResult struct {
	VolumeAttachments []VolumeAttachment
	NextCursor        *PageCursor
}

type McpListResult struct {
	Mcps       []Mcp
	NextCursor *PageCursor
}

type SkillListResult struct {
	Skills     []Skill
	NextCursor *PageCursor
}

type HookListResult struct {
	Hooks      []Hook
	NextCursor *PageCursor
}

type EnvListResult struct {
	Envs       []Env
	NextCursor *PageCursor
}

type InitScriptListResult struct {
	InitScripts []InitScript
	NextCursor  *PageCursor
}
