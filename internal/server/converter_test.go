package server

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	identityv1 "github.com/agynio/agents/.gen/go/agynio/api/identity/v1"
	notificationsv1 "github.com/agynio/agents/.gen/go/agynio/api/notifications/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToProtoVolumeIncludesTTL(t *testing.T) {
	ttl := "24h"
	volume := store.Volume{
		Meta: store.EntityMeta{
			ID:        uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Persistent:  true,
		MountPath:   "/data",
		Size:        "1Gi",
		Description: "volume",
		TTL:         &ttl,
	}

	protoVolume := toProtoVolume(volume)
	if protoVolume.Ttl == nil {
		t.Fatalf("expected ttl to be set")
	}
	if protoVolume.GetTtl() != ttl {
		t.Fatalf("expected ttl %q, got %q", ttl, protoVolume.GetTtl())
	}
}

func TestToProtoVolumeOmitsTTLWhenNil(t *testing.T) {
	volume := store.Volume{
		Meta: store.EntityMeta{
			ID:        uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Persistent:  true,
		MountPath:   "/data",
		Size:        "1Gi",
		Description: "volume",
	}

	protoVolume := toProtoVolume(volume)
	if protoVolume.Ttl != nil {
		t.Fatalf("expected ttl to be nil")
	}
}

func TestValidateSandboxName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "brave-otter"},
		{name: "sandbox1"},
		{name: "", wantErr: true},
		{name: "Brave", wantErr: true},
		{name: "brave_otter", wantErr: true},
		{name: strings.Repeat("a", 64), wantErr: true},
	}

	for _, test := range tests {
		err := validateSandboxName(test.name)
		if (err != nil) != test.wantErr {
			t.Fatalf("validateSandboxName(%q) error = %v, wantErr %v", test.name, err, test.wantErr)
		}
	}
}

func TestGenerateSandboxNameMatchesPattern(t *testing.T) {
	name, err := generateSandboxNameWithReader(bytes.NewReader([]byte{0, 0, 0xaa, 0xbb, 0xcc, 0xdd}))
	if err != nil {
		t.Fatalf("generate sandbox name: %v", err)
	}
	if err := validateSandboxName(name); err != nil {
		t.Fatalf("generated name %q failed validation: %v", name, err)
	}
	if !strings.HasSuffix(name, "-aabbccdd") {
		t.Fatalf("expected hex suffix, got %q", name)
	}
}

func TestCreateSandboxWithGeneratedNameRetriesCollisions(t *testing.T) {
	created := 0
	generated := []string{"brave-otter-00000001", "brave-otter-00000002"}
	generate := func() (string, error) {
		name := generated[created]
		created++
		return name, nil
	}
	create := func(name string) (store.Sandbox, error) {
		if name == generated[0] {
			return store.Sandbox{}, store.AlreadyExists("sandbox")
		}
		return store.Sandbox{Name: name}, nil
	}

	sandbox, err := createSandboxWithGeneratedName(2, generate, create)
	if err != nil {
		t.Fatalf("create sandbox with generated name: %v", err)
	}
	if sandbox.Name != generated[1] {
		t.Fatalf("expected retried generated name %q, got %q", generated[1], sandbox.Name)
	}
}

func TestCreateSandboxWithGeneratedNameExhaustsCollisions(t *testing.T) {
	generate := func() (string, error) { return "brave-otter-00000001", nil }
	create := func(string) (store.Sandbox, error) { return store.Sandbox{}, store.AlreadyExists("sandbox") }

	_, err := createSandboxWithGeneratedName(2, generate, create)
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected resource exhausted, got %v", err)
	}
}

