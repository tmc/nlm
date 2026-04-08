package interactiveaudio

import (
	"fmt"
	"strings"
	"sync"

	"google.golang.org/protobuf/encoding/protowire"
)

type outboundState struct {
	mu           sync.Mutex
	nextSequence int
	started      bool
	completed    bool
}

func (s *outboundState) startupFrames(audioOverviewID string) [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	audioOverviewID = strings.TrimSpace(audioOverviewID)
	if s.started || audioOverviewID == "" {
		return nil
	}
	s.started = true

	frames := [][]byte{encodeGroundedAgentStartFrame(s.nextSequence, audioOverviewID)}
	s.nextSequence += len(frames)
	return frames
}

func (s *outboundState) completionFrames() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.completed {
		return nil
	}
	s.completed = true
	frames := [][]byte{encodeSendAudioStopFrame(s.nextSequence)}
	s.nextSequence += len(frames)
	return frames
}

func encodeGroundedAgentStartFrame(sequence int, audioOverviewID string) []byte {
	var groundedAgentConfig []byte
	groundedAgentConfig = protowire.AppendTag(groundedAgentConfig, 1, protowire.BytesType)
	groundedAgentConfig = protowire.AppendString(groundedAgentConfig, audioOverviewID)
	groundedAgentConfig = protowire.AppendTag(groundedAgentConfig, 4, protowire.VarintType)
	groundedAgentConfig = protowire.AppendVarint(groundedAgentConfig, 1)

	var typedConfig []byte
	typedConfig = protowire.AppendTag(typedConfig, 1, protowire.BytesType)
	typedConfig = protowire.AppendString(typedConfig, "type.googleapis.com/gemfm.agents.GroundedAgentConfig")
	typedConfig = protowire.AppendTag(typedConfig, 2, protowire.BytesType)
	typedConfig = protowire.AppendBytes(typedConfig, groundedAgentConfig)

	var agent []byte
	agent = protowire.AppendTag(agent, 1, protowire.BytesType)
	agent = protowire.AppendString(agent, "grounded_agent")
	agent = protowire.AppendTag(agent, 2, protowire.BytesType)
	agent = protowire.AppendBytes(agent, typedConfig)

	var control []byte
	control = protowire.AppendTag(control, 1, protowire.BytesType)
	control = protowire.AppendBytes(control, agent)

	var payload []byte
	payload = protowire.AppendTag(payload, 1, protowire.BytesType)
	payload = protowire.AppendBytes(payload, control)
	return encodeOutboundFrame(sequence, payload)
}

func encodePlaybackFrame(sequence int, field protowire.Number, state string) []byte {
	var payload []byte
	var body []byte
	body = protowire.AppendTag(body, field, protowire.BytesType)
	body = protowire.AppendString(body, state)
	payload = protowire.AppendTag(payload, 6, protowire.BytesType)
	payload = protowire.AppendBytes(payload, body)
	return encodeOutboundFrame(sequence, payload)
}

func encodeMicrophoneFrame(sequence int, statusCode int32) []byte {
	var payload []byte
	var body []byte
	body = protowire.AppendTag(body, 1, protowire.VarintType)
	body = protowire.AppendVarint(body, uint64(statusCode))
	payload = protowire.AppendTag(payload, 7, protowire.BytesType)
	payload = protowire.AppendBytes(payload, body)
	return encodeOutboundFrame(sequence, payload)
}

func encodeSendAudioStopFrame(sequence int) []byte {
	var payload []byte
	var body []byte
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "")
	payload = protowire.AppendTag(payload, 4, protowire.BytesType)
	payload = protowire.AppendBytes(payload, body)
	return encodeOutboundFrame(sequence, payload)
}

func encodeOutboundFrame(sequence int, payload []byte) []byte {
	var inner []byte
	if sequence != 0 {
		inner = protowire.AppendTag(inner, 1, protowire.VarintType)
		inner = protowire.AppendVarint(inner, uint64(sequence))
	}
	inner = protowire.AppendTag(inner, 2, protowire.VarintType)
	inner = protowire.AppendVarint(inner, uint64(len(payload)))
	inner = protowire.AppendTag(inner, 4, protowire.BytesType)
	inner = protowire.AppendBytes(inner, payload)

	var outer []byte
	outer = protowire.AppendTag(outer, 2, protowire.BytesType)
	outer = protowire.AppendBytes(outer, inner)
	return outer
}

func sendFrames(sender interface{ Send([]byte) error }, frames [][]byte) error {
	for _, frame := range frames {
		if err := sender.Send(frame); err != nil {
			return fmt.Errorf("send data-channel frame: %w", err)
		}
	}
	return nil
}
