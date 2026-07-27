package method

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/proto"
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

func TestListRecentlyViewedProjectsFilterVariantsRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		wire string
		want string
		msg  proto.Message
	}{
		{"filter B", `[null,null,[]]`, `[null,null,[]]`, &pb.ListRecentlyViewedProjectsFilterB{}},
		{"filter C", `[[]]`, `[[],null]`, &pb.ListRecentlyViewedProjectsFilterC{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := beprotojson.Unmarshal([]byte(test.wire), test.msg); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got, err := beprotojson.Marshal(test.msg)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != test.want {
				t.Fatalf("round trip = %s, want %s", got, test.want)
			}
		})
	}
}
