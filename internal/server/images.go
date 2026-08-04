package server

import (
	"context"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	imagesv1 "github.com/agynio/agents/.gen/go/agynio/api/images/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ImagesClient is the slice of the catalog this service uses. References are
// validated on write so a typo fails where someone can fix it, rather than at
// workload start.
type ImagesClient interface {
	ResolveVersion(ctx context.Context, req *imagesv1.ResolveVersionRequest, opts ...grpc.CallOption) (*imagesv1.ResolveVersionResponse, error)
}

// imageReference is a catalog record and a tag within it, as an environment or
// an MCP names one.
type imageReference struct {
	ImageID uuid.UUID
	Tag     string
}

// validateImageReference checks that the reference resolves to an image the
// organization can see, of the type the slot requires, with a tag discovery has
// seen.
//
// The check is a point-in-time one. The reference stays late-bound: the image
// can be deleted, its tag can go gone, or its visibility can narrow afterwards,
// and the environment is then flagged unschedulable rather than repaired.
func (s *Server) validateImageReference(
	ctx context.Context,
	reference imageReference,
	organizationID uuid.UUID,
	required imagesv1.ImageType,
	field string,
) error {
	if s.images == nil {
		// Without a catalog client the service cannot validate, and refusing
		// every write would be worse than accepting one the orchestrator will
		// resolve again at workload start.
		return nil
	}

	resolved, err := s.images.ResolveVersion(ctx, &imagesv1.ResolveVersionRequest{
		Reference:              &imagesv1.ResolveVersionRequest_ImageId{ImageId: reference.ImageID.String()},
		Tag:                    reference.Tag,
		ConsumerOrganizationId: organizationID.String(),
		RequireType:            required,
	})
	if err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			return status.Errorf(codes.InvalidArgument, "%s: no image %s with tag %q is visible to this organization",
				field, reference.ImageID, reference.Tag)
		case codes.FailedPrecondition:
			return status.Errorf(codes.InvalidArgument, "%s: %s", field, status.Convert(err).Message())
		case codes.InvalidArgument:
			return status.Errorf(codes.InvalidArgument, "%s: %s", field, status.Convert(err).Message())
		default:
			return err
		}
	}

	// A tag the catalog knows but which vanished upstream resolves, so that a
	// reference to it can be reported rather than dangling. Accepting one on
	// write would be creating a reference already known to be unschedulable.
	if resolved.GetState() == imagesv1.ImageVersionState_IMAGE_VERSION_STATE_GONE {
		return status.Errorf(codes.InvalidArgument, "%s: tag %q is no longer present upstream",
			field, reference.Tag)
	}
	return nil
}

// parseImageReference reads the (id, tag) pair a request carries. Both or
// neither: an id with no tag names no version, and a tag with no id names no
// image.
func parseImageReference(rawID, tag, field string) (*imageReference, error) {
	if rawID == "" && tag == "" {
		return nil, nil
	}
	if rawID == "" || tag == "" {
		return nil, status.Errorf(codes.InvalidArgument, "%s: an image reference needs both an id and a tag", field)
	}
	id, err := uuid.Parse(rawID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s: %v", field, err)
	}
	return &imageReference{ImageID: id, Tag: tag}, nil
}

// applyEnvironmentImageUpdate settles the catalog references an update names.
// Both halves of a pair move together: an id without a tag names no version.
// Clearing the agent runtime pair is how an environment becomes workspace-only.
func (s *Server) applyEnvironmentImageUpdate(
	ctx context.Context,
	req *agentsv1.UpdateEnvironmentRequest,
	environmentID uuid.UUID,
	update *store.EnvironmentUpdate,
) error {
	if req.WorkspaceImageId == nil && req.WorkspaceImageTag == nil &&
		req.AgentRuntimeImageId == nil && req.AgentRuntimeImageTag == nil {
		return nil
	}

	existing, err := s.store.GetEnvironment(ctx, environmentID)
	if err != nil {
		return toStatusError(err)
	}

	workspace, err := mergedReference(req.WorkspaceImageId, req.WorkspaceImageTag,
		existing.WorkspaceImageID, existing.WorkspaceImageTag, "workspace_image")
	if err != nil {
		return err
	}
	agentRuntime, err := mergedReference(req.AgentRuntimeImageId, req.AgentRuntimeImageTag,
		existing.AgentRuntimeImageID, existing.AgentRuntimeImageTag, "agent_runtime_image")
	if err != nil {
		return err
	}

	if req.WorkspaceImageId != nil || req.WorkspaceImageTag != nil {
		if workspace == nil {
			// An environment with no workspace image names nothing to run.
			return status.Error(codes.InvalidArgument, "workspace_image: cannot be cleared")
		}
		if err := s.validateImageReference(ctx, *workspace, existing.OrganizationID,
			imagesv1.ImageType_IMAGE_TYPE_WORKSPACE, "workspace_image"); err != nil {
			return err
		}
		workspaceID := &workspace.ImageID
		update.WorkspaceImageID = &workspaceID
		update.WorkspaceImageTag = &workspace.Tag
	}

	if req.AgentRuntimeImageId != nil || req.AgentRuntimeImageTag != nil {
		if agentRuntime == nil {
			var cleared *uuid.UUID
			empty := ""
			update.AgentRuntimeImageID = &cleared
			update.AgentRuntimeImageTag = &empty
			return nil
		}
		if err := s.validateImageReference(ctx, *agentRuntime, existing.OrganizationID,
			imagesv1.ImageType_IMAGE_TYPE_AGENT_RUNTIME, "agent_runtime_image"); err != nil {
			return err
		}
		runtimeID := &agentRuntime.ImageID
		update.AgentRuntimeImageID = &runtimeID
		update.AgentRuntimeImageTag = &agentRuntime.Tag
	}
	return nil
}

