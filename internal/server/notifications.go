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
	agentUpdatedEvent   = "agent.updated"
	sandboxUpdatedEvent = "sandbox.updated"
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
		},
		Payload: payload,
		Source:  "agents",
	})
	if err != nil {
		log.Printf("agents: publish sandbox.updated: %v", err)
	}
}

func (s *Server) resolveAgentID(ctx context.Context, agentID *uuid.UUID, mcpID *uuid.UUID, hookID *uuid.UUID) (uuid.UUID, error) {
	if agentID != nil {
		return *agentID, nil
	}
	if mcpID != nil {
		mcp, err := s.store.GetMcp(ctx, *mcpID)
		if err != nil {
			return uuid.UUID{}, err
		}
		return mcp.AgentID, nil
	}
	if hookID != nil {
		hook, err := s.store.GetHook(ctx, *hookID)
		if err != nil {
			return uuid.UUID{}, err
		}
		return hook.AgentID, nil
	}
	return uuid.UUID{}, fmt.Errorf("missing target identifier")
}

func (s *Server) publishAgentUpdatedForVolume(ctx context.Context, volumeID uuid.UUID) {
	agentIDs, err := s.store.ListAgentIDsForVolume(ctx, volumeID)
	if err != nil {
		log.Printf("agents: list volume agents: %v", err)
		return
	}
	for _, agentID := range agentIDs {
		s.publishAgentUpdatedByID(ctx, agentID)
	}
}

// publishAgentUpdatedForEnv notifies the agent whose configuration the env is
// part of. An env on an environment is part of no agent's configuration — an
// environment belongs to an organization — and there is nothing to notify.
func (s *Server) publishAgentUpdatedForEnv(ctx context.Context, env store.Env) {
	if env.EnvironmentID != nil {
		return
	}
	s.publishAgentUpdatedForTarget(ctx, env.AgentID, env.McpID, env.HookID)
}

func (s *Server) publishAgentUpdatedForTarget(ctx context.Context, agentID *uuid.UUID, mcpID *uuid.UUID, hookID *uuid.UUID) {
	resolvedID, err := s.resolveAgentID(ctx, agentID, mcpID, hookID)
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
