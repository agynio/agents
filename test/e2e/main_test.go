//go:build e2e
// +build e2e

package e2e

import (
	"os"
)

var agentsAddr = envOrDefault("AGENTS_ADDR", "agents:50051")

const (
	testOrganizationID    = "11111111-1111-1111-1111-111111111111"
	testOrganizationIDAlt = "33333333-3333-3333-3333-333333333333"
)

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
