package api

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestGetArtifactProtoCorpusProjection(t *testing.T) {
	var files []string
	err := filepath.WalkDir("/tmp/nlm-traffic", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "notebooklm.google.com.jsonl" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil || len(files) == 0 {
		t.Skip("/tmp/nlm-traffic corpus is not available")
	}

	responses := 0
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
		for record := 1; scanner.Scan(); record++ {
			var entry struct {
				Request  struct{ URL string } `json:"request"`
				Response struct {
					Content struct{ Text, Encoding string } `json:"content"`
				} `json:"response"`
			}
			if json.Unmarshal(scanner.Bytes(), &entry) != nil || !strings.Contains(entry.Request.URL, "rpcids=v9rmvd") {
				continue
			}
			body := entry.Response.Content.Text
			if entry.Response.Content.Encoding == "base64" {
				decoded, err := base64.StdEncoding.DecodeString(body)
				if err != nil {
					t.Fatalf("%s:%d: base64 response: %v", file, record, err)
				}
				body = string(decoded)
			}
			wire, err := batchexecute.DecodeResponse(body)
			if err != nil {
				continue
			}
			for _, response := range wire.Responses {
				if response.ID != "v9rmvd" || len(response.Data) == 0 {
					continue
				}
				var generated pb.Artifact
				if err := beprotojson.Unmarshal(response.Data, &generated); err != nil {
					t.Fatalf("%s:%d: proto decode: %v", file, record, err)
				}
				var raw []interface{}
				if err := json.Unmarshal(response.Data, &raw); err != nil || len(raw) != 1 {
					t.Fatalf("%s:%d: raw decode: %v", file, record, err)
				}
				legacy := (&Client{}).parseArtifactFromResponse(raw[0])
				if legacy == nil {
					t.Fatalf("%s:%d: legacy artifact is nil", file, record)
				}
				got := artifactProjectionFromProto(&generated)
				assertEquivalent(t, file, legacy, got)
				responses++
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if responses < 2 {
		t.Fatalf("v9rmvd responses=%d, want at least 2", responses)
	}
}

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

func artifactProjectionFromProto(generated *pb.Artifact) *pb.Artifact {
	if generated == nil {
		return nil
	}
	artifact := &pb.Artifact{
		ArtifactId: generated.GetArtifactId(),
		Type:       generated.GetType(),
		State:      generated.GetState(),
		Sources:    generated.GetSources(),
	}
	if artifactProtoHasDownloadURL(generated.ProtoReflect()) {
		artifact.State = pb.ArtifactState_ARTIFACT_STATE_READY
	}
	return artifact
}

func artifactProtoHasDownloadURL(message protoreflect.Message) bool {
	var hasURL func(protoreflect.Message) bool
	hasURL = func(message protoreflect.Message) bool {
		found := false
		message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
			if field.IsList() {
				for i := 0; i < value.List().Len(); i++ {
					if artifactProtoValueHasDownloadURL(field, value.List().Get(i), hasURL) {
						found = true
						return false
					}
				}
				return true
			}
			if artifactProtoValueHasDownloadURL(field, value, hasURL) {
				found = true
				return false
			}
			return true
		})
		return found
	}
	return hasURL(message)
}

func artifactProtoValueHasDownloadURL(field protoreflect.FieldDescriptor, value protoreflect.Value, hasURL func(protoreflect.Message) bool) bool {
	switch field.Kind() {
	case protoreflect.StringKind:
		return strings.HasPrefix(value.String(), artifactDownloadURLPrefix)
	case protoreflect.MessageKind:
		return hasURL(value.Message())
	}
	return false
}
