package server

import (
	"context"
	"errors"
	"regexp"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	identityv1 "github.com/agynio/agents/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const generatedInstanceSuffixLength = 8

var instanceSuffixPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

func (s *Server) CreateInstance(ctx context.Context, req *agentsv1.CreateInstanceRequest) (*agentsv1.CreateInstanceResponse, error) {
	agentID, err := parseUUID(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
	}
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, toStatusError(err)
	}
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	callerUUID, err := parseUUID(callerID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	if err := s.requireAgentCanInitiate(ctx, callerUUID, agentID); err != nil {
		return nil, err
	}
	if agent.Nickname == "" {
		return nil, status.Error(codes.FailedPrecondition, "agent nickname is required to create an instance")
	}
	label := req.Label
	if label != nil && !instanceSuffixPattern.MatchString(req.GetLabel()) {
		return nil, status.Errorf(codes.InvalidArgument, "label must match %s", instanceSuffixPattern.String())
	}
	instance, err := s.createInstanceWithIdentity(ctx, agent, label)
	if err != nil {
		return nil, err
	}
	return &agentsv1.CreateInstanceResponse{Instance: toProtoAgentInstance(instance)}, nil
}

func (s *Server) createInstanceWithIdentity(ctx context.Context, agent store.Agent, label *string) (store.AgentInstance, error) {
	attempts := 1
	if label == nil {
		attempts = 5
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		suffix := generatedInstanceSuffix()
		if label != nil {
			suffix = *label
		}
		instance, err := s.store.CreateAgentInstance(ctx, store.AgentInstanceInput{
			AgentID:        agent.Meta.ID,
			OrganizationID: agent.OrganizationID,
			Label:          label,
			Suffix:         suffix,
			Nickname:       agent.Nickname,
		})
		if err != nil {
			if _, ok := err.(*store.AlreadyExistsError); ok && label == nil {
				lastErr = err
				continue
			}
			return store.AgentInstance{}, toStatusError(err)
		}
		if err := s.addAgentInstanceAuthorization(ctx, instance); err != nil {
			rollbackErr := s.rollbackCreatedInstance(ctx, instance)
			if rollbackErr != nil {
				return store.AgentInstance{}, status.Errorf(codes.Internal, "authorization write failed: %v; rollback: %v", err, rollbackErr)
			}
			return store.AgentInstance{}, status.Errorf(codes.Internal, "authorization write failed: %v", err)
		}
		if err := s.registerAgentInstanceIdentity(ctx, instance.Meta.ID); err != nil {
			rollbackErr := errors.Join(
				s.removeAgentInstanceAuthorization(ctx, instance),
				s.rollbackCreatedInstance(ctx, instance),
			)
			if rollbackErr != nil {
				return store.AgentInstance{}, status.Errorf(codes.Internal, "register instance identity: %v; rollback: %v", err, rollbackErr)
			}
			return store.AgentInstance{}, status.Errorf(codes.Internal, "register instance identity: %v", err)
		}
		if err := s.setAgentInstanceNickname(ctx, instance); err != nil {
			rollbackErr := errors.Join(
				s.removeAgentInstanceAuthorization(ctx, instance),
				s.rollbackCreatedInstance(ctx, instance),
			)
			if rollbackErr != nil {
				return store.AgentInstance{}, status.Errorf(codes.Internal, "set instance nickname: %v; rollback: %v", err, rollbackErr)
			}
			return store.AgentInstance{}, status.Errorf(codes.Internal, "set instance nickname: %v", err)
		}
		s.publishInstanceUpdated(ctx, instance)
		return instance, nil
	}
	return store.AgentInstance{}, toStatusError(lastErr)
}

func generatedInstanceSuffix() string {
	return uuid.NewString()[:generatedInstanceSuffixLength]
}

func (s *Server) rollbackCreatedInstance(ctx context.Context, instance store.AgentInstance) error {
	_, err := s.store.DeleteAgentInstance(ctx, instance.Meta.ID)
	return err
}

func (s *Server) registerAgentInstanceIdentity(ctx context.Context, instanceID uuid.UUID) error {
	_, err := s.identity.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{
		IdentityId:   instanceID.String(),
		IdentityType: identityv1.IdentityType_IDENTITY_TYPE_AGENT_INSTANCE,
	})
	return err
}

