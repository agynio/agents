package server

import (
	"context"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/google/uuid"
)

// A filter field reaching the struct but not the query returns every row, and
// the caller cannot tell: an environment listing showed every MCP in the
// database. These go through the handler against a real database, which is the
// only place that shows.
func TestEnvironmentScopedListsAreScoped(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()

	first := createTestEnvironment(ctx, t, server, organizationID, "first")
	second := createTestEnvironment(ctx, t, server, organizationID, "second")

	mustCreateMcp(ctx, t, server, first, "alpha")
	mustCreateMcp(ctx, t, server, second, "beta")
	mustCreateVolume(ctx, t, server, first, "data", "/data")
	mustCreateVolume(ctx, t, server, second, "cache", "/cache")

	mcps, err := server.ListMcps(ctx, &agentsv1.ListMcpsRequest{EnvironmentId: first})
	if err != nil {
		t.Fatalf("list mcps: %v", err)
	}
	if len(mcps.GetMcps()) != 1 || mcps.GetMcps()[0].GetName() != "alpha" {
		t.Fatalf("expected only the first environment's server, got %d", len(mcps.GetMcps()))
	}

	volumes, err := server.ListVolumes(ctx, &agentsv1.ListVolumesRequest{EnvironmentId: second})
	if err != nil {
		t.Fatalf("list volumes: %v", err)
	}
	if len(volumes.GetVolumes()) != 1 || volumes.GetVolumes()[0].GetName() != "cache" {
		t.Fatalf("expected only the second environment's volume, got %d", len(volumes.GetVolumes()))
	}

	scripts, err := server.ListInitScripts(ctx, &agentsv1.ListInitScriptsRequest{EnvironmentId: first})
	if err != nil {
		t.Fatalf("list init scripts: %v", err)
	}
	if len(scripts.GetInitScripts()) != 0 {
		t.Fatalf("expected no scripts on the first environment, got %d", len(scripts.GetInitScripts()))
	}
}

func mustCreateMcp(ctx context.Context, t *testing.T, server *Server, environmentID, name string) {
	t.Helper()
	if _, err := server.CreateMcp(ctx, &agentsv1.CreateMcpRequest{
		EnvironmentId: environmentID, Name: name, Command: "run",
	}); err != nil {
		t.Fatalf("create mcp %s: %v", name, err)
	}
}

func mustCreateVolume(ctx context.Context, t *testing.T, server *Server, environmentID, name, path string) {
	t.Helper()
	if _, err := server.CreateVolume(ctx, &agentsv1.CreateVolumeRequest{
		Target:    &agentsv1.CreateVolumeRequest_EnvironmentId{EnvironmentId: environmentID},
		Name:      name,
		MountPath: path,
	}); err != nil {
		t.Fatalf("create volume %s: %v", name, err)
	}
}
