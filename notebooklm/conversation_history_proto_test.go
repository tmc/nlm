package notebooklm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

func TestGetConversationHistoryChronological(t *testing.T) {
	client := New(Credentials{AuthToken: "auth", Cookies: "cookie"}, WithHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			// Actual RPC order: latest answer, latest question, earlier answer, earlier question.
			body := `[["wrb.fr","khqZz",[[["a2",null,2,"answer two"],["u2",null,1,"question two"],["a1",null,2,"answer one"],["u1",null,1,"question one"]]],null,null,null,"generic"]]`
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(")]}'\n\n" + body)), Request: req}, nil
		}),
	}))
	got, err := client.GetConversationHistory(context.Background(), "nb", "conv")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, message := range got {
		ids = append(ids, message.MessageID)
	}
	assertEquivalent(t, "chronological message IDs", []string{"u1", "a1", "u2", "a2"}, ids)
}
