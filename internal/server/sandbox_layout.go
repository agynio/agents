package server

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	notificationsv1 "github.com/agynio/agents/.gen/go/agynio/api/notifications/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const sandboxLayoutUpdatedEvent = "sandbox_layout.updated"

// maxSandboxTabs bounds a document nobody is meant to fill. A person keeps a
// handful of shells; a client looping on open would otherwise write a layout
// that costs memory in the container for every tab it names.
const maxSandboxTabs = 64

// shellIDPattern is the Terminal Proxy's, restated because a layout naming an
// id no ticket could ever be issued for is a tab that cannot be opened.
var shellIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// GetSandboxLayout returns the calling identity's tabs for a sandbox.
//
// The identity is never a request field. It comes from authenticated context,
// so there is no call that reads another person's tabs -- which is what makes
// "your own tabs on a shared sandbox" a property of the API rather than of
// every client remembering to filter.
func (s *Server) GetSandboxLayout(ctx context.Context, req *agentsv1.GetSandboxLayoutRequest) (*agentsv1.GetSandboxLayoutResponse, error) {
	sandboxID, identityID, err := s.authorizeLayoutAccess(ctx, req.GetSandboxId())
	if err != nil {
		return nil, err
	}
	layout, err := s.store.GetSandboxLayout(ctx, sandboxID, identityID)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.GetSandboxLayoutResponse{Layout: toProtoSandboxLayout(layout)}, nil
}

// SetSandboxLayout replaces the document, guarded by the version the caller
// read. Whole-document writes because opening, closing and reordering are one
// change to one thing; the version is what keeps a second device from
// overwriting a tab it never saw.
func (s *Server) SetSandboxLayout(ctx context.Context, req *agentsv1.SetSandboxLayoutRequest) (*agentsv1.SetSandboxLayoutResponse, error) {
	sandboxID, identityID, err := s.authorizeLayoutAccess(ctx, req.GetSandboxId())
	if err != nil {
		return nil, err
	}
	tabs, err := fromProtoSandboxTabs(req.GetTabs())
	if err != nil {
		return nil, err
	}

	layout, err := s.store.SetSandboxLayout(ctx, sandboxID, identityID, req.GetVersion(), tabs)
	if err != nil {
		if errors.Is(err, store.ErrLayoutVersionConflict) {
			// Not a failure of the caller's: the other device got there first,
			// and the answer is to refetch rather than to retry blind.
			return nil, status.Error(codes.FailedPrecondition, "sandbox layout version conflict; refetch and reapply")
		}
		return nil, toStatusError(err)
	}

	s.publishSandboxLayoutUpdated(ctx, layout)
	return &agentsv1.SetSandboxLayoutResponse{Layout: toProtoSandboxLayout(layout)}, nil
}

// SetSandboxLayoutDirectories records where each shell was, immediately before
// the Orchestrator stops the workload. Internal: the caller is the platform,
// not a person, and the identity it writes for is every identity with a layout
// on this sandbox rather than its own.
func (s *Server) SetSandboxLayoutDirectories(ctx context.Context, req *agentsv1.SetSandboxLayoutDirectoriesRequest) (*agentsv1.SetSandboxLayoutDirectoriesResponse, error) {
	sandboxID, err := parseUUID(req.GetSandboxId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "sandbox_id: %v", err)
	}

	directories := make(map[string]string, len(req.GetDirectories()))
	for _, entry := range req.GetDirectories() {
		shellID := strings.TrimSpace(entry.GetShellId())
		cwd := strings.TrimSpace(entry.GetCwd())
		if shellID == "" || cwd == "" || !strings.HasPrefix(cwd, "/") {
			continue
		}
		directories[shellID] = cwd
	}

	touched, err := s.store.SetSandboxLayoutDirectories(ctx, sandboxID, directories)
	if err != nil {
		return nil, toStatusError(err)
	}

	if touched > 0 {
		// Every layout of this sandbox may have moved, and each belongs to a
		// different person's room.
		identities, err := s.store.ListSandboxLayoutIdentities(ctx, sandboxID)
		if err == nil {
			for _, identityID := range identities {
				layout, err := s.store.GetSandboxLayout(ctx, sandboxID, identityID)
				if err != nil {
					continue
				}
				s.publishSandboxLayoutUpdated(ctx, layout)
			}
		}
	}

	return &agentsv1.SetSandboxLayoutDirectoriesResponse{TabsUpdated: int32(touched)}, nil
}