// mergedReference applies a partial update onto what is stored, so a caller
// can change a tag without restating the image.
func mergedReference(rawID, tag *string, storedID *uuid.UUID, storedTag, field string) (*imageReference, error) {
	id := ""
	if storedID != nil {
		id = storedID.String()
	}
	if rawID != nil {
		id = *rawID
	}
	resolvedTag := storedTag
	if tag != nil {
		resolvedTag = *tag
	}
	return parseImageReference(id, resolvedTag, field)
}

// validateMcpImage accepts either type an MCP may run: a purpose-built server
// image and a devcontainer are both legitimate ways to host one. An agent
// runtime is not, so it is refused after the fact rather than by narrowing the
// request.
func (s *Server) validateMcpImage(ctx context.Context, reference imageReference, organizationID uuid.UUID) error {
	if s.images == nil {
		return nil
	}
	resolved, err := s.images.ResolveVersion(ctx, &imagesv1.ResolveVersionRequest{
		Reference:              &imagesv1.ResolveVersionRequest_ImageId{ImageId: reference.ImageID.String()},
		Tag:                    reference.Tag,
		ConsumerOrganizationId: organizationID.String(),
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return status.Errorf(codes.InvalidArgument, "image: no image %s with tag %q is visible to this organization",
				reference.ImageID, reference.Tag)
		}
		return err
	}
	switch resolved.GetImage().GetType() {
	case imagesv1.ImageType_IMAGE_TYPE_MCP, imagesv1.ImageType_IMAGE_TYPE_WORKSPACE:
	default:
		return status.Errorf(codes.InvalidArgument, "image: an MCP runs an mcp or workspace image, not %s",
			resolved.GetImage().GetType())
	}
	if resolved.GetState() == imagesv1.ImageVersionState_IMAGE_VERSION_STATE_GONE {
		return status.Errorf(codes.InvalidArgument, "image: tag %q is no longer present upstream", reference.Tag)
	}
	return nil
}

// organizationOfAgent resolves the organization an agent sub-resource inherits,
// which is what visibility is enforced against.
func (s *Server) organizationOfAgent(ctx context.Context, agentID uuid.UUID) (uuid.UUID, error) {
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return uuid.UUID{}, toStatusError(err)
	}
	return agent.OrganizationID, nil
}

// applyMcpImageUpdate settles the catalog reference an update names, merging a
// partial change onto what is stored.
func (s *Server) applyMcpImageUpdate(
	ctx context.Context,
	req *agentsv1.UpdateMcpRequest,
	mcpID uuid.UUID,
	update *store.McpUpdate,
) error {
	if req.ImageId == nil && req.ImageTag == nil {
		return nil
	}

	existing, err := s.store.GetMcp(ctx, mcpID)
	if err != nil {
		return toStatusError(err)
	}
	reference, err := mergedReference(req.ImageId, req.ImageTag, existing.ImageID, existing.ImageTag, "image")
	if err != nil {
		return err
	}
	if reference == nil {
		var cleared *uuid.UUID
		empty := ""
		update.ImageID = &cleared
		update.ImageTag = &empty
		return nil
	}

	organizationID, err := s.organizationOfAgent(ctx, existing.AgentID)
	if err != nil {
		return err
	}
	if err := s.validateMcpImage(ctx, *reference, organizationID); err != nil {
		return err
	}
	referenceID := &reference.ImageID
	update.ImageID = &referenceID
	update.ImageTag = &reference.Tag
	return nil
}
