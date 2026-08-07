package server

import (
	"context"
	"strings"
	"testing"

	organizationsv1 "github.com/agynio/agents/.gen/go/agynio/api/organizations/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Every sandbox in an organization used to get one hardcoded number: the
// organization's own settings were written and validated by Organizations and
// read by nobody. What follows is what a new sandbox now records.

type stubOrganizationsClient struct {
	organization *organizationsv1.Organization
	err          error
	requested    []string
}

func (c *stubOrganizationsClient) GetOrganization(_ context.Context, req *organizationsv1.GetOrganizationRequest, _ ...grpc.CallOption) (*organizationsv1.GetOrganizationResponse, error) {
	c.requested = append(c.requested, req.GetId())
	if c.err != nil {
		return nil, c.err
	}
	return &organizationsv1.GetOrganizationResponse{Organization: c.organization}, nil
}

func organizationsStub(defaultIdle, maxIdle, ttl string) *stubOrganizationsClient {
	return &stubOrganizationsClient{organization: &organizationsv1.Organization{
		SandboxDefaultIdleTimeout: defaultIdle,
		SandboxMaxIdleTimeout:     maxIdle,
		SandboxDefaultTtl:         ttl,
	}}
}

func TestSandboxLifecycleUsesTheOrganizationDefault(t *testing.T) {
	organizations := organizationsStub("45m", "8h", "96h")
	server := &Server{organizations: organizations}
	organizationID := uuid.New()

	lifecycle, err := server.resolveSandboxLifecycle(context.Background(), organizationID, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if lifecycle.IdleTimeout != "45m" {
		t.Fatalf("expected the organization default 45m, got %q", lifecycle.IdleTimeout)
	}
	if lifecycle.TTL != "96h" {
		t.Fatalf("expected the organization ttl 96h, got %q", lifecycle.TTL)
	}
	if len(organizations.requested) != 1 || organizations.requested[0] != organizationID.String() {
		t.Fatalf("expected the sandbox's own organization to be read, got %v", organizations.requested)
	}
}

func TestSandboxLifecycleAcceptsARequestedTimeout(t *testing.T) {
	server := &Server{organizations: organizationsStub("30m", "8h", "72h")}

	lifecycle, err := server.resolveSandboxLifecycle(context.Background(), uuid.New(), strPtr("4h"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if lifecycle.IdleTimeout != "4h" {
		t.Fatalf("expected the requested 4h, got %q", lifecycle.IdleTimeout)
	}
}

// Naming the ceiling is the point: a silently clamped timeout is a number the
// engineer never sees and plans around wrongly.
func TestSandboxLifecycleRefusesAboveTheCeilingAndNamesIt(t *testing.T) {
	server := &Server{organizations: organizationsStub("30m", "2h", "72h")}

	_, err := server.resolveSandboxLifecycle(context.Background(), uuid.New(), strPtr("6h"))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
	if !strings.Contains(err.Error(), "2h") {
		t.Fatalf("expected the error to name the ceiling, got %q", err.Error())
	}
}

func TestSandboxLifecycleAcceptsExactlyTheCeiling(t *testing.T) {
	server := &Server{organizations: organizationsStub("30m", "2h", "72h")}

	lifecycle, err := server.resolveSandboxLifecycle(context.Background(), uuid.New(), strPtr("2h"))
	if err != nil {
		t.Fatalf("the ceiling itself must be allowed: %v", err)
	}
	if lifecycle.IdleTimeout != "2h" {
		t.Fatalf("expected 2h, got %q", lifecycle.IdleTimeout)
	}
}

func TestSandboxLifecycleRejectsAMalformedRequest(t *testing.T) {
	server := &Server{organizations: organizationsStub("30m", "8h", "72h")}

	for _, value := range []string{"soon", "0s", "-1h"} {
		if _, err := server.resolveSandboxLifecycle(context.Background(), uuid.New(), strPtr(value)); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("%q: expected InvalidArgument, got %v", value, err)
		}
	}
}

// A default above the ceiling is Organizations' invariant to keep. Should the
// two drift, a sandbox that names nothing must not be handed a value the
// organization refuses to anyone who asks for it.
func TestSandboxLifecycleBindsADriftedDefaultToTheCeiling(t *testing.T) {
	server := &Server{organizations: organizationsStub("6h", "2h", "72h")}

	lifecycle, err := server.resolveSandboxLifecycle(context.Background(), uuid.New(), nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if lifecycle.IdleTimeout != "2h" {
		t.Fatalf("expected the ceiling 2h to bind the drifted default, got %q", lifecycle.IdleTimeout)
	}
}

func strPtr(value string) *string { return &value }
