//go:build darwin

package interactiveaudio

import (
	"fmt"
	"strings"

	"github.com/tmc/apple/avfaudio"
)

// Config describes the requested local-audio mode.
type Config struct {
	TranscriptOnly bool
	NoMic          bool
	Speaker        string
	Mic            string
}

// Backend is the Darwin audio backend scaffold.
//
// The current implementation owns the AVFAudio objects we expect to wire into
// the eventual playback and capture graph. The actual media plumbing is still
// deferred to the session and codec layers.
type Backend struct {
	cfg        Config
	engine     avfaudio.AVAudioEngine
	player     avfaudio.AVAudioPlayerNode
	input      avfaudio.AVAudioInputNode
	graphReady bool
}

// New creates the Darwin backend.
func New(cfg Config) (*Backend, error) {
	cfg.Speaker = strings.TrimSpace(cfg.Speaker)
	cfg.Mic = strings.TrimSpace(cfg.Mic)

	b := &Backend{cfg: cfg}
	if cfg.TranscriptOnly {
		return b, nil
	}

	// Construct the AVFAudio objects now so the backend shape is explicit and
	// compile-time checked against the local apple bindings.
	b.engine = avfaudio.NewAVAudioEngine()
	b.player = avfaudio.NewAVAudioPlayerNode()
	b.input = avfaudio.NewAVAudioInputNode()
	b.graphReady = true
	return b, nil
}

// Close releases local AVFAudio resources.
func (b *Backend) Close() error {
	if b == nil {
		return nil
	}
	return nil
}

// TranscriptOnly reports whether local audio is disabled.
func (b *Backend) TranscriptOnly() bool {
	return b != nil && b.cfg.TranscriptOnly
}

// SupportsPlayback reports whether a local playback graph is available.
func (b *Backend) SupportsPlayback() bool {
	return b != nil && b.graphReady && !b.cfg.TranscriptOnly
}

// SupportsCapture reports whether a local microphone graph is available.
func (b *Backend) SupportsCapture() bool {
	return b != nil && b.graphReady && !b.cfg.TranscriptOnly && !b.cfg.NoMic
}

// StartPlayback reserves the AVAudioEngine/AVAudioPlayerNode path.
func (b *Backend) StartPlayback() error {
	if b == nil {
		return fmt.Errorf("interactive audio backend is nil")
	}
	if b.cfg.TranscriptOnly {
		return nil
	}
	if !b.graphReady {
		return fmt.Errorf("interactive audio playback graph is unavailable")
	}
	return fmt.Errorf("interactive audio playback graph is ready on darwin, but remote audio decode is not wired yet")
}

// StartCapture reserves the AVAudioEngine/AVAudioInputNode path.
func (b *Backend) StartCapture() error {
	if b == nil {
		return fmt.Errorf("interactive audio backend is nil")
	}
	if b.cfg.TranscriptOnly || b.cfg.NoMic {
		return nil
	}
	if !b.SupportsCapture() {
		return fmt.Errorf("interactive audio microphone graph is unavailable")
	}
	return fmt.Errorf("interactive audio microphone capture graph is ready on darwin, but outbound audio encode is not wired yet")
}
