package interactiveaudio

import (
	"strings"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestNewNotebookLMAPIOfferShape(t *testing.T) {
	api, err := newNotebookLMAPI()
	if err != nil {
		t.Fatalf("newNotebookLMAPI() error = %v", err)
	}

	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection() error = %v", err)
	}
	defer pc.Close()

	if _, err := pc.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendrecv},
	); err != nil {
		t.Fatalf("AddTransceiverFromKind() error = %v", err)
	}
	if _, err := pc.CreateDataChannel("data-channel", nil); err != nil {
		t.Fatalf("CreateDataChannel() error = %v", err)
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer() error = %v", err)
	}

	for _, want := range []string{
		"m=audio 9 UDP/TLS/RTP/SAVPF 111 63 9 0 8 13 110 126",
		"urn:ietf:params:rtp-hdrext:ssrc-audio-level",
		"http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time",
		"http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01",
		"urn:ietf:params:rtp-hdrext:sdes:mid",
		"a=rtcp-fb:111 transport-cc",
	} {
		if !strings.Contains(offer.SDP, want) {
			t.Fatalf("offer missing %q\n%s", want, offer.SDP)
		}
	}
	for _, notWant := range []string{
		"a=rtcp-fb:9 transport-cc",
		"a=rtcp-fb:0 transport-cc",
		"a=rtcp-fb:8 transport-cc",
	} {
		if strings.Contains(offer.SDP, notWant) {
			t.Fatalf("offer unexpectedly contains %q\n%s", notWant, offer.SDP)
		}
	}
}

func TestNormalizeOfferForNotebookLM(t *testing.T) {
	raw := strings.Join([]string{
		"v=0",
		"o=- 123 456 IN IP4 0.0.0.0",
		"s=-",
		"t=0 0",
		"a=group:BUNDLE 0 1",
		"a=msid-semantic: WMS *",
		"m=audio 9 UDP/TLS/RTP/SAVPF 111 63 9 0 8 13 110 126",
		"c=IN IP4 0.0.0.0",
		"a=ice-ufrag:ufrag",
		"a=ice-pwd:pwd",
		"a=sendrecv",
		"a=msid:stream track",
		"a=ssrc:1 cname:audio",
		"a=ssrc:1 msid:stream track",
		"a=ssrc:1 mslabel:stream",
		"a=ssrc:1 label:track",
		"a=candidate:1 1 udp 1 host.local 5000 typ host",
		"a=candidate:9 1 udp 1 10.0.0.2 4000 typ host ufrag abc",
		"a=candidate:2 1 udp 1 203.0.113.2 6000 typ srflx raddr 0.0.0.0 rport 0",
		"a=candidate:3 1 udp 1 10.8.64.9 31197 typ relay raddr 203.0.113.2 rport 6000",
		"a=candidate:1 2 udp 1 host.local 5000 typ host",
		"a=end-of-candidates",
		"m=application 9 UDP/DTLS/SCTP webrtc-datachannel",
		"c=IN IP4 0.0.0.0",
		"a=sendrecv",
		"a=sctp-port:5000",
		"a=max-message-size:1073741823",
		"a=ice-ufrag:ufrag",
		"a=ice-pwd:pwd",
		"a=candidate:10 1 udp 1 203.0.113.3 7000 typ srflx raddr 0.0.0.0 rport 0",
		"a=candidate:11 1 udp 1 10.8.192.5 9148 typ relay raddr 203.0.113.3 rport 7000",
	}, "\r\n") + "\r\n"

	got := normalizeOfferForNotebookLM(raw)
	for _, want := range []string{
		"o=- 123 456 IN IP4 127.0.0.1",
		"a=msid-semantic: WMS stream",
		"m=audio 31197 UDP/TLS/RTP/SAVPF 111 63 9 0 8 13 110 126",
		"c=IN IP4 10.8.64.9",
		"a=rtcp:9 IN IP4 0.0.0.0",
		"a=ice-options:trickle",
		"m=application 9148 UDP/DTLS/SCTP webrtc-datachannel",
		"c=IN IP4 10.8.192.5",
		"a=max-message-size:262144",
		"a=candidate:3 1 udp 1 10.8.64.9 31197 typ relay raddr 203.0.113.2 rport 6000",
		"a=candidate:11 1 udp 1 10.8.192.5 9148 typ relay raddr 203.0.113.3 rport 7000",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized offer missing %q\n%s", want, got)
		}
	}
	for _, notWant := range []string{
		"a=candidate:1 2 udp 1 host.local 5000 typ host",
		"a=end-of-candidates",
		"m=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\na=sendrecv",
		"a=max-message-size:1073741823",
		"a=ssrc:1 mslabel:stream",
		"a=ssrc:1 label:track",
		"a=candidate:9 1 udp 1 10.0.0.2 4000 typ host ufrag abc",
	} {
		if strings.Contains(got, notWant) {
			t.Fatalf("normalized offer unexpectedly contains %q\n%s", notWant, got)
		}
	}
	if strings.Count(got, "a=ice-options:trickle") != 2 {
		t.Fatalf("got %d ice-options lines, want 2\n%s", strings.Count(got, "a=ice-options:trickle"), got)
	}
	for _, want := range []string{
		"a=group:BUNDLE 0 1\r\na=msid-semantic: WMS stream\r\nm=audio 31197 UDP/TLS/RTP/SAVPF 111 63 9 0 8 13 110 126",
		"c=IN IP4 10.8.64.9\r\na=rtcp:9 IN IP4 0.0.0.0\r\na=candidate:1 1 udp 1 host.local 5000 typ host\r\na=candidate:2 1 udp 1 203.0.113.2 6000 typ srflx raddr 0.0.0.0 rport 0\r\na=candidate:3 1 udp 1 10.8.64.9 31197 typ relay raddr 203.0.113.2 rport 6000\r\na=ice-ufrag:ufrag",
		"m=application 9148 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 10.8.192.5\r\na=candidate:10 1 udp 1 203.0.113.3 7000 typ srflx raddr 0.0.0.0 rport 0\r\na=candidate:11 1 udp 1 10.8.192.5 9148 typ relay raddr 203.0.113.3 rport 7000\r\na=ice-ufrag:ufrag",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized offer missing ordered block %q\n%s", want, got)
		}
	}
}
