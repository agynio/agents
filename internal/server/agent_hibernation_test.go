package server

import (
	"strings"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// "0s" is the default and hibernates at the turn boundary, so hibernate_after
// admits the zero idle_timeout refuses. A value at or above idle_timeout is
// accepted as well: it means the stop comes first, not that the agent is
// misconfigured.
func TestHibernateAfterValidation(t *testing.T) {
	for _, value := range []string{"0s", "30s", "5m", "1h30m", "24h"} {
		if err := validateHibernateAfter(value); err != nil {
			t.Fatalf("%q should be accepted: %v", value, err)
		}
	}
	for _, value := range []string{"", "-1s", "soon", "30", "1 hour"} {
		if err := validateHibernateAfter(value); err == nil {
			t.Fatalf("%q should be rejected", value)
		}
	}
}

func TestHibernationTTLValidation(t *testing.T) {
	for _, value := range []string{"1h", "60m", "24h", "168h", "720h"} {
		if err := validateHibernationTTL(value); err != nil {
			t.Fatalf("%q should be accepted: %v", value, err)
		}
	}
	for _, value := range []string{"", "forever", "168", "1 hour"} {
		if err := validateHibernationTTL(value); err == nil {
			t.Fatalf("%q should be rejected", value)
		}
	}
}

// Naming the minimum is the point: a refusal that only says "too short" leaves
// the caller bisecting for the bound.
func TestHibernationTTLRefusesBelowTheMinimumAndNamesIt(t *testing.T) {
	for _, value := range []string{"59m59s", "30m", "0s", "-1h"} {
		err := validateHibernationTTL(value)
		if err == nil {
			t.Fatalf("%q should be rejected", value)
		}
		if !strings.Contains(err.Error(), "at least 1h") {
			t.Fatalf("%q: expected the error to name the 1h minimum, got %q", value, err.Error())
		}
	}
}

// Both are refused before anything is written, the way idle_timeout is. The
// store here has no pool, so reaching it would panic rather than refuse.
func TestCreateAgentRefusesHibernationSettingsBeforeTheStore(t *testing.T) {
	server := New(&store.Store{}, noopAuthorizationWriter{}, noopIdentityWriter{}, noopNotificationsClient{})

	tests := []struct {
		name  string
		apply func(*agentsv1.CreateAgentRequest)
		field string
	}{
		{
			name:  "a negative hibernate_after",
			apply: func(req *agentsv1.CreateAgentRequest) { req.HibernateAfter = protoString("-1s") },
			field: "hibernate_after",
		},
		{
			name:  "a hibernation_ttl below the minimum",
			apply: func(req *agentsv1.CreateAgentRequest) { req.HibernationTtl = protoString("30m") },
			field: "hibernation_ttl: must be at least 1h",
		},
		{
			name:  "an empty hibernation_ttl",
			apply: func(req *agentsv1.CreateAgentRequest) { req.HibernationTtl = protoString("") },
			field: "hibernation_ttl",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := createAgentRequest(uuid.New(), "alpha", uuid.NewString())
			test.apply(req)

			_, err := server.CreateAgent(identityContext(uuid.New()), req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("expected InvalidArgument, got %v", err)
			}
			if !strings.Contains(err.Error(), test.field) {
				t.Fatalf("expected the error to name %q, got %q", test.field, err.Error())
			}
		})
	}
}

func TestUpdateAgentRefusesHibernationSettingsBeforeTheStore(t *testing.T) {
	server := New(&store.Store{}, noopAuthorizationWriter{}, noopIdentityWriter{}, noopNotificationsClient{})

	tests := []struct {
		name  string
		req   *agentsv1.UpdateAgentRequest
		field string
	}{
		{
			name:  "a negative hibernate_after",
			req:   &agentsv1.UpdateAgentRequest{Id: uuid.NewString(), HibernateAfter: protoString("-5m")},
			field: "hibernate_after",
		},
		{
			name:  "a hibernation_ttl below the minimum",
			req:   &agentsv1.UpdateAgentRequest{Id: uuid.NewString(), HibernationTtl: protoString("59m")},
			field: "hibernation_ttl: must be at least 1h",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := server.UpdateAgent(identityContext(uuid.New()), test.req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("expected InvalidArgument, got %v", err)
			}
			if !strings.Contains(err.Error(), test.field) {
				t.Fatalf("expected the error to name %q, got %q", test.field, err.Error())
			}
		})
	}
}

