package server

import (
	"context"
	"testing"
	"time"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	notificationsv1 "github.com/agynio/agents/.gen/go/agynio/api/notifications/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestPublishSandboxUpdatedIncludesSnapshotPayload(t *testing.T) {
	notifications := &recordingNotificationsClient{}
	server := &Server{notifications: notifications}
	lastSessionAt := time.Date(2026, 7, 16, 12, 30, 0, 123, time.UTC)
	workloadID := uuid.New()
	sandbox := store.Sandbox{
		Meta: store.EntityMeta{
			ID: uuid.New(),
		},
		OrganizationID: uuid.New(),
		Name:           "brave-otter-aabbccdd",
		EnvironmentID:  uuid.New(),
		OwnerID:        uuid.New(),
		Status:         store.SandboxStatusRunning,
		IdleTimeout:    "30m",
		TTL:            "72h",
		LastSessionAt:  &lastSessionAt,
		WorkloadID:     &workloadID,
	}

	server.publishSandboxUpdated(context.Background(), sandbox)

	if len(notifications.published) != 1 {
		t.Fatalf("expected 1 publish request, got %d", len(notifications.published))
	}
	request := notifications.published[0]
	if request.GetEvent() != sandboxUpdatedEvent {
		t.Fatalf("expected event %q, got %q", sandboxUpdatedEvent, request.GetEvent())
	}
	assertRooms(t, request.GetRooms(), []string{
		"sandbox_owner:" + sandbox.OwnerID.String(),
		"sandbox_org:" + sandbox.OrganizationID.String(),
	})
	fields := request.GetPayload().GetFields()
	assertStringField(t, fields, "sandbox_id", sandbox.Meta.ID.String())
	assertStringField(t, fields, "organization_id", sandbox.OrganizationID.String())
	assertStringField(t, fields, "name", sandbox.Name)
	assertStringField(t, fields, "environment_id", sandbox.EnvironmentID.String())
	assertStringField(t, fields, "owner_id", sandbox.OwnerID.String())
	assertStringField(t, fields, "status", string(sandbox.Status))
	assertStringField(t, fields, "idle_timeout", sandbox.IdleTimeout)
	assertStringField(t, fields, "ttl", sandbox.TTL)
	assertStringField(t, fields, "last_session_at", lastSessionAt.Format(time.RFC3339Nano))
	assertStringField(t, fields, "workload_id", workloadID.String())
}

func TestPublishSandboxUpdatedUsesNullOptionalPayloadFields(t *testing.T) {
	notifications := &recordingNotificationsClient{}
	server := &Server{notifications: notifications}
	sandbox := store.Sandbox{
		Meta: store.EntityMeta{
			ID: uuid.New(),
		},
		OrganizationID: uuid.New(),
		Name:           "brave-otter-aabbccdd",
		EnvironmentID:  uuid.New(),
		OwnerID:        uuid.New(),
		Status:         store.SandboxStatusStarting,
		IdleTimeout:    "30m",
		TTL:            "72h",
	}

	server.publishSandboxUpdated(context.Background(), sandbox)

	fields := notifications.published[0].GetPayload().GetFields()
	if _, ok := fields["workload_id"]; ok {
		t.Fatalf("expected workload_id to be omitted")
	}
	if fields["last_session_at"].GetNullValue() != 0 {
		t.Fatalf("expected last_session_at null, got %v", fields["last_session_at"])
	}
}

func TestSandboxRuntimeStateUpdatedResponsePublishesNotification(t *testing.T) {
	notifications := &recordingNotificationsClient{}
	server := &Server{notifications: notifications}
	sandbox := store.Sandbox{
		Meta:           store.EntityMeta{ID: uuid.New()},
		OrganizationID: uuid.New(),
		Name:           "brave-otter-aabbccdd",
		EnvironmentID:  uuid.New(),
		OwnerID:        uuid.New(),
		Status:         store.SandboxStatusStopped,
		IdleTimeout:    "30m",
		TTL:            "72h",
	}

	response := server.sandboxRuntimeStateUpdatedResponse(context.Background(), sandbox)

	if response.GetSandbox().GetStatus() != agentsv1.SandboxStatus_SANDBOX_STATUS_STOPPED {
		t.Fatalf("expected stopped response, got %v", response.GetSandbox().GetStatus())
	}
	if len(notifications.published) != 1 {
		t.Fatalf("expected 1 publish request, got %d", len(notifications.published))
	}
	fields := notifications.published[0].GetPayload().GetFields()
	assertStringField(t, fields, "status", string(store.SandboxStatusStopped))
}

type recordingNotificationsClient struct {
	published []*notificationsv1.PublishRequest
}

func (c *recordingNotificationsClient) Publish(_ context.Context, req *notificationsv1.PublishRequest, _ ...grpc.CallOption) (*notificationsv1.PublishResponse, error) {
	c.published = append(c.published, req)
	return &notificationsv1.PublishResponse{}, nil
}

func (c *recordingNotificationsClient) Subscribe(context.Context, *notificationsv1.SubscribeRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[notificationsv1.SubscribeResponse], error) {
	return nil, status.Error(codes.Unimplemented, "subscribe")
}

func assertStringField(t *testing.T, fields map[string]*structpb.Value, name string, expected string) {
	t.Helper()
	field, ok := fields[name]
	if !ok {
		t.Fatalf("expected payload field %q", name)
	}
	if field.GetStringValue() != expected {
		t.Fatalf("expected field %q to be %q, got %q", name, expected, field.GetStringValue())
	}
}

func assertRooms(t *testing.T, actual []string, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("expected %d rooms, got %d", len(expected), len(actual))
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("room %d: expected %q, got %q", index, expected[index], actual[index])
		}
	}
}
