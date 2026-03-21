//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"os"

	"google.golang.org/grpc/metadata"
)

var agentsAddr = envOrDefault("AGENTS_ADDR", "agents:50051")

const (
	testOrganizationID = "11111111-1111-1111-1111-111111111111"
	testIdentityID     = "22222222-2222-2222-2222-222222222222"
	testIdentityType   = "user"
)

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func withTestIdentity(ctx context.Context) context.Context {
	md := metadata.Pairs(
		"x-identity-id", testIdentityID,
		"x-identity-type", testIdentityType,
	)
	return metadata.NewOutgoingContext(ctx, md)
}
