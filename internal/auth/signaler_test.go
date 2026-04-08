package auth

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
)

func TestBuildChooseServerRequest(t *testing.T) {
	got, err := json.Marshal(buildChooseServerRequest("notebook-123"))
	if err != nil {
		t.Fatalf("json.Marshal(buildChooseServerRequest()) error = %v", err)
	}
	want := `[[null,null,null,[9,5],null,[["tailwind"],[null,1],[[["discoveredSource"],["notebook-123"]]]],null,null,0,0]]`
	if string(got) != want {
		t.Fatalf("buildChooseServerRequest() = %s, want %s", got, want)
	}
}

func TestBuildMultiWatchForm(t *testing.T) {
	got := buildMultiWatchForm("notebook-123")

	tests := []struct {
		key  string
		want string
	}{
		{"count", "5"},
		{"ofs", "0"},
		{"req0___data__", `[[[1,[null,null,null,[9,5],null,[["tailwind"],[null,1],[[["discoveredSource"],["notebook-123"]]]],null,null,1],null,3]]]`},
		{"req1___data__", `[[[2,[null,null,null,[9,5],null,[["tailwind"],[null,1],[[["source"],["notebook-123"]]]],null,null,1],null,3]]]`},
		{"req2___data__", `[[[3,[null,null,null,[9,5],null,[["tailwind"],[1],[[["project"],["notebook-123"]]]],null,null,1],null,3]]]`},
		{"req3___data__", `[[[4,[null,null,null,[9,5],null,[["tailwind"],[null,1],[[["artifact"],["notebook-123"]]]],null,null,1],null,3]]]`},
		{"req4___data__", `[[[5,[null,null,null,[9,5],null,[["tailwind"],[null,1],[[["notes"],["notebook-123"]]]],null,null,1],null,3]]]`},
	}
	for _, tt := range tests {
		if got.Get(tt.key) != tt.want {
			t.Fatalf("%s = %q, want %q", tt.key, got.Get(tt.key), tt.want)
		}
	}

	encoded := got.Encode()
	parsed, err := url.ParseQuery(encoded)
	if err != nil {
		t.Fatalf("url.ParseQuery(%q) error = %v", encoded, err)
	}
	if parsed.Get("req4___data__") != tests[len(tests)-1].want {
		t.Fatalf("encoded form lost req4___data__")
	}
}

func TestParseBootstrapSID(t *testing.T) {
	chunk := `[[0,["c","sid-123","",8,14,30000]]]`
	body := []byte(fmt.Sprintf("%d\n%s\n", len(chunk), chunk))
	got, err := parseBootstrapSID(body)
	if err != nil {
		t.Fatalf("parseBootstrapSID() error = %v", err)
	}
	if got != "sid-123" {
		t.Fatalf("parseBootstrapSID() = %q, want sid-123", got)
	}
}

func TestParseBootstrapSIDLengthIncludesTrailingNewline(t *testing.T) {
	chunk := `[[0,["c","sid-123","",8,14,30000]]]`
	body := []byte(fmt.Sprintf("%d\n%s\n", len(chunk)+1, chunk))
	got, err := parseBootstrapSID(body)
	if err != nil {
		t.Fatalf("parseBootstrapSID() error = %v", err)
	}
	if got != "sid-123" {
		t.Fatalf("parseBootstrapSID() = %q, want sid-123", got)
	}
}

func TestParseChannelAID(t *testing.T) {
	chunk1 := `[[73,["noop"]]]`
	chunk2 := `[[74,[[[["3",[null,null,["1775602185348324"]]]]]]]]`
	body := []byte(fmt.Sprintf("%d\n%s\n%d\n%s\n", len(chunk1), chunk1, len(chunk2), chunk2))
	got, err := parseChannelAID(body, 0)
	if err != nil {
		t.Fatalf("parseChannelAID() error = %v", err)
	}
	if got != 74 {
		t.Fatalf("parseChannelAID() = %d, want 74", got)
	}
}

func TestParseChannelAIDAdjacentChunks(t *testing.T) {
	chunk1 := `[[73,["noop"]]]`
	chunk2 := `[[74,[[[["3",[null,null,["1775602185348324"]]]]]]]]`
	body := []byte(fmt.Sprintf("%d\n%s%d\n%s", len(chunk1), chunk1, len(chunk2), chunk2))
	got, err := parseChannelAID(body, 0)
	if err != nil {
		t.Fatalf("parseChannelAID() error = %v", err)
	}
	if got != 74 {
		t.Fatalf("parseChannelAID() = %d, want 74", got)
	}
}

func TestNewSignalerClientRequiresAuthorization(t *testing.T) {
	if _, err := NewSignalerClient("SAPISID=sapi", ""); err == nil {
		t.Fatalf("NewSignalerClient() error = nil, want error")
	}
}

func TestNewSignalerClientUsesExplicitAuthorization(t *testing.T) {
	got, err := NewSignalerClient("SAPISID=sapi", "Bearer token-123")
	if err != nil {
		t.Fatalf("NewSignalerClient() error = %v", err)
	}
	if got.authorization != "Bearer token-123" {
		t.Fatalf("authorization = %q, want Bearer token-123", got.authorization)
	}
}
