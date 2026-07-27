package api

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func TestGetConversationHistoryRequestEncoder(t *testing.T) {
	conversationID := "00000000-0000-4000-8000-000000000501"
	got := method.EncodeGetConversationHistoryArgs(&pb.GetConversationHistoryRequest{
		Context:        conversationRequestContext(),
		ConversationId: conversationID,
		Limit:          proto.Int32(20),
	})
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`[[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],null,null,%q,20]`, conversationID)
	if string(encoded) != want {
		t.Fatalf("conversation history args = %s, want %s", encoded, want)
	}
}

func TestConversationMessagesFromProto(t *testing.T) {
	got := conversationMessagesFromProto(&pb.GetConversationHistoryResponse{Messages: []*pb.ChatMessage{
		{MessageId: "user-1", Role: 1, Text: "Question"},
		{MessageId: "assistant-1", Role: 2, RichContent: &pb.RichContent{Segment: &pb.ContentSegment{Text: proto.String("Answer")}}},
		{MessageId: "empty", Role: 2},
	}}, nil)
	want := []ChatMessage{
		{MessageID: "user-1", Role: 1, Content: "Question"},
		{MessageID: "assistant-1", Role: 2, Content: "Answer"},
	}
	assertEquivalent(t, "conversation history projection", want, got)
}
