package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxVolumeNameLength = 64

var volumeNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

func validateVolumeName(name string) error {
	if name == "" {
		return fmt.Errorf("value is empty")
	}
	if len(name) > maxVolumeNameLength {
		return fmt.Errorf("must be at most %d characters", maxVolumeNameLength)
	}
	if !volumeNamePattern.MatchString(name) {
		return fmt.Errorf("must match %s", volumeNamePattern.String())
	}
	return nil
}

func validateMountPath(path string) error {
	if path == "" {
		return fmt.Errorf("value is empty")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("must be an absolute path")
	}
	return nil
}

// resolveVolumeSize settles persistence from the size, which the resource makes
// biconditional: a size means a provisioned disk, no size means scratch. A
// caller asking for persistence without a size is asking for a disk of no
// stated capacity, which is refused rather than defaulted.
func resolveVolumeSize(size string, persistent bool) (*string, bool, error) {
	trimmed := strings.TrimSpace(size)
	if trimmed == "" {
		if persistent {
			return nil, false, status.Error(codes.InvalidArgument, "size is required for a persistent volume")
		}
		return nil, false, nil
	}
	return &trimmed, true, nil
}

// requireVolumeWrite authorizes changing a volume through the target that owns
// it. A volume carries no tuples of its own.
func (s *Server) requireVolumeWrite(ctx context.Context, volume store.Volume) error {
	if volume.EnvironmentID != nil {
		environment, err := s.store.GetEnvironment(ctx, *volume.EnvironmentID)
		if err != nil {
			return toStatusError(err)
		}
		return s.requireEnvironmentWrite(ctx, environment)
	}
	return nil
}

// ListVolumeAttachments serves orchestrators released before volumes moved onto
// environments. An agent's attachments are its environment's volumes and an
// MCP's are its own, so an old orchestrator mounts the same volumes a new one
// would. Without it, upgrading this service first leaves every agent assembly
// failing on a missing RPC.
func (s *Server) ListVolumeAttachments(ctx context.Context, req *agentsv1.ListVolumeAttachmentsRequest) (*agentsv1.ListVolumeAttachmentsResponse, error) {
	switch {
	case req.GetAgentId() != "":
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		agent, err := s.store.GetAgent(ctx, agentID)
		if err != nil {
			return nil, toStatusError(err)
		}
		if agent.EnvironmentID == nil {
			return &agentsv1.ListVolumeAttachmentsResponse{}, nil
		}
		volumes, err := s.environmentVolumes(ctx, *agent.EnvironmentID)
		if err != nil {
			return nil, err
		}
		return &agentsv1.ListVolumeAttachmentsResponse{
			VolumeAttachments: attachmentsFor(volumes, func(a *agentsv1.VolumeAttachment) {
				a.Target = &agentsv1.VolumeAttachment_AgentId{AgentId: agentID.String()}
			}),
		}, nil
	case req.GetMcpId() != "":
		mcpID, err := parseUUID(req.GetMcpId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "mcp_id: %v", err)
		}
		mcp, err := s.store.GetMcp(ctx, mcpID)
		if err != nil {
			return nil, toStatusError(err)
		}
		result, err := s.store.ListVolumes(ctx, mcp.OrganizationID, store.VolumeFilter{McpID: &mcpID}, 0, nil)
		if err != nil {
			return nil, toStatusError(err)
		}
		return &agentsv1.ListVolumeAttachmentsResponse{
			VolumeAttachments: attachmentsFor(result.Volumes, func(a *agentsv1.VolumeAttachment) {
				a.Target = &agentsv1.VolumeAttachment_McpId{McpId: mcpID.String()}
			}),
		}, nil
	default:
		return nil, status.Error(codes.InvalidArgument, "agent_id or mcp_id is required")
	}
}

func (s *Server) environmentVolumes(ctx context.Context, environmentID uuid.UUID) ([]store.Volume, error) {
	environment, err := s.store.GetEnvironment(ctx, environmentID)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireEnvironmentConfigRead(ctx, environmentID); err != nil {
		return nil, err
	}
	result, err := s.store.ListVolumes(ctx, environment.OrganizationID, store.VolumeFilter{EnvironmentID: &environmentID}, 0, nil)
	if err != nil {
		return nil, toStatusError(err)
	}
	return result.Volumes, nil
}

// The attachment id is the volume's: nothing persists these any more, and a
// caller correlating one back to a volume gets the id it already knows.
func attachmentsFor(volumes []store.Volume, target func(*agentsv1.VolumeAttachment)) []*agentsv1.VolumeAttachment {
	attachments := make([]*agentsv1.VolumeAttachment, 0, len(volumes))
	for _, volume := range volumes {
		attachment := &agentsv1.VolumeAttachment{
			Meta:     toProtoEntityMeta(volume.Meta),
			VolumeId: volume.Meta.ID.String(),
		}
		target(attachment)
		attachments = append(attachments, attachment)
	}
	return attachments
}