func TestCreateSandboxWithGeneratedNamePropagatesNonCollision(t *testing.T) {
	wantErr := fmt.Errorf("store failed")
	generate := func() (string, error) { return "brave-otter-00000001", nil }
	create := func(string) (store.Sandbox, error) { return store.Sandbox{}, wantErr }

	_, err := createSandboxWithGeneratedName(2, generate, create)
	if err != wantErr {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestToProtoSandboxIncludesFoundationFields(t *testing.T) {
	lastSessionAt := time.Now().UTC()
	workloadID := uuid.New()
	sandbox := store.Sandbox{
		Meta: store.EntityMeta{
			ID:        uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		OrganizationID:  uuid.New(),
		Name:            "brave-otter",
		EnvironmentID:   uuid.New(),
		OwnerID:         uuid.New(),
		Status:          store.SandboxStatusRunning,
		IdleTimeout:     "30m",
		TTL:             "72h",
		LastSessionAt:   &lastSessionAt,
		EnvironmentName: "default",
		WorkloadID:      &workloadID,
	}

	protoSandbox := toProtoSandbox(sandbox)
	if protoSandbox.GetName() != sandbox.Name {
		t.Fatalf("expected name %q, got %q", sandbox.Name, protoSandbox.GetName())
	}
	if protoSandbox.GetStatus() != agentsv1.SandboxStatus_SANDBOX_STATUS_RUNNING {
		t.Fatalf("expected running status, got %v", protoSandbox.GetStatus())
	}
	if protoSandbox.GetEnvironmentName() != sandbox.EnvironmentName {
		t.Fatalf("expected environment name %q, got %q", sandbox.EnvironmentName, protoSandbox.GetEnvironmentName())
	}
	if protoSandbox.GetWorkloadId() != workloadID.String() {
		t.Fatalf("expected workload id %q, got %q", workloadID, protoSandbox.GetWorkloadId())
	}
	if protoSandbox.GetLastSessionAt() == nil {
		t.Fatalf("expected last_session_at")
	}
}

func TestCreateAgentValidatesAvailabilityBeforeIdentity(t *testing.T) {
	server := New(&store.Store{}, noopAuthorizationWriter{}, noopIdentityWriter{}, noopNotificationsClient{})

	_, err := server.CreateAgent(context.Background(), &agentsv1.CreateAgentRequest{
		OrganizationId: uuid.NewString(),
		Model:          uuid.NewString(),
		InitImage:      "ghcr.io/agynio/agent-init-codex:latest",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if !strings.Contains(err.Error(), "availability: must be internal or private") {
		t.Fatalf("expected availability error, got %v", err)
	}
}

type noopAuthorizationWriter struct{}

func (noopAuthorizationWriter) Check(context.Context, *authorizationv1.CheckRequest, ...grpc.CallOption) (*authorizationv1.CheckResponse, error) {
	return &authorizationv1.CheckResponse{Allowed: true}, nil
}

func (noopAuthorizationWriter) Write(context.Context, *authorizationv1.WriteRequest, ...grpc.CallOption) (*authorizationv1.WriteResponse, error) {
	return &authorizationv1.WriteResponse{}, nil
}

type noopIdentityWriter struct{}

func (noopIdentityWriter) RegisterIdentity(context.Context, *identityv1.RegisterIdentityRequest, ...grpc.CallOption) (*identityv1.RegisterIdentityResponse, error) {
	return &identityv1.RegisterIdentityResponse{}, nil
}

func (noopIdentityWriter) SetNickname(context.Context, *identityv1.SetNicknameRequest, ...grpc.CallOption) (*identityv1.SetNicknameResponse, error) {
	return &identityv1.SetNicknameResponse{}, nil
}

func (noopIdentityWriter) RemoveNickname(context.Context, *identityv1.RemoveNicknameRequest, ...grpc.CallOption) (*identityv1.RemoveNicknameResponse, error) {
	return &identityv1.RemoveNicknameResponse{}, nil
}

type noopNotificationsClient struct{}

func (noopNotificationsClient) Publish(context.Context, *notificationsv1.PublishRequest, ...grpc.CallOption) (*notificationsv1.PublishResponse, error) {
	return &notificationsv1.PublishResponse{}, nil
}

func (noopNotificationsClient) Subscribe(context.Context, *notificationsv1.SubscribeRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[notificationsv1.SubscribeResponse], error) {
	return nil, status.Error(codes.Unimplemented, "subscribe")
}

func TestToProtoAgentInstanceBuildsHandle(t *testing.T) {
	label := "research"
	pauseReason := "manual"
	instance := store.AgentInstance{
		Meta: store.EntityMeta{
			ID:        uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		AgentID:        uuid.New(),
		OrganizationID: uuid.New(),
		Label:          &label,
		Suffix:         label,
		State:          store.AgentInstanceStatePaused,
		PauseReason:    &pauseReason,
		LastActivityAt: time.Now(),
		Nickname:       "bob",
	}

	protoInstance := toProtoAgentInstance(instance)
	if protoInstance.GetHandle() != "@bob#research" {
		t.Fatalf("unexpected handle %q", protoInstance.GetHandle())
	}
	if protoInstance.GetState() != agentsv1.AgentInstanceState_AGENT_INSTANCE_STATE_PAUSED {
		t.Fatalf("unexpected state %v", protoInstance.GetState())
	}
	if protoInstance.GetLabel() != label {
		t.Fatalf("unexpected label %q", protoInstance.GetLabel())
	}
	if protoInstance.GetPauseReason() != pauseReason {
		t.Fatalf("unexpected pause reason %q", protoInstance.GetPauseReason())
	}
}
