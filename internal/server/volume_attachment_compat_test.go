package server

import (
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/google/uuid"
)

// Orchestrators released before this change list an agent's volumes as
// attachments and mount whatever comes back. Serving them the environment's
// volumes keeps that path correct while the two services are on different
// versions; returning nothing would silently start agents without their disks.
func TestListVolumeAttachmentsServesTheEnvironmentsVolumes(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()

	environmentID := createTestEnvironment(ctx, t, server, organizationID, "shared")
	mustCreateVolume(ctx, t, server, environmentID, "data", "/data")

	request := createAgentRequest(organizationID, "alpha")
	request.EnvironmentId = environmentID
	created, err := server.CreateAgent(ctx, request)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agentID := created.GetAgent().GetMeta().GetId()

	listed, err := server.ListVolumeAttachments(ctx, &agentsv1.ListVolumeAttachmentsRequest{AgentId: agentID})
	if err != nil {
		t.Fatalf("list volume attachments: %v", err)
	}
	attachments := listed.GetVolumeAttachments()
	if len(attachments) != 1 {
		t.Fatalf("expected the environment's one volume, got %d", len(attachments))
	}
	if attachments[0].GetAgentId() != agentID {
		t.Fatalf("expected the attachment targeted at %s, got %q", agentID, attachments[0].GetAgentId())
	}

	// The caller resolves the mount path through GetVolume, so the id has to
	// be the real one rather than a synthesised attachment id.
	volume, err := server.GetVolume(ctx, &agentsv1.GetVolumeRequest{Id: attachments[0].GetVolumeId()})
	if err != nil {
		t.Fatalf("get volume: %v", err)
	}
	if volume.GetVolume().GetMountPath() != "/data" {
		t.Fatalf("expected /data, got %q", volume.GetVolume().GetMountPath())
	}
}

// An agent in no environment has no volumes, which is an empty list rather than
// an error: the old orchestrator treats a failure here as an unassemblable
// agent.
func TestListVolumeAttachmentsForAnAgentWithoutAnEnvironment(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)

	created, err := server.CreateAgent(ctx, createAgentRequest(uuid.New(), "alpha"))
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	listed, err := server.ListVolumeAttachments(ctx, &agentsv1.ListVolumeAttachmentsRequest{
		AgentId: created.GetAgent().GetMeta().GetId(),
	})
	if err != nil {
		t.Fatalf("list volume attachments: %v", err)
	}
	if len(listed.GetVolumeAttachments()) != 0 {
		t.Fatalf("expected no attachments, got %d", len(listed.GetVolumeAttachments()))
	}
}

func TestListVolumeAttachmentsRequiresATarget(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)

	if _, err := server.ListVolumeAttachments(ctx, &agentsv1.ListVolumeAttachmentsRequest{}); err == nil {
		t.Fatal("expected an unfiltered listing to be refused")
	}
}
