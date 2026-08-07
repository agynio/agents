package server

import (
	"context"
	"time"

	organizationsv1 "github.com/agynio/agents/.gen/go/agynio/api/organizations/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// OrganizationsClient is the slice of Organizations this service uses: the
// sandbox lifecycle bounds a new sandbox records at creation.
type OrganizationsClient interface {
	GetOrganization(ctx context.Context, req *organizationsv1.GetOrganizationRequest, opts ...grpc.CallOption) (*organizationsv1.GetOrganizationResponse, error)
}

// sandboxLifecycle is what a sandbox stores at creation. Both are snapshotted:
// changing the organization's settings afterwards never moves a live sandbox.
type sandboxLifecycle struct {
	IdleTimeout string
	TTL         string
}

// resolveSandboxLifecycle settles the bounds for a new sandbox. A requested idle
// timeout wins up to the organization's ceiling; above it the request is refused
// naming the ceiling rather than clamped, because a silently reduced timeout is
// a number the caller never sees and plans around wrongly.
func (s *Server) resolveSandboxLifecycle(ctx context.Context, organizationID uuid.UUID, requested *string) (sandboxLifecycle, error) {
	if s.organizations == nil {
		return sandboxLifecycle{}, status.Error(codes.Internal, "organizations client is not configured")
	}
	response, err := s.organizations.GetOrganization(ctx, &organizationsv1.GetOrganizationRequest{Id: organizationID.String()})
	if err != nil {
		return sandboxLifecycle{}, err
	}
	organization := response.GetOrganization()
	if organization == nil {
		return sandboxLifecycle{}, status.Errorf(codes.NotFound, "organization %s not found", organizationID)
	}

	ceiling, err := time.ParseDuration(organization.GetSandboxMaxIdleTimeout())
	if err != nil {
		return sandboxLifecycle{}, status.Errorf(codes.Internal, "organization sandbox_max_idle_timeout: %v", err)
	}
	idleTimeout := organization.GetSandboxDefaultIdleTimeout()
	if requested != nil {
		idleTimeout = *requested
		asked, err := time.ParseDuration(idleTimeout)
		if err != nil {
			return sandboxLifecycle{}, status.Errorf(codes.InvalidArgument, "idle_timeout: %v", err)
		}
		if asked <= 0 {
			return sandboxLifecycle{}, status.Error(codes.InvalidArgument, "idle_timeout: must be greater than 0s")
		}
		if asked > ceiling {
			return sandboxLifecycle{}, status.Errorf(codes.InvalidArgument,
				"idle_timeout %s exceeds the organization's sandbox_max_idle_timeout of %s", asked, ceiling)
		}
	} else {
		// Organizations keeps the two consistent on write. Should they drift,
		// the ceiling binds: nobody asked for this number, so honouring the
		// organization's stated maximum misleads no one, where refusing would
		// leave it unable to start a sandbox at all.
		fallback, err := time.ParseDuration(idleTimeout)
		if err != nil {
			return sandboxLifecycle{}, status.Errorf(codes.Internal, "organization sandbox_default_idle_timeout: %v", err)
		}
		if fallback > ceiling {
			idleTimeout = organization.GetSandboxMaxIdleTimeout()
		}
	}
	if err := validateDurationString(idleTimeout); err != nil {
		return sandboxLifecycle{}, status.Errorf(codes.Internal, "organization sandbox_default_idle_timeout: %v", err)
	}
	ttl := organization.GetSandboxDefaultTtl()
	if err := validateDurationString(ttl); err != nil {
		return sandboxLifecycle{}, status.Errorf(codes.Internal, "organization sandbox_default_ttl: %v", err)
	}
	return sandboxLifecycle{IdleTimeout: idleTimeout, TTL: ttl}, nil
}
