package interactiveaudio

import (
	"encoding/json"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestParseInteractivityTokenResponse(t *testing.T) {
	raw := json.RawMessage(`["{\"lifetimeDuration\":\"86400s\",\"iceServers\":[{\"urls\":[\"stun:74.125.247.128:3478\"]},{\"urls\":[\"turn:74.125.247.128:3478?transport=udp\"],\"username\":\"user\",\"credential\":\"pass\"}],\"blockStatus\":\"NOT_BLOCKED\",\"iceTransportPolicy\":\"all\"}"]`)

	token, err := parseInteractivityTokenResponse(raw)
	if err != nil {
		t.Fatalf("parseInteractivityTokenResponse() error = %v", err)
	}
	if token.LifetimeDuration != "86400s" {
		t.Fatalf("LifetimeDuration = %q, want 86400s", token.LifetimeDuration)
	}
	if len(token.ICEServers) != 2 {
		t.Fatalf("got %d ice servers, want 2", len(token.ICEServers))
	}
	if token.BlockStatus != "NOT_BLOCKED" {
		t.Fatalf("BlockStatus = %q, want NOT_BLOCKED", token.BlockStatus)
	}
}

func TestInteractivityTokenCallUsesEmptyArgs(t *testing.T) {
	call := interactivityTokenCall("notebook-123")
	if call.ID != "Of0kDd" {
		t.Fatalf("ID = %q, want Of0kDd", call.ID)
	}
	if call.NotebookID != "notebook-123" {
		t.Fatalf("NotebookID = %q, want notebook-123", call.NotebookID)
	}
	if len(call.Args) != 0 {
		t.Fatalf("Args = %#v, want empty args", call.Args)
	}
}

func TestParseSDPExchangeResponse(t *testing.T) {
	raw := json.RawMessage(`["{\"sdp\":\"v=0\\r\\n\",\"type\":\"answer\"}\n"]`)

	desc, err := parseSDPExchangeResponse(raw)
	if err != nil {
		t.Fatalf("parseSDPExchangeResponse() error = %v", err)
	}
	if desc.Type != webrtc.SDPTypeAnswer {
		t.Fatalf("Type = %v, want answer", desc.Type)
	}
	if desc.SDP != "v=0\r\n" {
		t.Fatalf("SDP = %q, want %q", desc.SDP, "v=0\r\n")
	}
}

func TestSDPExchangePayloadMatchesCaptureShape(t *testing.T) {
	payload, err := json.Marshal(map[string]string{
		"sdp":  "v=0\r\n",
		"type": "offer",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	args, err := json.Marshal([]interface{}{string(payload)})
	if err != nil {
		t.Fatalf("json.Marshal(args) error = %v", err)
	}

	got := string(args)
	want := `["{\"sdp\":\"v=0\\r\\n\",\"type\":\"offer\"}"]`
	if got != want {
		t.Fatalf("args = %s, want %s", got, want)
	}
}
