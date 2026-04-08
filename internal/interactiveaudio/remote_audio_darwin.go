//go:build darwin

package interactiveaudio

import (
	"fmt"
	"io"

	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"
)

const maxOpusFrameSamples = 2880

func (s *session) handleRemoteTrack(track *webrtc.TrackRemote) error {
	if track.Kind() != webrtc.RTPCodecTypeAudio {
		if s.opts.Debug {
			fmt.Fprintf(s.stderr, "Ignoring remote %s track (%s)\n", track.Kind().String(), track.Codec().MimeType)
		}
		buf := make([]byte, 1500)
		for {
			if _, _, err := track.Read(buf); err != nil {
				return err
			}
		}
	}
	if s.backend == nil || s.backend.TranscriptOnly() {
		if s.opts.Debug {
			fmt.Fprintf(s.stderr, "Ignoring remote audio track (%s) in transcript-only mode\n", track.Codec().MimeType)
		}
		buf := make([]byte, 1500)
		for {
			if _, _, err := track.Read(buf); err != nil {
				return err
			}
		}
	}

	codec := track.Codec()
	sampleRate := int(codec.ClockRate)
	if sampleRate == 0 {
		sampleRate = 48000
	}
	channels := int(codec.Channels)
	if channels == 0 {
		channels = 2
	}
	decoder, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		return fmt.Errorf("create opus decoder: %w", err)
	}
	pcm := make([]int16, maxOpusFrameSamples*channels)
	if s.opts.Debug {
		fmt.Fprintf(s.stderr, "Playing remote audio track: codec=%s rate=%d channels=%d\n", codec.MimeType, sampleRate, channels)
	}

	for {
		packet, _, err := track.ReadRTP()
		if err != nil {
			if err == io.EOF {
				return err
			}
			return fmt.Errorf("read remote audio packet: %w", err)
		}
		if len(packet.Payload) == 0 {
			continue
		}
		s.markRemoteAudioActivity()
		frames, err := decoder.Decode(packet.Payload, pcm)
		if err != nil {
			if s.opts.Debug {
				fmt.Fprintf(s.stderr, "Skipping undecodable opus packet: %v\n", err)
			}
			continue
		}
		if frames == 0 {
			continue
		}
		if err := s.backend.WritePCM16(pcm[:frames*channels], sampleRate, channels); err != nil {
			return err
		}
	}
}
