package interactiveaudio

import (
	"runtime"
	"testing"
)

func TestNewTranscriptOnlyBackend(t *testing.T) {
	b, err := New(Config{TranscriptOnly: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !b.TranscriptOnly() {
		t.Fatalf("TranscriptOnly() = false, want true")
	}
	if err := b.StartPlayback(); err != nil {
		t.Fatalf("StartPlayback() transcript-only error = %v", err)
	}
	if err := b.StartCapture(); err != nil {
		t.Fatalf("StartCapture() transcript-only error = %v", err)
	}
}

func TestAudioSupportFlags(t *testing.T) {
	b, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if runtime.GOOS == "darwin" {
		if !b.SupportsPlayback() {
			t.Fatal("SupportsPlayback() = false, want true")
		}
		if !b.SupportsCapture() {
			t.Fatal("SupportsCapture() = false, want true")
		}
	} else {
		if b.SupportsPlayback() {
			t.Fatal("SupportsPlayback() = true, want false")
		}
		if b.SupportsCapture() {
			t.Fatal("SupportsCapture() = true, want false")
		}
	}
	if err := b.StartPlayback(); err == nil {
		t.Fatal("StartPlayback() error = nil, want error")
	}
	if err := b.StartCapture(); err == nil {
		t.Fatal("StartCapture() error = nil, want error")
	}
}
