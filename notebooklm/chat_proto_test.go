package notebooklm

import (
	"reflect"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestDeleteChatHistoryRequestEncoder(t *testing.T) {
	got := method.EncodeDeleteChatHistoryArgs(&pb.DeleteChatHistoryRequest{
		ProjectId: "project-1",
	})
	want := []interface{}{nil, nil, "project-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("delete chat history args = %#v, want %#v", got, want)
	}
}
