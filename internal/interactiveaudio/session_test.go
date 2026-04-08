package interactiveaudio

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRunValidatesOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts Options
		want string
	}{
		{
			name: "speaker selection not wired",
			opts: Options{
				Config: Config{
					TranscriptOnly: true,
					Speaker:        "Built-in Output",
				},
			},
			want: "speaker selection is not wired yet",
		},
		{
			name: "microphone selection not wired",
			opts: Options{
				Config: Config{
					TranscriptOnly: true,
					Mic:            "Built-in Microphone",
				},
			},
			want: "microphone selection is not wired yet",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Run(context.Background(), "auth", "cookies", "notebook", Options{
				Config:          tt.opts.Config,
				AudioOverviewID: "audio-123",
				Stdout:          io.Discard,
				Stderr:          io.Discard,
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRunRequiresAudioOverviewID(t *testing.T) {
	t.Parallel()

	err := Run(context.Background(), "auth", "cookies", "notebook", Options{
		Config: Config{TranscriptOnly: true},
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "requires audio overview id") {
		t.Fatalf("Run() error = %v, want audio overview id error", err)
	}
}

func TestRunRequiresAuthentication(t *testing.T) {
	t.Parallel()

	err := Run(context.Background(), "", "", "notebook", Options{
		Config: Config{TranscriptOnly: true},
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "requires authentication") {
		t.Fatalf("Run() error = %v, want authentication error", err)
	}
}

func TestRunRequiresNotebookID(t *testing.T) {
	t.Parallel()

	err := Run(context.Background(), "auth", "cookies", "", Options{
		Config: Config{TranscriptOnly: true},
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "missing notebook id") {
		t.Fatalf("Run() error = %v, want missing notebook id error", err)
	}
}

type stubSignaler struct {
	debug bool
	err   error
}

func (s *stubSignaler) SetDebug(debug bool) {
	s.debug = debug
}

func (s *stubSignaler) StartInteractiveAudioChannel(context.Context, string) error {
	return s.err
}

func TestSessionStartSignalerUnavailable(t *testing.T) {
	orig := newSignalerClient
	t.Cleanup(func() { newSignalerClient = orig })

	newSignalerClient = func(string, string) (signalerStarter, error) {
		return nil, errors.New("missing browser signaler auth")
	}

	var stderr bytes.Buffer
	s := &session{
		opts:       Options{Debug: true},
		notebookID: "notebook-123",
		cookies:    "cookie-a",
		stderr:     &stderr,
	}
	s.startSignaler(context.Background())

	if got := stderr.String(); !strings.Contains(got, "[signaler] unavailable: missing browser signaler auth") {
		t.Fatalf("stderr = %q, want unavailable message", got)
	}
}

func TestSessionStartSignalerFailure(t *testing.T) {
	orig := newSignalerClient
	t.Cleanup(func() { newSignalerClient = orig })

	newSignalerClient = func(string, string) (signalerStarter, error) {
		return &stubSignaler{err: errors.New("choose server: status 401")}, nil
	}

	var stderr bytes.Buffer
	s := &session{
		opts:       Options{Debug: true},
		notebookID: "notebook-123",
		cookies:    "cookie-a",
		stderr:     &stderr,
	}
	s.startSignaler(context.Background())

	if got := stderr.String(); !strings.Contains(got, "[signaler] start failed: choose server: status 401") {
		t.Fatalf("stderr = %q, want start failed message", got)
	}
}

func TestSessionHandleEventMarksPassivePlaybackFinal(t *testing.T) {
	t.Parallel()

	backend, err := New(Config{TranscriptOnly: true, NoMic: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var stderr bytes.Buffer
	s := &session{
		opts: Options{
			Config: Config{TranscriptOnly: true, NoMic: true},
		},
		renderer: NewRenderer(io.Discard, io.Discard, false),
		backend:  backend,
		stderr:   &stderr,
	}

	done, err := s.handleEvent(context.Background(), TTSEvent{
		EventType:   playbackCompletionEventType,
		UtteranceID: "utt-1",
		SegmentIdx:  37,
	})
	if err != nil {
		t.Fatalf("handleEvent() error = %v", err)
	}
	if done {
		t.Fatalf("handleEvent() done = true, want false")
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestSessionHandleEventIgnoresNonTerminalTTSEvent(t *testing.T) {
	t.Parallel()

	backend, err := New(Config{TranscriptOnly: true, NoMic: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	s := &session{
		opts: Options{
			Config: Config{TranscriptOnly: true, NoMic: true},
		},
		renderer: NewRenderer(io.Discard, io.Discard, false),
		backend:  backend,
		stderr:   io.Discard,
	}

	done, err := s.handleEvent(context.Background(), TTSEvent{
		EventType:   1,
		UtteranceID: "utt-1",
		SegmentIdx:  0,
	})
	if err != nil {
		t.Fatalf("handleEvent() error = %v", err)
	}
	if done {
		t.Fatalf("handleEvent() done = true, want false")
	}
}

func TestSessionHandleEventCompletesAfterPlaybackAck(t *testing.T) {
	t.Parallel()

	backend, err := New(Config{TranscriptOnly: true, NoMic: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var stderr bytes.Buffer
	s := &session{
		opts: Options{
			Config: Config{TranscriptOnly: true, NoMic: true},
		},
		renderer: NewRenderer(io.Discard, io.Discard, false),
		backend:  backend,
		stderr:   &stderr,
		finalTTS: true,
		stopSent: true,
	}

	done, err := s.handleEvent(context.Background(), PlaybackEvent{})
	if err != nil {
		t.Fatalf("handleEvent() error = %v", err)
	}
	if !done {
		t.Fatalf("handleEvent() done = false, want true")
	}
	if got := stderr.String(); !strings.Contains(got, "Playback complete.") {
		t.Fatalf("stderr = %q, want playback complete message", got)
	}
}
