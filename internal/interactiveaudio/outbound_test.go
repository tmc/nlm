package interactiveaudio

import (
	"encoding/base64"
	"testing"
)

func TestEncodePlaybackFrameRoundTrip(t *testing.T) {
	frame, err := DecodeFrame(encodePlaybackFrame(42, 1, ""))
	if err != nil {
		t.Fatalf("DecodeFrame() error = %v", err)
	}
	if frame.SequenceNumber != 42 {
		t.Fatalf("SequenceNumber = %d, want 42", frame.SequenceNumber)
	}
	ev, ok := frame.Event.(PlaybackEvent)
	if !ok {
		t.Fatalf("event type = %T, want PlaybackEvent", frame.Event)
	}
	if ev.State() != "" {
		t.Fatalf("playback state = %q, want empty", ev.State())
	}
}

func TestEncodeMicrophoneFrameRoundTrip(t *testing.T) {
	frame, err := DecodeFrame(encodeMicrophoneFrame(44, 3))
	if err != nil {
		t.Fatalf("DecodeFrame() error = %v", err)
	}
	if frame.SequenceNumber != 44 {
		t.Fatalf("SequenceNumber = %d, want 44", frame.SequenceNumber)
	}
	ev, ok := frame.Event.(MicrophoneEvent)
	if !ok {
		t.Fatalf("event type = %T, want MicrophoneEvent", frame.Event)
	}
	if ev.StatusCode != 3 {
		t.Fatalf("StatusCode = %d, want 3", ev.StatusCode)
	}
}

func TestOutboundStartupFrames(t *testing.T) {
	var state outboundState
	frames := state.startupFrames("58133c78-e2c4-4176-8974-09eb4647b82b")
	if len(frames) != 1 {
		t.Fatalf("startupFrames() returned %d frames, want 1", len(frames))
	}
	const want = "EnoQdiJ2CnQKcgoOZ3JvdW5kZWRfYWdlbnQSYAo0dHlwZS5nb29nbGVhcGlzLmNvbS9nZW1mbS5hZ2VudHMuR3JvdW5kZWRBZ2VudENvbmZpZxIoCiQ1ODEzM2M3OC1lMmM0LTQxNzYtODk3NC0wOWViNDY0N2I4MmIgAQ=="
	if got := base64.StdEncoding.EncodeToString(frames[0]); got != want {
		t.Fatalf("startup frame = %s, want %s", got, want)
	}
	if got := state.startupFrames("58133c78-e2c4-4176-8974-09eb4647b82b"); got != nil {
		t.Fatalf("second startupFrames() = %d frames, want nil", len(got))
	}
}

func TestOutboundStartupFramesRequireAudioOverviewID(t *testing.T) {
	var state outboundState
	if got := state.startupFrames(""); got != nil {
		t.Fatalf("startupFrames(\"\") = %d frames, want nil", len(got))
	}
}
