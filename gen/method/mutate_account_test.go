package method

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestEncodeMutateAccountArgs(t *testing.T) {
	req := &pb.MutateAccountRequest{
		Update: &pb.MutateAccountUpdateEnvelope{Update: &pb.MutateAccountUpdate{Changes: []*pb.MutateAccountChange{{
			CapabilitySet: &pb.MutateAccountCapabilitySet{Pairs: []*pb.MutateAccountCapabilityPair{{
				First:  2,
				Second: 5,
			}}},
		}}}},
		Context: &pb.RequestContext{
			Version: protoInt32(2),
			Surface: &pb.RequestSurface{Value: protoInt32(1)},
			Caps:    &pb.RequestClientCaps{Version: protoInt32(1), CapabilityCodes: []int32{1, 3}},
		},
	}
	got, err := json.Marshal(EncodeMutateAccountArgs(req))
	if err != nil {
		t.Fatal(err)
	}
	const want = `[[[null,[[null,null,null,null,[null,null,null,[[2,5]]]]]]],[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]]]`
	if string(got) != want {
		t.Fatalf("EncodeMutateAccountArgs() = %s, want %s", got, want)
	}
}

func protoInt32(v int32) *int32 { return &v }
