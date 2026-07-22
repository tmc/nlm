package api

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/proto"
)

func TestProjectDetailsProtoAdapterMatchesLegacyParser(t *testing.T) {
	raw := []byte(`[[["owner@example.com",1,[],["Travis Cline","https://example.com/avatar.png"]]],[true,true],1000,true]`)
	var wire pb.ProjectDetails
	if err := beprotojson.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := projectDetailsFromProto(&wire)
	want := &pb.ProjectDetails{OwnerName: "Travis Cline", IsPublic: true}
	assertEquivalent(t, "sharing adaptation", want, got)
}

func TestProjectDetailsProtoAdapterPrivateFlags(t *testing.T) {
	got := projectDetailsFromProto(&pb.ProjectDetails{
		Flags: &pb.ProjectDetailsFlags{Flag_0: proto.Bool(true)},
	})
	if !got.GetIsPublic() {
		t.Fatal("flag_0 should preserve private-response fallback")
	}
}
