package server

import (
	"context"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
)

// A hibernated sandbox is woken in place, not restarted: the Orchestrator
// resumes the workload the record names, so the reference survives the write
// that asks for it.
func TestSandboxRestartOnConnectUpdateHibernatedWakes(t *testing.T) {
	update, shouldUpdate, err := sandboxRestartOnConnectUpdate(store.SandboxStatusHibernated)
	if err != nil {
		t.Fatalf("restart on connect update: %v", err)
	}
	if !shouldUpdate {
		t.Fatal("expected a hibernated sandbox to be asked to start")
	}
	if update.Status == nil || *update.Status != store.SandboxStatusStarting {
		t.Fatalf("expected starting status, got %v", update.Status)
	}
	if update.ClearWorkloadID {
		t.Fatal("expected the hibernated workload to be kept for the wake")
	}
	if update.WorkloadID != nil {
		t.Fatalf("expected the workload reference to be left as it is, got %v", update.WorkloadID)
	}
}

// The Orchestrator writes hibernated through UpdateSandboxRuntimeState and
// reads it back through every sandbox read; an unmapped status is refused on
// the way in and panics on the way out.
func TestSandboxStatusesRoundTripThroughTheProto(t *testing.T) {
	for _, sandboxStatus := range []store.SandboxStatus{
		store.SandboxStatusStarting,
		store.SandboxStatusRunning,
		store.SandboxStatusHibernated,
		store.SandboxStatusStopped,
		store.SandboxStatusFailed,
		store.SandboxStatusTerminated,
	} {
		got, err := sandboxStatusFromProto(sandboxStatusToProto(sandboxStatus))
		if err != nil {
			t.Fatalf("%s: %v", sandboxStatus, err)
		}
		if got != sandboxStatus {
			t.Fatalf("expected %s back, got %s", sandboxStatus, got)
		}
	}
	if sandboxStatusToProto(store.SandboxStatusHibernated) != agentsv1.SandboxStatus_SANDBOX_STATUS_HIBERNATED {
		t.Fatalf("expected hibernated to map to SANDBOX_STATUS_HIBERNATED")
	}
}

func TestSandboxRuntimeStateUpdateFromProtoRecordsHibernation(t *testing.T) {
	workloadID := uuid.New()
	update, err := sandboxRuntimeStateUpdateFromProto(&agentsv1.UpdateSandboxRuntimeStateRequest{
		Status: ptr(agentsv1.SandboxStatus_SANDBOX_STATUS_HIBERNATED),
		WorkloadIdUpdate: &agentsv1.UpdateSandboxRuntimeStateRequest_WorkloadId{
			WorkloadId: workloadID.String(),
		},
	})
	if err != nil {
		t.Fatalf("runtime state update: %v", err)
	}
	if update.Status == nil || *update.Status != store.SandboxStatusHibernated {
		t.Fatalf("expected hibernated status, got %v", update.Status)
	}
	if update.WorkloadID == nil || *update.WorkloadID != workloadID {
		t.Fatalf("expected workload id %s, got %v", workloadID, update.WorkloadID)
	}
}

// A connect to a hibernated sandbox asks the Orchestrator for a wake: starting,
// with the workload it will resume still named, announced on sandbox.updated
// exactly as a stopped sandbox's restart is.
func TestEnsureSandboxRunningWakesAHibernatedSandbox(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	server.WithOrganizations(organizationsStub("30m", "8h", "72h"))
	organizationID := uuid.New()
	environmentID := createWorkspaceOnlyEnvironment(ctx, t, server, organizationID, "default")
	created, err := server.CreateSandbox(ctx, &agentsv1.CreateSandboxRequest{
		OrganizationId: organizationID.String(),
		EnvironmentId:  environmentID,
	})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	sandboxID := created.GetSandbox().GetMeta().GetId()

	// What the Orchestrator records once the idle timeout has put it to sleep.
	workloadID := uuid.NewString()
	hibernated, err := server.UpdateSandboxRuntimeState(context.Background(), &agentsv1.UpdateSandboxRuntimeStateRequest{
		Id:               sandboxID,
		Status:           ptr(agentsv1.SandboxStatus_SANDBOX_STATUS_HIBERNATED),
		WorkloadIdUpdate: &agentsv1.UpdateSandboxRuntimeStateRequest_WorkloadId{WorkloadId: workloadID},
	})
	if err != nil {
		t.Fatalf("record hibernation: %v", err)
	}
	if hibernated.GetSandbox().GetStatus() != agentsv1.SandboxStatus_SANDBOX_STATUS_HIBERNATED {
		t.Fatalf("expected hibernated, got %v", hibernated.GetSandbox().GetStatus())
	}

	// Asleep is not stopped: the default list keeps it.
	listed, err := server.ListSandboxes(ctx, &agentsv1.ListSandboxesRequest{OrganizationId: organizationID.String()})
	if err != nil {
		t.Fatalf("list sandboxes: %v", err)
	}
	if len(listed.GetSandboxes()) != 1 || listed.GetSandboxes()[0].GetMeta().GetId() != sandboxID {
		t.Fatalf("expected the hibernated sandbox in the default list, got %v", listed.GetSandboxes())
	}

	notifications := &recordingNotificationsClient{}
	server.notifications = notifications
	woken, err := server.EnsureSandboxRunning(ctx, &agentsv1.EnsureSandboxRunningRequest{Id: sandboxID})
	if err != nil {
		t.Fatalf("ensure sandbox running: %v", err)
	}
	if woken.GetSandbox().GetStatus() != agentsv1.SandboxStatus_SANDBOX_STATUS_STARTING {
		t.Fatalf("expected starting, got %v", woken.GetSandbox().GetStatus())
	}
	if woken.GetSandbox().GetWorkloadId() != workloadID {
		t.Fatalf("expected the hibernated workload %s to be kept, got %q", workloadID, woken.GetSandbox().GetWorkloadId())
	}

	fetched, err := server.GetSandbox(ctx, &agentsv1.GetSandboxRequest{Ref: &agentsv1.GetSandboxRequest_Id{Id: sandboxID}})
	if err != nil {
		t.Fatalf("get sandbox: %v", err)
	}
	if fetched.GetSandbox().GetStatus() != agentsv1.SandboxStatus_SANDBOX_STATUS_STARTING || fetched.GetSandbox().GetWorkloadId() != workloadID {
		t.Fatalf("expected starting on workload %s to be stored, got %v on %q", workloadID, fetched.GetSandbox().GetStatus(), fetched.GetSandbox().GetWorkloadId())
	}

	if len(notifications.published) != 1 {
		t.Fatalf("expected one sandbox.updated, got %d publishes", len(notifications.published))
	}
	request := notifications.published[0]
	if request.GetEvent() != sandboxUpdatedEvent {
		t.Fatalf("expected %s, got %s", sandboxUpdatedEvent, request.GetEvent())
	}
	fields := request.GetPayload().GetFields()
	assertStringField(t, fields, "status", "starting")
	assertStringField(t, fields, "workload_id", workloadID)
}
