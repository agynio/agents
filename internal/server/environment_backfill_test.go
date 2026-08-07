package server

import (
	"context"
	"errors"
	"testing"

	authorizationv1 "github.com/agynio/agents/.gen/go/agynio/api/authorization/v1"
	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

// OpenFGA refuses to write a tuple that already exists, and refuses the whole
// batch when one of them does. A repair pass runs on every startup, so the
// second run must not fail on what the first wrote.
type existingTupleWriter struct {
	existing map[string]bool
	attempts int
}

func (w *existingTupleWriter) Check(context.Context, *authorizationv1.CheckRequest, ...grpc.CallOption) (*authorizationv1.CheckResponse, error) {
	return &authorizationv1.CheckResponse{Allowed: true}, nil
}

func (w *existingTupleWriter) Write(_ context.Context, req *authorizationv1.WriteRequest, _ ...grpc.CallOption) (*authorizationv1.WriteResponse, error) {
	w.attempts++
	for _, tuple := range req.GetWrites() {
		key := tuple.GetUser() + "|" + tuple.GetRelation() + "|" + tuple.GetObject()
		if w.existing[key] {
			return nil, errors.New("cannot write a tuple which already exists: " + key)
		}
		w.existing[key] = true
	}
	return &authorizationv1.WriteResponse{}, nil
}

func TestEnvironmentBackfillIsIdempotent(t *testing.T) {
	authz := &existingTupleWriter{existing: map[string]bool{}}
	server := &Server{authz: authz}
	environmentID := uuid.New()
	organizationID := uuid.New()

	for run := 1; run <= 2; run++ {
		if err := server.addEnvironmentBaseAuthorization(context.Background(), environmentID, organizationID, store.EnvironmentAvailabilityInternal); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}
	if len(authz.existing) != 2 {
		t.Fatalf("expected the org and internal_access tuples, got %d", len(authz.existing))
	}
}

// A batch write would leave an environment holding one tuple unable to gain the
// other, because the batch fails on the one that exists.
func TestEnvironmentBackfillRepairsAPartialEnvironment(t *testing.T) {
	environmentID := uuid.New()
	organizationID := uuid.New()
	orgTuple := environmentOrganizationTuple(environmentID, organizationID)
	authz := &existingTupleWriter{existing: map[string]bool{
		orgTuple.GetUser() + "|" + orgTuple.GetRelation() + "|" + orgTuple.GetObject(): true,
	}}
	server := &Server{authz: authz}

	if err := server.addEnvironmentBaseAuthorization(context.Background(), environmentID, organizationID, store.EnvironmentAvailabilityInternal); err != nil {
		t.Fatalf("repair: %v", err)
	}
	internalAccess := environmentInternalAccessTuple(environmentID, organizationID)
	key := internalAccess.GetUser() + "|" + internalAccess.GetRelation() + "|" + internalAccess.GetObject()
	if !authz.existing[key] {
		t.Fatal("expected the missing internal_access tuple to be written")
	}
}
