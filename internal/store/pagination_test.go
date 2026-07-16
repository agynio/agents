package store

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestInboxPageTokenRoundTripIncludesAcceptedAtAndID(t *testing.T) {
	acceptedAt := time.Date(2026, 7, 16, 17, 10, 30, 123456789, time.FixedZone("offset", -7*60*60))
	id := uuid.New()

	decodedAcceptedAt, decodedID, err := DecodeInboxPageToken(EncodeInboxPageToken(acceptedAt, id))
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if !decodedAcceptedAt.Equal(acceptedAt) {
		t.Fatalf("expected accepted_at %s, got %s", acceptedAt, decodedAcceptedAt)
	}
	if decodedID != id {
		t.Fatalf("expected id %s, got %s", id, decodedID)
	}
}

func TestDecodeInboxPageTokenRejectsIDOnlyToken(t *testing.T) {
	_, _, err := DecodeInboxPageToken(EncodePageToken(uuid.New()))
	if err == nil {
		t.Fatalf("expected id-only token to be rejected")
	}
}
