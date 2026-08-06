package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	GRPCAddress                 string
	DatabaseURL                 string
	AuthorizationServiceAddress string
	IdentityServiceAddress      string
	NotificationsServiceAddress string
	// Optional: without it, image references are stored unvalidated.
	//
	// Named GRPC_TARGET rather than SERVICE_ADDRESS deliberately: Kubernetes
	// injects IMAGES_SERVICE_HOST and IMAGES_SERVICE_PORT for the images
	// Service, and a third IMAGES_SERVICE_* would read as one of them.
	ImagesGRPCTarget string
	// How often idle instances are swept. Configurable mainly so a test
	// deployment can watch the sweep without waiting a minute for it.
	InstanceIdleGCInterval time.Duration
}

func FromEnv() (Config, error) {
	cfg := Config{}
	cfg.InstanceIdleGCInterval = time.Minute
	if raw := os.Getenv("INSTANCE_IDLE_GC_INTERVAL"); raw != "" {
		interval, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("INSTANCE_IDLE_GC_INTERVAL: %w", err)
		}
		cfg.InstanceIdleGCInterval = interval
	}
	cfg.GRPCAddress = os.Getenv("GRPC_ADDRESS")
	if cfg.GRPCAddress == "" {
		cfg.GRPCAddress = ":50051"
	}
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must be set")
	}
	cfg.AuthorizationServiceAddress = os.Getenv("AUTHORIZATION_SERVICE_ADDRESS")
	if cfg.AuthorizationServiceAddress == "" {
		cfg.AuthorizationServiceAddress = "authorization:50051"
	}
	cfg.IdentityServiceAddress = os.Getenv("IDENTITY_SERVICE_ADDRESS")
	if cfg.IdentityServiceAddress == "" {
		cfg.IdentityServiceAddress = "identity:50051"
	}
	cfg.ImagesGRPCTarget = os.Getenv("IMAGES_GRPC_TARGET")
	if cfg.ImagesGRPCTarget == "" {
		cfg.ImagesGRPCTarget = "images:50051"
	}
	cfg.NotificationsServiceAddress = os.Getenv("NOTIFICATIONS_SERVICE_ADDRESS")
	if cfg.NotificationsServiceAddress == "" {
		cfg.NotificationsServiceAddress = "notifications:50051"
	}
	return cfg, nil
}
