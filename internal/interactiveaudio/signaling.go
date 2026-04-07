package interactiveaudio

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pion/webrtc/v4"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
)

// InteractivityToken is the decoded response from Of0kDd.
type InteractivityToken struct {
	LifetimeDuration   string      `json:"lifetimeDuration"`
	ICEServers         []ICEServer `json:"iceServers"`
	BlockStatus        string      `json:"blockStatus"`
	ICETransportPolicy string      `json:"iceTransportPolicy"`
}

// ICEServer is one ICE server entry from the interactivity token.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

func newRPCClient(authToken, cookies string, debug bool) *rpc.Client {
	var opts []batchexecute.Option
	if debug {
		opts = append(opts, batchexecute.WithDebug(true))
	}
	return rpc.New(authToken, cookies, opts...)
}

func fetchInteractivityToken(client *rpc.Client, notebookID string) (*InteractivityToken, error) {
	raw, err := client.Do(rpc.Call{
		ID:         rpc.RPCFetchInteractivityToken,
		NotebookID: notebookID,
		Args:       []interface{}{"[]"},
	})
	if err != nil {
		return nil, fmt.Errorf("fetch interactivity token: %w", err)
	}
	return parseInteractivityTokenResponse(raw)
}

func exchangeSDP(client *rpc.Client, notebookID, offerSDP string) (webrtc.SessionDescription, error) {
	payload, err := json.Marshal([]map[string]string{{
		"sdp":  offerSDP,
		"type": "offer",
	}})
	if err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("marshal sdp offer: %w", err)
	}

	raw, err := client.Do(rpc.Call{
		ID:         rpc.RPCSDPExchange,
		NotebookID: notebookID,
		Args:       []interface{}{string(payload)},
	})
	if err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("exchange sdp: %w", err)
	}
	return parseSDPExchangeResponse(raw)
}

func parseInteractivityTokenResponse(raw json.RawMessage) (*InteractivityToken, error) {
	text, err := decodeStringPayload(raw)
	if err != nil {
		return nil, err
	}

	var token InteractivityToken
	if err := json.Unmarshal([]byte(text), &token); err != nil {
		return nil, fmt.Errorf("decode interactivity token: %w", err)
	}
	return &token, nil
}

func parseSDPExchangeResponse(raw json.RawMessage) (webrtc.SessionDescription, error) {
	text, err := decodeStringPayload(raw)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	var answer struct {
		SDP  string `json:"sdp"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("decode sdp answer: %w", err)
	}

	sdpType, err := parseSDPType(answer.Type)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}
	return webrtc.SessionDescription{
		Type: sdpType,
		SDP:  answer.SDP,
	}, nil
}

func decodeStringPayload(raw json.RawMessage) (string, error) {
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil && len(values) > 0 {
		return strings.TrimSpace(values[0]), nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value), nil
	}

	return "", fmt.Errorf("unexpected signaling response: %s", string(raw))
}

func parseSDPType(s string) (webrtc.SDPType, error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "offer":
		return webrtc.SDPTypeOffer, nil
	case "pranswer":
		return webrtc.SDPTypePranswer, nil
	case "answer":
		return webrtc.SDPTypeAnswer, nil
	case "rollback":
		return webrtc.SDPTypeRollback, nil
	default:
		return webrtc.SDPType(0), fmt.Errorf("unknown sdp type %q", s)
	}
}

func toWebRTCICEServers(servers []ICEServer) []webrtc.ICEServer {
	out := make([]webrtc.ICEServer, 0, len(servers))
	for _, server := range servers {
		out = append(out, webrtc.ICEServer{
			URLs:       append([]string(nil), server.URLs...),
			Username:   server.Username,
			Credential: server.Credential,
		})
	}
	return out
}
