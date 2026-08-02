package server

import (
	"os"
	"strings"
	"testing"
)

// policyFor returns the one document in the chart's policy file that names the
// given RPC. The file holds several, and asserting against the whole of it only
// worked while every policy in it happened to agree: adding one that does allow
// the Gateway made a check of the file read as a check of the wrong policy.
func policyFor(t *testing.T, method string) string {
	t.Helper()
	policy, err := os.ReadFile("../../charts/agents/templates/authorization-policy.yaml")
	if err != nil {
		t.Fatalf("read authorization policy: %v", err)
	}
	for _, document := range strings.Split(string(policy), "\n---\n") {
		if strings.Contains(document, method) {
			return document
		}
	}
	t.Fatalf("no authorization policy names %s", method)
	return ""
}

func TestAuthorizationPolicyRestrictsSandboxRuntimeStateUpdate(t *testing.T) {
	content := policyFor(t, "/agynio.api.agents.v1.AgentsService/UpdateSandboxRuntimeState")

	assertContains(t, content, "{{ .Release.Name }}-update-sandbox-runtime-state")
	assertContains(t, content, "notPrincipals:")
	assertContains(t, content, "cluster.local/ns/{{ .Release.Namespace }}/sa/agents-orchestrator")

	if strings.Contains(content, "cluster.local/ns/{{ .Release.Namespace }}/sa/gateway") {
		t.Fatalf("sandbox runtime state update policy must not allow the gateway user path")
	}
}

// PauseInstance serves an unidentified caller, which the service can only read
// as the platform acting. Nothing in the service distinguishes the Orchestrator
// from any other workload in the mesh, so this policy is what makes that true;
// the Gateway is named because a user's deliberate pause arrives through it and
// is held to can_manage in the service.
func TestAuthorizationPolicyRestrictsPauseInstance(t *testing.T) {
	content := policyFor(t, "/agynio.api.agents.v1.AgentsService/PauseInstance")

	assertContains(t, content, "{{ .Release.Name }}-pause-instance")
	assertContains(t, content, "notPrincipals:")
	assertContains(t, content, "cluster.local/ns/{{ .Release.Namespace }}/sa/agents-orchestrator")
	assertContains(t, content, "cluster.local/ns/{{ .Release.Namespace }}/sa/gateway")
}

func assertContains(t *testing.T, content string, expected string) {
	t.Helper()
	if !strings.Contains(content, expected) {
		t.Fatalf("expected authorization policy to contain %q", expected)
	}
}
