package server

import (
	"testing"
	"time"

	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
)

func TestToProtoVolumeIncludesTTL(t *testing.T) {
	ttl := "24h"
	volume := store.Volume{
		Meta: store.EntityMeta{
			ID:        uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Persistent:  true,
		MountPath:   "/data",
		Size:        "1Gi",
		Description: "volume",
		TTL:         &ttl,
	}

	protoVolume := toProtoVolume(volume)
	if protoVolume.Ttl == nil {
		t.Fatalf("expected ttl to be set")
	}
	if protoVolume.GetTtl() != ttl {
		t.Fatalf("expected ttl %q, got %q", ttl, protoVolume.GetTtl())
	}
}

func TestToProtoVolumeOmitsTTLWhenNil(t *testing.T) {
	volume := store.Volume{
		Meta: store.EntityMeta{
			ID:        uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Persistent:  true,
		MountPath:   "/data",
		Size:        "1Gi",
		Description: "volume",
	}

	protoVolume := toProtoVolume(volume)
	if protoVolume.Ttl != nil {
		t.Fatalf("expected ttl to be nil")
	}
}
