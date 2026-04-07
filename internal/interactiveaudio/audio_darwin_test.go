//go:build darwin

package interactiveaudio

import "testing"

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
	if err := b.StartCapture(); err != nil {
		t.Fatalf("StartCapture() transcript-only error = %v", err)
	}
}

func TestDarwinBackendScaffold(t *testing.T) {
	b, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if b.TranscriptOnly() {
		t.Fatal("TranscriptOnly() = true, want false")
	}
	if !b.SupportsPlayback() {
		t.Fatal("SupportsPlayback() = false, want true")
	}
	if !b.SupportsCapture() {
		t.Fatal("SupportsCapture() = false, want true")
	}
	if err := b.StartPlayback(); err == nil {
		t.Fatal("StartPlayback() error = nil, want error")
	}
	if err := b.StartCapture(); err == nil {
		t.Fatal("StartCapture() error = nil, want error")
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
