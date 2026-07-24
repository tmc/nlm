package method

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestEncodeCancelGenerationArgs(t *testing.T) {
	got, err := json.Marshal(EncodeCancelGenerationArgs(&pb.CancelGenerationRequest{GenerationId: "generation-id"}))
	if err != nil {
		t.Fatal(err)
	}
	const want = `[null,"generation-id"]`
	if string(got) != want {
		t.Fatalf("EncodeCancelGenerationArgs() = %s, want %s", got, want)
	}
}
