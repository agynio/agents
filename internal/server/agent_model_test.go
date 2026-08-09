package server

import (
	"testing"

	"github.com/agynio/agents/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestResolveAgentModel(t *testing.T) {
	modelID := uuid.New()

	tests := []struct {
		name          string
		mode          store.LLMMode
		model         string
		modelName     string
		wantModel     uuid.UUID
		wantModelName string
		wantCode      codes.Code
	}{
		{
			name:      "platform requires a model",
			mode:      store.LLMModePlatform,
			wantCode:  codes.InvalidArgument,
			wantModel: uuid.Nil,
		},
		{
			name:      "platform accepts a model",
			mode:      store.LLMModePlatform,
			model:     modelID.String(),
			wantModel: modelID,
		},
		{
			name:      "platform rejects a model name",
			mode:      store.LLMModePlatform,
			model:     modelID.String(),
			modelName: "claude-sonnet-4-6",
			wantCode:  codes.InvalidArgument,
		},
		{
			name:     "native rejects a model",
			mode:     store.LLMModeNative,
			model:    modelID.String(),
			wantCode: codes.InvalidArgument,
		},
		// The common case, and always the case for a sandbox: the CLI keeps
		// its own default and its own picker.
		{
			name: "native accepts neither",
			mode: store.LLMModeNative,
		},
		{
			name:          "native accepts a model name",
			mode:          store.LLMModeNative,
			modelName:     "claude-sonnet-4-6",
			wantModelName: "claude-sonnet-4-6",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, modelName, err := resolveAgentModel(test.mode, test.model, test.modelName)
			if test.wantCode != codes.OK {
				if status.Code(err) != test.wantCode {
					t.Fatalf("expected %v, got %v", test.wantCode, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve agent model: %v", err)
			}
			if model != test.wantModel {
				t.Fatalf("expected model %s, got %s", test.wantModel, model)
			}
			if modelName != test.wantModelName {
				t.Fatalf("expected model name %q, got %q", test.wantModelName, modelName)
			}
		})
	}
}

// Whitespace is not a model reference, so it must fail the same way absence
// does rather than reaching the store.
func TestResolveAgentModelTreatsBlankAsUnset(t *testing.T) {
	if _, _, err := resolveAgentModel(store.LLMModePlatform, "   ", ""); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	_, modelName, err := resolveAgentModel(store.LLMModeNative, "", "   ")
	if err != nil {
		t.Fatalf("resolve agent model: %v", err)
	}
	if modelName != "" {
		t.Fatalf("expected empty model name, got %q", modelName)
	}
}
