package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	imagesv1 "github.com/agynio/agents/.gen/go/agynio/api/images/v1"
	notificationsv1 "github.com/agynio/agents/.gen/go/agynio/api/notifications/v1"
	organizationsv1 "github.com/agynio/agents/.gen/go/agynio/api/organizations/v1"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/agynio/agents/internal/config"
	"github.com/agynio/agents/internal/db"
	"github.com/agynio/agents/internal/server"
	"github.com/agynio/agents/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("agents-service: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("create connection pool: %w", err)
	}
	defer pool.Close()

	if err := db.ApplyMigrations(ctx, pool); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	grpcServer := grpc.NewServer()
	authzConn, err := grpc.NewClient(cfg.AuthorizationServiceAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to authorization service: %w", err)
	}
	defer func() {
		_ = authzConn.Close()
	}()
	authzClient := authorizationv1.NewAuthorizationServiceClient(authzConn)
	identityConn, err := grpc.NewClient(cfg.IdentityServiceAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to identity service: %w", err)
	}
	defer func() {
		_ = identityConn.Close()
	}()
	identity := server.NewIdentityWriter(identityConn)
	notificationsConn, err := grpc.NewClient(cfg.NotificationsServiceAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to notifications service: %w", err)
	}
	defer func() {
		_ = notificationsConn.Close()
	}()
	notificationsClient := notificationsv1.NewNotificationsServiceClient(notificationsConn)

	agentsService := server.New(store.New(pool), authzClient, identity, notificationsClient)

	// Optional so an install without the catalog still starts: references are
	// then stored unvalidated and resolved at workload start, as before.
	if cfg.ImagesGRPCTarget != "" {
		imagesConn, err := grpc.NewClient(cfg.ImagesGRPCTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("dial images: %w", err)
		}
		defer func() {
			_ = imagesConn.Close()
		}()
		agentsService.WithImages(imagesv1.NewImagesServiceClient(imagesConn))
	}

	// Required: CreateSandbox reads the organization's sandbox lifecycle bounds
	// rather than assuming a platform-wide number.
	organizationsConn, err := grpc.NewClient(cfg.OrganizationsGRPCTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial organizations: %w", err)
	}
	defer func() {
		_ = organizationsConn.Close()
	}()
	agentsService.WithOrganizations(organizationsv1.NewOrganizationsServiceClient(organizationsConn))

	agentsv1.RegisterAgentsServiceServer(grpcServer, agentsService)

	// Environments predating the environment authorization type carry no tuples,
	// so every check against them refuses. Idempotent, and runs before serving.
	agentsService.BackfillEnvironmentAuthorization(ctx)

	// Instances whose class sets an idle limit are paused once they pass it.
	// Started before Serve so a service that never receives a request still
	// reclaims what earlier runs left behind.
	go agentsService.RunInstanceIdleGC(ctx, cfg.InstanceIdleGCInterval)

	lis, err := net.Listen("tcp", cfg.GRPCAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.GRPCAddress, err)
	}

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	log.Printf("AgentsService listening on %s", cfg.GRPCAddress)

	if err := grpcServer.Serve(lis); err != nil {
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
