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
	Nickname       string
	Role           string
	Model          uuid.UUID
	Description    string
	Configuration  string
	Image          string
	InitImage      string
	IdleTimeout    *string
	Capabilities   []string
	Availability   AgentAvailability
	Resources      ComputeResources
}

type AgentAvailability string

const (
	AgentAvailabilityInternal AgentAvailability = "internal"
	AgentAvailabilityPrivate  AgentAvailability = "private"
)

type AgentRole string

const (
	AgentRoleOwner       AgentRole = "owner"
	AgentRoleMaintainer  AgentRole = "maintainer"
	AgentRoleParticipant AgentRole = "participant"
)

type SandboxStatus string

const (
	SandboxStatusStarting   SandboxStatus = "starting"
	SandboxStatusRunning    SandboxStatus = "running"
	SandboxStatusStopped    SandboxStatus = "stopped"
	SandboxStatusFailed     SandboxStatus = "failed"
	SandboxStatusTerminated SandboxStatus = "terminated"
)

type AgentRoleAssignment struct {
	AgentID    uuid.UUID
	IdentityID uuid.UUID
	Role       AgentRole
}

type AgentInstanceState string

const (
	AgentInstanceStateActive     AgentInstanceState = "active"
	AgentInstanceStatePaused     AgentInstanceState = "paused"
	AgentInstanceStateTerminated AgentInstanceState = "terminated"
)

type InboxItemSourceKind string

const (
	InboxItemSourceKindThread InboxItemSourceKind = "thread"
	InboxItemSourceKindDirect InboxItemSourceKind = "direct"
)

type AgentInstance struct {
	Meta           EntityMeta
	AgentID        uuid.UUID
	OrganizationID uuid.UUID
	Label          *string
	Suffix         string
	State          AgentInstanceState
	PauseReason    *string
	LastActivityAt time.Time
	Nickname       string
}

type AgentInstanceInput struct {
	AgentID        uuid.UUID
	OrganizationID uuid.UUID
	Label          *string
	Suffix         string
	Nickname       string
}

type AgentInstanceFilter struct {
	OrganizationID *uuid.UUID
	AgentID        *uuid.UUID
	StateIn        []AgentInstanceState
	HasUnacked     *bool
}

type AgentInstanceListResult struct {
	Instances  []AgentInstance
	NextCursor *PageCursor
}

type InboxItem struct {
	ID              uuid.UUID
	AgentInstanceID uuid.UUID
	SourceKind      InboxItemSourceKind
	ThreadID        *uuid.UUID
	MessageID       *uuid.UUID
	SenderID        uuid.UUID
	Body            string
	FileIDs         []uuid.UUID
	AcceptedAt      time.Time
	AckedAt         *time.Time
}

type InboxItemInput struct {
	AgentInstanceID uuid.UUID
	SourceKind      InboxItemSourceKind
	ThreadID        *uuid.UUID
	MessageID       *uuid.UUID
	SenderID        uuid.UUID
	Body            string
	FileIDs         []uuid.UUID
}

type InboxItemListResult struct {
	Items      []InboxItem
	NextCursor *InboxPageCursor
}

type Volume struct {
	Meta           EntityMeta
	OrganizationID uuid.UUID
	Persistent     bool
	MountPath      string
	Size           string
	Description    string
	TTL            *string
}

type VolumeAttachment struct {
	Meta     EntityMeta
	VolumeID uuid.UUID
	AgentID  *uuid.UUID
	McpID    *uuid.UUID
	HookID   *uuid.UUID
}

type ImagePullSecretAttachment struct {
	Meta              EntityMeta
	ImagePullSecretID uuid.UUID
	AgentID           *uuid.UUID
	McpID             *uuid.UUID
	HookID            *uuid.UUID
}

type Mcp struct {
	Meta        EntityMeta
	AgentID     uuid.UUID
	Name        string
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

type Environment struct {
	Meta           EntityMeta
	OrganizationID uuid.UUID
	Name           string
	FlavorID       uuid.UUID
	Image          string
	FlavorName     string
}

type Sandbox struct {
	Meta            EntityMeta
	OrganizationID  uuid.UUID
	Name            string
	EnvironmentID   uuid.UUID
	OwnerID         uuid.UUID
	Status          SandboxStatus
	IdleTimeout     string
	TTL             string
	LastSessionAt   *time.Time
	EnvironmentName string
	WorkloadID      *uuid.UUID
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
	Nickname      string
	Role          string
	Model         uuid.UUID
	Description   string
	Configuration string
	Image         string
	InitImage     string
	IdleTimeout   *string
	Capabilities  []string
	Availability  AgentAvailability
	Resources     ComputeResources
}

type AgentUpdate struct {
	Name          *string
	Nickname      *string
	Role          *string
	Model         *uuid.UUID
	Description   *string
	Configuration *string
	Image         *string
	InitImage     *string
	IdleTimeout   *string
	Capabilities  *[]string
	Availability  *AgentAvailability
	Resources     *ComputeResources
}

type VolumeInput struct {
	Persistent  bool
	MountPath   string
	Size        string
	Description string
	TTL         *string
}

type VolumeUpdate struct {
	Persistent  *bool
	MountPath   *string
	Size        *string
	Description *string
	TTL         *string
}

type VolumeAttachmentInput struct {
	VolumeID uuid.UUID
	AgentID  *uuid.UUID
	McpID    *uuid.UUID
	HookID   *uuid.UUID
}

type ImagePullSecretAttachmentInput struct {
	ImagePullSecretID uuid.UUID
	AgentID           *uuid.UUID
	McpID             *uuid.UUID
	HookID            *uuid.UUID
}

type McpInput struct {
	AgentID     uuid.UUID
	Name        string
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

type EnvironmentInput struct {
	Name     string
	FlavorID uuid.UUID
	Image    string
}

type EnvironmentUpdate struct {
	Name     *string
	FlavorID *uuid.UUID
	Image    *string
}

type SandboxInput struct {
	Name          string
	EnvironmentID uuid.UUID
	OwnerID       uuid.UUID
	Status        SandboxStatus
	IdleTimeout   string
	TTL           string
}

type SandboxUpdate struct {
	Status        *SandboxStatus
	LastSessionAt *time.Time
	WorkloadID    *uuid.UUID
}

type AgentFilter struct{}

type VolumeFilter struct{}

type VolumeAttachmentFilter struct {
	VolumeID *uuid.UUID
	AgentID  *uuid.UUID
	McpID    *uuid.UUID
	HookID   *uuid.UUID
}

type ImagePullSecretAttachmentFilter struct {
	ImagePullSecretID *uuid.UUID
	AgentID           *uuid.UUID
	McpID             *uuid.UUID
	HookID            *uuid.UUID
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

type EnvironmentFilter struct{}

type SandboxFilter struct {
	OwnerID           *uuid.UUID
	IncludeTerminated bool
}

type PageCursor struct {
	AfterID uuid.UUID
}

type InboxPageCursor struct {
	AfterAcceptedAt time.Time
	AfterID         uuid.UUID
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

type ImagePullSecretAttachmentListResult struct {
	ImagePullSecretAttachments []ImagePullSecretAttachment
	NextCursor                 *PageCursor
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

type EnvironmentListResult struct {
	Environments []Environment
	NextCursor   *PageCursor
}

type SandboxListResult struct {
	Sandboxes  []Sandbox
	NextCursor *PageCursor
}
