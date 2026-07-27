package rpcinfo

import (
	"errors"
	"testing"
)

func TestLookupKnownRPCIDs(t *testing.T) {
	tests := []struct {
		rpcID    string
		wantResp string // expected response message name
	}{
		{"CCqFvf", "Project"},                            // CreateProject
		{"wXbhsf", "ListRecentlyViewedProjectsResponse"}, // ListRecentlyViewedProjects
		{"hizoJc", "LoadSourceResponse"},                 // LoadSource
	}
	for _, tt := range tests {
		t.Run(tt.rpcID, func(t *testing.T) {
			m, err := Lookup(tt.rpcID)
			if err != nil {
				t.Fatalf("Lookup(%q): %v", tt.rpcID, err)
			}
			if got := string(m.Response.Descriptor().Name()); got != tt.wantResp {
				t.Errorf("response type = %q, want %q", got, tt.wantResp)
			}
			// A fresh response message must be constructible and non-nil.
			if m.NewResponse() == nil {
				t.Errorf("NewResponse() returned nil for %q", tt.rpcID)
			}
			if m.Request == nil {
				t.Errorf("request type is nil for %q", tt.rpcID)
			}
		})
	}
}

func TestLookupUnknown(t *testing.T) {
	_, err := Lookup("nope42")
	var unk ErrUnknownRPCID
	if !errors.As(err, &unk) {
		t.Fatalf("Lookup(unknown) error = %v, want ErrUnknownRPCID", err)
	}
	if unk.RPCID != "nope42" {
		t.Errorf("ErrUnknownRPCID.RPCID = %q, want nope42", unk.RPCID)
	}
}

func TestLookupAmbiguous(t *testing.T) {
	// R7cb6c is bound to both CreateAudioOverview and CreateVideoOverview.
	_, err := Lookup("R7cb6c")
	var amb ErrAmbiguousRPCID
	if !errors.As(err, &amb) {
		t.Fatalf("Lookup(R7cb6c) error = %v, want ErrAmbiguousRPCID", err)
	}
	if len(amb.Methods) < 2 {
		t.Errorf("expected >=2 ambiguous methods, got %d", len(amb.Methods))
	}
	// LookupAll must return them all.
	all, err := LookupAll("R7cb6c")
	if err != nil {
		t.Fatalf("LookupAll(R7cb6c): %v", err)
	}
	if len(all) < 2 {
		t.Errorf("LookupAll returned %d methods, want >=2", len(all))
	}
}

func TestRPCIDsNonEmpty(t *testing.T) {
	ids, err := RPCIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) < 40 {
		t.Errorf("expected >=40 bound rpc_ids, got %d", len(ids))
	}
	// Sorted and unique.
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("RPCIDs not strictly sorted at %d: %q, %q", i, ids[i-1], ids[i])
		}
	}
}