func (s *Server) setAgentInstanceNickname(ctx context.Context, instance store.AgentInstance) error {
	identityCtx, err := identityOutgoingContext(ctx)
	if err != nil {
		return err
	}
	_, err = s.identity.SetNickname(identityCtx, &identityv1.SetNicknameRequest{
		OrganizationId: instance.OrganizationID.String(),
		IdentityId:     instance.Meta.ID.String(),
		Nickname:       instance.Nickname,
		InstanceSuffix: &instance.Suffix,
	})
	return err
}

func (s *Server) GetInstance(ctx context.Context, req *agentsv1.GetInstanceRequest) (*agentsv1.GetInstanceResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	instance, err := s.store.GetAgentInstance(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.GetInstanceResponse{Instance: toProtoAgentInstance(instance)}, nil
}

func (s *Server) ListInstances(ctx context.Context, req *agentsv1.ListInstancesRequest) (*agentsv1.ListInstancesResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	filter := store.AgentInstanceFilter{}
	if req.GetOrganizationId() != "" {
		organizationID, err := parseUUID(req.GetOrganizationId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
		}
		filter.OrganizationID = &organizationID
	}
	if req.AgentId != nil {
		agentID, err := parseUUID(req.GetAgentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "agent_id: %v", err)
		}
		filter.AgentID = &agentID
	}
	for _, protoState := range req.GetStateIn() {
		state, err := agentInstanceStateFromProto(protoState)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "state_in: %v", err)
		}
		filter.StateIn = append(filter.StateIn, state)
	}
	if req.HasUnacked != nil {
		filter.HasUnacked = req.HasUnacked
	}
	result, err := s.store.ListAgentInstances(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	instances, nextToken := mapListResult(result.Instances, result.NextCursor, toProtoAgentInstance)
	return &agentsv1.ListInstancesResponse{Instances: instances, NextPageToken: nextToken}, nil
}

func (s *Server) PauseInstance(ctx context.Context, req *agentsv1.PauseInstanceRequest) (*agentsv1.PauseInstanceResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.GetPauseReason() == "" {
		return nil, status.Error(codes.InvalidArgument, "pause_reason must be provided")
	}
	if err := s.requireManageInstance(ctx, id); err != nil {
		return nil, err
	}
	instance, err := s.store.PauseAgentInstance(ctx, id, req.GetPauseReason())
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishInstanceUpdated(ctx, instance)
	return &agentsv1.PauseInstanceResponse{Instance: toProtoAgentInstance(instance)}, nil
}

