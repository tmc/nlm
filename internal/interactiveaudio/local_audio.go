package interactiveaudio

import (
	"fmt"
	"io"
	"sync"

	"github.com/hraban/opus"
	"github.com/pion/rtp"
)

const (
	uplinkSampleRate     = 48000
	uplinkChannels       = 1
	uplinkFrameSamples   = uplinkSampleRate / 50
	uplinkMaxOpusBytes   = 4000
	micActivityThreshold = 500
	micSilenceHangover   = 4
	maxPCM16             = 32767
	minPCM16             = -32768
)

type captureHandler func([]int16, int, int) error

type rtpPacketWriter interface {
	WriteRTP(*rtp.Packet) error
}

type localAudioSender struct {
	mu        sync.Mutex
	writer    rtpPacketWriter
	interrupt func() error
	stderr    io.Writer
	debug     bool
	enabled   bool

	encoder       *opus.Encoder
	resampler     pcmResampler
	pending       []int16
	packet        rtp.Packet
	buffer        []byte
	sequence      uint16
	timestamp     uint32
	turnPackets   int
	inSpeech      bool
	silenceFrames int
}

type pcmResampler struct {
	sourceRate int
	buf        []float64
	pos        float64
}

func newLocalAudioSender(
	writer rtpPacketWriter,
	interrupt func() error,
	stderr io.Writer,
	debug bool,
) (*localAudioSender, error) {
	if writer == nil {
		return nil, fmt.Errorf("interactive audio requires a local rtp writer")
	}
	if interrupt == nil {
		return nil, fmt.Errorf("interactive audio requires an interruption callback")
	}
	encoder, err := opus.NewEncoder(uplinkSampleRate, uplinkChannels, opus.AppVoIP)
	if err != nil {
		return nil, fmt.Errorf("create opus encoder: %w", err)
	}
	if err := encoder.SetBitrateToAuto(); err != nil {
		return nil, fmt.Errorf("set opus bitrate: %w", err)
	}
	if err := encoder.SetInBandFEC(true); err != nil {
		return nil, fmt.Errorf("enable opus fec: %w", err)
	}
	if err := encoder.SetPacketLossPerc(10); err != nil {
		return nil, fmt.Errorf("set opus packet loss: %w", err)
	}
	return &localAudioSender{
		writer:    writer,
		interrupt: interrupt,
		stderr:    stderr,
		debug:     debug,
		encoder:   encoder,
		buffer:    make([]byte, uplinkMaxOpusBytes),
	}, nil
}

func (s *localAudioSender) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

func (s *localAudioSender) SetEnabled(enabled bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enabled == enabled {
		return s.enabled
	}
	s.enabled = enabled
	if !enabled {
		s.resetTurnState()
	}
	return s.enabled
}

func (s *localAudioSender) ToggleEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = !s.enabled
	if !s.enabled {
		s.resetTurnState()
	}
	return s.enabled
}

func (s *localAudioSender) HandlePCM16(samples []int16, sampleRate, channels int) error {
	if len(samples) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled {
		s.resetTurnState()
		return nil
	}

	mono, err := s.resampler.push(samples, sampleRate, channels)
	if err != nil {
		return err
	}
	if len(mono) == 0 {
		return nil
	}
	s.pending = append(s.pending, mono...)
	for len(s.pending) >= uplinkFrameSamples {
		frame := append([]int16(nil), s.pending[:uplinkFrameSamples]...)
		s.pending = s.pending[uplinkFrameSamples:]
		if err := s.handleFrame(frame); err != nil {
			return err
		}
	}
	return nil
}

