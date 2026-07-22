package api

import (
	"reflect"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestGetConversationsProtoAdapter(t *testing.T) {
	raw := []byte(`[[["conversation-1"],["conversation-2"]]]`)
	var response pb.GetConversationsResponse
	if err := beprotojson.Unmarshal(raw, &response); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := conversationIDsFromProto(&response)
	want := []string{"conversation-1", "conversation-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("conversation IDs = %#v, want %#v", got, want)
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
