package method

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestListRecentlyViewedProjectsFullRequestKeepsLiveEncoder(t *testing.T) {
	const wire = `["project-id",null,[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],null,0,[[null,null,[]]]]`
	var req pb.ListRecentlyViewedProjectsRequest
	if err := beprotojson.Unmarshal([]byte(wire), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.GetProjectId() != "project-id" || req.GetContext() == nil || req.GetUnknown_5() != 0 {
		t.Fatalf("full request did not retain its modeled fields: %+v", req)
	}
	got, err := json.Marshal(EncodeListRecentlyViewedProjectsArgs(&req))
	if err != nil {
		t.Fatal(err)
	}
	const want = `[null,1,null,[2]]`
	if string(got) != want {
		t.Fatalf("live encoder = %s, want %s", got, want)
	}
}
