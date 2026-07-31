package server

import (
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
)

// CreateInstance is the single enforcement point for the class policy: both
// creation paths funnel through it and it already holds the class definition,
// so Threads reports the circumstances and this decides what they mean.
func TestResolveDefaultThread(t *testing.T) {
	originThreadID := uuid.New()
	explicitThreadID := uuid.New()

	tests := []struct {
		name   string
		policy store.AgentDefaultThread
		req    *agentsv1.CreateInstanceRequest
		want   *uuid.UUID
	}{
		{
			name:   "origin takes the thread that added the instance",
			policy: store.AgentDefaultThreadOrigin,
			req: &agentsv1.CreateInstanceRequest{
				Context: &agentsv1.CreateInstanceContext{ThreadId: protoString(originThreadID.String())},
			},
			want: &originThreadID,
		},
		{
			// "Do not infer a destination from whichever thread happened to add
			// this instance" -- not "may never have one".
			name:   "none infers nothing",
			policy: store.AgentDefaultThreadNone,
			req: &agentsv1.CreateInstanceRequest{
				Context: &agentsv1.CreateInstanceContext{ThreadId: protoString(originThreadID.String())},
			},
			want: nil,
		},
		{
			name:   "an explicit thread beats the policy",
			policy: store.AgentDefaultThreadNone,
			req: &agentsv1.CreateInstanceRequest{
				DefaultThreadId: protoString(explicitThreadID.String()),
				Context:         &agentsv1.CreateInstanceContext{ThreadId: protoString(originThreadID.String())},
			},
			want: &explicitThreadID,
		},
		{
			name:   "an explicit thread beats an origin context too",
			policy: store.AgentDefaultThreadOrigin,
			req: &agentsv1.CreateInstanceRequest{
				DefaultThreadId: protoString(explicitThreadID.String()),
				Context:         &agentsv1.CreateInstanceContext{ThreadId: protoString(originThreadID.String())},
			},
			want: &explicitThreadID,
		},
		{
			// agyn agents instantiate reports no circumstances at all.
			name:   "no context leaves the default unset",
			policy: store.AgentDefaultThreadOrigin,
			req:    &agentsv1.CreateInstanceRequest{},
			want:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveDefaultThread(store.Agent{DefaultThread: test.policy}, test.req)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			switch {
			case test.want == nil && got != nil:
				t.Fatalf("expected no default thread, got %s", got)
			case test.want != nil && got == nil:
				t.Fatalf("expected %s, got none", test.want)
			case test.want != nil && *got != *test.want:
				t.Fatalf("expected %s, got %s", test.want, got)
			}
		})
	}
}

func TestResolveDefaultThreadRejectsMalformedIDs(t *testing.T) {
	agent := store.Agent{DefaultThread: store.AgentDefaultThreadOrigin}

	if _, err := resolveDefaultThread(agent, &agentsv1.CreateInstanceRequest{
		DefaultThreadId: protoString("not-a-uuid"),
	}); err == nil {
		t.Fatal("expected an explicit malformed thread to be rejected")
	}
	if _, err := resolveDefaultThread(agent, &agentsv1.CreateInstanceRequest{
		Context: &agentsv1.CreateInstanceContext{ThreadId: protoString("not-a-uuid")},
	}); err == nil {
		t.Fatal("expected a malformed context thread to be rejected")
	}
}

// Unspecified is what a caller that has not chosen sends, and the documented
// defaults preserve today's behaviour rather than erroring.
func TestClassPolicyDefaults(t *testing.T) {
	defaultThread, err := agentDefaultThreadFromProto(agentsv1.AgentDefaultThread_AGENT_DEFAULT_THREAD_UNSPECIFIED)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if defaultThread != store.AgentDefaultThreadOrigin {
		t.Fatalf("expected origin, got %q", defaultThread)
	}

	finalMessage, err := agentFinalMessageFromProto(agentsv1.AgentFinalMessage_AGENT_FINAL_MESSAGE_UNSPECIFIED)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if finalMessage != store.AgentFinalMessageDiscard {
		t.Fatalf("expected discard, got %q", finalMessage)
	}
}

func TestClassPolicyRoundTrips(t *testing.T) {
	for _, value := range []store.AgentDefaultThread{store.AgentDefaultThreadOrigin, store.AgentDefaultThreadNone} {
		got, err := agentDefaultThreadFromProto(agentDefaultThreadToProto(value))
		if err != nil || got != value {
			t.Fatalf("expected %q to round trip, got %q (%v)", value, got, err)
		}
	}
	for _, value := range []store.AgentFinalMessage{store.AgentFinalMessageDiscard, store.AgentFinalMessageDefaultThread} {
		got, err := agentFinalMessageFromProto(agentFinalMessageToProto(value))
		if err != nil || got != value {
			t.Fatalf("expected %q to round trip, got %q (%v)", value, got, err)
		}
	}
}
