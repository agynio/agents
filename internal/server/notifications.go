package server

import (
	"context"
	"fmt"
	"log"
	"time"

	notificationsv1 "github.com/agynio/agents/.gen/go/agynio/api/notifications/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	agentUpdatedEvent       = "agent.updated"
	environmentUpdatedEvent = "environment.updated"
	sandboxUpdatedEvent     = "sandbox.updated"

	// Flat, alongside the per-environment room: the LLM Proxy caches an
	// environment's allowlist on a connection and cannot enumerate the
	// environments it serves. Same shape as egress_rules.
	environmentsRoom = "environments"
)

func (s *Server) publishAgentUpdated(ctx context.Context, agentID uuid.UUID, organizationID uuid.UUID) {
	payload, err := structpb.NewStruct(map[string]any{
		"agent_id":        agentID.String(),
		"organization_id": organizationID.String(),
	})
	if err != nil {
		log.Printf("agents: build agent.updated payload: %v", err)
		return
	}
	_, err = s.notifications.Publish(ctx, &notificationsv1.PublishRequest{
		Event:   agentUpdatedEvent,
		Rooms:   []string{fmt.Sprintf("agent:%s", agentID)},
		Payload: payload,
		Source:  "agents",
	})
	if err != nil {
		log.Printf("agents: publish agent.updated: %v", err)
	}
}

// publishEnvironmentUpdated announces a change to an environment or anything it
// owns, on one room. Every agent and sandbox running the environment is
// affected, so fanning this out per referencing agent would be the wrong shape.
func (s *Server) publishEnvironmentUpdated(ctx context.Context, environmentID uuid.UUID, organizationID uuid.UUID) {
	payload, err := structpb.NewStruct(map[string]any{
		"environment_id":  environmentID.String(),
		"organization_id": organizationID.String(),
	})
	if err != nil {
		log.Printf("agents: build environment.updated payload: %v", err)
		return
	}
	_, err = s.notifications.Publish(ctx, &notificationsv1.PublishRequest{
		Event:   environmentUpdatedEvent,
		Rooms:   []string{fmt.Sprintf("environment:%s", environmentID), environmentsRoom},
		Payload: payload,
		Source:  "agents",
	})
	if err != nil {
		log.Printf("agents: publish environment.updated: %v", err)
	}
}

// publishVolumeTargetUpdated announces a volume write to whichever owner it
// names.
func (s *Server) publishVolumeTargetUpdated(ctx context.Context, volume store.Volume) {
	if volume.EnvironmentID != nil {
		s.publishEnvironmentUpdated(ctx, *volume.EnvironmentID, volume.OrganizationID)
	}
}

func (s *Server) publishAgentUpdatedByID(ctx context.Context, agentID uuid.UUID) {
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		log.Printf("agents: fetch agent for notification: %v", err)
		return
	}
	s.publishAgentUpdated(ctx, agent.Meta.ID, agent.OrganizationID)
}

func (s *Server) publishSandboxUpdated(ctx context.Context, sandbox store.Sandbox) {
	fields := map[string]any{
		"sandbox_id":      sandbox.Meta.ID.String(),
		"organization_id": sandbox.OrganizationID.String(),
		"name":            sandbox.Name,
		"environment_id":  sandbox.EnvironmentID.String(),
		"owner_id":        sandbox.OwnerID.String(),
		"status":          string(sandbox.Status),
		"idle_timeout":    sandbox.IdleTimeout,
		"ttl":             sandbox.TTL,
		"last_session_at": nil,
	}
	if sandbox.LastSessionAt != nil {
		fields["last_session_at"] = sandbox.LastSessionAt.UTC().Format(time.RFC3339Nano)
	}
	if sandbox.WorkloadID != nil {
		fields["workload_id"] = sandbox.WorkloadID.String()
	}
	payload, err := structpb.NewStruct(fields)
	if err != nil {
		log.Printf("agents: build sandbox.updated payload: %v", err)
		return
	}
	_, err = s.notifications.Publish(ctx, &notificationsv1.PublishRequest{
		Event: sandboxUpdatedEvent,
		Rooms: []string{
			fmt.Sprintf("sandbox_owner:%s", sandbox.OwnerID),
			fmt.Sprintf("sandbox_org:%s", sandbox.OrganizationID),
			// Keyed by the sandbox rather than its owner, so it also reaches a
			// viewer who is not the owner -- the detail view of a sandbox an
			// organization owner did not create.
			fmt.Sprintf("sandbox:%s", sandbox.Meta.ID),
		},
		Payload: payload,
		Source:  "agents",
	})
	if err != nil {
		log.Printf("agents: publish sandbox.updated: %v", err)
	}
}

