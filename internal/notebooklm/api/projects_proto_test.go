package api

import (
	"reflect"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestListRecentlyViewedProjectsRequestEncoder(t *testing.T) {
	got := method.EncodeListRecentlyViewedProjectsArgs(&pb.ListRecentlyViewedProjectsRequest{})
	want := []interface{}{nil, 1, nil, []interface{}{2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("list recently viewed args = %#v, want %#v", got, want)
	}
}