// The Orchestrator reads hibernate as the proto states it, with no presence:
// a class that turned it off must say false, not leave the field out.
func TestToProtoAgentCarriesHibernation(t *testing.T) {
	agent := store.Agent{
		Meta:           store.EntityMeta{ID: uuid.New()},
		OrganizationID: uuid.New(),
		Name:           "alpha",
		Availability:   store.AgentAvailabilityInternal,
		Hibernate:      false,
		HibernateAfter: "30s",
		HibernationTTL: "24h",
	}

	protoAgent := toProtoAgent(agent)

	if protoAgent.GetHibernate() {
		t.Fatal("expected hibernate to be off")
	}
	if protoAgent.HibernateAfter == nil || protoAgent.GetHibernateAfter() != "30s" {
		t.Fatalf("expected hibernate_after 30s, got %v", protoAgent.HibernateAfter)
	}
	if protoAgent.HibernationTtl == nil || protoAgent.GetHibernationTtl() != "24h" {
		t.Fatalf("expected hibernation_ttl 24h, got %v", protoAgent.HibernationTtl)
	}
}

// The documented defaults are what an agent that names none records, and every
// read returns them -- the Orchestrator takes a missing hibernate as off.
func TestCreateAgentDefaultsHibernation(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()
	environmentID := createTestEnvironment(ctx, t, server, organizationID, "default")

	created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha", environmentID))
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	assertHibernation(t, "created", created.GetAgent(), true, "0s", "168h")

	fetched, err := server.GetAgent(ctx, &agentsv1.GetAgentRequest{Id: created.GetAgent().GetMeta().GetId()})
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	assertHibernation(t, "fetched", fetched.GetAgent(), true, "0s", "168h")

	listed, err := server.ListAgents(ctx, &agentsv1.ListAgentsRequest{OrganizationId: organizationID.String()})
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(listed.GetAgents()) != 1 {
		t.Fatalf("expected one agent, got %d", len(listed.GetAgents()))
	}
	assertHibernation(t, "listed", listed.GetAgents()[0], true, "0s", "168h")
}

// hibernate_after above idle_timeout is accepted -- the stop comes first -- and
// the minimum hibernation_ttl is itself allowed.
func TestCreateAgentPersistsHibernationSettings(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()
	environmentID := createTestEnvironment(ctx, t, server, organizationID, "default")

	request := createAgentRequest(organizationID, "alpha", environmentID)
	request.IdleTimeout = protoString("5m")
	request.Hibernate = ptr(false)
	request.HibernateAfter = protoString("10m")
	request.HibernationTtl = protoString("1h")
	created, err := server.CreateAgent(ctx, request)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	assertHibernation(t, "created", created.GetAgent(), false, "10m", "1h")

	fetched, err := server.GetAgent(ctx, &agentsv1.GetAgentRequest{Id: created.GetAgent().GetMeta().GetId()})
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	assertHibernation(t, "fetched", fetched.GetAgent(), false, "10m", "1h")
}

func TestUpdateAgentChangesHibernationSettings(t *testing.T) {
	ctx := identityContext(uuid.New())
	server := agentEnvironmentServer(ctx, t)
	organizationID := uuid.New()
	environmentID := createTestEnvironment(ctx, t, server, organizationID, "default")
	created, err := server.CreateAgent(ctx, createAgentRequest(organizationID, "alpha", environmentID))
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agentID := created.GetAgent().GetMeta().GetId()

	// On its own, hibernate is a field the update names, not an empty update.
	updated, err := server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{Id: agentID, Hibernate: ptr(false)})
	if err != nil {
		t.Fatalf("turn hibernation off: %v", err)
	}
	assertHibernation(t, "after turning hibernation off", updated.GetAgent(), false, "0s", "168h")

	updated, err = server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{
		Id:             agentID,
		Hibernate:      ptr(true),
		HibernateAfter: protoString("30s"),
		HibernationTtl: protoString("24h"),
	})
	if err != nil {
		t.Fatalf("update hibernation: %v", err)
	}
	assertHibernation(t, "after the update", updated.GetAgent(), true, "30s", "24h")

	_, err = server.UpdateAgent(ctx, &agentsv1.UpdateAgentRequest{Id: agentID, HibernationTtl: protoString("30m")})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected a hibernation_ttl below the minimum to be refused, got %v", err)
	}
	fetched, err := server.GetAgent(ctx, &agentsv1.GetAgentRequest{Id: agentID})
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	assertHibernation(t, "after the refused update", fetched.GetAgent(), true, "30s", "24h")
}

func assertHibernation(t *testing.T, what string, agent *agentsv1.Agent, hibernate bool, after string, ttl string) {
	t.Helper()
	if agent.GetHibernate() != hibernate {
		t.Fatalf("%s: expected hibernate %t, got %t", what, hibernate, agent.GetHibernate())
	}
	if agent.HibernateAfter == nil || agent.GetHibernateAfter() != after {
		t.Fatalf("%s: expected hibernate_after %q, got %v", what, after, agent.HibernateAfter)
	}
	if agent.HibernationTtl == nil || agent.GetHibernationTtl() != ttl {
		t.Fatalf("%s: expected hibernation_ttl %q, got %v", what, ttl, agent.HibernationTtl)
	}
}
