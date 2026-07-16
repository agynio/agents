package store

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultListPageSize int32 = 50
	MaxListPageSize     int32 = 100
)

func NormalizePageSize(size int32) int32 {
	if size <= 0 {
		return DefaultListPageSize
	}
	if size > MaxListPageSize {
		return MaxListPageSize
	}
	return size
}

func EncodePageToken(id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id.String()))
}

func DecodePageToken(token string) (uuid.UUID, error) {
	if token == "" {
		return uuid.UUID{}, errors.New("empty token")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("decode token: %w", err)
	}
	value, err := uuid.Parse(string(decoded))
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("parse token: %w", err)
	}
	return value, nil
}

func EncodeInboxPageToken(acceptedAt time.Time, id uuid.UUID) string {
	value := acceptedAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func DecodeInboxPageToken(token string) (time.Time, uuid.UUID, error) {
	if token == "" {
		return time.Time{}, uuid.UUID{}, errors.New("empty token")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return time.Time{}, uuid.UUID{}, fmt.Errorf("decode token: %w", err)
	}
	acceptedAtValue, idValue, ok := strings.Cut(string(decoded), "|")
	if !ok {
		return time.Time{}, uuid.UUID{}, fmt.Errorf("missing separator")
	}
	acceptedAt, err := time.Parse(time.RFC3339Nano, acceptedAtValue)
	if err != nil {
		return time.Time{}, uuid.UUID{}, fmt.Errorf("parse accepted_at: %w", err)
	}
	id, err := uuid.Parse(idValue)
	if err != nil {
		return time.Time{}, uuid.UUID{}, fmt.Errorf("parse id: %w", err)
	}
	return acceptedAt, id, nil
}