func (s *localAudioSender) handleFrame(frame []int16) error {
	speech := frameLooksLikeSpeech(frame)
	if speech {
		s.silenceFrames = 0
		if !s.inSpeech {
			if err := s.interrupt(); err != nil {
				return err
			}
			s.inSpeech = true
			s.turnPackets = 0
			if s.debug {
				fmt.Fprintln(s.stderr, "[mic] speech detected")
			}
		}
	} else if s.inSpeech {
		s.silenceFrames++
		if s.silenceFrames > micSilenceHangover {
			s.inSpeech = false
			s.silenceFrames = 0
			if s.debug {
				fmt.Fprintln(s.stderr, "[mic] speech ended")
			}
			return nil
		}
	} else {
		return nil
	}

	n, err := s.encoder.Encode(frame, s.buffer)
	if err != nil {
		return fmt.Errorf("encode microphone opus: %w", err)
	}
	if n == 0 {
		return nil
	}

	packet := &s.packet
	packet.Header.Version = 2
	packet.Header.Marker = s.turnPackets == 0
	packet.Header.SequenceNumber = s.sequence
	packet.Header.Timestamp = s.timestamp
	packet.Payload = append(packet.Payload[:0], s.buffer[:n]...)
	if err := s.writer.WriteRTP(packet); err != nil {
		return fmt.Errorf("write microphone rtp: %w", err)
	}

	s.sequence++
	s.timestamp += uplinkFrameSamples
	s.turnPackets++
	return nil
}

func (s *localAudioSender) resetTurnState() {
	s.pending = s.pending[:0]
	s.resampler.buf = s.resampler.buf[:0]
	s.resampler.pos = 0
	s.inSpeech = false
	s.silenceFrames = 0
	s.turnPackets = 0
}

func (r *pcmResampler) push(samples []int16, sampleRate, channels int) ([]int16, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("interactive audio microphone capture requires positive sample rate")
	}
	if channels <= 0 {
		return nil, fmt.Errorf("interactive audio microphone capture requires positive channel count")
	}
	if len(samples)%channels != 0 {
		return nil, fmt.Errorf("interactive audio microphone capture requires interleaved pcm aligned to channels")
	}

	mono := downmixToMono(samples, channels)
	if len(mono) == 0 {
		return nil, nil
	}
	if sampleRate == uplinkSampleRate {
		return mono, nil
	}

	if r.sourceRate != sampleRate {
		r.sourceRate = sampleRate
		r.buf = r.buf[:0]
		r.pos = 0
	}
	for _, sample := range mono {
		r.buf = append(r.buf, float64(sample))
	}
	if len(r.buf) < 2 {
		return nil, nil
	}

	step := float64(sampleRate) / float64(uplinkSampleRate)
	out := make([]int16, 0, len(mono)*uplinkSampleRate/sampleRate+2)
	for r.pos+1 < float64(len(r.buf)) {
		i := int(r.pos)
		frac := r.pos - float64(i)
		value := r.buf[i]*(1-frac) + r.buf[i+1]*frac
		out = append(out, clampPCM16(value))
		r.pos += step
	}

	drop := int(r.pos)
	if drop > 0 {
		tail := len(r.buf) - drop
		copy(r.buf[:tail], r.buf[drop:])
		r.buf = r.buf[:tail]
		r.pos -= float64(drop)
	}
	return out, nil
}

func downmixToMono(samples []int16, channels int) []int16 {
	if channels == 1 {
		return append([]int16(nil), samples...)
	}
	mono := make([]int16, 0, len(samples)/channels)
	for i := 0; i < len(samples); i += channels {
		sum := 0
		for ch := 0; ch < channels; ch++ {
			sum += int(samples[i+ch])
		}
		mono = append(mono, int16(sum/channels))
	}
	return mono
}

func frameLooksLikeSpeech(samples []int16) bool {
	if len(samples) == 0 {
		return false
	}
	var total int64
	for _, sample := range samples {
		total += int64(abs16(sample))
	}
	return total/int64(len(samples)) >= micActivityThreshold
}

func abs16(v int16) int32 {
	if v < 0 {
		return -int32(v)
	}
	return int32(v)
}

func clampPCM16(v float64) int16 {
	switch {
	case v > maxPCM16:
		return maxPCM16
	case v < minPCM16:
		return minPCM16
	default:
		return int16(v)
	}
}