func (s *Server) resolveAgentID(ctx context.Context, agentID *uuid.UUID, mcpID *uuid.UUID) (uuid.UUID, error) {
	if agentID != nil {
		return *agentID, nil
	}
	if mcpID != nil {
		mcp, err := s.store.GetMcp(ctx, *mcpID)
		if err != nil {
			return uuid.UUID{}, err
		}
		if mcp.AgentID == nil {
			return uuid.UUID{}, fmt.Errorf("mcp %s targets an environment", mcp.Meta.ID)
		}
		return *mcp.AgentID, nil
	}
	return uuid.UUID{}, fmt.Errorf("missing target identifier")
}

// publishAgentUpdatedForConfigTarget notifies the agent whose configuration the
// row is part of. A row on an environment is part of no agent's configuration —
// an environment belongs to an organization — and there is nothing to notify.
func (s *Server) publishAgentUpdatedForConfigTarget(ctx context.Context, agentID *uuid.UUID, mcpID *uuid.UUID, environmentID *uuid.UUID) {
	if environmentID != nil {
		environment, err := s.store.GetEnvironment(ctx, *environmentID)
		if err != nil {
			log.Printf("agents: fetch environment for notification: %v", err)
			return
		}
		s.publishEnvironmentUpdated(ctx, environment.Meta.ID, environment.OrganizationID)
		return
	}
	s.publishAgentUpdatedForTarget(ctx, agentID, mcpID)
}

func (s *Server) publishAgentUpdatedForTarget(ctx context.Context, agentID *uuid.UUID, mcpID *uuid.UUID) {
	resolvedID, err := s.resolveAgentID(ctx, agentID, mcpID)
	if err != nil {
		log.Printf("agents: resolve agent for notification: %v", err)
		return
	}
	s.publishAgentUpdatedByID(ctx, resolvedID)
}

const (
	instanceUpdatedEvent  = "instance.updated"
	inboxItemCreatedEvent = "message.created"
)

func (s *Server) publishInstanceUpdated(ctx context.Context, instance store.AgentInstance) {
	payload, err := structpb.NewStruct(map[string]any{
		"agent_instance_id": instance.Meta.ID.String(),
		"agent_id":          instance.AgentID.String(),
		"organization_id":   instance.OrganizationID.String(),
		"state":             string(instance.State),
	})
	if err != nil {
		log.Printf("agents: build instance.updated payload: %v", err)
		return
	}
	_, err = s.notifications.Publish(ctx, &notificationsv1.PublishRequest{
		Event:   instanceUpdatedEvent,
		Rooms:   []string{fmt.Sprintf("agent_instance:%s", instance.Meta.ID)},
		Payload: payload,
		Source:  "agents",
	})
	if err != nil {
		log.Printf("agents: publish instance.updated: %v", err)
	}
}

func (s *Server) publishInboxItemCreated(ctx context.Context, item store.InboxItem) {
	payload, err := structpb.NewStruct(map[string]any{
		"inbox_item_id":     item.ID.String(),
		"agent_instance_id": item.AgentInstanceID.String(),
		"source_kind":       string(item.SourceKind),
	})
	if err != nil {
		log.Printf("agents: build inbox message.created payload: %v", err)
		return
	}
	_, err = s.notifications.Publish(ctx, &notificationsv1.PublishRequest{
		Event:   inboxItemCreatedEvent,
		Rooms:   []string{fmt.Sprintf("instance_inbox:%s", item.AgentInstanceID)},
		Payload: payload,
		Source:  "agents",
	})
	if err != nil {
		log.Printf("agents: publish inbox message.created: %v", err)
	}
}