// authorizeLayoutAccess gates on can_connect rather than on a relation of its
// own. A layout describes shells, and anyone who may open a shell here may
// keep a record of the ones they opened.
func (s *Server) authorizeLayoutAccess(ctx context.Context, rawSandboxID string) (uuid.UUID, uuid.UUID, error) {
	sandboxID, err := parseUUID(rawSandboxID)
	if err != nil {
		return uuid.Nil, uuid.Nil, status.Errorf(codes.InvalidArgument, "sandbox_id: %v", err)
	}
	sandbox, err := s.store.GetSandbox(ctx, sandboxID)
	if err != nil {
		return uuid.Nil, uuid.Nil, toStatusError(err)
	}
	identityID, err := identityUUIDFromContext(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if err := s.requireSandboxRelation(ctx, identityID, sandbox.Meta.ID, "can_connect"); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return sandbox.Meta.ID, identityID, nil
}

// publishSandboxLayoutUpdated carries the sandbox id and the new version, not
// the document. A client holding that sandbox open refetches; the one that
// made the change recognizes its own version and does nothing.
func (s *Server) publishSandboxLayoutUpdated(ctx context.Context, layout store.SandboxLayout) {
	payload, err := structpb.NewStruct(map[string]any{
		"sandbox_id":  layout.SandboxID.String(),
		"identity_id": layout.IdentityID.String(),
		"version":     float64(layout.Version),
	})
	if err != nil {
		return
	}
	if _, err := s.notifications.Publish(ctx, &notificationsv1.PublishRequest{
		Event: sandboxLayoutUpdatedEvent,
		// Keyed by who the layout belongs to rather than by which sandbox it
		// is for: one subscription follows a person across every sandbox they
		// have open, which is what the room exists for -- their other device.
		Rooms:   []string{fmt.Sprintf("sandbox_layout:%s", layout.IdentityID)},
		Payload: payload,
		Source:  "agents",
	}); err != nil {
		return
	}
}

func fromProtoSandboxTabs(tabs []*agentsv1.SandboxTab) ([]store.SandboxTab, error) {
	if len(tabs) > maxSandboxTabs {
		return nil, status.Errorf(codes.InvalidArgument, "tabs: at most %d", maxSandboxTabs)
	}

	converted := make([]store.SandboxTab, 0, len(tabs))
	seen := make(map[string]struct{}, len(tabs))

	for i, tab := range tabs {
		shellID := strings.TrimSpace(tab.GetShellId())
		if !shellIDPattern.MatchString(shellID) {
			return nil, status.Errorf(codes.InvalidArgument, "tabs[%d].shell_id must match %s", i, shellIDPattern)
		}
		// Two tabs naming one shell would both attach to it, and the second
		// would displace the first inside the same strip.
		if _, duplicate := seen[shellID]; duplicate {
			return nil, status.Errorf(codes.InvalidArgument, "tabs[%d].shell_id %q appears twice", i, shellID)
		}
		seen[shellID] = struct{}{}

		entry := store.SandboxTab{ShellID: shellID, Number: tab.GetNumber()}

		if tab.NameOverride != nil {
			name := strings.TrimSpace(tab.GetNameOverride())
			if name != "" {
				if len(name) > 128 {
					return nil, status.Errorf(codes.InvalidArgument, "tabs[%d].name_override is too long", i)
				}
				entry.NameOverride = &name
			}
		}
		if tab.Cwd != nil {
			cwd := strings.TrimSpace(tab.GetCwd())
			if cwd != "" {
				if !strings.HasPrefix(cwd, "/") {
					return nil, status.Errorf(codes.InvalidArgument, "tabs[%d].cwd %q is not absolute", i, cwd)
				}
				entry.CWD = &cwd
			}
		}
		if tab.LastAttachedAt != nil {
			at := tab.GetLastAttachedAt().AsTime()
			entry.LastAttachedAt = &at
		}

		converted = append(converted, entry)
	}
	return converted, nil
}

func toProtoSandboxLayout(layout store.SandboxLayout) *agentsv1.SandboxLayout {
	tabs := make([]*agentsv1.SandboxTab, 0, len(layout.Tabs))
	for _, tab := range layout.Tabs {
		converted := &agentsv1.SandboxTab{ShellId: tab.ShellID, Number: tab.Number}
		if tab.NameOverride != nil {
			converted.NameOverride = tab.NameOverride
		}
		if tab.CWD != nil {
			converted.Cwd = tab.CWD
		}
		if tab.LastAttachedAt != nil {
			converted.LastAttachedAt = timestamppb.New(*tab.LastAttachedAt)
		}
		tabs = append(tabs, converted)
	}
	return &agentsv1.SandboxLayout{
		SandboxId:  layout.SandboxID.String(),
		IdentityId: layout.IdentityID.String(),
		Version:    layout.Version,
		Tabs:       tabs,
	}
}
