package api

import (
	"encoding/json"
	"reflect"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestArtifactMindMapConfigPromptRoundTrip(t *testing.T) {
	wire := []interface{}{"fabricated mind-map prompt", "en", nil, float64(1), float64(2), float64(3)}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}

	var got pb.ArtifactMindMapConfig
	if err := beprotojson.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetPrompt() != "fabricated mind-map prompt" {
		t.Fatalf("prompt = %q", got.GetPrompt())
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

func TestArtifactReportConfigMindMapDataJSONRoundTrip(t *testing.T) {
	wire := []interface{}{
		"",
		[]interface{}{float64(4), nil, "fabricated report prompt", "en", nil, nil, nil, nil, true},
		nil,
		`{"name":"root","children":[{"name":"child","children":[]}]}`,
	}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}

	var got pb.ArtifactReportConfig
	if err := beprotojson.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetMindMapDataJson() != `{"name":"root","children":[{"name":"child","children":[]}]}` {
		t.Fatalf("mind map data = %q", got.GetMindMapDataJson())
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
