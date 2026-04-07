package interactiveaudio

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/encoding/protowire"
)

// Frame is one decoded DataChannel payload.
type Frame struct {
	SequenceNumber int
	PayloadSize    int
	Event          Event
	RawPayload     []byte
}

// Event is a structured interactive-audio DataChannel payload.
type Event interface {
	MessageType() pb.AgentCommsMessageType
}

// UserUtterance is the progressive transcript for the user's speech.
type UserUtterance struct {
	Transcript  string
	IsFinal     bool
	UtteranceID string
}

func (UserUtterance) MessageType() pb.AgentCommsMessageType {
	return pb.AgentCommsMessageType_USER_UTTERANCE
}

// AgentUtterance is the transcript payload for the AI speaker.
type AgentUtterance struct {
	SpeakerID   int32
	Speakers    []string
	Transcript  string
	IsFinal     bool
	UtteranceID string
}

func (AgentUtterance) MessageType() pb.AgentCommsMessageType {
	return pb.AgentCommsMessageType_AGENT_UTTERANCE
}

// TTSTimestamp is the timestamp nested inside a TTS event.
type TTSTimestamp struct {
	EpochSeconds int64
	Nanos        int32
}

// TTSEvent is the TTS lifecycle payload.
type TTSEvent struct {
	EventType   int32
	UtteranceID string
	SegmentIdx  int32
	Timestamp   TTSTimestamp
}

func (TTSEvent) MessageType() pb.AgentCommsMessageType {
	return pb.AgentCommsMessageType_TTS_EVENT
}

// SendAudioEvent indicates playback start/stop triggers.
type SendAudioEvent struct {
	TriggerType int32
	UtteranceID string
}

func (SendAudioEvent) MessageType() pb.AgentCommsMessageType {
	return pb.AgentCommsMessageType_SEND_AUDIO_EVENT
}

// PlaybackEvent carries client-side playback state.
type PlaybackEvent struct {
	States []string
}

func (PlaybackEvent) MessageType() pb.AgentCommsMessageType {
	return pb.AgentCommsMessageType_PLAYBACK_EVENT
}

// State returns the first non-empty playback state string.
func (e PlaybackEvent) State() string {
	for _, state := range e.States {
		if strings.TrimSpace(state) != "" {
			return state
		}
	}
	return ""
}

// MicrophoneEvent carries microphone state changes.
type MicrophoneEvent struct {
	StatusCode int32
}

func (MicrophoneEvent) MessageType() pb.AgentCommsMessageType {
	return pb.AgentCommsMessageType_MICROPHONE_EVENT
}

// StatusMessage carries diagnostics and processing state.
type StatusMessage struct {
	Text   string
	Raw    []byte
	Fields []WireField
}

func (StatusMessage) MessageType() pb.AgentCommsMessageType {
	return pb.AgentCommsMessageType_STATUS_MESSAGE
}

// UnknownEvent preserves an undecoded payload.
type UnknownEvent struct {
	Type   pb.AgentCommsMessageType
	Raw    []byte
	Fields []WireField
}

func (e UnknownEvent) MessageType() pb.AgentCommsMessageType {
	return e.Type
}

// WireField is one protobuf field parsed from the wire.
type WireField struct {
	Number   protowire.Number
	WireType protowire.Type
	Value    []byte
}

func (f WireField) String() string {
	switch f.WireType {
	case protowire.VarintType:
		v, _ := protowire.ConsumeVarint(f.Value)
		return fmt.Sprintf("%d=%d", f.Number, v)
	case protowire.BytesType:
		if s, ok := wireString(f.Value); ok {
			return fmt.Sprintf("%d=%q", f.Number, s)
		}
		return fmt.Sprintf("%d=<%d bytes>", f.Number, len(f.Value))
	default:
		return fmt.Sprintf("%d=<wire type %d>", f.Number, f.WireType)
	}
}

