//go:build !darwin

package interactiveaudio

import (
	"fmt"
	"strings"
)

// Config describes the requested interactive-audio mode.
type Config struct {
	TranscriptOnly bool
	NoMic          bool
	Speaker        string
	Mic            string
}

// Backend is the portable fallback.
//
// It exists so transcript-only code can be wired up on non-macOS systems
// without introducing platform-specific audio dependencies.
type Backend struct {
	cfg Config
}

// New creates a no-op backend on non-Darwin platforms.
func New(cfg Config) (*Backend, error) {
	cfg.Speaker = strings.TrimSpace(cfg.Speaker)
	cfg.Mic = strings.TrimSpace(cfg.Mic)
	return &Backend{cfg: cfg}, nil
}

// Close releases the backend.
func (b *Backend) Close() error {
	return nil
}

// TranscriptOnly reports whether local audio is disabled.
func (b *Backend) TranscriptOnly() bool {
	if b == nil {
		return false
	}
	return b.cfg.TranscriptOnly
}

// SupportsPlayback reports whether local speaker playback is available.
func (b *Backend) SupportsPlayback() bool {
	return false
}

// SupportsCapture reports whether local microphone capture is available.
func (b *Backend) SupportsCapture() bool {
	return false
}

// StartPlayback always fails on non-Darwin platforms.
func (b *Backend) StartPlayback() error {
	if b != nil && b.cfg.TranscriptOnly {
		return nil
	}
	return fmt.Errorf("interactive audio playback requires darwin")
}

// StartCapture always fails on non-Darwin platforms.
func (b *Backend) StartCapture() error {
	if b != nil && (b.cfg.TranscriptOnly || b.cfg.NoMic) {
		return nil
	}
	return fmt.Errorf("interactive audio microphone capture requires darwin")
}
