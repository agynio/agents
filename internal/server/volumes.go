package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/agynio/agents/internal/store"
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
