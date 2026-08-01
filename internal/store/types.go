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
	// The environment supplying this agent's image and compute. Null on agents
	// written before environments existed, which still carry Image and
	// Resources of their own.
	EnvironmentID *uuid.UUID
	// Where an instance's default thread comes from when the platform creates
	// it, and what becomes of a turn's final text. Both describe how the agent
	// is written, so they belong to the class rather than to an instance.
	DefaultThread AgentDefaultThread
	FinalMessage  AgentFinalMessage
}

// AgentDefaultThread governs the automatic creation path only. Naming a thread
// explicitly is always allowed, so none means "infer nothing", not "never".
type AgentDefaultThread string

const (
	// AgentDefaultThreadOrigin takes the thread that added the instance.
	// Delegation creates sub-threads downward, so the origin is the thread the
	// instance owes an answer to.
	AgentDefaultThreadOrigin AgentDefaultThread = "origin"
	AgentDefaultThreadNone   AgentDefaultThread = "none"
)

// AgentFinalMessage decides whether the text an agent CLI produces at the end
// of a turn is a deliverable or an internal artifact.
type AgentFinalMessage string

const (
	AgentFinalMessageDiscard       AgentFinalMessage = "discard"
	AgentFinalMessageDefaultThread AgentFinalMessage = "default_thread"
)

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
	// Where this instance's untargeted messages go. Null when the class asked
	// for no inference and nobody has named one since.
	DefaultThreadID *uuid.UUID
}

type AgentInstanceInput struct {
	AgentID         uuid.UUID
	OrganizationID  uuid.UUID
	Label           *string
	Suffix          string
	Nickname        string
	DefaultThreadID *uuid.UUID
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
	OrganizationID    uuid.UUID
	ImagePullSecretID uuid.UUID
	AgentID           *uuid.UUID
	McpID             *uuid.UUID
	HookID            *uuid.UUID
	EnvironmentID     *uuid.UUID
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
	Meta           EntityMeta
	OrganizationID uuid.UUID
	Name           string
	Description    string
	AgentID        *uuid.UUID
	McpID          *uuid.UUID
	HookID         *uuid.UUID
	EnvironmentID  *uuid.UUID
	Value          *string
	SecretID       *uuid.UUID
}

type Environment struct {
	Meta           EntityMeta
	OrganizationID uuid.UUID
	Name           string
	Image          string
	// RunnerID is where workloads for this environment are placed, and Flavor
	// names an entry in that runner's reported catalog. Flavor is resolved at
	// workload start, not here.
	RunnerID *uuid.UUID
	Flavor   string
	// Superseded by RunnerID and Flavor; retained for callers still reading it.
	FlavorID   *uuid.UUID
	FlavorName string
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
	EnvironmentID *uuid.UUID
	DefaultThread AgentDefaultThread
	FinalMessage  AgentFinalMessage
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
	// EnvironmentID sets the reference and ClearEnvironmentID removes it; an
	// agent may have none, so the two cases are distinct.
	EnvironmentID      *uuid.UUID
	ClearEnvironmentID bool
	DefaultThread      *AgentDefaultThread
	FinalMessage       *AgentFinalMessage
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
	EnvironmentID     *uuid.UUID
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
	Name          string
	Description   string
	AgentID       *uuid.UUID
	McpID         *uuid.UUID
	HookID        *uuid.UUID
	EnvironmentID *uuid.UUID
	Value         *string
	SecretID      *uuid.UUID
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
	Image    string
	RunnerID *uuid.UUID
	Flavor   string
}

type EnvironmentUpdate struct {
	Name     *string
	Image    *string
	RunnerID *uuid.UUID
	Flavor   *string
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
	Status          *SandboxStatus
	LastSessionAt   *time.Time
	WorkloadID      *uuid.UUID
	ClearWorkloadID bool
}

type AgentFilter struct{}

type VolumeFilter struct{}

type VolumeAttachmentFilter struct {
	VolumeID *uuid.UUID
	AgentID  *uuid.UUID
	McpID    *uuid.UUID
	HookID   *uuid.UUID
}

// ImagePullSecretAttachmentFilter narrows a list of attachments.
// OrganizationID is what keeps a list inside one tenant; it is a pointer
// because the Agents Orchestrator lists an environment's attachments over the
// mesh without naming an organization, and the remaining fields narrow within
// whatever scope results.
type ImagePullSecretAttachmentFilter struct {
	OrganizationID    *uuid.UUID
	ImagePullSecretID *uuid.UUID
	AgentID           *uuid.UUID
	McpID             *uuid.UUID
	HookID            *uuid.UUID
	EnvironmentID     *uuid.UUID
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

// EnvFilter narrows a list of envs. OrganizationID is what keeps a list inside
// one tenant; it is a pointer because the Agents Orchestrator lists an
// environment's envs over the mesh without naming an organization, and the
// remaining fields narrow within whatever scope results.
type EnvFilter struct {
	OrganizationID *uuid.UUID
	AgentID        *uuid.UUID
	McpID          *uuid.UUID
	HookID         *uuid.UUID
	EnvironmentID  *uuid.UUID
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
