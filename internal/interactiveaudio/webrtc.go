package interactiveaudio

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
)

func newNotebookLMPeerConnection(config webrtc.Configuration) (*webrtc.PeerConnection, error) {
	api, err := newNotebookLMAPI()
	if err != nil {
		return nil, err
	}
	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("create peer connection: %w", err)
	}
	return pc, nil
}

func newNotebookLMAPI() (*webrtc.API, error) {
	mediaEngine := &webrtc.MediaEngine{}
	if err := registerNotebookLMAudioCodecs(mediaEngine); err != nil {
		return nil, err
	}
	if err := registerNotebookLMAudioExtensions(mediaEngine); err != nil {
		return nil, err
	}

	var settingEngine webrtc.SettingEngine
	settingEngine.SetSDPMediaLevelFingerprints(true)

	return webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(&interceptor.Registry{}),
		webrtc.WithSettingEngine(settingEngine),
	), nil
}

func registerNotebookLMAudioCodecs(mediaEngine *webrtc.MediaEngine) error {
	codecs := []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeOpus,
				ClockRate:    48000,
				Channels:     2,
				SDPFmtpLine:  "minptime=10;useinbandfec=1",
				RTCPFeedback: []webrtc.RTCPFeedback{{Type: "transport-cc"}},
			},
			PayloadType: 111,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    "audio/red",
				ClockRate:   48000,
				Channels:    2,
				SDPFmtpLine: "111/111",
			},
			PayloadType: 63,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeG722, ClockRate: 8000},
			PayloadType:        9,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000},
			PayloadType:        0,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMA, ClockRate: 8000},
			PayloadType:        8,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: "audio/CN", ClockRate: 8000},
			PayloadType:        13,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: "audio/telephone-event", ClockRate: 48000},
			PayloadType:        110,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: "audio/telephone-event", ClockRate: 8000},
			PayloadType:        126,
		},
	}

	for _, codec := range codecs {
		if err := mediaEngine.RegisterCodec(codec, webrtc.RTPCodecTypeAudio); err != nil {
			return fmt.Errorf("register audio codec %d: %w", codec.PayloadType, err)
		}
	}
	return nil
}

func registerNotebookLMAudioExtensions(mediaEngine *webrtc.MediaEngine) error {
	extensions := []string{
		"urn:ietf:params:rtp-hdrext:ssrc-audio-level",
		"http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time",
		"http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01",
		"urn:ietf:params:rtp-hdrext:sdes:mid",
	}
	for _, uri := range extensions {
		if err := mediaEngine.RegisterHeaderExtension(
			webrtc.RTPHeaderExtensionCapability{URI: uri},
			webrtc.RTPCodecTypeAudio,
		); err != nil {
			return fmt.Errorf("register audio header extension %q: %w", uri, err)
		}
	}
	return nil
}

