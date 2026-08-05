package server

import (
	"context"
	"testing"

	imagesv1 "github.com/agynio/agents/.gen/go/agynio/api/images/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeImages struct {
	response *imagesv1.ResolveVersionResponse
	err      error
	requests []*imagesv1.ResolveVersionRequest
}

func (f *fakeImages) ResolveVersion(_ context.Context, req *imagesv1.ResolveVersionRequest, _ ...grpc.CallOption) (*imagesv1.ResolveVersionResponse, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func present() *imagesv1.ResolveVersionResponse {
	return &imagesv1.ResolveVersionResponse{
		Image: &imagesv1.Image{
			Meta: &imagesv1.EntityMeta{Id: uuid.NewString()},
			Type: imagesv1.ImageType_IMAGE_TYPE_WORKSPACE,
		},
		State: imagesv1.ImageVersionState_IMAGE_VERSION_STATE_PRESENT,
	}
}

func TestParseImageReference(t *testing.T) {
	id := uuid.NewString()

	reference, err := parseImageReference(id, "1.2.3", "workspace_image")
	if err != nil || reference == nil || reference.Tag != "1.2.3" {
		t.Fatalf("reference = %+v, err = %v", reference, err)
	}

	reference, err = parseImageReference("", "", "workspace_image")
	if err != nil || reference != nil {
		t.Fatalf("an absent reference is not an error: %+v %v", reference, err)
	}

	// An id with no tag names no version, and a tag with no id names no image.
	for _, pair := range [][2]string{{id, ""}, {"", "1.2.3"}} {
		if _, err := parseImageReference(pair[0], pair[1], "workspace_image"); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("(%q, %q): code = %v, want InvalidArgument", pair[0], pair[1], status.Code(err))
		}
	}

	if _, err := parseImageReference("not-a-uuid", "1.2.3", "workspace_image"); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestValidateImageReferencePassesTheSlotAndConsumer(t *testing.T) {
	images := &fakeImages{response: present()}
	server := &Server{images: images}
	organizationID := uuid.New()
	reference := imageReference{ImageID: uuid.New(), Tag: "1.2.3"}

	if err := server.validateImageReference(context.Background(), reference, organizationID,
		imagesv1.ImageType_IMAGE_TYPE_WORKSPACE, "workspace_image"); err != nil {
		t.Fatalf("validateImageReference: %v", err)
	}
	if len(images.requests) != 1 {
		t.Fatalf("resolved %d times, want 1", len(images.requests))
	}
	request := images.requests[0]
	if request.GetConsumerOrganizationId() != organizationID.String() {
		t.Fatal("expected visibility to be enforced against the writing organization")
	}
	if request.GetRequireType() != imagesv1.ImageType_IMAGE_TYPE_WORKSPACE {
		t.Fatal("expected the slot's type to be required")
	}
	if request.GetTag() != "1.2.3" {
		t.Fatalf("tag = %q", request.GetTag())
	}
}

// A reference the organization cannot see is reported as a bad argument, not
// relayed as NotFound: the environment being written exists, the image it names
// does not.
func TestValidateImageReferenceTranslatesNotFound(t *testing.T) {
	server := &Server{images: &fakeImages{err: status.Error(codes.NotFound, "image not found")}}

	err := server.validateImageReference(context.Background(), imageReference{ImageID: uuid.New(), Tag: "1.2.3"},
		uuid.New(), imagesv1.ImageType_IMAGE_TYPE_WORKSPACE, "workspace_image")
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestValidateImageReferenceRejectsTheWrongSlot(t *testing.T) {
	server := &Server{images: &fakeImages{err: status.Error(codes.FailedPrecondition, "image is of type workspace, not agent_runtime")}}

	err := server.validateImageReference(context.Background(), imageReference{ImageID: uuid.New(), Tag: "1.2.3"},
		uuid.New(), imagesv1.ImageType_IMAGE_TYPE_AGENT_RUNTIME, "agent_runtime_image")
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

// A tag the catalog knows but which vanished upstream resolves, so a reference
// to it can be reported. Accepting one on write would create a reference
// already known to be unschedulable.
func TestValidateImageReferenceRejectsAGoneTag(t *testing.T) {
	response := present()
	response.State = imagesv1.ImageVersionState_IMAGE_VERSION_STATE_GONE
	server := &Server{images: &fakeImages{response: response}}

	err := server.validateImageReference(context.Background(), imageReference{ImageID: uuid.New(), Tag: "1.2.3"},
		uuid.New(), imagesv1.ImageType_IMAGE_TYPE_WORKSPACE, "workspace_image")
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

// Without a catalog client the service stores what it is given rather than
// refusing every write.
func TestValidateImageReferenceIsSkippedWithoutACatalog(t *testing.T) {
	server := &Server{}
	if err := server.validateImageReference(context.Background(), imageReference{ImageID: uuid.New(), Tag: "1.2.3"},
		uuid.New(), imagesv1.ImageType_IMAGE_TYPE_WORKSPACE, "workspace_image"); err != nil {
		t.Fatalf("validateImageReference: %v", err)
	}
}

// An MCP may run a purpose-built server image or a devcontainer; an agent
// runtime is neither.
func TestValidateMcpImageAcceptsMcpAndWorkspace(t *testing.T) {
	for _, imageType := range []imagesv1.ImageType{
		imagesv1.ImageType_IMAGE_TYPE_MCP,
		imagesv1.ImageType_IMAGE_TYPE_WORKSPACE,
	} {
		response := present()
		response.Image.Type = imageType
		server := &Server{images: &fakeImages{response: response}}
		if err := server.validateMcpImage(context.Background(), imageReference{ImageID: uuid.New(), Tag: "1"}, uuid.New()); err != nil {
			t.Fatalf("%v: %v", imageType, err)
		}
	}

	response := present()
	response.Image.Type = imagesv1.ImageType_IMAGE_TYPE_AGENT_RUNTIME
	server := &Server{images: &fakeImages{response: response}}
	err := server.validateMcpImage(context.Background(), imageReference{ImageID: uuid.New(), Tag: "1"}, uuid.New())
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

// A caller changing only the tag keeps the stored image.
func TestMergedReferenceAppliesAPartialUpdate(t *testing.T) {
	stored := uuid.New()
	tag := "2.0.0"

	reference, err := mergedReference(nil, &tag, &stored, "1.0.0", "workspace_image")
	if err != nil {
		t.Fatalf("mergedReference: %v", err)
	}
	if reference.ImageID != stored || reference.Tag != "2.0.0" {
		t.Fatalf("reference = %+v", reference)
	}

	// Clearing both is how a pair is removed.
	empty := ""
	reference, err = mergedReference(&empty, &empty, &stored, "1.0.0", "agent_runtime_image")
	if err != nil || reference != nil {
		t.Fatalf("reference = %+v, err = %v", reference, err)
	}
}
