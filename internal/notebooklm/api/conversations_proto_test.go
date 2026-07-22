package api

import (
	"encoding/json"
	"reflect"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestGetConversationsProtoAdapterMatchesLegacyParser(t *testing.T) {
	raw := []byte(`[[["conversation-1"],["conversation-2"]]]`)
	legacy, err := parseConversationIDsLegacy(raw)
	if err != nil {
		t.Fatalf("legacy parser: %v", err)
	}
	var response pb.GetConversationsResponse
	if err := beprotojson.Unmarshal(raw, &response); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := conversationIDsFromProto(&response)
	if !reflect.DeepEqual(legacy, got) {
		t.Fatalf("conversation IDs mismatch\n legacy: %#v\n proto: %#v", legacy, got)
	}
}

func TestGetConversationsRequestEncoder(t *testing.T) {
	got := method.EncodeGetConversationsArgs(&pb.GetConversationsRequest{
		Context:   &pb.RequestContext{},
		ProjectId: "project-1",
		Limit:     20,
	})
	want := []interface{}{[]interface{}{}, nil, "project-1", int64(20)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("get conversations args = %#v (%T), want %#v (%T)", got, got[3], want, want[3])
	}
}

func parseConversationIDsLegacy(raw []byte) ([]string, error) {
	var data []interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	ids := []string{}
	if len(data) > 0 {
		if outer, ok := data[0].([]interface{}); ok {
			for _, item := range outer {
				if arr, ok := item.([]interface{}); ok && len(arr) > 0 {
					if id, ok := arr[0].(string); ok {
						ids = append(ids, id)
					}
				}
			}
		}
	}
	return ids, nil
}