func (s *Server) ResumeInstance(ctx context.Context, req *agentsv1.ResumeInstanceRequest) (*agentsv1.ResumeInstanceResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.requireManageInstance(ctx, id); err != nil {
		return nil, err
	}
	instance, err := s.store.ResumeAgentInstance(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishInstanceUpdated(ctx, instance)
	return &agentsv1.ResumeInstanceResponse{Instance: toProtoAgentInstance(instance)}, nil
}

func (s *Server) DeleteInstance(ctx context.Context, req *agentsv1.DeleteInstanceRequest) (*agentsv1.DeleteInstanceResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.requireManageInstance(ctx, id); err != nil {
		return nil, err
	}
	instance, err := s.store.GetAgentInstance(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if instance.State == store.AgentInstanceStateTerminated {
		return &agentsv1.DeleteInstanceResponse{Instance: toProtoAgentInstance(instance)}, nil
	}
	if err := s.removeAgentInstanceAuthorization(ctx, instance); err != nil {
		return nil, status.Errorf(codes.Internal, "authorization delete failed: %v", err)
	}
	removedNickname := false
	if err := s.removeAgentNickname(ctx, instance.Meta.ID, instance.OrganizationID); err != nil {
		rollbackErr := s.addAgentInstanceAuthorization(ctx, instance)
		if rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "remove instance nickname: %v; rollback: %v", err, rollbackErr)
		}
		return nil, status.Errorf(codes.Internal, "remove instance nickname: %v", err)
	}
	removedNickname = true
	instance, err = s.store.DeleteAgentInstance(ctx, id)
	if err != nil {
		rollbackErr := s.addAgentInstanceAuthorization(ctx, instance)
		if removedNickname {
			rollbackErr = errors.Join(rollbackErr, s.setAgentInstanceNickname(ctx, instance))
		}
		if rollbackErr != nil {
			return nil, status.Errorf(codes.Internal, "delete instance failed: %v; rollback: %v", err, rollbackErr)
		}
		return nil, toStatusError(err)
	}
	s.publishInstanceUpdated(ctx, instance)
	return &agentsv1.DeleteInstanceResponse{Instance: toProtoAgentInstance(instance)}, nil
}

func (s *Server) requireManageInstance(ctx context.Context, id uuid.UUID) error {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return err
	}
	callerUUID, err := parseUUID(callerID)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	return s.requireAgentInstanceCanManage(ctx, callerUUID, id)
}