type fieldValue struct {
	number protowire.Number
	typ    protowire.Type
	value  []byte
	u64    uint64
}

func parseFields(b []byte) ([]fieldValue, error) {
	var fields []fieldValue
	for len(b) > 0 {
		number, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, fmt.Errorf("consume tag: %w", protowire.ParseError(n))
		}
		b = b[n:]

		switch typ {
		case protowire.VarintType:
			u, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return nil, fmt.Errorf("consume varint: %w", protowire.ParseError(n))
			}
			fields = append(fields, fieldValue{
				number: number,
				typ:    typ,
				value:  protowire.AppendVarint(nil, u),
				u64:    u,
			})
			b = b[n:]
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return nil, fmt.Errorf("consume bytes: %w", protowire.ParseError(n))
			}
			fields = append(fields, fieldValue{
				number: number,
				typ:    typ,
				value:  append([]byte(nil), v...),
			})
			b = b[n:]
		case protowire.Fixed32Type:
			v, n := protowire.ConsumeFixed32(b)
			if n < 0 {
				return nil, fmt.Errorf("consume fixed32: %w", protowire.ParseError(n))
			}
			fields = append(fields, fieldValue{
				number: number,
				typ:    typ,
				value:  protowire.AppendFixed32(nil, v),
			})
			b = b[n:]
		case protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return nil, fmt.Errorf("consume fixed64: %w", protowire.ParseError(n))
			}
			fields = append(fields, fieldValue{
				number: number,
				typ:    typ,
				value:  protowire.AppendFixed64(nil, v),
			})
			b = b[n:]
		default:
			return nil, fmt.Errorf("unsupported wire type %d", typ)
		}
	}
	return fields, nil
}

func fieldBytes(fields []fieldValue, number protowire.Number) [][]byte {
	var out [][]byte
	for _, field := range fields {
		if field.number == number && field.typ == protowire.BytesType {
			out = append(out, field.value)
		}
	}
	return out
}

func fieldString(fields []fieldValue, number protowire.Number) (string, bool) {
	for _, field := range fields {
		if field.number != number || field.typ != protowire.BytesType {
			continue
		}
		if s, ok := wireString(field.value); ok {
			return s, true
		}
	}
	return "", false
}

func fieldStringList(fields []fieldValue, number protowire.Number) []string {
	var out []string
	for _, raw := range fieldBytes(fields, number) {
		s, ok := wireString(raw)
		if !ok {
			continue
		}
		for _, part := range strings.Split(s, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func fieldVarint(fields []fieldValue, number protowire.Number) (int64, bool) {
	for _, field := range fields {
		if field.number == number && field.typ == protowire.VarintType {
			return int64(field.u64), true
		}
	}
	return 0, false
}

func fieldMessage(fields []fieldValue, number protowire.Number) ([]fieldValue, bool, error) {
	values := fieldBytes(fields, number)
	if len(values) == 0 {
		return nil, false, nil
	}
	nested, err := parseFields(values[0])
	if err != nil {
		return nil, false, err
	}
	return nested, true, nil
}

func firstString(fields []fieldValue, number protowire.Number) string {
	s, _ := fieldString(fields, number)
	return s
}

func firstInt(fields []fieldValue, number protowire.Number) int64 {
	v, _ := fieldVarint(fields, number)
	return v
}

func firstBool(fields []fieldValue, number protowire.Number) bool {
	return firstInt(fields, number) != 0
}

func toWireFields(fields []fieldValue) []WireField {
	out := make([]WireField, 0, len(fields))
	for _, field := range fields {
		out = append(out, WireField{
			Number:   field.number,
			WireType: field.typ,
			Value:    append([]byte(nil), field.value...),
		})
	}
	return out
}

func wireString(b []byte) (string, bool) {
	if !utf8.Valid(b) {
		return "", false
	}
	return string(b), true
}

func hexString(b []byte) string {
	return hex.EncodeToString(b)
}