func normalizeOfferForNotebookLM(sdp string) string {
	sessionLines, mediaSections := splitSDPSections(sdp)
	sessionLines = normalizeSessionLines(sessionLines, extractPrimaryStreamID(mediaSections))

	audioCandidates := []string(nil)
	for i, section := range mediaSections {
		if mediaSectionKind(section) != "audio" {
			continue
		}
		mediaSections[i] = normalizeMediaSection(section, nil)
		audioCandidates = append([]string(nil), extractCandidateLines(mediaSections[i])...)
	}
	for i, section := range mediaSections {
		if mediaSectionKind(section) == "audio" {
			continue
		}
		mediaSections[i] = normalizeMediaSection(section, audioCandidates)
	}

	var lines []string
	lines = append(lines, sessionLines...)
	for _, section := range mediaSections {
		lines = append(lines, section...)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

func splitSDPSections(sdp string) ([]string, [][]string) {
	rawLines := strings.Split(strings.ReplaceAll(strings.TrimRight(sdp, "\r\n"), "\r\n", "\n"), "\n")
	sessionLines := make([]string, 0, len(rawLines))
	mediaSections := [][]string{}
	var current []string
	for _, line := range rawLines {
		if strings.HasPrefix(line, "m=") {
			if len(current) > 0 {
				mediaSections = append(mediaSections, current)
			}
			current = []string{line}
			continue
		}
		if current == nil {
			sessionLines = append(sessionLines, line)
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		mediaSections = append(mediaSections, current)
	}
	return sessionLines, mediaSections
}

func normalizeSessionLines(lines []string, streamID string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "o=") {
			line = strings.Replace(line, " IN IP4 0.0.0.0", " IN IP4 127.0.0.1", 1)
		}
		if streamID != "" && strings.HasPrefix(line, "a=msid-semantic:") {
			line = "a=msid-semantic: WMS " + streamID
		}
		out = append(out, line)
	}
	return reorderSessionLines(out)
}

func normalizeMediaSection(lines, fallbackCandidates []string) []string {
	kind := mediaSectionKind(lines)
	out := make([]string, 0, len(lines))
	hasICEOptions := false
	hasRTCP := false
	hasCandidates := false
	hasMaxMessageSize := false
	for _, line := range lines {
		switch {
		case line == "a=end-of-candidates":
			continue
		case kind == "application" && line == "a=sendrecv":
			continue
		case strings.HasPrefix(line, "a=ssrc:") && (strings.Contains(line, " mslabel:") || strings.Contains(line, " label:")):
			continue
		case strings.HasPrefix(line, "a=max-message-size:"):
			if kind == "application" {
				line = "a=max-message-size:262144"
				hasMaxMessageSize = true
			}
		}
		if strings.HasPrefix(line, "a=candidate:") {
			var keep bool
			line, keep = sanitizeCandidateLine(line)
			if !keep {
				continue
			}
		}
		if strings.HasPrefix(line, "a=ice-options:") {
			hasICEOptions = true
		}
		if strings.HasPrefix(line, "a=rtcp:") {
			hasRTCP = true
		}
		if strings.HasPrefix(line, "a=candidate:") {
			hasCandidates = true
		}
		out = append(out, line)
	}

	if kind == "audio" && !hasRTCP {
		out = insertAfterFirstPrefix(out, "c=", "a=rtcp:9 IN IP4 0.0.0.0")
	}
	if !hasICEOptions {
		out = insertAfterFirstPrefix(out, "a=ice-pwd:", "a=ice-options:trickle")
	}
	if kind == "application" {
		if !hasCandidates && len(fallbackCandidates) > 0 {
			out = insertAfterFirstPrefix(out, "c=", fallbackCandidates...)
		}
		if !hasMaxMessageSize {
			out = insertAfterFirstPrefix(out, "a=sctp-port:", "a=max-message-size:262144")
		}
	}
	if candidate, ok := preferredCandidate(extractCandidateLines(out)); ok {
		out = rewriteMediaPortAndConnection(out, candidate)
	}
	return reorderMediaSection(kind, out)
}

func mediaSectionKind(lines []string) string {
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "m=") {
		return ""
	}
	fields := strings.Fields(strings.TrimPrefix(lines[0], "m="))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func extractCandidateLines(lines []string) []string {
	out := []string{}
	for _, line := range lines {
		if strings.HasPrefix(line, "a=candidate:") {
			out = append(out, line)
		}
	}
	return out
}

func extractPrimaryStreamID(mediaSections [][]string) string {
	for _, section := range mediaSections {
		for _, line := range section {
			if !strings.HasPrefix(line, "a=msid:") {
				continue
			}
			fields := strings.Fields(strings.TrimPrefix(line, "a=msid:"))
			if len(fields) > 0 && fields[0] != "" {
				return fields[0]
			}
		}
	}
	return ""
}

func candidateComponent(line string) string {
	fields := strings.Fields(strings.TrimPrefix(line, "a=candidate:"))
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

type sdpCandidate struct {
	address   string
	port      string
	transport string
	kind      string
	hasIP     bool
	ipFamily  string
}

func preferredCandidate(lines []string) (sdpCandidate, bool) {
	best := sdpCandidate{}
	bestRank := 1 << 30
	bestIndex := -1
	for i, line := range lines {
		candidate, ok := parseCandidate(line)
		if !ok {
			continue
		}
		rank := candidateRank(candidate)
		if rank < bestRank || (rank == bestRank && bestIndex == -1) {
			best = candidate
			bestRank = rank
			bestIndex = i
		}
	}
	return best, bestIndex >= 0
}

func parseCandidate(line string) (sdpCandidate, bool) {
	fields := strings.Fields(strings.TrimPrefix(line, "a=candidate:"))
	if len(fields) < 8 {
		return sdpCandidate{}, false
	}
	candidate := sdpCandidate{
		transport: strings.ToLower(fields[2]),
		address:   fields[4],
		port:      fields[5],
	}
	for i := 6; i+1 < len(fields); i++ {
		if fields[i] == "typ" {
			candidate.kind = fields[i+1]
			break
		}
	}
	if candidate.kind == "" {
		return sdpCandidate{}, false
	}
	if port, err := strconv.Atoi(candidate.port); err != nil || port <= 0 {
		return sdpCandidate{}, false
	}
	if ip := net.ParseIP(candidate.address); ip != nil {
		candidate.hasIP = true
		if ip.To4() != nil {
			candidate.ipFamily = "IP4"
		} else {
			candidate.ipFamily = "IP6"
		}
	}
	return candidate, true
}

func sanitizeCandidateLine(line string) (string, bool) {
	if candidateComponent(line) == "2" {
		return "", false
	}
	candidate, ok := parseCandidate(line)
	if ok && candidate.kind == "host" && candidate.hasIP {
		return "", false
	}
	return stripCandidateAttribute(line, "ufrag"), true
}

func candidateRank(candidate sdpCandidate) int {
	typeRank := map[string]int{
		"relay": 0,
		"srflx": 1,
		"prflx": 2,
		"host":  3,
	}
	rank, ok := typeRank[candidate.kind]
	if !ok {
		rank = 4
	}
	rank *= 100
	switch candidate.transport {
	case "udp":
		rank += 0
	case "tcp":
		rank += 10
	default:
		rank += 20
	}
	switch {
	case candidate.ipFamily == "IP4":
		rank += 0
	case candidate.ipFamily == "IP6":
		rank += 1
	case candidate.hasIP:
		rank += 2
	default:
		rank += 50
	}
	return rank
}

func rewriteMediaPortAndConnection(lines []string, candidate sdpCandidate) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "m="):
			fields := strings.Fields(line)
			if len(fields) > 1 {
				fields[1] = candidate.port
				line = strings.Join(fields, " ")
			}
		case candidate.hasIP && strings.HasPrefix(line, "c=IN "):
			line = "c=IN " + candidate.ipFamily + " " + candidate.address
		}
		out = append(out, line)
	}
	return out
}

