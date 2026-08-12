package notebooklm

import (
	"encoding/json"
	"reflect"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

// TestUniversalAudioDetailsPromptRoundTrip pins the wire position of the audio
// steer. Custom instructions for an Audio Overview travel in
// UniversalAudioDetails.prompt (field 1) on CreateUniversalArtifact; the legacy
// CreateAudioOverviewRequest.custom_instructions path is rejected by the server.
// If prompt ever stops encoding at index 0, instructions are silently dropped and
// the audio is generated with default treatment, which is not an observable error.
func TestUniversalAudioDetailsPromptRoundTrip(t *testing.T) {
	wire := []interface{}{
		"fabricated audio steer",
		float64(1),
		nil,
		nil,
		"en",
		nil,
		float64(2),
	}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}

	var got pb.UniversalAudioDetails
	if err := beprotojson.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetPrompt() != "fabricated audio steer" {
		t.Fatalf("prompt = %q, want %q", got.GetPrompt(), "fabricated audio steer")
	}
	if got.GetStyle() != 1 {
		t.Fatalf("style = %d, want 1", got.GetStyle())
	}
	if got.GetLanguage() != "en" {
		t.Fatalf("language = %q, want %q", got.GetLanguage(), "en")
	}
	if got.GetEnabled() != 2 {
		t.Fatalf("enabled = %d, want 2", got.GetEnabled())
	}

	encoded, err := beprotojson.Marshal(&got)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip interface{}
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roundTrip, wire) {
		t.Fatalf("round trip = %#v, want %#v", roundTrip, wire)
	}
}
