//go:build darwin

package interactiveaudio

import (
	"testing"
	"time"
)

func TestDarwinTranscriptOnlyBackend(t *testing.T) {
	b, err := New(Config{TranscriptOnly: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !b.TranscriptOnly() {
		t.Fatalf("TranscriptOnly() = false, want true")
	}
	if b.SupportsPlayback() {
		t.Fatal("SupportsPlayback() = true, want false")
	}
	if b.SupportsCapture() {
		t.Fatal("SupportsCapture() = true, want false")
	}
	if err := b.StartPlayback(); err != nil {
		t.Fatalf("StartPlayback() transcript-only error = %v", err)
	}
	if err := b.StartCapture(nil); err != nil {
		t.Fatalf("StartCapture() transcript-only error = %v", err)
	}
}

func TestDarwinPlaybackBackend(t *testing.T) {
	b, err := New(Config{NoMic: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if b.TranscriptOnly() {
		t.Fatal("TranscriptOnly() = true, want false")
	}
	if !b.SupportsPlayback() {
		t.Fatal("SupportsPlayback() = false, want true")
	}
	if b.SupportsCapture() {
		t.Fatal("SupportsCapture() = true, want false")
	}
	if err := b.StartPlayback(); err != nil {
		t.Fatalf("StartPlayback() error = %v", err)
	}
	if err := b.StartCapture(nil); err != nil {
		t.Fatalf("StartCapture() error = %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestDarwinCaptureRequiresHandler(t *testing.T) {
	b, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := b.StartCapture(nil); err == nil {
		t.Fatal("StartCapture(nil) error = nil, want error")
	}
}

func TestShouldStartPlayback(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		queued   int
		primedAt time.Time
		want     bool
	}{
		{name: "empty", queued: 0, want: false},
		{name: "threshold reached", queued: playbackStartBuffers, primedAt: now, want: true},
		{name: "not enough buffers yet", queued: playbackStartBuffers - 1, primedAt: now, want: false},
		{name: "aged enough", queued: 1, primedAt: now.Add(-playbackStartMaxDelay), want: true},
	}

	for _, tt := range tests {
		if got := shouldStartPlayback(tt.queued, tt.primedAt, now); got != tt.want {
			t.Errorf("%s: shouldStartPlayback(%d, %v) = %v, want %v", tt.name, tt.queued, tt.primedAt, got, tt.want)
		}
	}
}

func TestMissingPacketCount(t *testing.T) {
	tests := []struct {
		name     string
		haveLast bool
		last     uint16
		current  uint16
		want     int
	}{
		{name: "first packet", current: 100, want: 0},
		{name: "contiguous", haveLast: true, last: 100, current: 101, want: 0},
		{name: "single gap", haveLast: true, last: 100, current: 102, want: 1},
		{name: "multiple gaps", haveLast: true, last: 100, current: 105, want: 4},
		{name: "wraparound", haveLast: true, last: 65535, current: 1, want: 1},
		{name: "reordered packet", haveLast: true, last: 100, current: 99, want: 0},
	}

	for _, tt := range tests {
		if got := missingPacketCount(tt.haveLast, tt.last, tt.current); got != tt.want {
			t.Errorf("%s: missingPacketCount(%v, %d, %d) = %d, want %d", tt.name, tt.haveLast, tt.last, tt.current, got, tt.want)
		}
	}
}
