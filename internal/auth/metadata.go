package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

const (
	MetaIdentityID   = "x-identity-id"
	MetaIdentityType = "x-identity-type"
)

type RequestIdentity struct {
	IdentityID   uuid.UUID
	IdentityType string
}

type identityKey struct{}

func ExtractIdentity(ctx context.Context) (RequestIdentity, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return RequestIdentity{}, fmt.Errorf("metadata missing")
	}

	identityID, err := parseMetadataUUID(md, MetaIdentityID)
	if err != nil {
		return RequestIdentity{}, fmt.Errorf("%s: %w", MetaIdentityID, err)
	}
	identityType, err := metadataValue(md, MetaIdentityType)
	if err != nil {
		return RequestIdentity{}, fmt.Errorf("%s: %w", MetaIdentityType, err)
	}

	return RequestIdentity{
		IdentityID:   identityID,
		IdentityType: identityType,
	}, nil
}

func IdentityFromContext(ctx context.Context) RequestIdentity {
	value := ctx.Value(identityKey{})
	if value == nil {
		panic("request identity missing from context")
	}
	identity, ok := value.(RequestIdentity)
	if !ok {
		panic("request identity has unexpected type")
	}
	return identity
}

func WithIdentity(ctx context.Context, id RequestIdentity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

func metadataValue(md metadata.MD, key string) (string, error) {
	values := md.Get(key)
	if len(values) != 1 {
		return "", fmt.Errorf("expected single value")
	}
	value := values[0]
	if value == "" {
		return "", fmt.Errorf("value is empty")
	}
	return value, nil
}

func parseMetadataUUID(md metadata.MD, key string) (uuid.UUID, error) {
	value, err := metadataValue(md, key)
	if err != nil {
		return uuid.UUID{}, err
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, err
	}
	return parsed, nil
}
