package server

import (
	"os"
	"strings"
	"testing"
)

func TestAuthorizationPolicyRestrictsSandboxRuntimeStateUpdate(t *testing.T) {
	policy, err := os.ReadFile("../../charts/agents/templates/authorization-policy.yaml")
	if err != nil {
		t.Fatalf("read authorization policy: %v", err)
	}
	content := string(policy)

	assertContains(t, content, "{{ .Release.Name }}-update-sandbox-runtime-state")
	assertContains(t, content, "/agynio.api.agents.v1.AgentsService/UpdateSandboxRuntimeState")
	assertContains(t, content, "notPrincipals:")
	assertContains(t, content, "cluster.local/ns/{{ .Release.Namespace }}/sa/agents-orchestrator")

	if strings.Contains(content, "cluster.local/ns/{{ .Release.Namespace }}/sa/gateway") {
		t.Fatalf("sandbox runtime state update policy must not allow the gateway user path")
	}
}

func assertContains(t *testing.T, content string, expected string) {
	t.Helper()
	if !strings.Contains(content, expected) {
		t.Fatalf("expected authorization policy to contain %q", expected)
	}
}
