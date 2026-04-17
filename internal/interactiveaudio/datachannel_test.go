package interactiveaudio

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skipIfFixtureMissing skips the test when path is absent.
//
// Convention for capture-driven tests in this repo: any test whose inputs
// live under docs/captures/ (gitignored — contains auth cookies and session
// tokens) MUST guard the load with this helper (or an inline equivalent)
// before opening the file. Use t.Skipf, never t.Fatalf, when the fixture is
// merely absent: that distinguishes "contributor does not have a local
// capture" from "decoder is broken on a fixture we expect to exist," and it
// keeps `go test ./...` green on a fresh checkout.
//
// Tests that depend on checked-in testdata (internal/method goldens) or on
// files the test itself wrote earlier in the same run should keep using
// t.Fatalf — those files are expected to exist, and their absence really is
// a bug.
//
// Add a sibling copy of this helper in any new package that grows the same
// pattern; promote to a shared internal/testutil package only once a third
// consumer needs it (currently only this package and
// internal/cmd/interactiveaudiodecode hold capture-driven tests).
func skipIfFixtureMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("fixture absent at %s; capture it via the interactive-audio capture tool to enable this test", path)
	}
}

func TestDecodeInteractiveAudioCapture(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "captures", "interactive-audio", "webrtc-datachannel.jsonl")
	skipIfFixtureMissing(t, path)
	frames, err := DecodeCaptureFile(path)
	if err != nil {
		t.Fatalf("decode capture: %v", err)
	}
	if got := len(frames); got < 40 {
		t.Fatalf("decoded %d frames, want at least 40", got)
	}

	var (
		sawUser, sawAgent, sawMic, sawStatus, sawTTS, sawSend, sawPlayback bool
		lastUser                                                           string
		sawAgentTranscript                                                 string
	)

	for _, frame := range frames {
		switch ev := frame.Event.(type) {
		case PlaybackEvent:
			sawPlayback = true
			_ = ev.State()
		case SendAudioEvent:
			sawSend = true
			if ev.TriggerType == 0 || ev.UtteranceID == "" {
				t.Fatalf("send audio event missing data: %+v", ev)
			}
		case MicrophoneEvent:
			sawMic = true
		case TTSEvent:
			sawTTS = true
		case StatusMessage:
			sawStatus = true
			if ev.Text == "" {
				t.Fatalf("status message text is empty: %+v", ev)
			}
		case UserUtterance:
			sawUser = true
			lastUser = ev.Transcript
		case AgentUtterance:
			sawAgent = true
			if sawAgentTranscript == "" && strings.Contains(ev.Transcript, "Well, hello there!") {
				sawAgentTranscript = ev.Transcript
			}
		case UnknownEvent:
			// The capture contains one extra GroundedAgentCustomMessage control frame
			// on field 5. Preserve it as UnknownEvent without failing the decoder.
		default:
			t.Fatalf("unexpected event type %T", ev)
		}
	}

	if !sawPlayback || !sawSend || !sawMic || !sawTTS || !sawUser || !sawAgent || !sawStatus {
		t.Fatalf("missing event kinds: playback=%v send=%v mic=%v tts=%v user=%v agent=%v status=%v",
			sawPlayback, sawSend, sawMic, sawTTS, sawUser, sawAgent, sawStatus)
	}
	if lastUser != "Hey there. This is a test message. One two three." {
		t.Fatalf("last user transcript = %q, want final interactive transcript", lastUser)
	}
	if sawAgentTranscript == "" {
		t.Fatalf("did not find expected agent transcript in interactive capture")
	}
}

func TestDecodePlaybackCapture(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "captures", "audio-playback", "webrtc-datachannel.jsonl")
	skipIfFixtureMissing(t, path)
	frames, err := DecodeCaptureFile(path)
	if err != nil {
		t.Fatalf("decode capture: %v", err)
	}
	if got := len(frames); got != 42 {
		t.Fatalf("decoded %d frames, want 42", got)
	}

	first, ok := frames[0].Event.(PlaybackEvent)
	if !ok {
		t.Fatalf("first event type = %T, want PlaybackEvent", frames[0].Event)
	}
	if first.State() != "" {
		t.Fatalf("first playback state = %q, want empty", first.State())
	}

	var foundAgent bool
	for _, frame := range frames {
		ev, ok := frame.Event.(AgentUtterance)
		if !ok {
			continue
		}
		if strings.Contains(ev.Transcript, "Have you ever tried to test an application") {
			foundAgent = true
			break
		}
	}
	if !foundAgent {
		t.Fatalf("did not find expected playback transcript in capture")
	}
}

func TestDecodeBase64Frame(t *testing.T) {
	frame, err := DecodeBase64Frame("EgoIKhAEIgQyAhoA")
	if err != nil {
		t.Fatalf("decode base64 frame: %v", err)
	}
	ev, ok := frame.Event.(PlaybackEvent)
	if !ok {
		t.Fatalf("event type = %T, want PlaybackEvent", frame.Event)
	}
	if ev.State() != "" {
		t.Fatalf("playback state = %q, want empty", ev.State())
	}
}