func (s *Server) WriteInboxItem(ctx context.Context, req *agentsv1.WriteInboxItemRequest) (*agentsv1.WriteInboxItemResponse, error) {
	input, err := directInboxInputFromRequest(req)
	if err != nil {
		return nil, err
	}
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	callerUUID, err := parseUUID(callerID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	if input.SenderID != callerUUID {
		return nil, status.Error(codes.PermissionDenied, "sender_id must match caller identity")
	}
	if err := s.requireAgentInstanceCanWriteInbox(ctx, callerUUID, input.AgentInstanceID); err != nil {
		return nil, err
	}
	item, err := s.store.CreateInboxItem(ctx, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	s.publishInboxItemCreated(ctx, item)
	return &agentsv1.WriteInboxItemResponse{Item: toProtoInboxItem(item)}, nil
}

func (s *Server) FanoutInboxItem(ctx context.Context, req *agentsv1.FanoutInboxItemRequest) (*agentsv1.FanoutInboxItemResponse, error) {
	input, err := fanoutInboxInputFromRequest(req)
	if err != nil {
		return nil, err
	}
	item, inserted, err := s.store.FanoutInboxItem(ctx, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	if inserted {
		s.publishInboxItemCreated(ctx, item)
	}
	return &agentsv1.FanoutInboxItemResponse{Item: toProtoInboxItem(item)}, nil
}

func (s *Server) GetUnackedInboxItems(ctx context.Context, req *agentsv1.GetUnackedInboxItemsRequest) (*agentsv1.GetUnackedInboxItemsResponse, error) {
	instanceID, err := parseUUID(req.GetAgentInstanceId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_instance_id: %v", err)
	}
	if err := requireSelfInstance(ctx, instanceID); err != nil {
		return nil, err
	}
	cursor, err := decodeInboxPageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	result, err := s.store.ListUnackedInboxItems(ctx, instanceID, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	items, nextToken := mapInboxListResult(result.Items, result.NextCursor, toProtoInboxItem)
	return &agentsv1.GetUnackedInboxItemsResponse{Items: items, NextPageToken: nextToken}, nil
}

func (s *Server) AckInboxItems(ctx context.Context, req *agentsv1.AckInboxItemsRequest) (*agentsv1.AckInboxItemsResponse, error) {
	instanceID, err := parseUUID(req.GetAgentInstanceId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_instance_id: %v", err)
	}
	if err := requireSelfInstance(ctx, instanceID); err != nil {
		return nil, err
	}
	itemIDs, err := parseUUIDList(req.GetItemIds(), "item_ids")
	if err != nil {
		return nil, err
	}
	ackedCount, err := s.store.AckInboxItems(ctx, instanceID, itemIDs)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.AckInboxItemsResponse{AckedCount: ackedCount}, nil
}

func (s *Server) GetUnackedInboxCount(ctx context.Context, req *agentsv1.GetUnackedInboxCountRequest) (*agentsv1.GetUnackedInboxCountResponse, error) {
	instanceID, err := parseUUID(req.GetAgentInstanceId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "agent_instance_id: %v", err)
	}
	if err := requireSelfInstance(ctx, instanceID); err != nil {
		return nil, err
	}
	var threadID *uuid.UUID
	if req.ThreadId != nil {
		parsedThreadID, err := parseUUID(req.GetThreadId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "thread_id: %v", err)
		}
		threadID = &parsedThreadID
	}
	count, err := s.store.CountUnackedInboxItems(ctx, instanceID, threadID)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentsv1.GetUnackedInboxCountResponse{Count: count}, nil
}

func directInboxInputFromRequest(req *agentsv1.WriteInboxItemRequest) (store.InboxItemInput, error) {
	instanceID, err := parseUUID(req.GetAgentInstanceId())
	if err != nil {
		return store.InboxItemInput{}, status.Errorf(codes.InvalidArgument, "agent_instance_id: %v", err)
	}
	senderID, err := parseUUID(req.GetSenderId())
	if err != nil {
		return store.InboxItemInput{}, status.Errorf(codes.InvalidArgument, "sender_id: %v", err)
	}
	fileIDs, err := parseUUIDList(req.GetFileIds(), "file_ids")
	if err != nil {
		return store.InboxItemInput{}, err
	}
	return store.InboxItemInput{
		AgentInstanceID: instanceID,
		SourceKind:      store.InboxItemSourceKindDirect,
		SenderID:        senderID,
		Body:            req.GetBody(),
		FileIDs:         fileIDs,
	}, nil
}

func fanoutInboxInputFromRequest(req *agentsv1.FanoutInboxItemRequest) (store.InboxItemInput, error) {
	instanceID, err := parseUUID(req.GetAgentInstanceId())
	if err != nil {
		return store.InboxItemInput{}, status.Errorf(codes.InvalidArgument, "agent_instance_id: %v", err)
	}
	threadID, err := parseUUID(req.GetThreadId())
	if err != nil {
		return store.InboxItemInput{}, status.Errorf(codes.InvalidArgument, "thread_id: %v", err)
	}
	messageID, err := parseUUID(req.GetMessageId())
	if err != nil {
		return store.InboxItemInput{}, status.Errorf(codes.InvalidArgument, "message_id: %v", err)
	}
	senderID, err := parseUUID(req.GetSenderId())
	if err != nil {
		return store.InboxItemInput{}, status.Errorf(codes.InvalidArgument, "sender_id: %v", err)
	}
	fileIDs, err := parseUUIDList(req.GetFileIds(), "file_ids")
	if err != nil {
		return store.InboxItemInput{}, err
	}
	return store.InboxItemInput{
		AgentInstanceID: instanceID,
		SourceKind:      store.InboxItemSourceKindThread,
		ThreadID:        &threadID,
		MessageID:       &messageID,
		SenderID:        senderID,
		Body:            req.GetBody(),
		FileIDs:         fileIDs,
	}, nil
}

func parseUUIDList(values []string, field string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, len(values))
	for i, value := range values {
		id, err := parseUUID(value)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "%s[%d]: %v", field, i, err)
		}
		ids[i] = id
	}
	return ids, nil
}

func requireSelfInstance(ctx context.Context, instanceID uuid.UUID) error {
	identityID, err := identityIDFromContext(ctx)
	if err != nil {
		return err
	}
	if identityID != instanceID.String() {
		return status.Error(codes.PermissionDenied, "caller must be the agent instance")
	}
	return nil
}
