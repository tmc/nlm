package method

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func TestActOnSourcesRequestEncoder(t *testing.T) {
	req := &pb.ActOnSourcesRequest{
		SourceGroups: []*pb.ActOnSourcesSourceGroup{
			{Source: &pb.SourceIdList{SourceId: "source-a"}},
			{Source: &pb.SourceIdList{SourceId: "source-b"}},
		},
		Options: &pb.ActOnSourcesOptions{Unknown_7: 2, Unknown_10: 2},
		ChatOptions: &pb.ChatStreamOptions{
			Mode:          2,
			CitationModes: &pb.Int32List{Value: 1},
			FollowUp:      &pb.ChatFollowUpOptions{Enabled: 1, Modes: []int32{1, 3}},
		},
		PromptHistory: &pb.ActOnSourcesPromptHistory{
			Prompt:    "fabricated prompt",
			Unknown_3: "fabricated prompt context",
			History: []*pb.ActOnSourcesHistoryEntry{
				{Text: proto.String("fabricated answer"), Unknown_3: 2},
				{Text: proto.String("fabricated question"), Unknown_3: 1},
				{Text: proto.String(""), Unknown_3: 1},
			},
		},
		ConversationId: "00000000-0000-4000-8000-000000000001",
	}
	got, err := json.Marshal(EncodeActOnSourcesArgs(req))
	if err != nil {
		t.Fatal(err)
	}
	const want = `[[[["source-a"]],[["source-b"]]],[null,null,null,null,null,null,2,null,null,2],null,null,null,null,null,[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],["fabricated prompt",[["fabricated answer",null,2],["fabricated question",null,1],["",null,1]],"fabricated prompt context"],"00000000-0000-4000-8000-000000000001"]`
	if string(got) != want {
		t.Fatalf("EncodeActOnSourcesArgs() = %s, want %s", got, want)
	}
}