func TestRendererTTYAndPipedOutput(t *testing.T) {
	t.Run("piped", func(t *testing.T) {
		var out bytes.Buffer
		var status bytes.Buffer

		r := NewRenderer(&out, &status, false)
		if err := r.Handle(UserUtterance{Transcript: "Hey.", IsFinal: false}); err != nil {
			t.Fatalf("handle user: %v", err)
		}
		if err := r.Handle(UserUtterance{Transcript: "Hey there.", IsFinal: true}); err != nil {
			t.Fatalf("handle user final: %v", err)
		}
		if err := r.Handle(AgentUtterance{
			Speakers:   []string{"Host Speaker", "Expert Speaker"},
			Transcript: "Welcome.",
			IsFinal:    true,
		}); err != nil {
			t.Fatalf("handle agent: %v", err)
		}
		if err := r.Finish(); err != nil {
			t.Fatalf("finish: %v", err)
		}

		if got := out.String(); got != "[YOU] Hey there.\n[HOST] Welcome.\n" {
			t.Fatalf("stdout = %q", got)
		}
		if got := status.String(); got != "" {
			t.Fatalf("stderr = %q, want empty", got)
		}
	})

	t.Run("piped incremental host output after tts start", func(t *testing.T) {
		var out bytes.Buffer
		var status bytes.Buffer

		r := NewRenderer(&out, &status, false)
		if err := r.Handle(AgentUtterance{
			Speakers:    []string{"Host Speaker"},
			Transcript:  "First chunk.",
			UtteranceID: "utt-1",
		}); err != nil {
			t.Fatalf("handle queued agent: %v", err)
		}
		if got := out.String(); got != "" {
			t.Fatalf("stdout before tts = %q, want empty", got)
		}
		if err := r.Handle(TTSEvent{EventType: 1, UtteranceID: "utt-1", SegmentIdx: 0}); err != nil {
			t.Fatalf("handle tts start: %v", err)
		}
		if err := r.Handle(AgentUtterance{
			Speakers:    []string{"Host Speaker"},
			Transcript:  "Second chunk.",
			UtteranceID: "utt-2",
		}); err != nil {
			t.Fatalf("handle active agent: %v", err)
		}
		if err := r.Handle(TTSEvent{EventType: 2, UtteranceID: "utt-1", SegmentIdx: 1}); err != nil {
			t.Fatalf("handle tts end: %v", err)
		}
		if err := r.Finish(); err != nil {
			t.Fatalf("finish: %v", err)
		}

		want := "[HOST] First chunk.\n[HOST] Second chunk.\n"
		if got := out.String(); got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
		if got := status.String(); got != "" {
			t.Fatalf("stderr = %q, want empty", got)
		}
	})

	t.Run("tty", func(t *testing.T) {
		var out bytes.Buffer
		var status bytes.Buffer

		r := NewRenderer(&out, &status, true)
		if err := r.Handle(MicrophoneEvent{StatusCode: 1}); err != nil {
			t.Fatalf("handle mic: %v", err)
		}
		if err := r.Handle(UserUtterance{Transcript: "Hey there.", IsFinal: true}); err != nil {
			t.Fatalf("handle user: %v", err)
		}
		if err := r.Handle(AgentUtterance{
			Speakers:   []string{"Host Speaker", "Expert Speaker"},
			Transcript: "Hello",
			IsFinal:    true,
		}); err != nil {
			t.Fatalf("handle agent: %v", err)
		}
		if err := r.Finish(); err != nil {
			t.Fatalf("finish: %v", err)
		}

		if got := out.String(); !strings.Contains(got, "You: Hey there.") || !strings.Contains(got, ansiBold+"Host Speaker"+ansiReset+": Hello") {
			t.Fatalf("stdout = %q", got)
		}
		if got := status.String(); !strings.Contains(got, "[user speaking]") {
			t.Fatalf("stderr = %q, want mic status", got)
		}
	})

	t.Run("tty incremental host output after tts start", func(t *testing.T) {
		var out bytes.Buffer
		var status bytes.Buffer

		r := NewRenderer(&out, &status, true)
		if err := r.Handle(AgentUtterance{
			Speakers:    []string{"Host Speaker"},
			Transcript:  "First chunk.",
			UtteranceID: "utt-1",
		}); err != nil {
			t.Fatalf("handle queued agent: %v", err)
		}
		if got := out.String(); got != "" {
			t.Fatalf("stdout before tts = %q, want empty", got)
		}
		if err := r.Handle(TTSEvent{EventType: 1, UtteranceID: "utt-1", SegmentIdx: 0}); err != nil {
			t.Fatalf("handle tts start: %v", err)
		}
		if err := r.Handle(AgentUtterance{
			Speakers:    []string{"Host Speaker"},
			Transcript:  "Second chunk.",
			UtteranceID: "utt-2",
		}); err != nil {
			t.Fatalf("handle active agent: %v", err)
		}
		if err := r.Finish(); err != nil {
			t.Fatalf("finish: %v", err)
		}

		if got := out.String(); !strings.Contains(got, ansiBold+"Host Speaker"+ansiReset+": First chunk.") || !strings.Contains(got, ansiBold+"Host Speaker"+ansiReset+": Second chunk.") {
			t.Fatalf("stdout = %q, want incremental host lines", got)
		}
	})
}