func stripCandidateAttribute(line, attr string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return line
	}
	out := make([]string, 0, len(fields))
	for i := 0; i < len(fields); i++ {
		if fields[i] == attr && i+1 < len(fields) {
			i++
			continue
		}
		out = append(out, fields[i])
	}
	return strings.Join(out, " ")
}

func reorderSessionLines(lines []string) []string {
	order := []func(string) bool{
		func(line string) bool { return strings.HasPrefix(line, "v=") },
		func(line string) bool { return strings.HasPrefix(line, "o=") },
		func(line string) bool { return strings.HasPrefix(line, "s=") },
		func(line string) bool { return strings.HasPrefix(line, "t=") },
		func(line string) bool { return strings.HasPrefix(line, "a=group:") },
		func(line string) bool { return line == "a=extmap-allow-mixed" },
		func(line string) bool { return strings.HasPrefix(line, "a=msid-semantic:") },
	}
	return reorderLines(lines, order)
}

func reorderMediaSection(kind string, lines []string) []string {
	if kind == "audio" {
		return reorderLines(lines, []func(string) bool{
			func(line string) bool { return strings.HasPrefix(line, "m=") },
			func(line string) bool { return strings.HasPrefix(line, "c=") },
			func(line string) bool { return strings.HasPrefix(line, "a=rtcp:") },
			func(line string) bool { return strings.HasPrefix(line, "a=candidate:") },
			func(line string) bool { return strings.HasPrefix(line, "a=ice-ufrag:") },
			func(line string) bool { return strings.HasPrefix(line, "a=ice-pwd:") },
			func(line string) bool { return strings.HasPrefix(line, "a=ice-options:") },
			func(line string) bool { return strings.HasPrefix(line, "a=fingerprint:") },
			func(line string) bool { return strings.HasPrefix(line, "a=setup:") },
			func(line string) bool { return strings.HasPrefix(line, "a=mid:") },
			func(line string) bool { return strings.HasPrefix(line, "a=extmap:1 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=extmap:2 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=extmap:3 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=extmap:4 ") },
			isDirectionLine,
			func(line string) bool { return strings.HasPrefix(line, "a=msid:") },
			func(line string) bool { return line == "a=rtcp-mux" },
			func(line string) bool { return line == "a=rtcp-rsize" },
			func(line string) bool { return strings.HasPrefix(line, "a=rtpmap:111 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=rtcp-fb:111 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=fmtp:111 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=rtpmap:63 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=fmtp:63 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=rtpmap:9 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=rtpmap:0 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=rtpmap:8 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=rtpmap:13 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=rtpmap:110 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=rtpmap:126 ") },
			func(line string) bool { return strings.HasPrefix(line, "a=ssrc:") },
		})
	}
	if kind == "application" {
		return reorderLines(lines, []func(string) bool{
			func(line string) bool { return strings.HasPrefix(line, "m=") },
			func(line string) bool { return strings.HasPrefix(line, "c=") },
			func(line string) bool { return strings.HasPrefix(line, "a=candidate:") },
			func(line string) bool { return strings.HasPrefix(line, "a=ice-ufrag:") },
			func(line string) bool { return strings.HasPrefix(line, "a=ice-pwd:") },
			func(line string) bool { return strings.HasPrefix(line, "a=ice-options:") },
			func(line string) bool { return strings.HasPrefix(line, "a=fingerprint:") },
			func(line string) bool { return strings.HasPrefix(line, "a=setup:") },
			func(line string) bool { return strings.HasPrefix(line, "a=mid:") },
			func(line string) bool { return strings.HasPrefix(line, "a=sctp-port:") },
			func(line string) bool { return strings.HasPrefix(line, "a=max-message-size:") },
		})
	}
	return lines
}

func reorderLines(lines []string, order []func(string) bool) []string {
	out := make([]string, 0, len(lines))
	used := make([]bool, len(lines))
	for _, match := range order {
		for i, line := range lines {
			if used[i] || !match(line) {
				continue
			}
			out = append(out, line)
			used[i] = true
		}
	}
	for i, line := range lines {
		if used[i] {
			continue
		}
		out = append(out, line)
	}
	return out
}

func isDirectionLine(line string) bool {
	switch line {
	case "a=sendrecv", "a=sendonly", "a=recvonly", "a=inactive":
		return true
	default:
		return false
	}
}

func insertAfterFirstPrefix(lines []string, prefix string, inserts ...string) []string {
	if len(inserts) == 0 {
		return append([]string(nil), lines...)
	}
	out := make([]string, 0, len(lines)+len(inserts))
	inserted := false
	for _, line := range lines {
		out = append(out, line)
		if inserted || !strings.HasPrefix(line, prefix) {
			continue
		}
		out = append(out, inserts...)
		inserted = true
	}
	if inserted {
		return out
	}
	out = append(out, inserts...)
	return out
}
