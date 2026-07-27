package api

import (
	"reflect"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestGenerateNotebookGuideRequestEncoder(t *testing.T) {
	got := method.EncodeGenerateNotebookGuideArgs(&pb.GenerateNotebookGuideRequest{
		ProjectId: "project-1",
	})
	want := []interface{}{
		"project-1",
		[]interface{}{2, nil, []interface{}{1}, []interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1, 3}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("generate guide args = %#v, want %#v", got, want)
	}
}
