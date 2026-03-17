//go:build e2e
// +build e2e

package e2e

import "os"

var teamsAddr = envOrDefault("TEAMS_ADDR", "teams:50051")

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
