package api

import (
	"encoding/json"
	"testing"

	"github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestLoadSourceRequestEncoder(t *testing.T) {
	req := &pb.LoadSourceRequest{
		Source:  &pb.SourceIdList{SourceId: "source-1"},
		Mode:    &pb.Int32List{Value: 2},
		Context: conversationRequestContext(),
	}
	got, err := json.Marshal(method.EncodeLoadSourceArgs(req))
	if err != nil {
		t.Fatal(err)
	}
	want := `[["source-1"],[2],[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]]]`
	if string(got) != want {
		t.Fatalf("load source args = %s, want %s", got, want)
	}
}
