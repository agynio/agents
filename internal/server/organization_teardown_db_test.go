package server

import (
	"context"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The teardown deletes across a dozen interlinked tables whose foreign keys are
// the thing being ordered against, so it is exercised against a real database
// rather than a fake store. Runs against AGENTS_TEST_DATABASE_URL -- a scratch
// database, emptied first -- and is skipped when it is unset.
func TestDeleteOrganizationResourcesClearsTheOrganization(t *testing.T) {
	ctx := context.Background()
	authz := &recordingAuthorizationWriter{}
	server, backing := environmentAuthorizationServer(ctx, t, authz)

	organizationID := uuid.New()
	otherOrganizationID := uuid.New()

	environment, err := backing.CreateEnvironment(ctx, organizationID, store.EnvironmentInput{
		Name:         "env",
		Image:        "ghcr.io/agynio/environment:latest",
		Availability: store.EnvironmentAvailabilityPrivate,
		LLMMode:      store.LLMModePlatform,
		// NOT NULL, and a nil slice encodes as NULL rather than '{}'.
		LLMAllowedModels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	agent, err := backing.CreateAgent(ctx, organizationID, store.AgentInput{
		Name:          "agent",
		EnvironmentID: &environment.Meta.ID,
		Availability:  store.AgentAvailabilityPrivate,
		DefaultThread: store.AgentDefaultThreadOrigin,
		FinalMessage:  store.AgentFinalMessageDefaultThread,
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	// A skill and a sandbox are what the delete order exists for: skills
	// reference agents and sandboxes reference environments, both NO ACTION.
	if _, err := backing.CreateSkill(ctx, store.SkillInput{
		AgentID:     agent.Meta.ID,
		Name:        "skill",
		Body:        "body",
		Description: "a skill",
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	if _, err := backing.CreateSandbox(ctx, organizationID, store.SandboxInput{
		Name:          "sandbox",
		EnvironmentID: environment.Meta.ID,
		OwnerID:       uuid.New(),
		Status:        store.SandboxStatusRunning,
		IdleTimeout:   "30m",
		TTL:           "72h",
	}); err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}

	kept, err := backing.CreateEnvironment(ctx, otherOrganizationID, store.EnvironmentInput{
		Name:         "env",
		Image:        "ghcr.io/agynio/environment:latest",
		Availability: store.EnvironmentAvailabilityPrivate,
		LLMMode:      store.LLMModePlatform,
		// NOT NULL, and a nil slice encodes as NULL rather than '{}'.
		LLMAllowedModels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateEnvironment (other org): %v", err)
	}

	// Internal RPC: no identity in the context, and none required.
	if _, err := server.DeleteOrganizationResources(ctx, &agentsv1.DeleteOrganizationResourcesRequest{
		OrganizationId: organizationID.String(),
	}); err != nil {
		t.Fatalf("DeleteOrganizationResources: %v", err)
	}

	if _, err := backing.GetAgent(ctx, agent.Meta.ID); err == nil {
		t.Fatal("expected the agent to be gone")
	}
	if _, err := backing.GetEnvironment(ctx, environment.Meta.ID); err == nil {
		t.Fatal("expected the environment to be gone")
	}
	// The other organization is untouched.
	if _, err := backing.GetEnvironment(ctx, kept.Meta.ID); err != nil {
		t.Fatalf("expected the other organization's environment to survive: %v", err)
	}

	// Tuples come off before the rows, so at least one delete-only write went
	// out ahead of the sweep.
	if len(authz.writes) == 0 {
		t.Fatal("expected authorization tuple deletes")
	}

	// The cascade retries a step it is unsure finished, so a second call has to
	// succeed on the now-empty organization.
	if _, err := server.DeleteOrganizationResources(ctx, &agentsv1.DeleteOrganizationResourcesRequest{
		OrganizationId: organizationID.String(),
	}); err != nil {
		t.Fatalf("second DeleteOrganizationResources: %v", err)
	}

	_, err = server.DeleteOrganizationResources(ctx, &agentsv1.DeleteOrganizationResourcesRequest{
		OrganizationId: "not-a-uuid",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}
