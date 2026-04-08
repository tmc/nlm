package interactiveaudio

import (
	"io"
	"testing"

	"github.com/pion/rtp"
)

type recordingRTPWriter struct {
	packets []*rtp.Packet
}

func (w *recordingRTPWriter) WriteRTP(packet *rtp.Packet) error {
	clone := *packet
	clone.Payload = append([]byte(nil), packet.Payload...)
	w.packets = append(w.packets, &clone)
	return nil
}

func TestLocalAudioSenderInterruptsOncePerTurn(t *testing.T) {
	writer := &recordingRTPWriter{}
	interrupts := 0
	sender, err := newLocalAudioSender(writer, func() error {
		interrupts++
		return nil
	}, io.Discard, false)
	if err != nil {
		t.Fatalf("newLocalAudioSender() error = %v", err)
	}

	speech := repeatedPCM(1200, uplinkFrameSamples)
	silence := repeatedPCM(0, uplinkFrameSamples*(micSilenceHangover+2))

	if err := sender.HandlePCM16(speech, uplinkSampleRate, 1); err != nil {
		t.Fatalf("HandlePCM16(speech) error = %v", err)
	}
	if err := sender.HandlePCM16(silence, uplinkSampleRate, 1); err != nil {
		t.Fatalf("HandlePCM16(silence) error = %v", err)
	}
	if err := sender.HandlePCM16(speech, uplinkSampleRate, 1); err != nil {
		t.Fatalf("HandlePCM16(second speech) error = %v", err)
	}

	if interrupts != 2 {
		t.Fatalf("interrupts = %d, want 2", interrupts)
	}
	if len(writer.packets) < 2 {
		t.Fatalf("packets = %d, want at least 2", len(writer.packets))
	}
	if !writer.packets[0].Marker {
		t.Fatal("first packet marker = false, want true")
	}
	if !writer.packets[len(writer.packets)-1].Marker {
		t.Fatal("second turn first packet marker = false, want true")
	}
}

func TestPCMResamplerDownmixesStereo(t *testing.T) {
	var r pcmResampler
	stereo := []int16{1000, -1000, 500, -500, 300, -300, 900, -900}
	mono, err := r.push(stereo, uplinkSampleRate, 2)
	if err != nil {
		t.Fatalf("push() error = %v", err)
	}
	if len(mono) != len(stereo)/2 {
		t.Fatalf("len(mono) = %d, want %d", len(mono), len(stereo)/2)
	}
	for i, sample := range mono {
		if sample != 0 {
			t.Fatalf("mono[%d] = %d, want 0", i, sample)
		}
	}
}

func repeatedPCM(sample int16, n int) []int16 {
	out := make([]int16, n)
	for i := range out {
		out[i] = sample
	}
	return out
}
